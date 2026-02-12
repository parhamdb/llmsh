package agent

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// OutputRouter controls where agent output goes.
// Design: stdout = final result only, stderr = streaming/progress.
// This enables clean piping: `llmsh sort.llmsh list.txt | head -5`
type OutputRouter struct {
	Stdout  io.Writer // Final result output
	Stderr  io.Writer // Streaming, progress, logs
	Verbose bool
	Quiet   bool
	isTTY   bool
}

// NewOutputRouter creates an output router with smart defaults.
func NewOutputRouter(verbose, quiet bool) *OutputRouter {
	return &OutputRouter{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Verbose: verbose,
		Quiet:   quiet,
		isTTY:   term.IsTerminal(int(os.Stderr.Fd())),
	}
}

// StreamText writes streaming text deltas to stderr (visible progress).
func (o *OutputRouter) StreamText(text string) {
	if o.Quiet {
		return
	}
	fmt.Fprint(o.Stderr, text)
}

// FinalResult writes the final output to stdout.
func (o *OutputRouter) FinalResult(text string) {
	fmt.Fprint(o.Stdout, text)
}

// ToolStart logs that a tool is being executed.
func (o *OutputRouter) ToolStart(name string) {
	if o.Quiet {
		return
	}
	if o.isTTY {
		fmt.Fprintf(o.Stderr, "\033[90m⚡ %s\033[0m", name)
	}
}

// ToolDone logs tool completion.
func (o *OutputRouter) ToolDone(name string) {
	if o.Quiet {
		return
	}
	if o.isTTY {
		fmt.Fprintf(o.Stderr, "\033[90m ✓\033[0m\n")
	}
}

// ToolError logs tool errors.
func (o *OutputRouter) ToolError(name string, err string) {
	if o.isTTY {
		fmt.Fprintf(o.Stderr, "\033[31m ✗ %s\033[0m\n", err)
	} else {
		fmt.Fprintf(o.Stderr, "tool error [%s]: %s\n", name, err)
	}
}

// Info writes informational messages to stderr.
func (o *OutputRouter) Info(format string, args ...any) {
	if o.Quiet {
		return
	}
	fmt.Fprintf(o.Stderr, format+"\n", args...)
}

// Debug writes verbose debug info to stderr.
func (o *OutputRouter) Debug(format string, args ...any) {
	if !o.Verbose {
		return
	}
	fmt.Fprintf(o.Stderr, "\033[90m[debug] "+format+"\033[0m\n", args...)
}
