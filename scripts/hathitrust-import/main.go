// hathitrust-import loads volumes downloaded from HathiTrust into the moml
// tables, so that the citation detector, the linker and chambers treat them as
// they treat Gale's volumes (issue #402). It is the side-corpus import
// (scripts/sidecorpus-import) with a fuller catalogue record and a record of
// provenance.
//
// A manifest (JSON; stephen-on-pleading.json is the first) lists the volumes
// and their catalogue fields. The page text is not in the repository:
// HathiTrust refuses scripted downloads, so each volume is downloaded by hand
// from its page at https://hdl.handle.net/2027/<htid> (Download, format
// "Text (.zip)", range "Whole item", logged in through a member library) and
// the zips are put, under the names HathiTrust gives them, in one directory.
//
//	go run ./scripts/hathitrust-import/ --check   # read the zips, write nothing
//	go run ./scripts/hathitrust-import/           # import
//
// Each volume becomes a row of moml.volumes with its own edition
// (bibliographicid = psmid) in the manifest's work, the edition's subjects, a
// row of moml.hathitrust_volumes, and a row of moml.page and moml.page_ocrtext
// for each page. A page's pageid is its HathiTrust sequence number times ten,
// zero-padded to five digits, MOML's convention, so sequence 1 is 00010.
//
// A volume already in moml.volumes is skipped, and a volume whose download is
// missing is reported and passed over, so the program can be run again as
// downloads arrive. Each volume is one transaction.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	flag "github.com/spf13/pflag"

	"github.com/lmullen/legal-modernism/go/db"
)

// Manifest is the list of volumes to import, all editions of one work.
type Manifest struct {
	Description string   `json:"description"`
	WorkID      int      `json:"work_id"`
	Volumes     []Volume `json:"volumes"`
}

// Volume is one HathiTrust item and the catalogue fields recorded for it.
type Volume struct {
	PSMID            string   `json:"psmid"`
	HTID             string   `json:"htid"`
	HTRecord         string   `json:"ht_record"`
	OCLC             *string  `json:"oclc"`
	Rights           string   `json:"rights"`
	DigitizedBy      *string  `json:"digitized_by"`
	SourceLibrary    *string  `json:"source_library"`
	Year             int      `json:"year"`
	Title            string   `json:"title"`
	Author           *string  `json:"author"`
	EditionStatement *string  `json:"edition_statement"`
	Imprint          *string  `json:"imprint"`
	Collation        *string  `json:"collation"`
	PublicationPlace *string  `json:"publication_place"`
	Subjects         []string `json:"subjects"`
	MOMLPSMID        *string  `json:"moml_psmid"`
	Relation         *string  `json:"relation"`
	Note             *string  `json:"note"`
}

// LogID returns the key-value pairs that identify the volume in a log line.
func (v Volume) LogID(extra ...any) []any {
	return append([]any{"psmid", v.PSMID, "htid", v.HTID, "year", v.Year}, extra...)
}

