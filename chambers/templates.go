package main

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// parseTemplates parses every page template under templates/ together with
// baseof.html and the partials (the files whose names begin with an
// underscore), so block overrides and shared partials work in every page. The
// pages are discovered rather than listed, so a template file is served as
// soon as it exists.
func parseTemplates() map[string]*template.Template {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		panic("reading embedded templates: " + err.Error())
	}
	shared := []string{"templates/baseof.html"}
	var pages []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == "baseof.html":
		case strings.HasPrefix(name, "_"):
			shared = append(shared, "templates/"+name)
		default:
			pages = append(pages, name)
		}
	}
	tmpls := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		files := append(append([]string{}, shared...), "templates/"+page)
		tmpls[page] = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, files...))
	}
	return tmpls
}
