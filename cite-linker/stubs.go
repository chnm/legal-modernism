package main

import "github.com/lmullen/legal-modernism/go/citations"

// stubIndex is the set of cite strings in legalhist.stub_cases: the cases no
// source holds but that the corpus cites often enough to treat as real (issue
// #248). Keys take the form buildStandardCite builds, "{volume}
// {reporter_standard} {page}", which is what db/stub_cases.sql writes, so a
// citation reaches its stub under exactly the string the registry was built
// from.
//
// The index is probed last, after every source and the page-range indexes have
// missed, and only when the failure tier would have been reporter_absent. That
// gate is what keeps the registry honest with respect to the sources: a stub
// exists only for a reporter no source knows, so a stale registry -- one built
// before a dataset covering the reporter was imported -- cannot capture
// citations that now reach a real case or a real miss deeper in the cascade.
//
// A nil index is valid and never matches, so a run against a database whose
// registry has not been built behaves exactly as before.
type stubIndex map[string]struct{}

// lookup reports the first of the given standard-form cite strings that names a
// stub. The forms are the ones volumeForms produced, most faithful first, so on
// a single-volume reporter a citation detected as "Toth 123" reaches the stub
// keyed "1 Toth 123" through its variant, the same way it reaches CAP.
func (ix stubIndex) lookup(forms []string) (string, bool) {
	for _, f := range forms {
		if _, ok := ix[f]; ok {
			return f, true
		}
	}
	return "", false
}

// linkStub records a link to the stub keyed by cite. The key is recorded both
// as StubCite, the column the stub is joined on, and as CiteLinked, the cite
// string that matched, which is what every other successful probe records.
func linkStub(result *citations.LinkResult, cite string) *citations.LinkResult {
	result.Status = citations.StatusLinkedStub
	result.MatchTier = citations.TierStubDirect
	result.StubCite = &cite
	result.CiteLinked = &cite
	return result
}
