package linker

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lmullen/legal-modernism/go/citations"
)

// Source is where a linker reads the citations still to be linked and writes
// what it made of them: one corpus's citations_unlinked and citation_links
// tables. cite-linker and cite-linker-cap differ in nothing else.
type Source interface {
	// StreamUnprocessedCitations delivers every citation not yet in
	// citation_links to fn in batches of at most batchSize, in one streaming
	// pass; Run applies backpressure inside fn.
	StreamUnprocessedCitations(ctx context.Context, batchSize int, fn func([]citations.UnlinkedCitation) error) error
	// SaveLinkResults inserts a batch of results, ignoring a citation that
	// already has a row, so that re-processing is idempotent.
	SaveLinkResults(ctx context.Context, results []*citations.LinkResult) error
}

// Options sizes a run.
type Options struct {
	BatchSize int // citations per batch, and per insert
	Workers   int // concurrent link-and-insert workers, each on one connection
	// ProgressEvery is how often a "linking progress" line is logged; 0 logs
	// none. A count that does not move between two lines is the sign of a
	// blocked run.
	ProgressEvery time.Duration
}

// Summary is what a run did. Processed counts the citations whose results were
// committed; a batch whose insert failed is counted in FailedBatches and
// FailedRows instead and is left for the next run to pick up.
type Summary struct {
	Processed     int64
	FailedBatches int64
	FailedRows    int64
}

// Run links every unprocessed citation src delivers, with opts.Workers workers
// linking and saving in batches, until the stream ends or ctx is cancelled.
// The error is the stream's: a cancelled context surfaces here as an error
// from whichever query was in flight, so a driver that was interrupted checks
// its own signal state before treating it as a failure. Committed batches are
// saved either way, and re-processing is idempotent, so an interrupted run is
// simply resubmitted.
func Run(ctx context.Context, t *Tables, src Source, opts Options) (Summary, error) {
	// Bounded pipeline. A single streaming reader (this goroutine, inside
	// StreamUnprocessedCitations) feeds batches to a fixed pool of insert
	// workers through a bounded channel. The channel capacity bounds how many
	// batches are in flight, so the reader blocks — applying backpressure —
	// when the workers fall behind, instead of buffering the whole 62M-row
	// table in memory.
	batchCh := make(chan []citations.UnlinkedCitation, opts.Workers)
	var wg sync.WaitGroup
	var processed atomic.Int64
	var failedBatches atomic.Int64
	var failedRows atomic.Int64

	stopHeartbeat := func() {}
	if opts.ProgressEvery > 0 {
		stopHeartbeat = startProgressHeartbeat(opts.ProgressEvery, &processed,
			func(n int64, elapsed time.Duration) {
				slog.Info("linking progress",
					"processed", n,
					"elapsed", elapsed.Round(time.Second).String(),
					"rows_per_sec", int64(float64(n)/elapsed.Seconds()))
			})
	}

	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batchCh {
				select {
				case <-ctx.Done():
					continue // drain the channel without doing work
				default:
				}

				results := make([]*citations.LinkResult, len(batch))
				statusCounts := make(map[string]int)
				for j := range batch {
					r := t.Link(&batch[j])
					results[j] = r
					statusCounts[r.Status]++
				}

				if err := src.SaveLinkResults(ctx, results); err != nil {
					if ctx.Err() != nil {
						// Shutting down. The insert was cancelled in flight, so
						// this batch is simply not committed and will be picked
						// up by the next run — not a failure worth an ERROR.
						slog.Warn("batch not saved because of shutdown", "size", len(results))
						continue
					}
					failedBatches.Add(1)
					failedRows.Add(int64(len(results)))
					slog.Error("could not save batch results", "size", len(results), "error", err)
					continue
				}

				processed.Add(int64(len(batch)))
				attrs := []any{"size", len(results)}
				for status, count := range statusCounts {
					attrs = append(attrs, status, count)
				}
				slog.Debug("saved batch", attrs...)
			}
		}()
	}

	// Stream the whole unprocessed set in one pass, pushing batches into the
	// bounded channel. The send blocks when the channel is full (backpressure).
	streamErr := src.StreamUnprocessedCitations(ctx, opts.BatchSize, func(batch []citations.UnlinkedCitation) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchCh <- batch:
			return nil
		}
	})
	close(batchCh)
	wg.Wait()
	stopHeartbeat()

	return Summary{
		Processed:     processed.Load(),
		FailedBatches: failedBatches.Load(),
		FailedRows:    failedRows.Load(),
	}, streamErr
}

// startProgressHeartbeat calls report every interval with the current count and
// the time elapsed since the heartbeat started, until the returned stop function
// is called. Reporting on a timer rather than per batch keeps the output
// readable in a Slurm log, and a count that does not move between consecutive
// reports is what makes a blocked run visible. stop blocks until the heartbeat
// goroutine has exited, so no report can be emitted after it returns.
func startProgressHeartbeat(interval time.Duration, processed *atomic.Int64, report func(n int64, elapsed time.Duration)) (stop func()) {
	started := time.Now()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				report(processed.Load(), time.Since(started))
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}
