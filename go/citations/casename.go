package citations

import (
	"regexp"
	"sort"
	"strings"
)

// How a citation's case name was found. The linker weights the name as
// evidence by it, the way match_tier records why a link was made.
const (
	// CaseNameDirect: the scan found the name printed before this citation.
	CaseNameDirect = "direct"
	// CaseNameParallel: this citation follows another with nothing but
	// punctuation between ("Smith v. Jones, 2 Cromp. M. & R. 240; 3 Dowl.
	// 751"), so it is a parallel cite of that citation's case.
	CaseNameParallel = "parallel"
	// CaseNameTable: a table of cases elides a repeated plaintiff ("Summers
	// v. Farish, 10 Cal. 847 170, v. Hoover, 42 Ind. 153"); the plaintiff was
	// taken from the row above.
	CaseNameTable = "table"
)

// maxCaseNameLen bounds what is recorded. The longest real name in two
// samples of 1,000 pages was 83 characters; anything longer is a scan that
// ran through something that is not a name.
const maxCaseNameLen = 120

// backwardSeek is how many tokens before a citation the scan looks at. A
// token is a word or the space after it, so this is about fourteen words;
// eyecite's figure, from the median case-name length in CourtListener.
const backwardSeek = 28

// NameCitations finds the case name printed before each citation and records
// it on the citation as CaseName, with CaseNameSource saying how it was found
// (issue #387). It is eyecite's find_case_name, which walks backward from a
// citation through the tokens before it until a rule says the name has
// ended, with the rules the treatises need added: the OCR's spellings of
// "v.", the annotations treatises print between a name and a cite, parallel
// cites, and tables of cases. Each rule is explained where it is applied.
//
// cites are a document's citations as DetectDocument keeps them, in any
// order, with Start and End set; they are sorted by position here. A
// citation that gets no name keeps CaseName and CaseNameSource empty.
func NameCitations(text string, cites []*Citation) {
	if len(cites) == 0 {
		return
	}
	ordered := make([]*Citation, len(cites))
	copy(ordered, cites)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Start != ordered[j].Start {
			return ordered[i].Start < ordered[j].Start
		}
		return ordered[i].End > ordered[j].End
	})

	toks, idx := tokenize(text, ordered)

	type found struct {
		plaintiff, defendant string
		form                 string // "", "In re", "Ex parte"
		hasV                 bool
		vTok                 int
	}
	names := make([]found, len(ordered))

	for i, c := range ordered {
		c.CaseName, c.CaseNameSource = "", ""
		if idx[i] < 0 {
			continue
		}
		st := scanBack(toks, idx[i])
		f := found{hasV: st.vTok != -1, vTok: st.vTok}

		// "Stonor's Trusts, Re, 27 Ch. D. 195" and "LAWRENCE, EX PARTE, 4
		// Cow. 417": the scan stopped at the stop word with nothing after
		// it, and the name is the capitalised words before it.
		if strings.TrimSpace(st.candidate) == "" && st.stopWord != "" && (st.stopWord == "re" || st.stopWord == "in re" || st.stopWord == "parte") {
			if name := reversedInRe(toks, st.start-2); name != "" {
				st.candidate = name
			}
		}
		if strings.TrimSpace(st.candidate) == "" {
			continue
		}
		switch st.stopWord {
		case "in re", "re":
			f.form = "In re"
		case "parte":
			f.form = "Ex parte"
		}

		if st.vTok != -1 {
			// Split at the versus token itself rather than at a literal
			// "v.", so an OCR'd "r." splits too.
			pl := strings.TrimSpace(extractText(toks, st.start, st.vTok))
			pl = reLeadingArticle.ReplaceAllString(pl, "")
			f.plaintiff = cleanName(strings.Trim(pl, " ,("))
			f.defendant = cleanName(strings.TrimSpace(extractText(toks, st.vTok+1, st.titleStart)))
		} else {
			f.defendant = cleanName(st.candidate)
		}
		names[i] = f
	}

	// A table of cases elides a repeated plaintiff: "Summers v. Farish, 10
	// Cal. 847 170, v. Hoover, 42 Ind. 153" is Summers v. Hoover. When the
	// candidate began at the versus and the token before the versus is a
	// number, a cite or a comma -- a table row, never running text -- the
	// plaintiff is the previous named citation's.
	var prevPlaintiff string
	table := make([]bool, len(ordered))
	for i := range ordered {
		f := &names[i]
		if f.plaintiff != "" {
			prevPlaintiff = f.plaintiff
			continue
		}
		if !f.hasV || f.defendant == "" || prevPlaintiff == "" {
			continue
		}
		j := f.vTok - 1
		for j >= 0 && toks[j].kind == tokSpace {
			j--
		}
		if j >= 0 && (toks[j].kind == tokCite || reAllDigits.MatchString(toks[j].text) || strings.Trim(toks[j].text, ",") == "") {
			f.plaintiff = prevPlaintiff
			table[i] = true
		}
	}

	for i, c := range ordered {
		f := names[i]
		name := f.defendant
		switch {
		case f.plaintiff != "" && f.defendant != "":
			name = f.plaintiff + " v. " + f.defendant
		case f.plaintiff != "":
			name = f.plaintiff + " v."
			if isJunkWords(f.plaintiff) {
				name = ""
			}
		case f.form != "" && f.defendant != "":
			name = f.form + " " + f.defendant
			if isJunkName(f.defendant) {
				name = ""
			}
		default:
			if isJunkName(name) {
				name = ""
			}
		}
		if name == "" || len(name) > maxCaseNameLen {
			continue
		}
		c.CaseName = name
		c.CaseNameSource = CaseNameDirect
		if table[i] {
			c.CaseNameSource = CaseNameTable
		}
	}

	// "Smith v. Jones, 2 Cromp. M. & R. 240; 3 Dowl. P. C. 751": a cite that
	// follows another with only punctuation, a year, or an annotation
	// between is a parallel cite of the same case. The walk goes in order,
	// so a chain of three inherits down the line.
	for i, c := range ordered {
		if c.CaseName != "" || idx[i] < 0 {
			continue
		}
		j := idx[i] - 1
		for j >= 0 && (toks[j].kind == tokSpace || toks[j].kind == tokAnnot ||
			(toks[j].kind == tokWord && (strings.Trim(toks[j].text, ";,.:([") == "" || reYearToken.MatchString(toks[j].text)))) {
			j--
		}
		if j >= 0 && toks[j].kind == tokCite {
			if src := ordered[toks[j].cite]; src.CaseName != "" {
				c.CaseName = src.CaseName
				c.CaseNameSource = CaseNameParallel
			}
		}
	}
}

