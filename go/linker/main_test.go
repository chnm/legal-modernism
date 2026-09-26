package linker

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain keeps Load's and Run's INFO lines out of the test output unless
// LAW_DEBUG asks for them, the way the programs themselves are silenced.
func TestMain(m *testing.M) {
	if os.Getenv("LAW_DEBUG") == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	os.Exit(m.Run())
}
