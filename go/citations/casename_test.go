package citations

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nameTest runs the detectors over text the way cite-detector-moml does --
// DetectDocument, so the shadows are gone before the names are read -- and
// returns CleanCite -> (CaseName, CaseNameSource) for every citation kept.
func detectNames(t *testing.T, text string, finders ...Finder) map[string][2]string {
	t.Helper()
	if len(finders) == 0 {
		finders = []Finder{GenericDetector}
	}
	doc := sources.NewDoc("test-names", text)
	kept, _ := DetectDocument(doc, finders, sources.NewOCRReplacer(nil))
	got := make(map[string][2]string, len(kept))
	for _, c := range kept {
		got[c.CleanCite()] = [2]string{c.CaseName, c.CaseNameSource}
	}
	return got
}

// TestNameCitations covers eyecite's rules and each rule the treatises
// needed on top of them, with the shapes the two samples of 1,000 pages
// showed (issue #387). Each case lists every citation the text yields and
// the name expected on it; "" is no name.
func TestNameCitations(t *testing.T) {
	tests := []struct {
		name string
		text string
		want map[string][2]string
	}{
		{
			name: "plain Bluebook form, eyecite's own test",
			text: "Lissner v. Test, 1 U.S. 1 (1982).",
			want: map[string][2]string{"1 U.S. 1": {"Lissner v. Test", CaseNameDirect}},
		},
		{
			name: "name in parentheses, treatise footnote style",
			text: "costs. (Robb v. Macdonald, 12 Abb. 213.) And the Superior Court",
			want: map[string][2]string{"12 Abb. 213": {"Robb v. Macdonald", CaseNameDirect}},
		},
		{
			name: "no name before a statute",
			text: "as enacted; see 17 & 18 Vict. c. 104, s. 266.",
			want: map[string][2]string{"18 Vict. c. 104": {"", ""}},
		},
		{
			name: "OCR reads v. as r.",
			text: "16-5; Crary r. Goodman, 12 N. Y. 266, 268; N. Y.",
			want: map[string][2]string{"12 N. Y. 266": {"Crary v. Goodman", CaseNameDirect}},
		},
		{
			name: "OCR reads v. as a lone period, in a table of cases",
			text: "ii. 1047, 1056. HART . SEIXAS, 21 Wend. 40; 1. 24t, 290.",
			want: map[string][2]string{"21 Wend. 40": {"HART v. SEIXAS", CaseNameDirect}},
		},
		{
			name: "vs. and an uppercase V.",
			text: "exempt. IIinely vs. Wyatt, 1 Bay, 102. BOYNTON V. PAGE, 13 Wend. 425.",
			want: map[string][2]string{
				"1 Bay 102":    {"IIinely v. Wyatt", CaseNameDirect},
				"13 Wend. 425": {"BOYNTON v. PAGE", CaseNameDirect},
			},
		},
		{
			name: "special term annotation between name and cite",
			text: "(Dewey v. Hoag, sp. t., 15 Barb. 365; Moulton v. Townsend, sp. t, 16 How. 306.)",
			want: map[string][2]string{
				"15 Barb. 365": {"Dewey v. Hoag", CaseNameDirect},
				"16 How. 306":  {"Moulton v. Townsend", CaseNameDirect},
			},
		},
		{
			name: "annotation in parentheses and a judge before the name",
			text: "Valton v. National Loan So. (sp. t., 19 How. 515). (o) Parke, B., Embrey v. Owen, 6 Exch. 871.",
			want: map[string][2]string{
				"19 How. 515": {"Valton v. National Loan So.", CaseNameDirect},
				"6 Exch. 871": {"Embrey v. Owen", CaseNameDirect},
			},
		},
		{
			name: "an annotation before the name ends it rather than being skipped",
			text: "distinction (Ib.; Emery v. Pease, 20 N. Y. 62; Despard v. Walbridge, 15 N. Y. 374).",
			want: map[string][2]string{
				"20 N. Y. 62":  {"Emery v. Pease", CaseNameDirect},
				"15 N. Y. 374": {"Despard v. Walbridge", CaseNameDirect},
			},
		},
		{
			name: "parallel cites inherit the name across ; : and a parenthesis",
			text: "Barr v. Jackson, 1 Phil. 582 : 9 Jur. 609; 1 Y. & C. C. C. 585. Davies v. Robinson, 65 Ark. 219 (45 S. W. Rep. 471).",
			want: map[string][2]string{
				"1 Phil. 582":         {"Barr v. Jackson", CaseNameDirect},
				"9 Jur. 609":          {"Barr v. Jackson", CaseNameParallel},
				"1 Y. & C. C. C. 585": {"Barr v. Jackson", CaseNameParallel},
				"65 Ark. 219":         {"Davies v. Robinson", CaseNameDirect},
				"45 S. W. Rep. 471":   {"Davies v. Robinson", CaseNameParallel},
			},
		},
		{
			name: "a semicolon between two different cases does not inherit",
			text: "see Price v. Parker, 1 Salk. 178; Boucher v. Lawson, 2 Str. 200.",
			want: map[string][2]string{
				"1 Salk. 178": {"Price v. Parker", CaseNameDirect},
				"2 Str. 200":  {"Boucher v. Lawson", CaseNameDirect},
			},
		},
		{
			name: "word hyphenated across a line, and no space before the cite",
			text: "see Cru- ger v. Douglass, 8 Barb. 81. WILSON v. CLAPHAM,1 Jac. & W. 36; ii. 64.",
			want: map[string][2]string{
				"8 Barb. 81":     {"Cruger v. Douglass", CaseNameDirect},
				"1 Jac. & W. 36": {"WILSON v. CLAPHAM", CaseNameDirect},
			},
		},
		{
			name: "table of cases: an earlier cite belongs to the previous row, and an elided plaintiff is the row above's",
			text: "483 Bass v. Comstock, 38 N. Y. 21 483 Bassett v. Crowell, 2 Lans. 419 808 Summers v. Farish, 10 Cal. 847 170, v. Hoover, 42 Ind. 153 710",
			want: map[string][2]string{
				"38 N. Y. 21": {"Bass v. Comstock", CaseNameDirect},
				"2 Lans. 419": {"Bassett v. Crowell", CaseNameDirect},
				"10 Cal. 847": {"Summers v. Farish", CaseNameDirect},
				"42 Ind. 153": {"Summers v. Hoover", CaseNameTable},
			},
		},
		{
			name: "In re, Ex parte, and the reversed UK form",
			text: "In re Barrington, 2 Scho. & Les. 112; Ex parte Wilson, 1 Atk. 218; Stonor's Trusts, Re, 27 Ch. D. 195.",
			want: map[string][2]string{
				"2 Scho. & Les. 112": {"In re Barrington", CaseNameDirect},
				"1 Atk. 218":         {"Ex parte Wilson", CaseNameDirect},
				"27 Ch. D. 195":      {"In re Stonor's Trusts", CaseNameDirect},
			},
		},
		{
			name: "an OCR-lowered initial next to the v, an ampersand in the plaintiff, a fused footnote number",
			text: "Sea Insurance Co. v. lladden, 13 Q. B. D. 706. Waverly Stone & Gravel Co. v. Waterloo Ry. Co., 239 Fed. 561. 4Tuller v. Talbot, 23 Ill. 357.",
			want: map[string][2]string{
				"13 Q. B. D. 706": {"Sea Insurance Co. v. lladden", CaseNameDirect},
				"239 Fed. 561":    {"Waverly Stone & Gravel Co. v. Waterloo Ry. Co.", CaseNameDirect},
				"23 Ill. 357":     {"Tuller v. Talbot", CaseNameDirect},
			},
		},
		{
			name: "trailing sentence period trimmed, abbreviation kept",
			text: "MASON v. WHITE. 11 Barb. 173; Hart v. City Theatres Co. 215 N. Y. 322.",
			want: map[string][2]string{
				"11 Barb. 173":  {"MASON v. WHITE", CaseNameDirect},
				"215 N. Y. 322": {"Hart v. City Theatres Co.", CaseNameDirect},
			},
		},
		{
			name: "the plaintiff keeps its lowercase words",
			text: "(Bank of Genesee v. Spencer, 15 How. 412.)",
			want: map[string][2]string{"15 How. 412": {"Bank of Genesee v. Spencer", CaseNameDirect}},
		},
		{
			name: "junk is not a name: a page number, a bare conjunction",
			text: "Traverse Act (31 & 32 Vict. c. 70). ring, 30 Ky. L. 270, 302, 5 So. 569.",
			want: map[string][2]string{
				"32 Vict. c. 70": {"", ""},
				"30 Ky. L. 270":  {"", ""},
				"5 So. 569":      {"", ""},
			},
		},
		{
			name: "stray OCR mark touching the cite (issue #396)",
			text: "cited. Darnell v. State, *18 Ark. 321; Dyer v. Walker, 40 Pa. St. 157;",
			want: map[string][2]string{
				"18 Ark. 321":    {"Darnell v. State", CaseNameDirect},
				"40 Pa. St. 157": {"Dyer v. Walker", CaseNameDirect},
			},
		},
		{
			name: "degree sign fused to the name (issue #396)",
			text: "as held in Clark v. Merriam,°5 Conn. 576, and",
			want: map[string][2]string{"5 Conn. 576": {"Clark v. Merriam", CaseNameDirect}},
		},
		{
			name: "comma hidden behind a period (issue #396)",
			text: "Vroom, 527; Moore v. Thompson,. 69 N. C. 121; and",
			want: map[string][2]string{"69 N. C. 121": {"Moore v. Thompson", CaseNameDirect}},
		},
		{
			name: "a court parenthetical between name and cite is skipped (issue #398)",
			text: "Smith v. Jones (C. C.) 120 Fed. 50; Brown v. White (C. C. A., 7th Cir.) 130 Fed. 9; Doe v. Roe (Tex. Civ. App.) 120 S. W. 50.",
			want: map[string][2]string{
				"120 Fed. 50":  {"Smith v. Jones", CaseNameDirect},
				"130 Fed. 9":   {"Brown v. White", CaseNameDirect},
				"120 S. W. 50": {"Doe v. Roe", CaseNameDirect},
			},
		},
		{
			name: "a year in parentheses is still the scan's year rule, not a court parenthetical (issue #398)",
			text: "Objection to discharge must be specific. In re Hixon (1899), S. Dist. Ia., Woolson, J., 93 Fed., 440; 1 A. B. R., 610.",
			want: map[string][2]string{
				"93 Fed. 440":    {"In re Hixon", CaseNameDirect},
				"1 A. B. R. 610": {"In re Hixon", CaseNameParallel},
			},
		},
		{
			name: "words that are not a party get no name (issue #398)",
			text: "settled. Id., 12 Mass. 100. As provided by Section 4, 13 Mass. 100. Decided in October, 14 Mass. 100. Laws of 1890, 15 Mass. 100.",
			want: map[string][2]string{
				"12 Mass. 100": {"", ""},
				"13 Mass. 100": {"", ""},
				"14 Mass. 100": {"", ""},
				"15 Mass. 100": {"", ""},
			},
		},
		{
			name: "a lone sovereign or corporate tail gets no name, a two-party name keeps its abbreviations (issue #398)",
			text: "held so in State, 12 Ind. 100; and In re Co., 13 Ind. 100; Brown v. Erie R. Co., 12 N. Y. 266; U. S. v. Jones, 3 Cranch 9.",
			want: map[string][2]string{
				"12 Ind. 100":  {"", ""},
				"13 Ind. 100":  {"", ""},
				"12 N. Y. 266": {"Brown v. Erie R. Co.", CaseNameDirect},
				"3 Cranch 9":   {"U. S. v. Jones", CaseNameDirect},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectNames(t, tt.text)
			for cite, want := range tt.want {
				g, ok := got[cite]
				require.True(t, ok, "citation %q not detected; detected: %v", cite, keys(got))
				assert.Equal(t, want[0], g[0], "case name of %q", cite)
				assert.Equal(t, want[1], g[1], "source of %q", cite)
			}
			assert.Len(t, got, len(tt.want), "unexpected citations: %v", keys(got))
		})
	}
}

