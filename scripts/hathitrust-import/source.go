package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// errNoSource means the directory holds no download for a volume.
var errNoSource = errors.New("no download found")

// Page is the text of one page of a HathiTrust item.
type Page struct {
	Seq  int // HathiTrust's sequence number, counted from 1
	Text string
}

// PageID is the page's id in moml.page: the sequence number times ten,
// zero-padded to five digits, as Gale numbers its pages.
func (p Page) PageID() string {
	return fmt.Sprintf("%05d", p.Seq*10)
}

// squash reduces a name to its lowercase letters and digits, so that an item
// id can be found in a file name whatever was done to its punctuation:
// HathiTrust names the download of uc2.ark:/13960/t0pr7q84f
// uc2-ark--13960-t0pr7q84f-1795204993.zip, the last number a timestamp.
func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// findSource returns the zip file or directory in dir that holds the item's
// page text: the one whose name contains the item id, or failing that the id
// without its library prefix (the part after the first full stop).
func findSource(dir, htid string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errNoSource
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", dir, err)
	}

	keys := []string{squash(htid)}
	if _, local, ok := strings.Cut(htid, "."); ok && len(squash(local)) >= 6 {
		keys = append(keys, squash(local))
	}

	for _, key := range keys {
		var found []string
		for _, e := range entries {
			if !e.IsDir() && !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
				continue
			}
			if strings.Contains(squash(e.Name()), key) {
				found = append(found, filepath.Join(dir, e.Name()))
			}
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0], nil
		default:
			return "", fmt.Errorf("%d downloads match %s, keep one: %s", len(found), htid, strings.Join(found, ", "))
		}
	}
	return "", errNoSource
}

// readPages reads an item's pages from a zip file or a directory, in sequence
// order. Each page is a .txt file named for its sequence number.
func readPages(source string) ([]Page, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", source, err)
	}

	var fsys fs.FS
	if info.IsDir() {
		fsys = os.DirFS(source)
	} else {
		zr, err := zip.OpenReader(source)
		if err != nil {
			return nil, fmt.Errorf("opening zip %s: %w", source, err)
		}
		defer zr.Close()
		fsys = zr
	}
	return pagesFromFS(fsys)
}

var digits = regexp.MustCompile(`[0-9]+`)

// seqFromName takes the sequence number from a page's file name: the last run
// of digits in it (00000012.txt, or seq_12.txt), which must be positive.
func seqFromName(name string) (int, error) {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	runs := digits.FindAllString(base, -1)
	if len(runs) == 0 {
		return 0, fmt.Errorf("no page number in file name %q", name)
	}
	seq, err := strconv.Atoi(runs[len(runs)-1])
	if err != nil || seq <= 0 || seq > 9999 {
		return 0, fmt.Errorf("file name %q does not give a page number from 1 to 9999", name)
	}
	return seq, nil
}

func pagesFromFS(fsys fs.FS) ([]Page, error) {
	var pages []Page
	names := map[int]string{}

	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip directories, anything that is not text, the files macOS adds
		// to an archive, and the notice HathiTrust puts in every download
		// (00000000-hathitrust-colophon.txt), which is not a page.
		if d.IsDir() || !strings.EqualFold(path.Ext(name), ".txt") ||
			strings.HasPrefix(path.Base(name), ".") || strings.Contains(name, "__MACOSX") ||
			strings.Contains(strings.ToLower(path.Base(name)), "colophon") {
			return nil
		}
		seq, err := seqFromName(name)
		if err != nil {
			return err
		}
		if other, ok := names[seq]; ok {
			return fmt.Errorf("pages %q and %q have the same number %d", other, name, seq)
		}
		names[seq] = name

		f, err := fsys.Open(name)
		if err != nil {
			return fmt.Errorf("opening %s: %w", name, err)
		}
		defer f.Close()
		raw, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		pages = append(pages, Page{Seq: seq, Text: cleanText(raw)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, errors.New("no .txt pages found")
	}

	sort.Slice(pages, func(i, j int) bool { return pages[i].Seq < pages[j].Seq })
	return pages, nil
}

// cleanText makes OCR text safe to store: PostgreSQL refuses a NUL byte and
// text that is not valid UTF-8.
func cleanText(raw []byte) string {
	s := strings.ToValidUTF8(string(raw), "\uFFFD")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimPrefix(s, "\xEF\xBB\xBF")
}

func countBlank(pages []Page) int {
	n := 0
	for _, p := range pages {
		if strings.TrimSpace(p.Text) == "" {
			n++
		}
	}
	return n
}

func countChars(pages []Page) int {
	n := 0
	for _, p := range pages {
		n += len(p.Text)
	}
	return n
}
