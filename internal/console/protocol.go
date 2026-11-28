// Package console opens interactive shells in containers and on nodes, and
// bridges them to a browser terminal over a WebSocket.
//
// The browser and the API speak a small protocol on the socket. Binary
// messages carry raw terminal bytes: keystrokes from the browser, output to
// it. Text messages carry JSON control messages, so a resize can never be
// mistaken for typed input.
package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Control messages from the browser.
const (
	TypeResize = "resize"
)

// Status messages to the browser.
const (
	// TypeReady says the shell started, and which one it is.
	TypeReady = "ready"
	// TypeExit says the shell ended by itself, with its exit code.
	TypeExit = "exit"
	// TypeClosed says the API ended the session, e.g. after the idle timeout.
	TypeClosed = "closed"
	// TypeError says the shell couldn't be opened or broke.
	TypeError = "error"
)

// Terminal sizes outside these bounds are a broken client, not a big screen.
const (
	MaxCols = 1000
	MaxRows = 500
)

// Message is a JSON control or status message. Only the fields that belong
// to its type are set.
type Message struct {
	Type    string `json:"type"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	Shell   string `json:"shell,omitempty"`
	Code    *int   `json:"code,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ParseControl reads a control message from the browser and validates it.
func ParseControl(data []byte) (Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("invalid control message: %w", err)
	}
	switch m.Type {
	case TypeResize:
		if _, _, err := ValidSize(m.Cols, m.Rows); err != nil {
			return m, err
		}
		return m, nil
	case "":
		return m, fmt.Errorf("control message has no type")
	}
	return m, fmt.Errorf("unknown control message %q", m.Type)
}

// ValidSize checks a terminal size, in the unsigned form Docker takes.
func ValidSize(cols, rows int) (uint, uint, error) {
	if cols < 1 || rows < 1 || cols > MaxCols || rows > MaxRows {
		return 0, 0, fmt.Errorf("terminal size %dx%d is out of range", cols, rows)
	}
	return uint(cols), uint(rows), nil
}

// ParseSize reads the initial size from the cols and rows query parameters,
// falling back to 80x24 when they're missing or out of range.
func ParseSize(cols, rows string) (uint, uint) {
	c, err1 := strconv.Atoi(cols)
	r, err2 := strconv.Atoi(rows)
	if err1 != nil || err2 != nil {
		return 80, 24
	}
	uc, ur, err := ValidSize(c, r)
	if err != nil {
		return 80, 24
	}
	return uc, ur
}

func exitMessage(code int) Message {
	return Message{Type: TypeExit, Code: &code}
}

// The shell wrapper announces itself with an OSC escape sequence before it
// execs the real shell: ESC ] 6973 ; <pid> ; <shell> BEL. xterm ignores OSC
// codes it doesn't know, but we strip it anyway. The pid is the shell's pid
// inside the container, which is how an exec is killed later: Docker has no
// API to kill an exec.
var (
	markerPrefix = []byte("\x1b]6973;")
	markerEnd    = byte('\a')
)

// markerLimit bounds how much output is held back while waiting for the
// marker, so a shell that never prints it still shows its output.
const markerLimit = 128

// wrapperScript picks bash if the container has it, else sh, then
// announces itself and replaces itself with that shell, keeping its pid.
// args are the arguments to the shell, e.g. "-l".
func wrapperScript(args string) string {
	return `if command -v bash >/dev/null 2>&1; then s=bash; else s=sh; fi; ` +
		`printf '\033]6973;%s;%s\007' "$$" "$s"; exec "$s" ` + args
}

// Marker is what the shell wrapper announced.
type Marker struct {
	PID   int
	Shell string
}

// markerScanner strips the wrapper's marker from the start of a shell's
// output. Feed it every chunk in order; it returns the bytes to pass on.
type markerScanner struct {
	buf    []byte
	done   bool
	marker *Marker
}

// Feed takes the next chunk of output and returns what to forward. Once the
// marker is found, or the output can't be the marker, it passes everything
// through.
func (s *markerScanner) Feed(chunk []byte) []byte {
	if s.done {
		return chunk
	}
	s.buf = append(s.buf, chunk...)

	n := len(s.buf)
	if n > len(markerPrefix) {
		n = len(markerPrefix)
	}
	if !bytes.Equal(s.buf[:n], markerPrefix[:n]) {
		return s.flush()
	}
	if len(s.buf) < len(markerPrefix) {
		return nil
	}
	end := bytes.IndexByte(s.buf, markerEnd)
	if end < 0 {
		if len(s.buf) > markerLimit {
			return s.flush()
		}
		return nil
	}
	if m, ok := parseMarker(s.buf[len(markerPrefix):end]); ok {
		s.marker = &m
		s.buf = s.buf[end+1:]
	}
	return s.flush()
}

func (s *markerScanner) flush() []byte {
	s.done = true
	out := s.buf
	s.buf = nil
	return out
}

func parseMarker(body []byte) (Marker, bool) {
	pid, shell, ok := bytes.Cut(body, []byte(";"))
	if !ok {
		return Marker{}, false
	}
	n, err := strconv.Atoi(string(pid))
	if err != nil || n < 1 {
		return Marker{}, false
	}
	if string(shell) != "bash" && string(shell) != "sh" {
		return Marker{}, false
	}
	return Marker{PID: n, Shell: string(shell)}, true
}
