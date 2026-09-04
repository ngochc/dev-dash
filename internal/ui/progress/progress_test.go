package progress

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunNonTerminalOutput(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var output bytes.Buffer
		called := false
		if err := run(&output, "Working", func() error {
			called = true
			return nil
		}, false, time.Millisecond); err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if !called {
			t.Fatal("run() did not call action")
		}
		if got, want := output.String(), "Working...\nWorking: done\n"; got != want {
			t.Errorf("run() output = %q, want %q", got, want)
		}
	})

	t.Run("failure", func(t *testing.T) {
		var output bytes.Buffer
		actionErr := errors.New("action failed")
		err := run(&output, "Working", func() error { return actionErr }, false, time.Millisecond)
		if err != actionErr {
			t.Fatalf("run() error = %v, want unchanged action error %v", err, actionErr)
		}
		if got, want := output.String(), "Working...\nWorking: failed\n"; got != want {
			t.Errorf("run() output = %q, want %q", got, want)
		}
	})
}

func TestRunTerminalWritesImmediateFrameAndFinalStatus(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output, "Working", func() error {
		if got, want := output.String(), "\rWorking |"; got != want {
			t.Errorf("output at action start = %q, want immediate frame %q", got, want)
		}
		return nil
	}, true, time.Hour); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got, want := output.String(), "\rWorking |\rWorking: done\x1b[K\n"; got != want {
		t.Errorf("run() output = %q, want %q", got, want)
	}
}

func TestRunTerminalStopsTickerBeforeReturning(t *testing.T) {
	output := newObservedWriter()
	if err := run(output, "Working", func() error {
		select {
		case <-output.writes:
		case <-time.After(time.Second):
			t.Fatal("initial progress frame was not written")
		}
		select {
		case <-output.writes:
		case <-time.After(time.Second):
			t.Fatal("animated progress frame was not written")
		}
		return nil
	}, true, time.Millisecond); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	atReturn := output.String()
	if wantSuffix := "\rWorking: done\x1b[K\n"; !bytes.HasSuffix([]byte(atReturn), []byte(wantSuffix)) {
		t.Fatalf("run() output = %q, want suffix %q", atReturn, wantSuffix)
	}
	time.Sleep(10 * time.Millisecond)
	if got := output.String(); got != atReturn {
		t.Errorf("output changed after run returned: before %q, after %q", atReturn, got)
	}
}

type observedWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	writes chan struct{}
}

func newObservedWriter() *observedWriter {
	return &observedWriter{writes: make(chan struct{}, 16)}
}

func (w *observedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written, err := w.buffer.Write(data)
	select {
	case w.writes <- struct{}{}:
	default:
	}
	return written, err
}

func (w *observedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}
