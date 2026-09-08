package main

import "testing"

// TestParseTemplates guards the page list in parseTemplates: every page must
// parse against baseof.html, and a template file added without being listed is
// unreachable, so the test also checks the pages it expects to be served.
func TestParseTemplates(t *testing.T) {
	tmpls := parseTemplates()
	for _, page := range []string{"home.html", "dashboard.html", "tiers.html"} {
		if _, ok := tmpls[page]; !ok {
			t.Errorf("parseTemplates() is missing %s", page)
		}
	}
}
