package citations

import (
	"sort"
	"strings"
)

// ocrConfusions are the substitutions the OCR makes inside a reporter
// abbreviation, each as the pair (what the OCR wrote, what the text said). The
// list is the one issue #247 proposes: the letter-shape confusions seen in the
// non-whitelisted tail ("Vill." for "Will.", "Fcd." for "Fed.", "Cornst." for
// "Comst."), plus the digit-for-letter readings that stripInteriorDigits cannot
// repair because the digit replaced a letter rather than being inserted
// ("I1l." for "Ill."). Both directions of each confusion are listed because the
// OCR is not consistent about which way it errs.
//
// This is a candidate generator, not a normalizer: every string it produces is
// looked up as an exact spelling in legalhist.whitelist, and a reading that is
// not already a known spelling is discarded. The whitelist keeps its exact
// match semantics and the detector goes on recording what the OCR said.
var ocrConfusions = [][2]string{
	{"V", "W"}, {"W", "V"}, {"v", "w"}, {"w", "v"},
	{"rn", "m"}, {"m", "rn"},
	{"li", "h"}, {"h", "li"},
	{"c", "e"}, {"e", "c"},
	{"t", "l"}, {"l", "t"},
	{"f", "s"}, {"s", "f"},
	{"1", "l"}, {"l", "1"}, {"1", "I"}, {"I", "1"}, {"I", "l"}, {"l", "I"},
	{"0", "O"}, {"O", "0"},
	{"5", "S"}, {"S", "5"},
	{"8", "B"}, {"B", "8"},
	{"u", "n"}, {"n", "u"},
	{"ii", "n"}, {"n", "ii"},
	{"cl", "d"}, {"d", "cl"},
	{"vv", "w"}, {"w", "vv"},
	{"C", "G"}, {"G", "C"},
}

// Rules name how a corrected spelling was derived from the one the OCR wrote.
// digit_letter exists for the ambiguity check rather than for proposing: the
// digit strip assumes the OCR inserted the digit, but it may instead have
// replaced a letter ("Mic." scanned as "M1c."), and PR #241 rejected a strip
// whenever some letter in the digit's place gave a different reporter that was
// common enough to compete. Reading the digit as every letter reproduces that
// check; a reading found only this way is reported for review, never seeded.
const (
	RuleDigit       = "digit"        // stripInteriorDigits: a letter-flanked digit removed
	RuleConfusion   = "confusion"    // one ocrConfusions substitution at one site
	RuleDigitLetter = "digit_letter" // a letter-flanked digit read as some letter
)

const asciiLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Reading is one corrected spelling proposed for a scanned one.
type Reading struct {
	Corrected string
	Rule      string
}

