package citations

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetect_RecordsSpan checks that every detector reports where in the text
// each citation was found, for both matching strategies (the anchored match at
// each starting place that the generic detectors use, and the plain FindAll the
// single-volume detectors use).
// The text has a multi-byte character ahead of the citations so that the
// offsets are exercised as byte offsets, which is what slicing a Go string
// needs, rather than rune offsets.
func TestDetect_RecordsSpan(t *testing.T) {
	text := `Résumé of the cases: see 30 Missis. 673 and 12 F1ed. 45, then Hob. 423
	and Toth., 234; compare 5 Ld. Raym. 45.`
	doc := sources.NewDoc("test-spans", text)

	detectors := []*Detector{
		GenericDetector,
		GenericOCRDigitDetector,
		NewSingleVolDetector("Hob.", "Hob."),
		NewSingleVolDetector("Toth", "Toth"),
		NewSingleVolDetector("Raym.", "Raym."),
	}
	for _, d := range detectors {
		t.Run(d.Reporter, func(t *testing.T) {
			cites := d.Detect(doc)
			require.NotEmpty(t, cites, "the text holds a citation for every detector")
			for _, c := range cites {
				assert.Equal(t, c.Raw, text[c.Start:c.End], "span must locate Raw")
				assert.Less(t, c.Start, c.End)
			}
		})
	}
}

// TestRemoveShadows runs real detectors over short texts, the way
// cite-detector-moml runs every detector over a page, so the spans come from
// Detect rather than being typed by hand.
func TestRemoveShadows(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		detectors []Finder
		want      []string // CleanCite of every citation kept, in order
	}{
		{
			// The defect in #267: the single-volume detector for Calthrop
			// matches the tail of a citation to the California Reports.
			name:      "same abbreviation inside a volumed citation",
			text:      "The rule in 123 Cal. 185 was different.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Cal.", "Cal.")},
			want:      []string{"123 Cal. 185"},
		},
		{
			// The single-volume abbreviation is the last word of a longer
			// one, which is the case an optional leading volume in the regex
			// could not have caught.
			name:      "abbreviation is the tail of a longer abbreviation",
			text:      "See 5 Ld. Raym. 45 for the point.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Raym.", "Raym.")},
			want:      []string{"5 Ld. Raym. 45"},
		},
		{
			name:      "single-volume match inside a longer single-volume match",
			text:      "See Ch. Cas. 45 for the point.",
			detectors: []Finder{NewSingleVolDetector("Ch. Cas.", "Ch. Cas."), NewSingleVolDetector("Cas.", "Cas.")},
			want:      []string{"Ch. Cas. 45"},
		},
		{
			// A single-volume reporter cited with the redundant volume 1. The
			// volumed form is kept and the linker's volumeForms still probes
			// the bare form, so no link is lost by dropping the shadow.
			name:      "redundant volume 1 on a single-volume reporter",
			text:      "See 1 Toth 123 for the point.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Toth", "Toth")},
			want:      []string{"1 Toth 123"},
		},
		{
			name:      "a bare single-volume citation is kept",
			text:      "See Hob. 423 for the ruling.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Hob.", "Hob.")},
			want:      []string{"Hob. 423"},
		},
		{
			name:      "adjacent citations are not shadows of each other",
			text:      "See Cal. 185; Hob. 12 for the ruling.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Cal.", "Cal."), NewSingleVolDetector("Hob.", "Hob.")},
			want:      []string{"Cal. 185", "Hob. 12"},
		},
		{
			// Two detectors whose abbreviations are prefixes of one another
			// match the same span. That is the same citation twice, not a
			// shadow; the unique index on citations_unlinked collapses it.
			// Both abbreviations have to reach stemMinAbbrLen for the shorter
			// one to stem this far -- "Toth" no longer reaches "Tothill".
			name:      "equal spans from two detectors are both kept",
			text:      "The federal view in Baldwin 125 was different.",
			detectors: []Finder{NewSingleVolDetector("Baldw.", "Baldw"), NewSingleVolDetector("Baldw.", "Baldwin")},
			want:      []string{"Baldwin 125", "Baldwin 125"},
		},
		{
			name:      "no citations",
			text:      "Nothing to see here.",
			detectors: []Finder{GenericDetector, NewSingleVolDetector("Hob.", "Hob.")},
			want:      []string{},
		},
		{
			// A reporter cited by year: the generic detector's reading without
			// the year is inside the year detector's span and names no case in
			// particular, so only the citation that carries the year is kept.
			name:      "year-cited citation shadows its year-less reading",
			text:      "Hinton v. Doe [1906] 2 K. B. 171.",
			detectors: []Finder{GenericDetector, NewYearDetector("K.B.", "K. B.")},
			want:      []string{"[1906] 2 K. B. 171"},
		},
		{
			// The year detector matches only its own reporter, so a neighbouring
			// citation to another reporter, year or no year, is untouched.
			name:      "year-cited citation does not shadow its neighbours",
			text:      "(1911) 28 T. L. R. 93; (1912) 1 K. B. 158; 76 J. P. 12.",
			detectors: []Finder{GenericDetector, NewYearDetector("K.B.", "K. B.")},
			want:      []string{"28 T. L. R. 93", "76 J. P. 12", "[1912] 1 K. B. 158"},
		},
		{
			// Without a year the generic reading is all there is, and it stays.
			name:      "year-less citation to a year-cited reporter is kept",
			text:      "See 2 K. B. 293 for the point.",
			detectors: []Finder{GenericDetector, NewYearDetector("K.B.", "K. B.")},
			want:      []string{"2 K. B. 293"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sources.NewDoc("test-shadows", tt.text)
			var found []*Citation
			for _, d := range tt.detectors {
				found = append(found, d.Detect(doc)...)
			}
			kept := RemoveShadows(found)

			got := make([]string, 0, len(kept))
			for _, c := range kept {
				got = append(got, c.CleanCite())
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRemoveShadows_VolumedInsideWiderSpan pins down the rule directly, with
// hand-built citations, for the one case the detectors cannot produce: a
// citation that carries a volume yet lies strictly inside another span that
// carries no year. It must survive, because among year-less citations only the
// single-volume detectors produce shadows.
func TestRemoveShadows_VolumedInsideWiderSpan(t *testing.T) {
	vol := 5
	inner := &Citation{Raw: "5 Cal. 185", Volume: &vol, ReporterAbbr: "Cal.", Page: 185, Start: 3, End: 13}
	outer := &Citation{Raw: "N. 5 Cal. 185", Volume: nil, ReporterAbbr: "N. 5 Cal.", Page: 185, Start: 0, End: 13}

	kept := RemoveShadows([]*Citation{inner, outer})
	assert.Equal(t, []*Citation{inner, outer}, kept)

	// The same span relation with a year on the outer citation is the
	// year-cited case, and then the inner reading goes.
	year := 1905
	outer.Year = &year
	kept = RemoveShadows([]*Citation{inner, outer})
	assert.Equal(t, []*Citation{outer}, kept)
}
