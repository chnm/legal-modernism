package linker

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
)

// TestYearGate pins what the gate does with a citation's SourceYear: nothing
// known means nothing refused, and a known year refuses only a later case.
func TestYearGate(t *testing.T) {
	years := map[int64]int{1: 1849, 2: 1850, 3: 1851}

	g := newYearGate(&citations.UnlinkedCitation{})
	assert.False(t, g.known)
	for id := range years {
		assert.True(t, admits(g, years, id), "unknown source year admits case %d", id)
	}
	assert.True(t, admits(g, years, int64(99)), "unknown source year admits an undated case")
	assert.False(t, g.refused)

	g = newYearGate(&citations.UnlinkedCitation{SourceYear: ptr(1850)})
	assert.True(t, g.known)
	assert.Equal(t, 1850, g.published)
	assert.True(t, admits(g, years, int64(1)), "an earlier case")
	assert.True(t, admits(g, years, int64(2)), "a case of the same year")
	assert.False(t, g.refused, "nothing refused yet")
	assert.False(t, admits(g, years, int64(3)), "a later case")
	assert.True(t, g.refused, "the refusal is remembered")
	assert.True(t, admits(g, years, int64(99)), "an undated case is admitted even after a refusal")
}