// confusionVariants returns every spelling one ocrConfusions substitution away
// from s, applied at a single site, without s itself and without duplicates,
// in a deterministic order.
func confusionVariants(s string) []string {
	seen := map[string]bool{s: true}
	var out []string
	for _, c := range ocrConfusions {
		from, to := c[0], c[1]
		for i := 0; i+len(from) <= len(s); i++ {
			if !strings.HasPrefix(s[i:], from) {
				continue
			}
			v := s[:i] + to + s[i+len(from):]
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	sort.Strings(out)
	return out
}

// readings returns the corrected spellings proposed for a scanned one, in
// order of rule specificity: the digit strip, then the confusion table, then a
// letter-flanked digit read as each letter. A spelling several rules reach is
// reported once, under the first.
func readings(found string) []Reading {
	seen := map[string]bool{found: true}
	var out []Reading
	add := func(corrected, rule string) {
		if !seen[corrected] {
			seen[corrected] = true
			out = append(out, Reading{Corrected: corrected, Rule: rule})
		}
	}
	add(stripInteriorDigits(found), RuleDigit)
	for _, v := range confusionVariants(found) {
		add(v, RuleConfusion)
	}
	for _, loc := range reInteriorDigit.FindAllStringIndex(found, -1) {
		// loc spans letter, digit, letter; the digit is the middle byte.
		i := loc[0] + 1
		for _, l := range asciiLetters {
			add(found[:i]+string(l)+found[i+1:], RuleDigitLetter)
		}
	}
	return out
}

// WhitelistLookup is what the resolver knows about the current whitelist and
// corpus: for each spelling, where the whitelist sends it, whether that target
// is itself a canonical reporter_standard, and how often the spelling occurs in
// moml_citations.citations_unlinked.
type WhitelistLookup struct {
	Standard  map[string]string // whitelisted spelling -> reporter_standard ("" for junk)
	Junk      map[string]bool   // whitelisted spelling -> junk
	Canonical map[string]bool   // reporter_standard -> true
	Freq      map[string]int    // spelling -> rows in citations_unlinked
}

// Candidate is one reading that landed on a whitelisted spelling.
type Candidate struct {
	Reading
	Standard  string // the reporter the whitelist sends the corrected spelling to
	Canonical bool   // the corrected spelling is itself a reporter_standard
	Freq      int    // corpus frequency of the corrected spelling
}

// Resolution is the verdict for one scanned spelling.
type Resolution struct {
	Kind       string      // "proposed", "ambiguous", "tojunk", or "unresolved"
	Best       Candidate   // the proposal, when Kind is "proposed"
	Candidates []Candidate // every reading that landed on a whitelisted spelling
}

const (
	KindProposed   = "proposed"
	KindAmbiguous  = "ambiguous"
	KindToJunk     = "tojunk"
	KindUnresolved = "unresolved"
)

// resolve decides what the readings of found amount to. Readings that land on
// no whitelisted spelling are discarded. When the survivors point at more than
// one reporter, the reporters are compared by the corpus frequency of the
// spellings that reach them; if the runner-up reaches at least threshold times
// the winner's frequency the reading is in real doubt and the spelling is
// reported as ambiguous rather than proposed. That is the rule PR #241 applied
// by hand ("M1d." reads as Md. but also as Mod. at 71%; "M1o." reads as Moo. at
// under 3%, which is no competition). Within the winning reporter the canonical
// spelling is preferred, then the most frequent one.
func resolve(found string, lookup *WhitelistLookup, threshold float64) Resolution {
	var cands []Candidate
	for _, r := range readings(found) {
		std, ok := lookup.Standard[r.Corrected]
		if !ok {
			continue
		}
		cands = append(cands, Candidate{
			Reading:   r,
			Standard:  std,
			Canonical: lookup.Canonical[r.Corrected],
			Freq:      lookup.Freq[r.Corrected],
		})
	}
	res := Resolution{Kind: KindUnresolved, Candidates: cands}
	if len(cands) == 0 {
		return res
	}

	// Junk readings only count when nothing points at a reporter.
	byReporter := map[string]int{}
	var reporters []string
	for _, c := range cands {
		if lookup.Junk[c.Corrected] {
			continue
		}
		if _, ok := byReporter[c.Standard]; !ok {
			reporters = append(reporters, c.Standard)
		}
		byReporter[c.Standard] += c.Freq
	}
	if len(reporters) == 0 {
		res.Kind = KindToJunk
		return res
	}
	sort.Slice(reporters, func(i, j int) bool {
		if byReporter[reporters[i]] != byReporter[reporters[j]] {
			return byReporter[reporters[i]] > byReporter[reporters[j]]
		}
		return reporters[i] < reporters[j]
	})
	winner := reporters[0]
	if len(reporters) > 1 {
		if float64(byReporter[reporters[1]]) >= threshold*float64(byReporter[winner]) {
			res.Kind = KindAmbiguous
			return res
		}
	}

	best := -1
	for i, c := range cands {
		if lookup.Junk[c.Corrected] || c.Standard != winner {
			continue
		}
		if best < 0 || better(c, cands[best]) {
			best = i
		}
	}
	res.Kind = KindProposed
	res.Best = cands[best]
	return res
}

// better orders two candidates for the same reporter: a reading from a
// proposing rule before a review-only one, then canonical first, then the more
// frequent spelling, then the lexically smaller one for determinism.
func better(a, b Candidate) bool {
	if (a.Rule == RuleDigitLetter) != (b.Rule == RuleDigitLetter) {
		return a.Rule != RuleDigitLetter
	}
	if a.Canonical != b.Canonical {
		return a.Canonical
	}
	if a.Freq != b.Freq {
		return a.Freq > b.Freq
	}
	return a.Corrected < b.Corrected
}