func main() {
	initLogger()

	manifestPath := flag.String("manifest", "scripts/hathitrust-import/stephen-on-pleading.json", "path to the manifest of volumes")
	dir := flag.String("dir", "tmp/hathitrust", "directory holding the zips downloaded from HathiTrust")
	check := flag.Bool("check", false, "read the manifest and the downloads and report on them without connecting to the database")
	flag.Parse()

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		slog.Error("failed to read the manifest", "path", *manifestPath, "error", err)
		os.Exit(1)
	}
	slog.Info("read the manifest", "path", *manifestPath, "work_id", manifest.WorkID, "volumes", len(manifest.Volumes))

	ctx := context.Background()
	var pool *pgxpool.Pool
	if !*check {
		pool, err = db.Connect(ctx)
		if err != nil {
			slog.Error("failed to connect to the database", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
	}

	var imported, present, missing int
	for _, v := range manifest.Volumes {
		if pool != nil {
			exists, err := volumeExists(ctx, pool, v.PSMID)
			if err != nil {
				slog.Error("failed to look up the volume", v.LogID("error", err)...)
				os.Exit(1)
			}
			if exists {
				slog.Info("volume is already in the database, skipping", v.LogID()...)
				present++
				continue
			}
		}

		source, err := findSource(*dir, v.HTID)
		if errors.Is(err, errNoSource) {
			slog.Warn("no download found for the volume", v.LogID("dir", *dir, "url", handleURL(v.HTID))...)
			missing++
			continue
		}
		if err != nil {
			slog.Error("failed to find the volume's download", v.LogID("error", err)...)
			os.Exit(1)
		}

		pages, err := readPages(source)
		if err != nil {
			slog.Error("failed to read the volume's pages", v.LogID("source", source, "error", err)...)
			os.Exit(1)
		}

		if pool == nil {
			slog.Info("read volume", v.LogID("source", source, "pages", len(pages),
				"blank_pages", countBlank(pages), "characters", countChars(pages))...)
			continue
		}

		if err := importVolume(ctx, pool, manifest.WorkID, v, pages); err != nil {
			slog.Error("failed to import the volume", v.LogID("source", source, "error", err)...)
			os.Exit(1)
		}
		slog.Info("imported volume", v.LogID("source", source, "pages", len(pages))...)
		imported++
	}

	slog.Info("done importing HathiTrust volumes", "imported", imported,
		"already_present", present, "missing_download", missing, "check_only", *check)
	if missing > 0 {
		os.Exit(2)
	}
}

func initLogger() {
	env := os.Getenv("LAW_DEBUG")
	level := slog.LevelInfo
	if env == "debug" || env == "true" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func readManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// validate checks what the database would otherwise refuse one volume at a
// time, so a bad manifest fails before anything is written.
func (m *Manifest) validate() error {
	if m.WorkID <= 0 {
		return errors.New("manifest has no work_id")
	}
	if len(m.Volumes) == 0 {
		return errors.New("manifest has no volumes")
	}
	psmids := map[string]bool{}
	htids := map[string]bool{}
	for i, v := range m.Volumes {
		switch {
		case v.PSMID == "" || v.HTID == "" || v.HTRecord == "" || v.Rights == "" || v.Title == "":
			return fmt.Errorf("volume %d (%s): psmid, htid, ht_record, rights and title are required", i+1, v.PSMID)
		case v.Year <= 0:
			return fmt.Errorf("volume %s: year is required", v.PSMID)
		case psmids[v.PSMID]:
			return fmt.Errorf("volume %s: psmid appears twice", v.PSMID)
		case htids[v.HTID]:
			return fmt.Errorf("volume %s: htid %s appears twice", v.PSMID, v.HTID)
		case (v.MOMLPSMID == nil) != (v.Relation == nil):
			return fmt.Errorf("volume %s: moml_psmid and relation go together", v.PSMID)
		}
		psmids[v.PSMID] = true
		htids[v.HTID] = true
	}
	return nil
}

func handleURL(htid string) string {
	return "https://hdl.handle.net/2027/" + htid
}

func volumeExists(ctx context.Context, pool *pgxpool.Pool, psmid string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM moml.volumes WHERE psmid = $1);`, psmid).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("querying moml.volumes: %w", err)
	}
	return exists, nil
}

// importVolume writes one volume and everything that hangs from it in a
// single transaction.
func importVolume(ctx context.Context, pool *pgxpool.Pool, workID int, v Volume, pages []Page) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// The volume is its own edition, in a work that already exists. Gale's
	// volumes that are not part of a numbered set have total_volumes 0 and
	// current_volume 0, and so do these.
	_, err = tx.Exec(ctx, `
		INSERT INTO moml.editions
		  (bibliographicid, bibliographicid_type, author, author_role, total_volumes,
		   book_collation, publication_place, publication_city, language, work_id)
		VALUES ($1, 'HathiTrust', $2, 'author', 0, $3, $4, $4, 'English', $5);`,
		v.PSMID, v.Author, v.Collation, v.PublicationPlace, workID)
	if err != nil {
		return fmt.Errorf("inserting edition: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO moml.volumes
		  (psmid, bibliographicid, current_volume, display_title, full_title,
		   edition_statement, imprint, year, total_pages, source_library, webid, product_link)
		VALUES ($1, $1, 0, $2, $2, $3, $4, $5, $6, $7, $8, $9);`,
		v.PSMID, v.Title, v.EditionStatement, v.Imprint, v.Year, len(pages),
		v.SourceLibrary, v.HTID, handleURL(v.HTID))
	if err != nil {
		return fmt.Errorf("inserting volume: %w", err)
	}

	for i, subject := range v.Subjects {
		_, err = tx.Exec(ctx, `
			INSERT INTO moml.edition_subjects (bibliographicid, position, subject)
			VALUES ($1, $2, $3);`, v.PSMID, i+1, subject)
		if err != nil {
			return fmt.Errorf("inserting subject %q: %w", subject, err)
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO moml.hathitrust_volumes
		  (psmid, htid, ht_record, oclc, rights, digitized_by, moml_psmid, relation, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
		v.PSMID, v.HTID, v.HTRecord, v.OCLC, v.Rights, v.DigitizedBy, v.MOMLPSMID, v.Relation, v.Note)
	if err != nil {
		return fmt.Errorf("inserting provenance: %w", err)
	}

	pageRows := make([][]any, len(pages))
	textRows := make([][]any, len(pages))
	for i, p := range pages {
		pageRows[i] = []any{p.PageID(), v.PSMID}
		textRows[i] = []any{p.PageID(), v.PSMID, p.Text}
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"moml", "page"},
		[]string{"pageid", "psmid"}, pgx.CopyFromRows(pageRows))
	if err != nil {
		return fmt.Errorf("copying pages: %w", err)
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"moml", "page_ocrtext"},
		[]string{"pageid", "psmid", "ocrtext"}, pgx.CopyFromRows(textRows))
	if err != nil {
		return fmt.Errorf("copying page text: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}
