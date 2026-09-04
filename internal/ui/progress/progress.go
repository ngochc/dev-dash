package progress

import (
	"fmt"
	"io"
	"time"

	"golang.org/x/term"
)

const defaultInterval = 100 * time.Millisecond

// Run reports progress while action executes and returns the action's error unchanged.
func Run(output io.Writer, label string, action func() error) error {
	terminal := false
	if descriptor, ok := output.(interface{ Fd() uintptr }); ok {
		terminal = term.IsTerminal(int(descriptor.Fd()))
	}
	return run(output, label, action, terminal, defaultInterval)
}

func run(output io.Writer, label string, action func() error, terminal bool, interval time.Duration) (err error) {
	if !terminal {
		fmt.Fprintf(output, "%s...\n", label)
		err = action()
		fmt.Fprintf(output, "%s: %s\n", label, status(err == nil))
		return err
	}

	frames := [...]byte{'|', '/', '-', '\\'}
	fmt.Fprintf(output, "\r%s %c", label, frames[0])

	ticker := time.NewTicker(interval)
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		frame := 1
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(output, "\r%s %c", label, frames[frame])
				frame = (frame + 1) % len(frames)
			case <-stop:
				return
			}
		}
	}()

	defer func() {
		panicValue := recover()
		ticker.Stop()
		close(stop)
		<-stopped
		fmt.Fprintf(output, "\r%s: %s\x1b[K\n", label, status(err == nil && panicValue == nil))
		if panicValue != nil {
			panic(panicValue)
		}
	}()

	err = action()
	return err
}

func status(succeeded bool) string {
	if succeeded {
		return "done"
	}
	return "failed"
}
