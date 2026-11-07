package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// assetVersion is a hash of every embedded static file and data file, which
// changes with any deploy that changes one of them. A page puts it in the
// URLs it fetches (`?v=`), so that a browser holding a copy of the old file
// asks for the new one rather than pairing new code with old data, which is
// what the first visualizations' hour-long cache let happen.
var assetVersion = hashFS(map[string]fs.FS{"static": staticFS, "data": dataFS})

func init() {
	funcMap["assetVersion"] = func() string { return assetVersion }
}

// hashFS hashes the contents of the given file systems, in a fixed order.
func hashFS(systems map[string]fs.FS) string {
	names := make([]string, 0, len(systems))
	for name := range systems {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		var paths []string
		fs.WalkDir(systems[name], ".", func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
		sort.Strings(paths)
		for _, p := range paths {
			b, err := fs.ReadFile(systems[name], p)
			if err != nil {
				panic("embedded " + name + ": " + err.Error())
			}
			h.Write([]byte(name + "/" + p + "\n"))
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// withETags serves the files of an embedded file system through next, the
// file server, with an ETag of each file's content and Cache-Control:
// no-cache, so that a browser revalidates a module or image on every load
// and gets a 304 while it is unchanged, and the new file as soon as a deploy
// changes it. Embedded files have no modification time, so the file server
// alone sends no validator at all. The ETags are computed once per path.
func withETags(next http.Handler, fsys fs.FS) http.Handler {
	var etags sync.Map
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		etag, ok := etags.Load(path)
		if !ok {
			if b, err := fs.ReadFile(fsys, path); err == nil {
				sum := sha256.Sum256(b)
				etag = `"` + hex.EncodeToString(sum[:8]) + `"`
				etags.Store(path, etag)
				ok = true
			}
		}
		if ok {
			w.Header().Set("ETag", etag.(string))
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
