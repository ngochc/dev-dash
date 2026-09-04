package picker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestPickOneUsesFZFAndMapsValue(t *testing.T) {
	input := &descriptorReader{Reader: strings.NewReader("")}
	output := &descriptorWriter{}
	picker := newTerminalPicker(input, output)
	picker.isTerminal = func(int) bool { return true }
	picker.lookPath = func(name string) (string, error) {
		if name != "fzf" {
			t.Fatalf("lookPath name = %q, want fzf", name)
		}
		return "/tools/fzf", nil
	}
	picker.runFZF = func(ctx context.Context, executable string, arguments []string, records []byte, stderr io.Writer) ([]byte, error) {
		if executable != "/tools/fzf" {
			t.Errorf("executable = %q, want /tools/fzf", executable)
		}
		wantArguments := []string{"--delimiter=\t", "--with-nth=2..", "--prompt=Repository: ", "--header=↑/↓ move • Enter select • Esc cancel", "--sync", "--bind=start:pos(2)"}
		if !reflect.DeepEqual(arguments, wantArguments) {
			t.Errorf("arguments = %#v, want %#v", arguments, wantArguments)
		}
		if got, want := string(records), "1\tDuplicate label\n2\tDuplicate label [default]\n"; got != want {
			t.Errorf("records = %q, want %q", got, want)
		}
		if stderr != output {
			t.Error("stderr was not wired to picker output")
		}
		return []byte("2\tDuplicate label [default]\n"), nil
	}

	got, err := picker.PickOne(context.Background(), "Repository", []Option{
		{Value: "owner/first", Label: "Duplicate label"},
		{Value: "owner/second", Label: "Duplicate label"},
	}, "owner/second")
	if err != nil {
		t.Fatalf("PickOne() error = %v", err)
	}
	if got != "owner/second" {
		t.Errorf("PickOne() = %q, want owner/second", got)
	}
}

func TestPickOneRejectsUnavailableDefault(t *testing.T) {
	output := &descriptorWriter{}
	picker := newTerminalPicker(&descriptorReader{Reader: strings.NewReader("")}, output)
	picker.isTerminal = func(int) bool { return true }
	picker.lookPath = func(string) (string, error) { return "/tools/fzf", nil }
	called := false
	picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
		called = true
		return nil, nil
	}

	_, err := picker.PickOne(context.Background(), "Repository", testOptions(), "missing")
	if got, want := fmt.Sprint(err), `default selection "missing" is not an option`; got != want {
		t.Fatalf("PickOne() error = %q, want %q", got, want)
	}
	if called {
		t.Fatal("PickOne() ran fzf for an unavailable default")
	}
	if output.Len() != 0 {
		t.Errorf("picker output = %q, want no prompt", output.String())
	}
}

func TestPickManyUsesFZFMultiAndPreservesSelectionOrder(t *testing.T) {
	picker := fzfPicker(t)
	picker.runFZF = func(_ context.Context, _ string, arguments []string, _ []byte, _ io.Writer) ([]byte, error) {
		want := []string{"--delimiter=\t", "--with-nth=2..", "--prompt=Repositories: ", "--header=↑/↓ move • Tab toggle • Enter confirm • Esc cancel", "--multi"}
		if !reflect.DeepEqual(arguments, want) {
			t.Errorf("arguments = %#v, want %#v", arguments, want)
		}
		return []byte("3\tThird\n1\tFirst\n3\tThird\n"), nil
	}

	got, err := picker.PickMany(context.Background(), "Repositories", testOptions())
	if err != nil {
		t.Fatalf("PickMany() error = %v", err)
	}
	want := []string{"third", "first"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PickMany() = %#v, want %#v", got, want)
	}
}

func TestFZFContextAndErrorsPreserveCauses(t *testing.T) {
	t.Run("cancelled context", func(t *testing.T) {
		picker := fzfPicker(t)
		called := false
		picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
			called = true
			return nil, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := picker.PickOne(ctx, "Title", testOptions(), "")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PickOne() error = %v, want context.Canceled", err)
		}
		if called {
			t.Fatal("PickOne() ran fzf for cancelled context")
		}
	})

	t.Run("context cancelled during process", func(t *testing.T) {
		picker := fzfPicker(t)
		ctx, cancel := context.WithCancel(context.Background())
		picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
			cancel()
			return nil, errors.New("process killed")
		}
		_, err := picker.PickOne(ctx, "Title", testOptions(), "")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PickOne() error = %v, want context.Canceled", err)
		}
	})

	t.Run("process error", func(t *testing.T) {
		picker := fzfPicker(t)
		processError := errors.New("process failed")
		picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
			return nil, processError
		}
		_, err := picker.PickOne(context.Background(), "Title", testOptions(), "")
		if !errors.Is(err, processError) {
			t.Fatalf("PickOne() error = %v, want process error", err)
		}
	})
}

func TestFZFCancellationAndEmptyOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		output []byte
		err    error
	}{
		{name: "exit one", err: fakeExitError(1)},
		{name: "exit 130", err: fakeExitError(130)},
		{name: "empty output", output: []byte(" \n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			picker := fzfPicker(t)
			picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
				return test.output, test.err
			}
			if _, err := picker.PickOne(context.Background(), "Title", testOptions(), ""); !errors.Is(err, ErrCancelled) {
				t.Fatalf("PickOne() error = %v, want ErrCancelled", err)
			}
		})
	}
}

func TestFZFRejectsMalformedOutput(t *testing.T) {
	picker := fzfPicker(t)
	picker.runFZF = func(context.Context, string, []string, []byte, io.Writer) ([]byte, error) {
		return []byte("missing delimiter\n"), nil
	}
	if _, err := picker.PickOne(context.Background(), "Title", testOptions(), ""); err == nil || errors.Is(err, ErrCancelled) {
		t.Fatalf("PickOne() error = %v, want malformed output error", err)
	}
}

func TestUnavailableFZFFallsBackToNumberedPicker(t *testing.T) {
	input := &descriptorReader{Reader: strings.NewReader("2\n")}
	output := &descriptorWriter{}
	picker := newTerminalPicker(input, output)
	picker.isTerminal = func(int) bool { return true }
	picker.lookPath = func(string) (string, error) { return "", errors.New("not found") }

	got, err := picker.PickOne(context.Background(), "Repository", testOptions(), "")
	if err != nil {
		t.Fatalf("PickOne() error = %v", err)
	}
	if got != "second" {
		t.Errorf("PickOne() = %q, want second", got)
	}
	if !strings.Contains(output.String(), "2. Second") {
		t.Errorf("output = %q, want numbered fallback", output.String())
	}
}

func TestNumberedPickOneRepromptsInvalidInput(t *testing.T) {
	var output bytes.Buffer
	picker := New(strings.NewReader("wrong\n4\n2\n"), &output)
	got, err := picker.PickOne(context.Background(), "Repository", testOptions(), "")
	if err != nil {
		t.Fatalf("PickOne() error = %v", err)
	}
	if got != "second" {
		t.Errorf("PickOne() = %q, want second", got)
	}
	if count := strings.Count(output.String(), "Enter a number from 1 to 3."); count != 2 {
		t.Errorf("invalid instruction count = %d, want 2; output = %q", count, output.String())
	}
}

func TestNumberedPickOneUsesDefault(t *testing.T) {
	var output bytes.Buffer
	picker := New(strings.NewReader("\n"), &output)

	got, err := picker.PickOne(context.Background(), "Repository", testOptions(), "second")
	if err != nil {
		t.Fatalf("PickOne() error = %v", err)
	}
	if got != "second" {
		t.Errorf("PickOne() = %q, want second", got)
	}
	want := "Repository\n  1. First\n  2. Second [default]\n  3. Third\n" +
		"Type a number and press Enter; press Enter alone to use Second.\n" +
		"Select [1-3]: "
	if output.String() != want {
		t.Errorf("picker output = %q, want %q", output.String(), want)
	}
}

func TestNumberedPickManyDeduplicatesInEnteredOrder(t *testing.T) {
	var output bytes.Buffer
	picker := New(strings.NewReader("3,1,3,2\n"), &output)
	got, err := picker.PickMany(context.Background(), "Repositories", testOptions())
	if err != nil {
		t.Fatalf("PickMany() error = %v", err)
	}
	want := []string{"third", "first", "second"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PickMany() = %#v, want %#v", got, want)
	}
}

func TestNumberedPickCancellation(t *testing.T) {
	for _, input := range []string{"\n", ""} {
		picker := New(strings.NewReader(input), io.Discard)
		if _, err := picker.PickOne(context.Background(), "Repository", testOptions(), ""); !errors.Is(err, ErrCancelled) {
			t.Errorf("PickOne() with %q error = %v, want ErrCancelled", input, err)
		}
		picker = New(strings.NewReader(input), io.Discard)
		if _, err := picker.PickMany(context.Background(), "Repositories", testOptions()); !errors.Is(err, ErrCancelled) {
			t.Errorf("PickMany() with %q error = %v, want ErrCancelled", input, err)
		}
	}
}