// ---------------------------------------------------------------- tokens

type tokKind int

const (
	tokWord  tokKind = iota // a run of non-space characters, punctuation attached
	tokSpace                // the single space between words
	tokCite                 // a detected citation, one token however many words
	tokStop                 // one of eyecite's stop words, "v" among them
	tokAnnot                // an annotation treatises print between name and cite
)

type nameToken struct {
	kind tokKind
	text string
	cite int    // index into the ordered citations, for tokCite
	stop string // the stop word, lowercased, for tokStop
	cap  bool   // counts as capitalised: an OCR'd initial next to the v
}

// eyecite's STOP_WORDS. "in re" and "see also" are two words and are merged
// by appendText.
var stopWords = map[string]bool{
	"v": true, "re": true, "quoting": true, "e.g.": true, "parte": true,
	"denied": true, "citing": true, "aff'd": true, "affirmed": true,
	"remanded": true, "see": true, "granted": true, "dismissed": true, "cf": true,
}

// stripPunct trims the non-alphanumerics eyecite allows around a stop word.
func stripPunct(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return !isAlnum(r) })
}

func isAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func first(s string) byte {
	if s == "" {
		return 0
	}
	return s[0]
}
func isUpperByte(b byte) bool { return b >= 'A' && b <= 'Z' }
func isLowerByte(b byte) bool { return b >= 'a' && b <= 'z' }
func isAlphaByte(b byte) bool { return isUpperByte(b) || isLowerByte(b) }
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// appendText splits a stretch of text between citations into word and space
// tokens, the way eyecite's Tokenizer.append_text does: split on single
// spaces, a space token between words, so that an empty part (two spaces in
// a row) still yields a space. Stop words are marked.
func appendText(toks []nameToken, s string) []nameToken {
	start := len(toks)
	for _, part := range strings.Split(s, " ") {
		if part != "" {
			toks = append(toks, nameToken{kind: tokWord, text: part}, nameToken{kind: tokSpace, text: " "})
		} else {
			toks = append(toks, nameToken{kind: tokSpace, text: " "})
		}
	}
	toks = toks[:len(toks)-1]
	for i := start; i < len(toks); i++ {
		if toks[i].kind != tokWord {
			continue
		}
		core := strings.ToLower(stripPunct(toks[i].text))
		if (core == "in" || core == "see") && i+2 < len(toks) && toks[i+1].kind == tokSpace && toks[i+2].kind == tokWord {
			next := strings.ToLower(stripPunct(toks[i+2].text))
			if (core == "in" && next == "re") || (core == "see" && next == "also") {
				merged := nameToken{kind: tokStop, text: toks[i].text + " " + toks[i+2].text, stop: core + " " + next}
				toks = append(toks[:i], append([]nameToken{merged}, toks[i+3:]...)...)
				continue
			}
		}
		if stopWords[core] {
			toks[i].kind = tokStop
			toks[i].stop = core
		}
	}
	return toks
}