// TestNameCitations_SurnameShadow is the interplay with issue #367: the
// single-volume spelling "Comst" reads "Comstock, 38" as a citation beside
// "38 N. Y. 21". Without RemoveShadows dropping it, the scan would start
// from the junk reading and give the real citation no name.
func TestNameCitations_SurnameShadow(t *testing.T) {
	got := detectNames(t, "297 Bass v. Comstock, 38 N. Y. 21 483 Bassett v. Crowell, 2 Lans. 419",
		GenericDetector, NewSingleVolDetector("Comst.", "Comst"))
	assert.Equal(t, map[string][2]string{
		"38 N. Y. 21": {"Bass v. Comstock", CaseNameDirect},
		"2 Lans. 419": {"Bassett v. Crowell", CaseNameDirect},
	}, got)
}

// TestNameCitations_CaseNameReading is the interplay with issue #373: the
// generic detector read "594 Coryell r. Cain, 16" as a citation to a reporter
// called "Coryell r. Cain,", which never links and swallowed the name of the
// citation beside it.
func TestNameCitations_CaseNameReading(t *testing.T) {
	got := detectNames(t, "591, 594 Coryell r. Cain, 16 Cal. 567 548 Cosby v. Wickliffe, 1 Dana 100")
	assert.Equal(t, map[string][2]string{
		"16 Cal. 567": {"Coryell v. Cain", CaseNameDirect},
		"1 Dana 100":  {"Cosby v. Wickliffe", CaseNameDirect},
	}, got)
}

