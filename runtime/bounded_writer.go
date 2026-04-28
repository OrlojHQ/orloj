package agentruntime

import (
	"bytes"
	"fmt"
	"sync"
)

// DefaultMaxToolOutputBytes is the default cap for tool stdout/stderr capture.
// 16 MiB is generous for legitimate tool output while preventing OOM from
// runaway processes.
const DefaultMaxToolOutputBytes = 16 * 1024 * 1024

// ErrOutputLimitExceeded is returned when a tool's stdout or stderr exceeds
// the configured maximum.
var ErrOutputLimitExceeded = fmt.Errorf("tool output exceeded maximum allowed size")

// BoundedWriter wraps a bytes.Buffer and stops accepting writes once the
// configured maximum is reached. It is safe for concurrent use.
type BoundedWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	max      int
	exceeded bool
}

// NewBoundedWriter returns a writer that accepts at most max bytes.
func NewBoundedWriter(max int) *BoundedWriter {
	return &BoundedWriter{max: max}
}

// Write implements io.Writer. Once the cap is reached, further bytes are
// silently discarded and Exceeded() returns true.
func (w *BoundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.exceeded {
		// Accept the write (so the process doesn't get a broken-pipe) but
		// discard the data.
		return len(p), nil
	}
	remaining := w.max - w.buf.Len()
	if len(p) > remaining {
		// Write what fits, then mark exceeded.
		if remaining > 0 {
			w.buf.Write(p[:remaining])
		}
		w.exceeded = true
		return len(p), nil
	}
	return w.buf.Write(p)
}

// String returns the captured output.
func (w *BoundedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// Exceeded reports whether the cap was hit.
func (w *BoundedWriter) Exceeded() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.exceeded
}