func TestNumberedPickerDisplaysKeyboardHelp(t *testing.T) {
	for _, test := range []struct {
		name  string
		multi bool
		input string
		want  string
	}{
		{
			name:  "single",
			input: "2\n",
			want: "Repository\n  1. First\n  2. Second\n  3. Third\n" +
				"Type a number and press Enter; press Enter alone to cancel.\n" +
				"Select [1-3]: ",
		},
		{
			name:  "multiple",
			multi: true,
			input: "2,1\n",
			want: "Repository\n  1. First\n  2. Second\n  3. Third\n" +
				"Type comma-separated numbers and press Enter; press Enter alone to cancel.\n" +
				"Select [1-3, comma-separated]: ",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			picker := New(strings.NewReader(test.input), &output)
			if test.multi {
				_, _ = picker.PickMany(context.Background(), "Repository", testOptions())
			} else {
				_, _ = picker.PickOne(context.Background(), "Repository", testOptions(), "")
			}
			if got := output.String(); got != test.want {
				t.Errorf("picker output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfirmDisplaysDefaultKeyboardHelp(t *testing.T) {
	for _, test := range []struct {
		name         string
		defaultValue bool
		want         string
	}{
		{name: "yes", defaultValue: true, want: "Continue? [Y/n] (Enter = yes): "},
		{name: "no", want: "Continue? [y/N] (Enter = no): "},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			picker := New(strings.NewReader("\n"), &output)
			_, _ = picker.Confirm("Continue?", test.defaultValue)
			if got := output.String(); got != test.want {
				t.Errorf("Confirm() output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInputDisplaysEnterKeyboardHelp(t *testing.T) {
	for _, test := range []struct {
		name         string
		defaultValue string
		want         string
	}{
		{name: "without default", want: "Value: (Enter to continue) "},
		{name: "with default", defaultValue: "saved", want: "Value: [saved] (Enter to keep default): "},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			picker := New(strings.NewReader("\n"), &output)
			_, _ = picker.Input("Value:", test.defaultValue)
			if got := output.String(); got != test.want {
				t.Errorf("Input() output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfirmDefaultsAndReprompts(t *testing.T) {
	for _, test := range []struct {
		name         string
		input        string
		defaultValue bool
		want         bool
		wantInvalid  bool
	}{
		{name: "default yes", input: "\n", defaultValue: true, want: true},
		{name: "default no", input: "\n", defaultValue: false, want: false},
		{name: "explicit yes", input: "YES\n", want: true},
		{name: "explicit no", input: "No\n", defaultValue: true, want: false},
		{name: "reprompt", input: "maybe\ny\n", want: true, wantInvalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			picker := New(strings.NewReader(test.input), &output)
			got, err := picker.Confirm("Continue?", test.defaultValue)
			if err != nil {
				t.Fatalf("Confirm() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Confirm() = %v, want %v", got, test.want)
			}
			if strings.Contains(output.String(), "Enter yes or no.") != test.wantInvalid {
				t.Errorf("output = %q, invalid instruction mismatch", output.String())
			}
		})
	}
}

func TestInputTrimsAndUsesDefault(t *testing.T) {
	var output bytes.Buffer
	picker := New(strings.NewReader("   \n  entered value  \n"), &output)
	first, err := picker.Input("Value:", "default")
	if err != nil {
		t.Fatalf("first Input() error = %v", err)
	}
	second, err := picker.Input("Value:", "default")
	if err != nil {
		t.Fatalf("second Input() error = %v", err)
	}
	if first != "default" || second != "entered value" {
		t.Errorf("Input() values = (%q, %q), want (default, entered value)", first, second)
	}
}

func TestSequentialPromptsShareBufferedInput(t *testing.T) {
	picker := New(strings.NewReader("1\ny\ncustom\n"), io.Discard)
	selected, err := picker.PickOne(context.Background(), "Repository", testOptions(), "")
	if err != nil {
		t.Fatalf("PickOne() error = %v", err)
	}
	confirmed, err := picker.Confirm("Continue?", false)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	entered, err := picker.Input("Value:", "")
	if err != nil {
		t.Fatalf("Input() error = %v", err)
	}
	if selected != "first" || !confirmed || entered != "custom" {
		t.Errorf("sequential values = (%q, %v, %q)", selected, confirmed, entered)
	}
}

func TestSecretReadsOneBufferedLineWithoutEcho(t *testing.T) {
	var output bytes.Buffer
	picker := New(strings.NewReader("raw PAT value\nnext value\n"), &output)
	value, err := picker.Secret("PAT:")
	if err != nil {
		t.Fatalf("Secret() error = %v", err)
	}
	next, err := picker.Input("Next:", "")
	if err != nil {
		t.Fatalf("Input() error = %v", err)
	}
	if value != "raw PAT value" || next != "next value" {
		t.Fatalf("values = %q/%q", value, next)
	}
	if strings.Contains(output.String(), value) || output.String() != "PAT: Next: (Enter to continue) " {
		t.Fatalf("output = %q", output.String())
	}
}

func fzfPicker(t *testing.T) *terminalPicker {
	t.Helper()
	picker := newTerminalPicker(&descriptorReader{Reader: strings.NewReader("")}, &descriptorWriter{})
	picker.isTerminal = func(int) bool { return true }
	picker.lookPath = func(string) (string, error) { return "/tools/fzf", nil }
	return picker
}

func testOptions() []Option {
	return []Option{
		{Value: "first", Label: "First"},
		{Value: "second", Label: "Second"},
		{Value: "third", Label: "Third"},
	}
}

type descriptorReader struct {
	*strings.Reader
}

func (*descriptorReader) Fd() uintptr { return 10 }

type descriptorWriter struct {
	bytes.Buffer
}

func (*descriptorWriter) Fd() uintptr { return 11 }

type fakeExitError int

func (err fakeExitError) Error() string { return fmt.Sprintf("exit %d", err) }
func (err fakeExitError) ExitCode() int { return int(err) }