// TestNameCitations_Length: a scan that ran through something that is not a
// name is dropped rather than recorded.
func TestNameCitations_Length(t *testing.T) {
	c := &Citation{Raw: "1 U.S. 1", Start: 0, End: 8}
	long := "Aaaaaaaaaaaaaaaaaaaa Bbbbbbbbbbbbbbbbbbbb Cccccccccccccccccccc Dddddddddddddddddddd Eeeeeeeeeeeeeeeeeeee Ffffffffffffffffffff Gggggggggggggggggggg"
	text := long + " v. Smith, 1 U.S. 1"
	c.Start, c.End = len(text)-8, len(text)
	NameCitations(text, []*Citation{c})
	assert.Empty(t, c.CaseName)
	assert.Empty(t, c.CaseNameSource)
}

// TestIsJunkName: what a name that stands alone cannot be (issue #398), and
// the bare names that are kept because they may be real.
func TestIsJunkName(t *testing.T) {
	junk := []string{"C. C.)", "D. C., N. Y.)", "Tex. Civ. App.)", "No. 2)", "Co.", "Co..", "R. Co.", "R. R. Co.",
		"Ry. Co.", "Ins. Co.", "U. S.", "N. Y.", "L. B.", "S.C.", "II.", "and", "the", "By the", "In", "Id.",
		"Vide", "Section", "Sections", "Sec.", "Section 4", "Laws of", "Laws of 1890", "Act No.", "Chapter",
		"Rule", "Revised Statutes", "October", "Court of Appeals", "Supreme Court", "Kent's", "Kent’s",
		"State", "People", "Commonwealth", "United States"}
	for _, s := range junk {
		assert.True(t, isJunkName(s), "%q should be junk", s)
	}
	real := []string{"Smith", "Anon", "Anonymous", "May", "March", "Case", "Canal Co.", "Erie R. R. Co.",
		"Pullman Co.", "New York", "Boston", "King", "Smith's Appeal", "Wills", "G. W. Ry. Co.",
		"In re Bradfi)rd", "Tynemouth R. 1). C."}
	for _, s := range real {
		assert.False(t, isJunkName(s), "%q should be kept", s)
	}
	// A plaintiff with no defendant keeps a sovereign or an abbreviation.
	for _, s := range []string{"State", "U. S.", "Rex"} {
		assert.False(t, isJunkWords(s), "plaintiff %q should be kept", s)
	}
	assert.True(t, isJunkWords("al."))
}

func keys(m map[string][2]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
