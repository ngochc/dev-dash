package picker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var ErrCancelled = errors.New("selection cancelled")

type Option struct {
	Value string
	Label string
}

type Picker interface {
	PickOne(context.Context, string, []Option, string) (string, error)
	PickMany(context.Context, string, []Option) ([]string, error)
	Confirm(string, bool) (bool, error)
	Input(string, string) (string, error)
}

type terminalPicker struct {
	reader     *bufio.Reader
	input      io.Reader
	output     io.Writer
	lookPath   func(string) (string, error)
	isTerminal func(int) bool
	runFZF     func(context.Context, string, []string, []byte, io.Writer) ([]byte, error)
}

type fileDescriptor interface {
	Fd() uintptr
}

func New(input io.Reader, output io.Writer) Picker {
	return newTerminalPicker(input, output)
}

func newTerminalPicker(input io.Reader, output io.Writer) *terminalPicker {
	return &terminalPicker{
		reader:     bufio.NewReader(input),
		input:      input,
		output:     output,
		lookPath:   exec.LookPath,
		isTerminal: term.IsTerminal,
		runFZF:     executeFZF,
	}
}

func (p *terminalPicker) PickOne(ctx context.Context, title string, options []Option, defaultValue string) (string, error) {
	values, err := p.pick(ctx, title, options, false, defaultValue)
	if err != nil {
		return "", err
	}
	return values[0], nil
}

func (p *terminalPicker) PickMany(ctx context.Context, title string, options []Option) ([]string, error) {
	return p.pick(ctx, title, options, true, "")
}

func (p *terminalPicker) pick(ctx context.Context, title string, options []Option, multi bool, defaultValue string) ([]string, error) {
	if len(options) == 0 {
		return nil, ErrCancelled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defaultIndex := -1
	if defaultValue != "" {
		for index, option := range options {
			if option.Value == defaultValue {
				defaultIndex = index
				break
			}
		}
		if defaultIndex == -1 {
			return nil, fmt.Errorf("default selection %q is not an option", defaultValue)
		}
	}
	if executable, ok := p.fzfExecutable(); ok {
		return p.pickFZF(ctx, executable, title, options, multi, defaultIndex)
	}
	return p.pickNumbered(title, options, multi, defaultIndex)
}

func (p *terminalPicker) fzfExecutable() (string, bool) {
	input, inputOK := p.input.(fileDescriptor)
	output, outputOK := p.output.(fileDescriptor)
	if !inputOK || !outputOK || !p.isTerminal(int(input.Fd())) || !p.isTerminal(int(output.Fd())) {
		return "", false
	}
	executable, err := p.lookPath("fzf")
	if err != nil {
		return "", false
	}
	return executable, true
}

func (p *terminalPicker) pickFZF(ctx context.Context, executable, title string, options []Option, multi bool, defaultIndex int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("run fzf: %w", err)
	}

	var input bytes.Buffer
	for index, option := range options {
		label := option.Label
		if index == defaultIndex {
			label += " [default]"
		}
		fmt.Fprintf(&input, "%d\t%s\n", index+1, label)
	}
	header := "↑/↓ move • Enter select • Esc cancel"
	if multi {
		header = "↑/↓ move • Tab toggle • Enter confirm • Esc cancel"
	}
	arguments := []string{"--delimiter=\t", "--with-nth=2..", "--prompt=" + title + ": ", "--header=" + header}
	if defaultIndex >= 0 {
		arguments = append(arguments, "--sync", fmt.Sprintf("--bind=start:pos(%d)", defaultIndex+1))
	}
	if multi {
		arguments = append(arguments, "--multi")
	}
	output, err := p.runFZF(ctx, executable, arguments, input.Bytes(), p.output)
	if err != nil {
		if code, ok := err.(interface{ ExitCode() int }); ok && (code.ExitCode() == 1 || code.ExitCode() == 130) {
			return nil, ErrCancelled
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("run fzf: %w", ctxErr)
		}
		return nil, fmt.Errorf("run fzf: %w", err)
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return nil, ErrCancelled
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if !multi && len(lines) != 1 {
		return nil, fmt.Errorf("parse fzf selection: expected one result, got %d", len(lines))
	}
	values := make([]string, 0, len(lines))
	seen := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		indexText, _, found := strings.Cut(strings.TrimSuffix(line, "\r"), "\t")
		if !found {
			return nil, fmt.Errorf("parse fzf selection %q: missing delimiter", line)
		}
		index, parseErr := strconv.Atoi(indexText)
		if parseErr != nil || index < 1 || index > len(options) {
			return nil, fmt.Errorf("parse fzf selection %q: invalid index", line)
		}
		if _, duplicate := seen[index]; duplicate {
			continue
		}
		seen[index] = struct{}{}
		values = append(values, options[index-1].Value)
	}
	if len(values) == 0 {
		return nil, ErrCancelled
	}
	return values, nil
}

