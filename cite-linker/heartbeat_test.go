package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
