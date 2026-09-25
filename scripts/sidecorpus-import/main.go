package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	flag "github.com/spf13/pflag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/schollz/progressbar/v3"
)

// Book is one side-corpus volume. A volume with no bibliographicid in the CSV
// is its own edition, keyed by its psmid, as MOML's single volumes are.
type Book struct {
	PSMID           string
	BibliographicID string
	WebID           string
	Year            int
	ProductLink     string
	AuthorByLine    string
	Title           string
	CurrentVolume   int
}

func main() {
	initLogger()

	csvPath := flag.String("csv", "tmp/side_corpus.csv", "path to the CSV file")
	tmpDir := flag.String("dir", "tmp", "path to the directory containing page text directories")
	subject := flag.String("subject", "US", "Gale subject term recorded for each edition, which moml.us_treatises filters on")
	flag.Parse()

	ctx := context.Background()

	db, err := dbConnect(ctx)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	books, err := readCSV(*csvPath)
	if err != nil {
		slog.Error("failed to read CSV", "error", err)
		os.Exit(1)
	}
	slog.Info("read books from CSV", "count", len(books))

	// Count total pages across all books for the progress bar.
	totalPages := 0
	for _, book := range books {
		dir := filepath.Join(*tmpDir, book.PSMID)
		entries, err := os.ReadDir(dir)
		if err != nil {
			slog.Error("failed to read page directory", "psmid", book.PSMID, "dir", dir, "error", err)
			os.Exit(1)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
				totalPages++
			}
		}
	}
	slog.Info("counted total pages", "pages", totalPages)

	pb := progressbar.Default(int64(totalPages))

	for _, book := range books {
		tx, err := db.Begin(ctx)
		if err != nil {
			slog.Error("failed to begin transaction", "psmid", book.PSMID, "error", err)
			os.Exit(1)
		}

		err = importBook(ctx, tx, book, *subject)
		if err != nil {
			tx.Rollback(ctx)
			slog.Error("failed to import book metadata", "psmid", book.PSMID, "error", err)
			os.Exit(1)
		}

		dir := filepath.Join(*tmpDir, book.PSMID)
		err = importPages(ctx, tx, book.PSMID, dir, pb)
		if err != nil {
			tx.Rollback(ctx)
			slog.Error("failed to import pages", "psmid", book.PSMID, "error", err)
			os.Exit(1)
		}

		err = tx.Commit(ctx)
		if err != nil {
			slog.Error("failed to commit transaction", "psmid", book.PSMID, "error", err)
			os.Exit(1)
		}

		slog.Info("imported book", "psmid", book.PSMID, "title", book.Title)
	}

	slog.Info("import complete")
}

func initLogger() {
	env := os.Getenv("LAW_DEBUG")
	level := slog.LevelInfo
	if env == "debug" || env == "true" {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Debug("set up the logger", "level", level)
}

func dbConnect(ctx context.Context) (*pgxpool.Pool, error) {
	connstr := os.Getenv("LAW_DBSTR")
	if connstr == "" {
		return nil, fmt.Errorf("LAW_DBSTR environment variable is not set")
	}
	timeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	db, err := pgxpool.Connect(timeout, connstr)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.Ping(timeout); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return db, nil
}

func readCSV(path string) ([]Book, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening CSV: %w", err)
	}
	defer f.Close()

	// Strip UTF-8 BOM if present.
	bom := make([]byte, 3)
	if _, err := io.ReadFull(f, bom); err != nil {
		return nil, fmt.Errorf("reading BOM: %w", err)
	}
	var reader io.Reader
	if bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		reader = f // BOM consumed, continue reading rest of file
	} else {
		reader = io.MultiReader(bytes.NewReader(bom), f) // No BOM, prepend bytes back
	}

	r := csv.NewReader(reader)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	var books []Book
	for i, row := range records[1:] {
		if len(row) < 8 {
			return nil, fmt.Errorf("row %d has %d fields, expected 8", i+2, len(row))
		}

		psmid := strings.TrimSpace(row[6])

		year, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			return nil, fmt.Errorf("parsing year %q on row %d: %w", row[1], i+2, err)
		}

		// A volume with no bibliographicid (empty or the literal "NULL") is its
		// own edition and not part of a numbered set, so its volume number is 0
		// whatever the CSV says. A volume of a set keeps the CSV's number.
		bibID := strings.TrimSpace(row[0])
		curVol := 0
		if bibID == "NULL" || bibID == "" {
			bibID = psmid
		} else if v := strings.TrimSpace(row[3]); v != "" {
			curVol, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("parsing current volume %q on row %d: %w", row[3], i+2, err)
			}
		}

		books = append(books, Book{
			PSMID:           psmid,
			BibliographicID: bibID,
			WebID:           strings.TrimSpace(row[5]),
			Year:            year,
			ProductLink:     strings.TrimSpace(row[4]),
			Title:           strings.TrimSpace(row[2]),
			CurrentVolume:   curVol,
			AuthorByLine:    strings.TrimSpace(row[7]),
		})
	}
	return books, nil
}