// tokenize builds the document's token list with each citation as a single
// token and returns the token index of each citation, in the order given.
// The text is whitespace-normalized as it goes: the OCR separates lines with
// "\n\t\t\t", and the scan's rules assume single spaces (compare issue #283).
// A citation whose span overlaps the previous one's is folded into it, as
// eyecite skips overlapping tokens; it shares that citation's index.
func tokenize(text string, cites []*Citation) ([]nameToken, []int) {
	var toks []nameToken
	foldInto := make([]int, len(cites))
	pos := 0
	last := -1
	for i, c := range cites {
		if c.Start < pos || c.Start < 0 || c.End > len(text) || c.End < c.Start {
			foldInto[i] = last
			continue
		}
		if c.Start > pos {
			toks = appendText(toks, reSpace.ReplaceAllString(text[pos:c.Start], " "))
		}
		foldInto[i] = i
		last = i
		toks = append(toks, nameToken{kind: tokCite, text: text[c.Start:c.End], cite: i})
		pos = c.End
	}
	if pos < len(text) {
		toks = appendText(toks, reSpace.ReplaceAllString(text[pos:], " "))
	}

	toks = splitFusedVersus(toks)
	toks = dehyphenate(toks)
	unfuseFootnotes(toks)
	markOCRVersus(toks)
	markOCRInitials(toks)
	markAnnotations(toks)
	markCourtParens(toks)

	where := make(map[int]int, len(cites))
	for j, t := range toks {
		if t.kind == tokCite {
			where[t.cite] = j
		}
	}
	idx := make([]int, len(cites))
	for i := range cites {
		if foldInto[i] < 0 {
			idx[i] = -1
			continue
		}
		idx[i] = where[foldInto[i]]
	}
	return toks, idx
}

// ---------------------------------------------------------------- the scan

