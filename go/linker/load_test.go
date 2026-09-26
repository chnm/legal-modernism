package linker

import (
	"context"
	"errors"
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore is a citations.LinkerStore over canned tables; a non-nil fail is
// returned by the loader named in failAt.
type fakeStore struct {
	whitelist map[string]*citations.WhitelistEntry
	capCites  map[string]int64
	erCites   map[string]citations.ERCase
	capSpans  []citations.CaseSpan[int64]
	erSpans   []citations.CaseSpan[string]
	failAt    string
	fail      error
}

func (f *fakeStore) step(name string) error {
	if f.failAt == name {
		return f.fail
	}
	return nil
}

func (f *fakeStore) GetReporterWhitelist(context.Context) (map[string]*citations.WhitelistEntry, error) {
	return f.whitelist, f.step("whitelist")
}
func (f *fakeStore) GetDiffVols(context.Context) (map[string]map[int]*citations.DiffVolEntry, error) {
	return nil, f.step("diffvols")
}
func (f *fakeStore) StreamUnprocessedCitations(context.Context, int, func([]citations.UnlinkedCitation) error) error {
	return nil
}
func (f *fakeStore) LoadCAPCitations(context.Context) (map[string]int64, error) {
	return f.capCites, f.step("cap")
}
func (f *fakeStore) LoadFreelawCites(context.Context) (map[string]int64, error) {
	return nil, f.step("freelaw")
}
func (f *fakeStore) LoadReporterAltAbbrs(context.Context) (map[string][]string, error) {
	return nil, f.step("alts")
}
func (f *fakeStore) LoadCodeReporterCitations(context.Context) (map[string]int64, error) {
	return nil, f.step("code")
}
func (f *fakeStore) LoadEnglishReportsCitations(context.Context) (map[string]citations.ERCase, error) {
	return f.erCites, f.step("er")
}
func (f *fakeStore) LoadCAPCaseSpans(context.Context) ([]citations.CaseSpan[int64], error) {
	return f.capSpans, f.step("capspans")
}
func (f *fakeStore) LoadERCaseSpans(context.Context) ([]citations.CaseSpan[string], error) {
	return f.erSpans, f.step("erspans")
}
func (f *fakeStore) LoadStubCases(context.Context) (map[string]struct{}, error) {
	return nil, f.step("stubs")
}
func (f *fakeStore) LoadTreatiseYears(context.Context) (map[string]int, error) {
	return nil, f.step("years")
}
func (f *fakeStore) LoadCAPCaseYears(context.Context) (map[int64]int, error) { return nil, nil }
func (f *fakeStore) LoadCodeReporterYears(context.Context) (map[int64]int, error) {
	return nil, nil
}
func (f *fakeStore) LoadERCaseYears(context.Context) (map[string]int, error) { return nil, nil }
func (f *fakeStore) SaveLinkResults(context.Context, []*citations.LinkResult) error {
	return nil
}

func TestLoad(t *testing.T) {
	std := "Mass."
	store := &fakeStore{
		whitelist: map[string]*citations.WhitelistEntry{"Mass.": {ReporterStandard: &std}},
		capCites:  map[string]int64{"17 Mass. 478": 478, "2 Mass. 1": 2},
		erCites:   map[string]citations.ERCase{"1 Keb 5": {ID: "er-5", Cases: 1}},
		capSpans:  span17Mass,
		erSpans:   []citations.CaseSpan[string]{{Cite: "1 Keb 5", ID: "er-5"}},
	}
	tables, err := Load(context.Background(), store)
	require.NoError(t, err)

	stats := tables.Stats()
	assert.Equal(t, 1, stats.USReporters)
	assert.Equal(t, 2, stats.USVolumes)
	assert.Equal(t, 1, stats.UKReporters)
	assert.Equal(t, 1, stats.UKVolumes)
	assert.Equal(t, 1, stats.CAPVolumes)
	assert.Equal(t, len(span17Mass), stats.CAPSpans)
	assert.Equal(t, 1, stats.ERVolumes)
	assert.Equal(t, 1, stats.ERSpans)

	// The tables link: the whitelist and the CAP map were both loaded.
	got := tables.Link(&citations.UnlinkedCitation{Volume: ptr(2), ReporterAbbr: "Mass.", Page: 1})
	assert.Equal(t, citations.StatusLinkedCAP, got.Status)
}

// TestLoadNamesTheFailedStep checks that a loader's error comes back wrapped
// with the step, so the driver's startup message says what did not load.
func TestLoadNamesTheFailedStep(t *testing.T) {
	sentinel := errors.New("boom")
	for _, step := range []string{"whitelist", "diffvols", "cap", "freelaw", "alts", "code", "er", "capspans", "erspans", "stubs", "years"} {
		_, err := Load(context.Background(), &fakeStore{failAt: step, fail: sentinel})
		require.Error(t, err, step)
		assert.True(t, errors.Is(err, sentinel), step)
		assert.NotEqual(t, sentinel.Error(), err.Error(), "the step must be named: %s", step)
	}
}
