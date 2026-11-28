package console

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

const (
	// LabelConsole marks the helper containers that hold node shells, on
	// top of docktrine.role=helper, so leftovers can be found.
	LabelConsole = "docktrine.console"
	// LabelConsoleUser records who opened a node shell.
	LabelConsoleUser = "docktrine.console.user"
	// LabelConsoleInstance is the fingerprint of the Docktrine installation
	// that started a helper, so it can clean up after itself.
	LabelConsoleInstance = "docktrine.console.instance"

	// Docker's default detach keys are ctrl-p ctrl-q, which would swallow
	// ctrl-p (previous command) in the shell. Nobody types ctrl-] twice in
	// a row by accident.
	detachKeys = "ctrl-],ctrl-]"

	term = "TERM=xterm-256color"
)

// ErrClosed is returned by Wait once the session was closed from our side.
var ErrClosed = errors.New("the shell was closed")

// Shell is one interactive shell: its output to read, its input to write.
type Shell interface {
	// Read returns the shell's output, with the wrapper's marker removed.
	Read(p []byte) (int, error)
	// Write sends keystrokes to the shell.
	Write(p []byte) (int, error)
	Resize(cols, rows uint) error
	// Started returns what the shell announced once it's up, else nil.
	Started() *Marker
	// Wait returns the exit code once the output has ended.
	Wait() (int, error)
	// Close ends the shell and everything it started.
	Close() error
}

// session is a Shell over a hijacked Docker attach stream. The container
// and node shells differ only in how they resize, wait and kill.
type session struct {
	hijack  types.HijackedResponse
	scanner markerScanner
	pending []byte

	mu     sync.Mutex
	marker *Marker
	closed bool

	resize func(ctx context.Context, cols, rows uint) error
	wait   func() (int, error)
	kill   func(marker *Marker)
	once   sync.Once
}

func (s *session) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		n, err := s.hijack.Reader.Read(p)
		out := s.scanner.Feed(p[:n])
		if s.scanner.marker != nil {
			s.mu.Lock()
			s.marker = s.scanner.marker
			s.mu.Unlock()
		}
		// out may alias p; keep a copy for the next call
		s.pending = append(s.pending[:0], out...)
		if err != nil {
			if len(s.pending) > 0 {
				break
			}
			return 0, err
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *session) Write(p []byte) (int, error) {
	return s.hijack.Conn.Write(p)
}

func (s *session) Resize(cols, rows uint) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.resize(ctx, cols, rows)
}

func (s *session) Started() *Marker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.marker
}

func (s *session) Wait() (int, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return -1, ErrClosed
	}
	return s.wait()
}

func (s *session) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		marker := s.marker
		s.mu.Unlock()
		s.hijack.Close()
		s.kill(marker)
	})
	return nil
}

// OpenContainer starts a shell in a running container with docker exec,
// bash if the container has it, else sh. It returns the container's name.
func OpenContainer(ctx context.Context, cli *client.Client, id string, cols, rows uint) (Shell, string, error) {
	inspect, err := cli.ContainerInspect(ctx, id)
	if errdefs.IsNotFound(err) {
		return nil, "", fmt.Errorf("container %s not found", id)
	} else if err != nil {
		return nil, "", err
	}
	name := trimSlash(inspect.Name)
	if inspect.State == nil || !inspect.State.Running {
		return nil, name, fmt.Errorf("%s isn't running; start it to open a shell", name)
	}

	size := &[2]uint{rows, cols}
	created, err := cli.ContainerExecCreate(ctx, inspect.ID, container.ExecOptions{
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		DetachKeys:   detachKeys,
		ConsoleSize:  size,
		Env:          []string{term},
		Cmd:          []string{"sh", "-c", wrapperScript("")},
	})
	if err != nil {
		return nil, name, fmt.Errorf("starting a shell in %s: %w", name, err)
	}
	hijack, err := cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{Tty: true, ConsoleSize: size})
	if err != nil {
		return nil, name, fmt.Errorf("attaching to the shell in %s: %w", name, err)
	}

	s := &session{hijack: hijack}
	s.resize = func(ctx context.Context, cols, rows uint) error {
		return cli.ContainerExecResize(ctx, created.ID, container.ResizeOptions{Width: cols, Height: rows})
	}
	s.wait = func() (int, error) {
		// The stream ends a moment before Docker records the exit code
		for i := 0; i < 20; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			info, err := cli.ContainerExecInspect(ctx, created.ID)
			cancel()
			if err != nil {
				return -1, err
			}
			if !info.Running {
				return info.ExitCode, nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		return -1, fmt.Errorf("the shell's output ended but it is still running")
	}
	s.kill = func(marker *Marker) {
		killExec(cli, inspect.ID, created.ID, marker)
	}
	return s, name, nil
}