var (
	reYearParen        = regexp.MustCompile(`^\(\d{4}\)`)
	reLeadingArticle   = regexp.MustCompile(`^(of|the|an|and)\s+`)
	reLeadingArticleCI = regexp.MustCompile(`(?i)^(of|the|an|and)\b`)
	reCapWord          = regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]*\b`)
	reEndsDigits       = regexp.MustCompile(`\b\d+\b$`)
	reYearToken        = regexp.MustCompile(`^[-(\[]?\d{4}[)\],;]*$`)
	reAllDigits        = regexp.MustCompile(`^\d+[,.]?$`)
	reLeadingStop      = regexp.MustCompile(`(?i)^(?:see also|see|cf\.?|citing|quoting|in)\s+`)
)

// scanState is what the backward scan found.
type scanState struct {
	vTok       int    // token index of the versus, or -1
	start      int    // first token of the candidate name
	titleStart int    // one past the last token of the candidate name
	candidate  string // the text between them
	stopWord   string // the stop word the scan ended on, if any
}

func extractText(toks []nameToken, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(toks) {
		end = len(toks)
	}
	if start >= end {
		return ""
	}
	var b strings.Builder
	for _, t := range toks[start:end] {
		b.WriteString(t.text)
	}
	return b.String()
}

// capitalised is eyecite's word_str[0].isupper(), with the OCR'd initials
// markOCRInitials found counted as capitals.
func capitalised(t nameToken) bool { return isUpperByte(first(t.text)) || t.cap }

// scanBack is eyecite's _scan_for_case_boundaries: walk backward from the
// citation at citeIdx and stop where a rule says the name has ended.
func scanBack(toks []nameToken, citeIdx int) scanState {
	st := scanState{vTok: -1, start: -1}
	titleStart := citeIdx - 1
	// The OCR often drops the space before a citation: "CLAPHAM,1 Jac. & W.
	// 36". eyecite would leave the word out of the name.
	if citeIdx > 0 && toks[citeIdx-1].kind == tokWord {
		titleStart = citeIdx
	}
	caseNameLen := 0
	plaintiffLen := 0
	seenName := false
	lo := citeIdx - backwardSeek
	if lo < -1 {
		lo = -1
	}
	for index := citeIdx - 1; index > lo; index-- {
		w := toks[index]
		ws := w.text
		if ws == "," {
			continue
		}
		caseNameLen++
		if st.vTok != -1 && strings.TrimSpace(ws) != "" {
			plaintiffLen++
		}

		// Another citation, once a name word has been seen, belongs to the
		// previous entry: "Bass v. Comstock, 38 N. Y. 21 483 Bassett v.
		// Crowell, 2 Lans. 419" in a table of cases, or the preceding
		// footnote. eyecite treats every earlier cite as a parallel cite of
		// this one and would put Bass v. Comstock on both.
		if w.kind == tokCite && seenName {
			st.start = index + 2
			st.candidate = extractText(toks, st.start, titleStart)
			break
		}
		// A citation or an annotation between the name and this cite is
		// skipped, and the name ends before it: "Smith v. Jones, 1 Salk.
		// 178, 3 Dowl. 751" and "Dewey v. Hoag, sp. t., 15 Barb. 365".
		if w.kind == tokCite || (w.kind == tokAnnot && !seenName) {
			titleStart = index - 1
			continue
		}
		// An annotation after a name word ends the name, as eyecite's stop
		// words do: "(Ib.; Emery v. Pease, 20 N. Y. 62".
		if w.kind == tokAnnot {
			st.start = index + 2
			st.candidate = extractText(toks, st.start, titleStart)
			break
		}
		if w.kind == tokWord || w.kind == tokStop {
			seenName = true
		}
		// Terminal punctuation ends the name.
		if strings.HasSuffix(ws, ";") || strings.HasSuffix(ws, "”") || strings.HasSuffix(ws, "\"") {
			st.start = index + 2
			st.candidate = extractText(toks, st.start, titleStart)
			break
		}
		// A year before the cite, "(1850)", is skipped like a cite.
		if reYearParen.MatchString(ws) {
			titleStart = index - 1
			continue
		}
		// An opening paren, after the first few tokens, ends the name.
		if strings.HasPrefix(ws, "(") && caseNameLen > 3 {
			st.start = index
			if ws == "(" || (len(ws) > 1 && isAlphaByte(ws[1]) && isLowerByte(ws[1])) {
				st.start = index + 2
			}
			st.candidate = extractText(toks, st.start, titleStart)
			break
		}
		// After the versus: a word that is not capitalised ends the name,
		// other than of/the/an/and, "&" and "&c." (Waverly Stone & Gravel
		// Co.), and the OCR'd initial markOCRInitials flagged.
		if st.vTok != -1 && !capitalised(w) && strings.TrimSpace(ws) != "" &&
			ws != "of" && ws != "the" && ws != "an" && ws != "and" && ws != "&" && ws != "&c." && ws != "&c.," {
			st.start = index + 2
			st.candidate = reLeadingArticle.ReplaceAllString(extractText(toks, st.start, titleStart), "")
			break
		}
		// The versus: remember it and go on to find the plaintiff.
		if w.kind == tokStop && w.stop == "v" {
			st.vTok = index
			st.start = index - 2
			st.candidate = extractText(toks, st.start, titleStart)
			continue
		}
		// After the versus, a capitalised abbreviation of five letters or
		// more is likely the end of the previous sentence; any other stop
		// word ends the name.
		capAbbrev := st.vTok != -1 && isUpperByte(first(ws)) && len(ws) > 4 && strings.HasSuffix(ws, ".") && plaintiffLen > 1
		if capAbbrev || w.kind == tokStop {
			st.start = index + 2
			st.candidate = extractText(toks, st.start, titleStart)
			if w.kind == tokStop {
				st.stopWord = w.stop
			}
			break
		}
		// Before any versus: a lowercase word ends the name, and the name
		// starts at the first capitalised word after it.
		if st.vTok == -1 && !capitalised(w) && strings.TrimSpace(ws) != "" && isAlphaByte(first(ws)) &&
			ws != "of" && ws != "the" && ws != "an" && ws != "and" {
			if ws == "ex" || ws == "rel." {
				continue
			}
			st.start = index + 2
			cand := extractText(toks, st.start, titleStart)
			if m := reCapWord.FindStringIndex(cand); m != nil {
				cand = cand[m[0]:]
			} else {
				cand = ""
			}
			st.candidate = cand
			break
		}
		if index == 0 {
			st.candidate = reLeadingArticleCI.ReplaceAllString(extractText(toks, 0, titleStart), "")
			st.start = 0
			if reEndsDigits.MatchString(st.candidate) {
				st.candidate = ""
			}
		}
	}
	st.titleStart = titleStart
	return st
}

// cleanName is the last pass over a plaintiff, defendant or bare name: the
// stop word or leading "In" eyecite strips, the punctuation the OCR leaves,
// the sentence period the scan keeps on a name ("MASON v. WHITE."), and a
// rejection of what cannot be a name at all: fewer than two letters, or a
// leading digit.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	s = reLeadingStop.ReplaceAllString(s, "")
	s = strings.TrimLeft(s, "'\"*([ ,.;:-")
	s = trimTrailingJunk(s)
	s = trimTrailingPeriod(s)
	// Again, for the comma the period hid: "Moore v. Thompson,. 69 N. C. 121".
	s = trimTrailingJunk(s)
	letters := 0
	for _, r := range s {
		if isAlphaByte(byte(r)) {
			letters++
		}
	}
	if letters < 2 || s == "" || isDigitByte(s[0]) {
		return ""
	}
	return s
}

// trailingJunk is what no name ends with: the punctuation the scan stops
// beside and the OCR's stray marks, which the dropped-space rule in scanBack
// takes in when they touch the cite ("Darnell v. State, *18 Ark. 321" is 48
// Ark. 321, issue #396).
const trailingJunk = " ,;:(*°$§£!?~#<>/\\\"|^_=+&-"

// reTrailingNumber is a number after a comma, the head of a split volume or
// a year: "Cole v. Cassidy, 1 38 Mass. 437", "Whyte v. Robertsoni, 1891".
var reTrailingNumber = regexp.MustCompile(`,\s*\d+$`)

// trimTrailingJunk drops trailingJunk, a trailing number after a comma, and
// an apostrophe that does not follow a letter ("Lade v. Shepherd,'") from
// the end of a name, keeping a possessive ("Smiths'").
func trimTrailingJunk(s string) string {
	for {
		t := strings.TrimRight(s, trailingJunk)
		t = reTrailingNumber.ReplaceAllString(t, "")
		if n := len(t); n > 0 && t[n-1] == '\'' && (n == 1 || !isAlphaByte(t[n-2])) {
			t = t[:n-1]
		}
		if t == s {
			return s
		}
		s = t
	}
}

// trimTrailingPeriod drops the sentence period the scan keeps on a name
// ("WHITE." -> "WHITE") while leaving an abbreviation ("Co.", "R. R.") alone.
func trimTrailingPeriod(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name
	}
	last := name
	if i := strings.LastIndex(name, " "); i >= 0 {
		last = name[i+1:]
	}
	core := strings.TrimSuffix(last, ".")
	letters := 0
	for _, r := range core {
		if isAlphaByte(byte(r)) {
			letters++
		}
	}
	if letters >= 4 && !strings.Contains(core, ".") {
		return strings.TrimSuffix(name, ".")
	}
	return name
}

// ------------------------------------------- what the treatises need added

// reOCRVersus is what the OCR makes of "v.": r. a. c. t. s. u. '. and a lone
// period, and the spelled-out vs.; "ads." (ad sectam) and "agt." are the
// nineteenth century's own forms. They count only between two capitalised
// words, which is where no reporter abbreviation has them.
var reOCRVersus = regexp.MustCompile(`^[(\[]?(?:v|V|vs|VS|r|a|c|t|s|u|rs|'|ads|agt|\.)[.:,']*$`)

// reOCRVersusLoose are the forms that are never an initial, so they count
// after a number or a comma too: a table of cases whose plaintiff is elided
// ("170, v. Hoover", "289 r. Smith").
var reOCRVersusLoose = regexp.MustCompile(`^(?:v|V|vs|r|')[.:,']*$`)

func capWord(t nameToken) bool {
	s := strings.TrimLeft(t.text, "([")
	return t.kind == tokWord && isUpperByte(first(s))
}

func markOCRVersus(toks []nameToken) {
	for i := 2; i+2 < len(toks); i++ {
		t := toks[i]
		if t.kind != tokWord || !reOCRVersus.MatchString(t.text) {
			continue
		}
		leftOK := capWord(toks[i-2]) ||
			(reOCRVersusLoose.MatchString(t.text) && (isDigitByte(first(toks[i-2].text)) || toks[i-2].text == ","))
		if toks[i-1].kind == tokSpace && toks[i+1].kind == tokSpace && leftOK && capWord(toks[i+2]) {
			toks[i].kind = tokStop
			toks[i].stop = "v"
		}
	}
}

// markOCRInitials: a word next to the versus whose first letter the OCR
// lowered ("Sea Insurance Co. v. lladden") is a name all the same.
func markOCRInitials(toks []nameToken) {
	for i := range toks {
		if toks[i].kind != tokStop || toks[i].stop != "v" {
			continue
		}
		for _, j := range []int{i - 2, i + 2} {
			if j >= 0 && j < len(toks) && toks[j].kind == tokWord && isLowerByte(first(toks[j].text)) {
				toks[j].cap = true
			}
		}
	}
}

// reFusedVersus: the OCR drops the space before the versus, "289,v. Foot" in
// a table of cases or "FLEURYv. ROGET" in a running head.
var reFusedVersus = regexp.MustCompile(`^(\d+[,.]?|[A-Z][A-Za-z']{3,})(v\.|vs\.|r\.)$`)

func splitFusedVersus(toks []nameToken) []nameToken {
	out := make([]nameToken, 0, len(toks))
	for _, t := range toks {
		if t.kind == tokWord {
			if m := reFusedVersus.FindStringSubmatch(t.text); m != nil {
				out = append(out, nameToken{kind: tokWord, text: m[1]}, nameToken{kind: tokSpace, text: " "}, nameToken{kind: tokWord, text: m[2]})
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// dehyphenate joins a word broken across a line, "Cru- ger" -> "Cruger",
// when the continuation starts with a lowercase letter.
func dehyphenate(toks []nameToken) []nameToken {
	out := make([]nameToken, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind == tokWord && len(t.text) > 1 && strings.HasSuffix(t.text, "-") &&
			i+2 < len(toks) && toks[i+1].kind == tokSpace && toks[i+2].kind == tokWord &&
			isLowerByte(first(toks[i+2].text)) {
			out = append(out, nameToken{kind: tokWord, text: strings.TrimSuffix(t.text, "-") + toks[i+2].text})
			i += 2
			continue
		}
		out = append(out, t)
	}
	return out
}

// reFusedFootnote: a footnote number the OCR fused to the name, "4Tuller".
var reFusedFootnote = regexp.MustCompile(`^\d{1,2}([A-Z][a-z].*)$`)

func unfuseFootnotes(toks []nameToken) {
	for i := range toks {
		if toks[i].kind == tokWord {
			if m := reFusedFootnote.FindStringSubmatch(toks[i].text); m != nil {
				toks[i].text = m[1]
			}
		}
	}
}

// The annotations treatises print between a case name and its citation --
// "Dewey v. Hoag, sp. t., 15 Barb. 365" (special term), "per Denio, J.,",
// "S. C.,", "[Ct. of Errors]", "affirmed,", "supra,", a judge before the
// name ("Parke, B., Embrey v. Owen"), a bracketed page number. The scan
// skips them when they sit between the name and the cite, and lets them end
// the name otherwise.
var (
	reAnnot1 = regexp.MustCompile(`^\[\d+\][,;.]*$|(?i)^[(\[]?(?:supra|ante|post|ib\.|ibid\.|affirmed|reversed|affirming|reversing|approved|overruled|overruling|followed|cited|distinguished|dissenting|dictum|semble|contra|\(in\s+error\)|\[?in\s+error\]?)[,;.]*[)\]]?$`)
	reAnnot2 = regexp.MustCompile(`^[(\[]?[A-Z][a-z]+,\s+(?:B|J|C\.\s?J|L\.\s?J|V\.\s?C|M\.\s?R)\.,?$|(?i)^[(\[]?(?:(?:sp|gen|spec|special|general)\.?\s+t(?:\.|erm)?|S\.\s*C\.|in\s+error|on\s+appeal|in\s+equity|\d+(?:st|nd|rd|th)\s+dist\.|per\s+[A-Z][\w'.]+)[,;.]*$`)
	reAnnot3 = regexp.MustCompile(`^(?:per\s+[A-Z][\w'.]+,?\s+(?:J|Ch|C\.\s*J)\.|\[?Ct\.\s+of\s+(?:Errors|Appeals)\]?|[A-Z][\w'.]+,?\s+J\.,?\s+dissenting|Ct\.\s+of\s+App\.)[,;.]*$`)
)

func markAnnotations(toks []nameToken) {
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokWord && toks[i].kind != tokStop {
			continue
		}
		if i+4 < len(toks) && toks[i+2].kind == tokWord && toks[i+4].kind == tokWord {
			if reAnnot3.MatchString(toks[i].text + " " + toks[i+2].text + " " + toks[i+4].text) {
				toks[i].kind, toks[i+2].kind, toks[i+4].kind = tokAnnot, tokAnnot, tokAnnot
				i += 4
				continue
			}
		}
		if i+2 < len(toks) && toks[i+2].kind == tokWord {
			if reAnnot2.MatchString(toks[i].text + " " + toks[i+2].text) {
				toks[i].kind, toks[i+2].kind = tokAnnot, tokAnnot
				i += 2
				continue
			}
		}
		if reAnnot1.MatchString(toks[i].text) {
			toks[i].kind = tokAnnot
		}
	}
}

// reCourtParenWord is one word of a court parenthetical: an abbreviation
// ("C.", "Civ.", "App.", "N.", "Cir."), an ordinal ("7th", "2d"), or a year,
// with the parenthesis and comma it may carry.
var reCourtParenWord = regexp.MustCompile(`^\(?(?:[A-Z][A-Za-z]{0,7}\.|\d+(?:st|nd|rd|th|d)\.?|\d{4}),?\)?[,.;]?$`)

// maxCourtParenWords bounds a court parenthetical; the longest in the
// samples, "(C. C. A., 7th Cir.)", is five words.
const maxCourtParenWords = 6

// markCourtParens marks the court parenthetical printed between a name and
// its cite -- "Smith v. Jones (C. C.) 120 Fed. 50", "(D. C., N. Y.)",
// "(Tex. Civ. App.)", "(C. C. A., 7th Cir.)" -- as an annotation, so the
// scan skips it and reads the name before it. Without this the scan stopped
// at the opening parenthesis and recorded "C. C.)" as the name (issue #398).
func markCourtParens(toks []nameToken) {
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokWord || !strings.Contains(toks[i].text, ")") {
			continue
		}
		// A year alone, "(1899),", is left to scanBack's year rule, which
		// skips it; as an annotation it would end the name before it ("In re
		// Hixon (1899), S. Dist. Ia., Woolson, J., 93 Fed. 440").
		words, court := 0, false
		for j := i; j >= 0 && words < maxCourtParenWords; j-- {
			if toks[j].kind == tokSpace {
				continue
			}
			if toks[j].kind != tokWord || !reCourtParenWord.MatchString(toks[j].text) {
				break
			}
			if j < i && strings.Contains(toks[j].text, ")") {
				break
			}
			words++
			if !reYearToken.MatchString(toks[j].text) {
				court = true
			}
			if strings.HasPrefix(toks[j].text, "(") {
				if !court {
					break
				}
				for k := j; k <= i; k++ {
					if toks[k].kind == tokWord {
						toks[k].kind = tokAnnot
					}
				}
				break
			}
		}
	}
}

// What a lone party cannot be (issue #398). The scan takes the capitalised
// words before a cite, and where no versus is printed it often takes words
// that are not a party at all: a signal or connective ("Id.", "and", "By
// the"), the head of a statute ("Section", "Laws of", "Act No."), a month,
// a court, an author's possessive ("Kent's 2 Comm."), or the tail of a
// corporate name the scan cut short ("Co.", "R. R. Co."). They are checked
// only where the whole recorded name is one party -- a bare name, "In re X",
// or a plaintiff with no defendant -- so "R. R. Co. v. Smith" and "U. S. v.
// Jones" keep their abbreviated party.
var (
	junkNameWords = map[string]bool{
		"id": true, "ib": true, "ibid": true, "idem": true, "vide": true, "see": true, "note": true,
		"in": true, "by": true, "by the": true, "the": true, "and": true, "of": true, "al": true, "et al": true,
		"section": true, "sections": true, "sec": true, "secs": true, "sect": true, "sects": true,
		"stat": true, "stats": true, "statute": true, "statutes": true, "act": true, "acts": true,
		"act of": true, "acts of": true, "act no": true, "the act": true, "law": true, "laws": true,
		"laws of": true, "chapter": true, "chap": true, "rule": true, "rules": true, "article": true,
		"art": true, "subdivision": true, "form": true, "no": true, "nos": true, "revised statutes": true,
		"judgment": true, "charges": true, "appeal": true, "court": true, "court of appeals": true,
		"supreme court": true, "house of lords": true, "year book": true, "law rep": true,
		"january": true, "february": true, "april": true, "june": true, "july": true, "august": true,
		"september": true, "october": true, "november": true, "december": true,
	}
	// A sovereign alone names no case: it is the plaintiff of one whose
	// defendant the scan never reached.
	junkSovereigns = map[string]bool{
		"state": true, "people": true, "commonwealth": true, "united states": true,
		"rex": true, "reg": true, "regina": true,
	}
	// Words that are an initial, a short abbreviation ("Co.", "Ry.", "St.",
	// "S.C."), a corporate abbreviation, or a roman numeral: a name made of
	// nothing else is the tail of a longer one, a judge's title, or a place.
	reAbbrevOnlyWord = regexp.MustCompile(`^(?:(?:[A-Z][a-z]?\.)+|Ins\.|Rail\.|Soc\.|Bk\.|Mfg\.|Ass'n|Assn\.|[IVXL]+\.?)$`)
	rePossessiveWord = regexp.MustCompile(`^[A-Z][a-z]+(?:'|’)s$`)
)

// maxAbbrevOnlyWords is the longest name of nothing but abbreviations that is
// junk: "R. R. Co." is the tail of a railroad's name, "G. W. Ry. Co." a name.
const maxAbbrevOnlyWords = 3

// junkKey is a name lowercased without its periods, for the word lists.
func junkKey(s string) string {
	key := strings.ToLower(strings.TrimRight(strings.TrimSpace(s), ".,;: "))
	return strings.ReplaceAll(key, ".", "")
}

// isJunkWords reports whether a party is words that are never a party: a
// cut court parenthetical, all lowercase, or a signal, statute head, month
// or court, with or without a number after it. It is the test for a
// plaintiff with no defendant, where "State v." and "U. S. v." are kept.
func isJunkWords(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	// A court parenthetical the scan cut in half: "C. C.)", "Tex. Civ. App.)".
	// Only at the end, since the OCR puts a stray one inside a real name
	// ("In re Bradfi)rd").
	if strings.HasSuffix(strings.TrimRight(s, ".,; "), ")") && !strings.Contains(s, "(") {
		return true
	}
	if strings.ToLower(s) == s {
		return true
	}
	key := junkKey(s)
	if junkNameWords[key] {
		return true
	}
	// "Section 4", "Laws of 1890", "No. 2": a junk word and a number.
	fields := strings.Fields(key)
	return len(fields) > 1 && reAllDigits.MatchString(fields[len(fields)-1]) &&
		junkNameWords[strings.Join(fields[:len(fields)-1], " ")]
}

// isJunkName reports whether a name that stands alone, with no versus,
// cannot be a case name: isJunkWords, a bare sovereign, an author's
// possessive, or nothing but abbreviations.
func isJunkName(s string) bool {
	if isJunkWords(s) || junkSovereigns[junkKey(s)] {
		return true
	}
	words := strings.Fields(s)
	if len(words) == 1 && rePossessiveWord.MatchString(words[0]) {
		return true
	}
	// Up to three words: "G. W. Ry. Co." is the Great Western Railway.
	if len(words) > maxAbbrevOnlyWords {
		return false
	}
	for _, w := range words {
		w = strings.ReplaceAll(strings.TrimRight(w, ",;:"), "..", ".")
		if !reAbbrevOnlyWord.MatchString(w) {
			return false
		}
	}
	return true
}

// reversedInRe reads "Stonor's Trusts, Re," or "LAWRENCE, EX PARTE,": the
// stop word is at stopIdx and the name is the capitalised words before it.
func reversedInRe(toks []nameToken, stopIdx int) string {
	if stopIdx < 0 || stopIdx >= len(toks) || toks[stopIdx].kind != tokStop {
		return ""
	}
	var words []string
	for j := stopIdx - 2; j >= 0 && len(words) < 6; j -= 2 {
		if toks[j+1].kind != tokSpace || toks[j].kind != tokWord {
			break
		}
		w := toks[j].text
		if strings.HasSuffix(w, ";") || (strings.HasSuffix(w, ".") && len(w) > 3) {
			break
		}
		if !isUpperByte(first(strings.TrimLeft(w, "(["))) && w != "of" && w != "the" && w != "and" {
			break
		}
		words = append([]string{w}, words...)
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Trim(strings.Join(words, " "), " ,([")
}
