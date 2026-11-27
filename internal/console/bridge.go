package console

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// WebSocket message types, from RFC 6455.
const (
	TextMessage   = 1
	BinaryMessage = 2
	closeMessage  = 8
	pingMessage   = 9
)

// Conn is the part of a WebSocket connection the bridge uses. Both
// gofiber's and gorilla's connections satisfy it.
type Conn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadDeadline(t time.Time) error
	SetPongHandler(h func(appData string) error)
	Close() error
}

// Options bound a session.
type Options struct {
	// IdleTimeout closes a session nobody has typed in for this long.
	IdleTimeout time.Duration
	// MaxDuration closes a session this long after it opened, idle or not.
	MaxDuration time.Duration
	// PingInterval is how often the browser is pinged, so a connection
	// that silently died is noticed.
	PingInterval time.Duration
}

const (
	DefaultIdleTimeout = 30 * time.Minute
	DefaultMaxDuration = 12 * time.Hour
	defaultPing        = 30 * time.Second

	// Bigger than a pasted script, smaller than a paste that's a mistake
	maxInputMessage = 1 << 20
)

func (o Options) withDefaults() Options {
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = DefaultIdleTimeout
	}
	if o.MaxDuration <= 0 {
		o.MaxDuration = DefaultMaxDuration
	}
	if o.PingInterval <= 0 {
		o.PingInterval = defaultPing
	}
	return o
}

// Result is how a session ended, for the log.
type Result struct {
	// ExitCode is set when the shell exited by itself.
	ExitCode *int
	Reason   string
}

func (r Result) String() string {
	if r.ExitCode != nil {
		return fmt.Sprintf("the shell exited with %d", *r.ExitCode)
	}
	return r.Reason
}

// Bridge connects a shell to a browser until either side ends, the session
// goes idle or hits its time limit. It always closes both before returning,
// and returns only once nothing uses conn any more.
func Bridge(conn Conn, sh Shell, opts Options) Result {
	opts = opts.withDefaults()

	// Writes come from the output pump and from here; WebSocket
	// connections allow only one writer at a time
	var wmu sync.Mutex
	write := func(t int, data []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return conn.WriteMessage(t, data)
	}
	send := func(m Message) error {
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return write(TextMessage, data)
	}

	done := make(chan Result, 2)
	finish := func(r Result) {
		select {
		case done <- r:
		default:
		}
	}

	var lastInput atomic.Int64
	lastInput.Store(time.Now().UnixNano())
	pongWait := 2*opts.PingInterval + 10*time.Second

	var wg sync.WaitGroup
	wg.Add(2)

	// Shell output to the browser
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		ready := false
		for {
			n, err := sh.Read(buf)
			if !ready {
				if m := sh.Started(); m != nil {
					ready = true
					send(Message{Type: TypeReady, Shell: m.Shell})
				}
			}
			if n > 0 {
				if werr := write(BinaryMessage, buf[:n]); werr != nil {
					finish(Result{Reason: "the browser went away"})
					return
				}
			}
			if err != nil {
				code, werr := sh.Wait()
				if errors.Is(werr, ErrClosed) {
					finish(Result{Reason: "closed"})
					return
				}
				if werr != nil {
					send(Message{Type: TypeError, Message: werr.Error()})
					finish(Result{Reason: "error: " + werr.Error()})
					return
				}
				if !ready && (code == 126 || code == 127) {
					// The wrapper never ran: there's no sh to run it
					send(Message{Type: TypeError, Message: "This container has no shell (sh) to open."})
				}
				send(exitMessage(code))
				finish(Result{ExitCode: &code})
				return
			}
		}
	}()

	// Browser input to the shell
	go func() {
		defer wg.Done()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			t, data, err := conn.ReadMessage()
			if err != nil {
				finish(Result{Reason: "closed by the browser"})
				return
			}
			conn.SetReadDeadline(time.Now().Add(pongWait))
			switch t {
			case BinaryMessage:
				if len(data) > maxInputMessage {
					continue
				}
				lastInput.Store(time.Now().UnixNano())
				// A write after the shell exited fails; the output pump
				// reports the exit
				sh.Write(data)
			case TextMessage:
				m, err := ParseControl(data)
				if err != nil {
					continue
				}
				if m.Type == TypeResize {
					cols, rows, _ := ValidSize(m.Cols, m.Rows)
					sh.Resize(cols, rows)
				}
			}
		}
	}()

	ping := time.NewTicker(opts.PingInterval)
	defer ping.Stop()
	limit := time.NewTimer(opts.MaxDuration)
	defer limit.Stop()

	var res Result
wait:
	for {
		select {
		case res = <-done:
			break wait
		case <-ping.C:
			idle := time.Since(time.Unix(0, lastInput.Load()))
			if idle >= opts.IdleTimeout {
				send(Message{Type: TypeClosed, Reason: fmt.Sprintf("Closed after %s without input.", humanize(opts.IdleTimeout))})
				res = Result{Reason: "idle for " + humanize(opts.IdleTimeout)}
				break wait
			}
			wmu.Lock()
			err := conn.WriteControl(pingMessage, nil, time.Now().Add(10*time.Second))
			wmu.Unlock()
			if err != nil {
				res = Result{Reason: "the browser went away"}
				break wait
			}
		case <-limit.C:
			send(Message{Type: TypeClosed, Reason: fmt.Sprintf("Sessions close after %s.", humanize(opts.MaxDuration))})
			res = Result{Reason: "hit the " + humanize(opts.MaxDuration) + " limit"}
			break wait
		}
	}

	sh.Close()
	wmu.Lock()
	conn.WriteControl(closeMessage, closePayload(1000), time.Now().Add(time.Second))
	wmu.Unlock()
	conn.Close()
	wg.Wait()
	return res
}

func closePayload(code uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, code)
	return b
}

// humanize prints whole hours or minutes, as in "30 minutes".
func humanize(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return plural(int(d/time.Hour), "hour")
	case d >= time.Minute && d%time.Minute == 0:
		return plural(int(d/time.Minute), "minute")
	}
	return d.String()
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
