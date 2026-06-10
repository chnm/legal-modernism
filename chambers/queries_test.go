package main

import "testing"

func strptr(s string) *string { return &s }

func TestMomlPageURLs(t *testing.T) {
	productLink := "http://link.galegroup.com/apps/doc/F0103227568/MOML?sid=dhxml"

	tests := []struct {
		name        string
		productLink *string
		momlPage    string
		gmu         string
		columbia    string
	}{
		{
			name:        "with page",
			productLink: strptr(productLink),
			momlPage:    "06870",
			gmu:         "https://link.gale.com/apps/doc/F0103227568/MOML?u=viva_gmu&sid=dhxml&pg=687",
			columbia:    "https://link.gale.com/apps/doc/F0103227568/MOML?u=columbiau&sid=dhxml&pg=687",
		},
		{
			name:        "no page",
			productLink: strptr(productLink),
			momlPage:    "",
			gmu:         "https://link.gale.com/apps/doc/F0103227568/MOML?u=viva_gmu&sid=dhxml",
			columbia:    "https://link.gale.com/apps/doc/F0103227568/MOML?u=columbiau&sid=dhxml",
		},
		{
			name:        "nil product link",
			productLink: nil,
			momlPage:    "06870",
			gmu:         "",
			columbia:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CitationDetail{ProductLink: tt.productLink, MomlPage: tt.momlPage}
			if got := c.MomlPageURL(); got != tt.gmu {
				t.Errorf("MomlPageURL() = %q, want %q", got, tt.gmu)
			}
			if got := c.MomlPageURLColumbia(); got != tt.columbia {
				t.Errorf("MomlPageURLColumbia() = %q, want %q", got, tt.columbia)
			}
		})
	}
}
