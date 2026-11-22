package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeqFromName(t *testing.T) {
	tests := []struct {
		name    string
		want    int
		wantErr bool
	}{
		{"00000012.txt", 12, false},
		{"mdp.35112104595550/00000001.txt", 1, false},
		{"seq_45.txt", 45, false},
		{"mdp.35112104595550-seq_7.txt", 7, false},
		{"00000000.txt", 0, true},
		{"notes.txt", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := seqFromName(tt.name)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPageID(t *testing.T) {
	assert.Equal(t, "00010", Page{Seq: 1}.PageID())
	assert.Equal(t, "06780", Page{Seq: 678}.PageID())
}

func TestPagesFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"item/00000002.txt":                     {Data: []byte("second\x00 page")},
		"item/00000001.txt":                     {Data: []byte("\xEF\xBB\xBFfirst page")},
		"item/00000010.txt":                     {Data: []byte("bad \xff byte")},
		"item/00000003.txt":                     {Data: nil},
		"item/meta.json":                        {Data: []byte("{}")},
		"item/00000000-hathitrust-colophon.txt": {Data: []byte("notice")},
		"__MACOSX/item/._00001.txt":             {Data: []byte("junk")},
	}
	pages, err := pagesFromFS(fsys)
	require.NoError(t, err)
	assert.Equal(t, []Page{
		{Seq: 1, Text: "first page"},
		{Seq: 2, Text: "second page"},
		{Seq: 3, Text: ""},
		{Seq: 10, Text: "bad \uFFFD byte"},
	}, pages)
	assert.Equal(t, 1, countBlank(pages))

	_, err = pagesFromFS(fstest.MapFS{"a/00000001.txt": {}, "b/1.txt": {}})
	assert.ErrorContains(t, err, "same number")

	_, err = pagesFromFS(fstest.MapFS{"readme.md": {}})
	assert.ErrorContains(t, err, "no .txt pages")
}

func TestFindSource(t *testing.T) {
	dir := t.TempDir()
	touch := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o644))
	}
	touch("mdp-35112104595550-1795204509.zip")
	touch("uc2-ark--13960-t0pr7q84f-1795204993.zip")
	touch("33433008600300.zip")
	touch("nyp.33433008600292.pdf")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "uc1.b4280003"), 0o755))

	tests := []struct {
		htid string
		want string
	}{
		{"mdp.35112104595550", "mdp-35112104595550-1795204509.zip"},
		{"uc2.ark:/13960/t0pr7q84f", "uc2-ark--13960-t0pr7q84f-1795204993.zip"},
		{"nyp.33433008600300", "33433008600300.zip"},
		{"uc1.b4280003", "uc1.b4280003"},
	}
	for _, tt := range tests {
		t.Run(tt.htid, func(t *testing.T) {
			got, err := findSource(dir, tt.htid)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, tt.want), got)
		})
	}

	_, err := findSource(dir, "nyp.33433008600292")
	assert.ErrorIs(t, err, errNoSource, "a PDF is not a download of page text")

	_, err = findSource(filepath.Join(dir, "absent"), "mdp.35112104595550")
	assert.ErrorIs(t, err, errNoSource)

	touch("mdp-35112104595550-1795204509 (1).zip")
	_, err = findSource(dir, "mdp.35112104595550")
	assert.ErrorContains(t, err, "2 downloads match")
}

func TestReadPagesFromZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.zip")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	for name, text := range map[string]string{"item/00000001.txt": "one", "item/00000002.txt": "two"} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(text))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	pages, err := readPages(path)
	require.NoError(t, err)
	assert.Equal(t, []Page{{Seq: 1, Text: "one"}, {Seq: 2, Text: "two"}}, pages)
}