func executeFZF(ctx context.Context, executable string, arguments []string, input []byte, stderr io.Writer) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Stdin = bytes.NewReader(input)
	command.Stderr = stderr
	return command.Output()
}

func (p *terminalPicker) pickNumbered(title string, options []Option, multi bool, defaultIndex int) ([]string, error) {
	fmt.Fprintln(p.output, title)
	for index, option := range options {
		label := option.Label
		if index == defaultIndex {
			label += " [default]"
		}
		fmt.Fprintf(p.output, "  %d. %s\n", index+1, label)
	}
	if multi {
		fmt.Fprintln(p.output, "Type comma-separated numbers and press Enter; press Enter alone to cancel.")
	} else if defaultIndex >= 0 {
		fmt.Fprintf(p.output, "Type a number and press Enter; press Enter alone to use %s.\n", options[defaultIndex].Label)
	} else {
		fmt.Fprintln(p.output, "Type a number and press Enter; press Enter alone to cancel.")
	}

	for {
		if multi {
			fmt.Fprintf(p.output, "Select [1-%d, comma-separated]: ", len(options))
		} else {
			fmt.Fprintf(p.output, "Select [1-%d]: ", len(options))
		}
		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if line == "" {
			if defaultIndex >= 0 {
				return []string{options[defaultIndex].Value}, nil
			}
			return nil, ErrCancelled
		}

		indexes, valid := parseIndexes(line, len(options), multi)
		if !valid {
			if multi {
				fmt.Fprintf(p.output, "Enter numbers from 1 to %d separated by commas.\n", len(options))
			} else {
				fmt.Fprintf(p.output, "Enter a number from 1 to %d.\n", len(options))
			}
			continue
		}
		values := make([]string, len(indexes))
		for index, optionIndex := range indexes {
			values[index] = options[optionIndex-1].Value
		}
		return values, nil
	}
}

func parseIndexes(line string, optionCount int, multi bool) ([]int, bool) {
	parts := strings.Split(line, ",")
	if !multi && len(parts) != 1 {
		return nil, false
	}
	indexes := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		index, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || index < 1 || index > optionCount {
			return nil, false
		}
		if _, duplicate := seen[index]; duplicate {
			continue
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	return indexes, len(indexes) > 0
}

func (p *terminalPicker) Confirm(prompt string, defaultValue bool) (bool, error) {
	for {
		choices := "[y/N] (Enter = no): "
		if defaultValue {
			choices = "[Y/n] (Enter = yes): "
		}
		fmt.Fprintf(p.output, "%s %s", prompt, choices)
		line, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "":
			return defaultValue, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.output, "Enter yes or no.")
		}
	}
}

func (p *terminalPicker) Input(prompt, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(p.output, "%s (Enter to continue) ", prompt)
	} else {
		fmt.Fprintf(p.output, "%s [%s] (Enter to keep default): ", prompt, defaultValue)
	}
	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func (p *terminalPicker) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		if errors.Is(err, io.EOF) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("read prompt input: %w", err)
	}
	return strings.TrimSpace(line), nil
}
