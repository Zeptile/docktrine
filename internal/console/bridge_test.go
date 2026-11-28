package console

import (
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeConn is a WebSocket whose browser side the test drives: it pushes
// messages in with send and reads what the bridge wrote from out.
type fakeConn struct {
	in     chan [2]any
	out    chan [2]any
	closed chan struct{}
	once   sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{in: make(chan [2]any, 16), out: make(chan [2]any, 64), closed: make(chan struct{})}
}

func (c *fakeConn) send(t int, data string) { c.in <- [2]any{t, []byte(data)} }

func (c *fakeConn) ReadMessage() (int, []byte, error) {
	select {
	case m, ok := <-c.in:
		if !ok {
			return 0, nil, io.EOF
		}
		return m[0].(int), m[1].([]byte), nil
	case <-c.closed:
		return 0, nil, errors.New("closed")
	}
}

func (c *fakeConn) WriteMessage(t int, data []byte) error {
	select {
	case <-c.closed:
		return errors.New("closed")
	default:
	}
	c.out <- [2]any{t, append([]byte(nil), data...)}
	return nil
}

func (c *fakeConn) WriteControl(int, []byte, time.Time) error { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error           { return nil }
func (c *fakeConn) SetPongHandler(func(string) error)         {}
func (c *fakeConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// fakeShell echoes input back as output, and exits when it reads "exit".
type fakeShell struct {
	mu      sync.Mutex
	output  chan []byte
	input   []byte
	resizes [][2]uint
	marker  *Marker
	code    int
	exited  chan struct{}
	closed  bool
	once    sync.Once
}

func newFakeShell() *fakeShell {
	return &fakeShell{output: make(chan []byte, 16), exited: make(chan struct{})}
}

func (s *fakeShell) Read(p []byte) (int, error) {
	select {
	case b := <-s.output:
		return copy(p, b), nil
	case <-s.exited:
		return 0, io.EOF
	}
}

func (s *fakeShell) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.input = append(s.input, p...)
	s.mu.Unlock()
	if string(p) == "exit" {
		s.end(3)
		return len(p), nil
	}
	s.output <- p
	return len(p), nil
}

func (s *fakeShell) end(code int) {
	s.mu.Lock()
	s.code = code
	s.mu.Unlock()
	s.once.Do(func() { close(s.exited) })
}

func (s *fakeShell) Resize(cols, rows uint) error {
	s.mu.Lock()
	s.resizes = append(s.resizes, [2]uint{cols, rows})
	s.mu.Unlock()
	return nil
}

func (s *fakeShell) Started() *Marker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.marker
}

func (s *fakeShell) Wait() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return -1, ErrClosed
	}
	return s.code, nil
}

func (s *fakeShell) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.once.Do(func() { close(s.exited) })
	return nil
}

func (s *fakeShell) wasClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// next returns the next message the bridge sent the browser.
func next(t *testing.T, c *fakeConn) (int, []byte) {
	t.Helper()
	select {
	case m := <-c.out:
		return m[0].(int), m[1].([]byte)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
	}
	return 0, nil
}

func nextStatus(t *testing.T, c *fakeConn) Message {
	t.Helper()
	for {
		typ, data := next(t, c)
		if typ != TextMessage {
			continue
		}
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("bad status message %q: %v", data, err)
		}
		return m
	}
}

func run(conn *fakeConn, sh *fakeShell, opts Options) chan Result {
	done := make(chan Result, 1)
	go func() { done <- Bridge(conn, sh, opts) }()
	return done
}

func result(t *testing.T, done chan Result) Result {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("the bridge didn't return")
	}
	return Result{}
}