// killExec ends an exec that is still running after its stream closed, as
// one left in `top` or `tail -f` would be. Docker can't kill an exec, so
// this signals the shell's pid from a second exec in the same container.
// When the shell (the session leader) dies, the kernel hangs up whatever
// still runs in its terminal.
func killExec(cli *client.Client, containerID, execID string, marker *Marker) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	running := func() bool {
		info, err := cli.ContainerExecInspect(ctx, execID)
		return err == nil && info.Running
	}
	// A shell at its prompt exits by itself once its input closes
	for i := 0; i < 4 && running(); i++ {
		time.Sleep(250 * time.Millisecond)
	}
	if !running() || marker == nil {
		return
	}
	pid := strconv.Itoa(marker.PID)
	kill, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd: []string{"sh", "-c", `kill -HUP "$0" 2>/dev/null; sleep 1; kill -KILL "$0" 2>/dev/null`, pid},
	})
	if err != nil {
		return
	}
	cli.ContainerExecStart(ctx, kill.ID, container.ExecStartOptions{Detach: true})
}

// OpenNode starts a root shell on the node itself, from a privileged
// helper container in the host's PID namespace that enters the namespaces
// of PID 1. It works over any Docker transport, SSH included. labels are
// added to the helper, which is removed when the session closes.
func OpenNode(ctx context.Context, cli *client.Client, cols, rows uint, labels map[string]string) (Shell, error) {
	if err := deploy.EnsureHelperImage(ctx, cli); err != nil {
		return nil, err
	}

	all := map[string]string{deploy.LabelRole: "helper", LabelConsole: "node"}
	for k, v := range labels {
		all[k] = v
	}
	// Docker set HOSTNAME to the helper's id; unset, bash reads the host's
	script := `cd "$HOME" 2>/dev/null; unset HOSTNAME; ` + wrapperScript("-l")
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:     deploy.HelperImage,
			Cmd:       []string{"nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "--", "sh", "-c", script},
			Env:       []string{term, "HOME=/root", "USER=root", "LOGNAME=root"},
			Tty:       true,
			OpenStdin: true,
			// Stdin closes when the attach does, so the shell ends even if
			// the API dies without removing the helper
			StdinOnce:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Labels:       all,
		},
		&container.HostConfig{
			Privileged:  true,
			PidMode:     "host",
			NetworkMode: "none",
			AutoRemove:  true,
			ConsoleSize: [2]uint{rows, cols},
		},
		nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("creating the helper container: %w", err)
	}
	remove := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Errors are expected when AutoRemove got there first; anything
		// that really failed is left for Sweep
		cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
	}

	hijack, err := cli.ContainerAttach(ctx, created.ID, container.AttachOptions{
		Stream: true, Stdin: true, Stdout: true, Stderr: true, DetachKeys: detachKeys,
	})
	if err != nil {
		remove()
		return nil, fmt.Errorf("attaching to the helper container: %w", err)
	}
	// Registered before the start, so a shell that exits at once is seen
	waitCtx, cancelWait := context.WithCancel(context.Background())
	waitC, errC := cli.ContainerWait(waitCtx, created.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		cancelWait()
		hijack.Close()
		remove()
		return nil, fmt.Errorf("starting the helper container: %w", err)
	}

	s := &session{hijack: hijack}
	s.resize = func(ctx context.Context, cols, rows uint) error {
		return cli.ContainerResize(ctx, created.ID, container.ResizeOptions{Width: cols, Height: rows})
	}
	s.wait = func() (int, error) {
		select {
		case res := <-waitC:
			if res.Error != nil {
				return -1, errors.New(res.Error.Message)
			}
			return int(res.StatusCode), nil
		case err := <-errC:
			return -1, err
		case <-time.After(15 * time.Second):
			return -1, fmt.Errorf("the shell's output ended but it is still running")
		}
	}
	s.kill = func(*Marker) {
		cancelWait()
		remove()
	}
	return s, nil
}

// Owner says whose node shell helpers a sweep may remove early: those of
// this installation (by its fingerprint) created before this API started.
// An API has no sessions from before its start, so those were left behind
// by an earlier run that died. Any helper older than a session can last is
// left behind too, whoever made it.
type Owner struct {
	Instance string
	Started  time.Time
}

// Leftover reports whether a helper with these labels, created at created,
// can't belong to a live session.
func (o Owner) Leftover(labels map[string]string, created, now time.Time) bool {
	if created.Before(now.Add(-DefaultMaxDuration)) {
		return true
	}
	return o.Instance != "" && labels[LabelConsoleInstance] == o.Instance && created.Before(o.Started)
}

// Sweep removes the node shell helpers on a node that were left behind.
// Helpers normally go with their session, and with their input if the API
// dies, but a shell busy running something outlives its input.
func Sweep(ctx context.Context, cli *client.Client, owner Owner) (int, error) {
	list, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", deploy.LabelRole+"=helper"),
			filters.Arg("label", LabelConsole),
		),
	})
	if err != nil {
		return 0, err
	}
	removed := 0
	now := time.Now()
	for _, c := range list {
		if !owner.Leftover(c.Labels, time.Unix(c.Created, 0), now) {
			continue
		}
		if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err == nil {
			removed++
		}
	}
	return removed, nil
}

func trimSlash(name string) string {
	if len(name) > 0 && name[0] == '/' {
		return name[1:]
	}
	return name
}