func pageIDFromFilename(filename string) (string, error) {
	name := strings.TrimSuffix(filename, ".txt")
	pageNum, err := strconv.Atoi(name)
	if err != nil {
		return "", fmt.Errorf("parsing page number from %q: %w", filename, err)
	}
	return fmt.Sprintf("%05d", pageNum*10), nil
}

// importBook records a volume, its edition, and the edition's subject. The
// edition and subject may already exist when the CSV holds several volumes of
// one set. Without a subject an edition drops out of moml.us_treatises, whose
// filters on subjects are never true for an empty list.
func importBook(ctx context.Context, tx pgx.Tx, book Book, subject string) error {
	timeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	queryEdition := `
	INSERT INTO moml.editions (bibliographicid, author)
	VALUES ($1, NULLIF($2, ''))
	ON CONFLICT DO NOTHING;`

	_, err := tx.Exec(timeout, queryEdition, book.BibliographicID, book.AuthorByLine)
	if err != nil {
		return fmt.Errorf("inserting edition for %s: %w", book.PSMID, err)
	}

	queryVolume := `
	INSERT INTO moml.volumes
	  (psmid, bibliographicid, current_volume, display_title, full_title, year, webid, product_link)
	VALUES ($1, $2, $3, $4, $4, $5, $6, $7)
	ON CONFLICT DO NOTHING;`

	_, err = tx.Exec(timeout, queryVolume,
		book.PSMID, book.BibliographicID, book.CurrentVolume, book.Title, book.Year,
		book.WebID, book.ProductLink)
	if err != nil {
		return fmt.Errorf("inserting volume %s: %w", book.PSMID, err)
	}

	querySubject := `
	INSERT INTO moml.edition_subjects (bibliographicid, position, subject)
	VALUES ($1, 1, $2)
	ON CONFLICT DO NOTHING;`

	_, err = tx.Exec(timeout, querySubject, book.BibliographicID, subject)
	if err != nil {
		return fmt.Errorf("inserting subject for %s: %w", book.PSMID, err)
	}

	return nil
}

func importPages(ctx context.Context, tx pgx.Tx, psmid string, dir string, pb *progressbar.ProgressBar) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	queryPage := `
	INSERT INTO moml.page (pageid, psmid)
	VALUES ($1, $2)
	ON CONFLICT DO NOTHING;`

	queryOCRText := `
	INSERT INTO moml.page_ocrtext (pageid, psmid, ocrtext)
	VALUES ($1, $2, $3)
	ON CONFLICT DO NOTHING;`

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		pageid, err := pageIDFromFilename(entry.Name())
		if err != nil {
			return fmt.Errorf("computing pageid for %s: %w", entry.Name(), err)
		}

		textBytes, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading text file %s: %w", entry.Name(), err)
		}
		ocrtext := string(textBytes)

		timeout, cancel := context.WithTimeout(ctx, 10*time.Second)

		_, err = tx.Exec(timeout, queryPage, pageid, psmid)
		if err != nil {
			cancel()
			return fmt.Errorf("inserting page %s/%s: %w", psmid, pageid, err)
		}

		_, err = tx.Exec(timeout, queryOCRText, pageid, psmid, ocrtext)
		cancel()
		if err != nil {
			return fmt.Errorf("inserting page_ocrtext %s/%s: %w", psmid, pageid, err)
		}

		pb.Add(1)
	}

	return nil
}