func TestBridgeRelaysAndReportsExit(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	sh.marker = &Marker{PID: 9, Shell: "bash"}
	sh.output <- []byte("$ ")
	done := run(conn, sh, Options{})

	if m := nextStatus(t, conn); m.Type != TypeReady || m.Shell != "bash" {
		t.Fatalf("first status %+v, want ready with bash", m)
	}
	if typ, data := next(t, conn); typ != BinaryMessage || string(data) != "$ " {
		t.Fatalf("got %d %q, want the prompt as binary", typ, data)
	}

	conn.send(BinaryMessage, "ls\r")
	if typ, data := next(t, conn); typ != BinaryMessage || string(data) != "ls\r" {
		t.Fatalf("echo: got %d %q", typ, data)
	}

	conn.send(TextMessage, `{"type":"resize","cols":100,"rows":30}`)
	conn.send(TextMessage, `{"type":"resize","cols":0,"rows":30}`)
	conn.send(TextMessage, `not json`)
	conn.send(BinaryMessage, "exit")

	m := nextStatus(t, conn)
	if m.Type != TypeExit || m.Code == nil || *m.Code != 3 {
		t.Fatalf("got %+v, want exit 3", m)
	}
	r := result(t, done)
	if r.ExitCode == nil || *r.ExitCode != 3 {
		t.Errorf("result %+v, want exit code 3", r)
	}

	sh.mu.Lock()
	defer sh.mu.Unlock()
	if len(sh.resizes) != 1 || sh.resizes[0] != [2]uint{100, 30} {
		t.Errorf("resizes %v, want only 100x30", sh.resizes)
	}
	// Control messages are never typed into the shell
	if string(sh.input) != "ls\rexit" {
		t.Errorf("shell input %q", sh.input)
	}
}

func TestBridgeClosesShellWhenBrowserLeaves(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	done := run(conn, sh, Options{})

	close(conn.in)
	r := result(t, done)
	if r.ExitCode != nil {
		t.Errorf("result %+v, want no exit code", r)
	}
	if !sh.wasClosed() {
		t.Error("the shell was left running after the browser left")
	}
}

func TestBridgeIdleTimeout(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	done := run(conn, sh, Options{IdleTimeout: 50 * time.Millisecond, PingInterval: 20 * time.Millisecond})

	m := nextStatus(t, conn)
	if m.Type != TypeClosed {
		t.Fatalf("got %+v, want closed", m)
	}
	result(t, done)
	if !sh.wasClosed() {
		t.Error("the idle shell was left running")
	}
}

func TestBridgeInputKeepsSessionAlive(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	done := run(conn, sh, Options{IdleTimeout: 150 * time.Millisecond, PingInterval: 20 * time.Millisecond})

	// Typing more often than the timeout keeps it open
	for i := 0; i < 6; i++ {
		conn.send(BinaryMessage, "x")
		next(t, conn)
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case r := <-done:
		t.Fatalf("closed while in use: %+v", r)
	default:
	}
	if m := nextStatus(t, conn); m.Type != TypeClosed {
		t.Fatalf("got %+v, want closed once idle", m)
	}
	result(t, done)
}

func TestBridgeMaxDuration(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	done := run(conn, sh, Options{MaxDuration: 50 * time.Millisecond})

	if m := nextStatus(t, conn); m.Type != TypeClosed {
		t.Fatalf("got %+v, want closed", m)
	}
	result(t, done)
	if !sh.wasClosed() {
		t.Error("the shell outlived its time limit")
	}
}

func TestBridgeNoShell(t *testing.T) {
	conn, sh := newFakeConn(), newFakeShell()
	sh.output <- []byte("exec: \"sh\": executable file not found in $PATH\r\n")
	done := run(conn, sh, Options{})
	next(t, conn)
	sh.end(127)

	if m := nextStatus(t, conn); m.Type != TypeError {
		t.Fatalf("got %+v, want an error about the missing shell", m)
	}
	if m := nextStatus(t, conn); m.Type != TypeExit || *m.Code != 127 {
		t.Fatalf("got %+v, want exit 127", m)
	}
	result(t, done)
}

func TestHumanize(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute: "30 minutes",
		time.Minute:      "1 minute",
		12 * time.Hour:   "12 hours",
		90 * time.Second: "1m30s",
	}
	for d, want := range cases {
		if got := humanize(d); got != want {
			t.Errorf("humanize(%v) = %q, want %q", d, got, want)
		}
	}
}
