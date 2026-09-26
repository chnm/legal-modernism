package linker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memSource is a Source over a fixed list of batches, recording what was
// saved and failing the saves it is told to.
type memSource struct {
	batches   [][]citations.UnlinkedCitation
	streamErr error        // returned after every batch has been delivered
	failSave  map[int]bool // batch index (by first citation) -> fail its save

	mu    sync.Mutex
	saved [][]*citations.LinkResult
}

func (s *memSource) StreamUnprocessedCitations(ctx context.Context, batchSize int, fn func([]citations.UnlinkedCitation) error) error {
	for _, b := range s.batches {
		if err := fn(b); err != nil {
			return err
		}
	}
	return s.streamErr
}

func (s *memSource) SaveLinkResults(ctx context.Context, results []*citations.LinkResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, b := range s.batches {
		if len(b) > 0 && len(results) > 0 && b[0].ID == results[0].CitationID && s.failSave[i] {
			return errors.New("insert failed")
		}
	}
	s.saved = append(s.saved, results)
	return nil
}

func batchOf(n int) []citations.UnlinkedCitation {
	b := make([]citations.UnlinkedCitation, n)
	for i := range b {
		b[i] = citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10}
	}
	return b
}

// TestRun drives the worker loop with an in-memory source: every batch is
// linked and saved, a batch whose save fails is counted and left behind, and
// the stream's error comes back to the driver.
func TestRun(t *testing.T) {
	std := "U.S."
	tables := NewTables(map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &std}},
		nil, map[string]int64{"5 U.S. 10": 111}, nil, nil, nil, nil, nil, nil, nil, Years{})

	src := &memSource{
		batches:  [][]citations.UnlinkedCitation{batchOf(3), batchOf(2), batchOf(4)},
		failSave: map[int]bool{1: true},
	}
	sum, err := Run(context.Background(), tables, src, Options{BatchSize: 4, Workers: 2})
	require.NoError(t, err)
	assert.Equal(t, Summary{Processed: 7, FailedBatches: 1, FailedRows: 2}, sum)

	require.Len(t, src.saved, 2)
	for _, batch := range src.saved {
		for _, r := range batch {
			assert.Equal(t, citations.StatusLinkedCAP, r.Status, "every citation was linked")
		}
	}
}

func TestRunReturnsStreamError(t *testing.T) {
	tables := NewTables(map[string]*citations.WhitelistEntry{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, Years{})
	src := &memSource{batches: [][]citations.UnlinkedCitation{batchOf(1)}, streamErr: errors.New("connection lost")}
	sum, err := Run(context.Background(), tables, src, Options{BatchSize: 1, Workers: 1})
	assert.EqualError(t, err, "connection lost")
	assert.Equal(t, int64(1), sum.Processed, "batches delivered before the error are still saved")
}

func TestStartProgressHeartbeat(t *testing.T) {
	var processed atomic.Int64
	processed.Store(42)

	var mu sync.Mutex
	var counts []int64
	var elapseds []time.Duration

	stop := startProgressHeartbeat(5*time.Millisecond, &processed,
		func(n int64, elapsed time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			counts = append(counts, n)
			elapseds = append(elapseds, elapsed)
		})

	// It reports repeatedly on the timer, not just once.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(counts) >= 3
	}, 2*time.Second, time.Millisecond, "heartbeat did not report repeatedly")

	stop()

	mu.Lock()
	atStop := len(counts)
	firstCount := counts[0]
	firstElapsed := elapseds[0]
	mu.Unlock()

	// It reports the live count and a positive elapsed time.
	assert.Equal(t, int64(42), firstCount, "heartbeat should report the current processed count")
	assert.Positive(t, firstElapsed, "heartbeat should report elapsed time since it started")

	// stop() waits for the goroutine to exit, so nothing is reported afterward.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, atStop, len(counts), "heartbeat kept reporting after stop returned")
}
