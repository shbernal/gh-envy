// Package cmdutil holds what every command shares: I/O streams, the API client
// factory and the --json exporter.
package cmdutil

import (
	"bytes"
	"errors"
	"io"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/cli/go-gh/v2/pkg/text"
)

// IO carries the streams and terminal facts commands render against.
type IO struct {
	In           io.Reader
	Out          io.Writer
	ErrOut       io.Writer
	StdoutTTY    bool
	StdinTTY     bool
	ColorEnabled bool
	Width        int
	Now          func() time.Time
}

// System returns IO bound to the process's terminal.
func System() *IO {
	t := term.FromEnv()
	width, _, err := t.Size()
	if err != nil || width <= 0 {
		width = 80
	}
	return &IO{
		In:           os.Stdin,
		Out:          t.Out(),
		ErrOut:       t.ErrOut(),
		StdoutTTY:    t.IsTerminalOutput(),
		StdinTTY:     term.IsTerminal(os.Stdin),
		ColorEnabled: t.IsColorEnabled(),
		Width:        width,
		Now:          time.Now,
	}
}

// Test returns IO writing to buffers, as a terminal when tty is true.
func Test(tty bool) (*IO, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return &IO{
		In:        &bytes.Buffer{},
		Out:       out,
		ErrOut:    errOut,
		StdoutTTY: tty,
		StdinTTY:  tty,
		Width:     120,
		Now:       func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) },
	}, out, errOut
}

// CanPrompt reports whether interactive prompts are possible.
func (io *IO) CanPrompt() bool {
	return io.StdinTTY && io.StdoutTTY
}

// Ago renders a timestamp relative to now.
func (io *IO) Ago(t time.Time) string {
	return text.RelativeTimeAgo(io.Now(), t)
}

// Color wraps s in an ANSI color when color is enabled.
func (io *IO) Color(code, s string) string {
	if !io.ColorEnabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Green, Red, Yellow, Gray and Bold are the palette core gh uses.
func (io *IO) Green(s string) string  { return io.Color("32", s) }
func (io *IO) Red(s string) string    { return io.Color("31", s) }
func (io *IO) Yellow(s string) string { return io.Color("33", s) }
func (io *IO) Gray(s string) string   { return io.Color("90", s) }
func (io *IO) Bold(s string) string   { return io.Color("1", s) }

// SuccessIcon is the check mark gh prints after mutations.
func (io *IO) SuccessIcon() string { return io.Green("✓") }

// WarningIcon is the exclamation mark gh prints before warnings.
func (io *IO) WarningIcon() string { return io.Yellow("!") }

// ErrCancel signals a prompt the user aborted; it exits with status 2.
var ErrCancel = errors.New("cancelled")

// ErrSilent signals an error already reported to the user.
var ErrSilent = errors.New("silent")
