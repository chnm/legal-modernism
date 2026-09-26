package citations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCorpusStoreWithoutCorpus pins that a store built as a bare literal, which
// knows no tables, refuses to run rather than streaming nothing and reporting
// success.
func TestCorpusStoreWithoutCorpus(t *testing.T) {
	var s CorpusStore
	ctx := context.Background()

	assert.ErrorIs(t, s.Prepare(ctx), ErrNoCorpus)
	assert.ErrorIs(t, s.StreamUnprocessedCitations(ctx, 10, func([]UnlinkedCitation) error { return nil }), ErrNoCorpus)
	assert.ErrorIs(t, s.SaveLinkResults(ctx, []*LinkResult{{Status: StatusNoMatch}}), ErrNoCorpus)
}
