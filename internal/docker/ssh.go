package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SSHArgs builds the ssh arguments that run cmd on the host of an ssh:// URL.
func SSHArgs(rawURL string, cmd ...string) ([]string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "ssh" || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid ssh host %q", rawURL)
	}

	args := []string{"-o", "ConnectTimeout=10", "-o", "BatchMode=yes"}
	if u.Port() != "" {
		args = append(args, "-p", u.Port())
	}
	target := u.Hostname()
	if u.User != nil {
		target = u.User.Username() + "@" + target
	}
	args = append(args, "--", target)
	return append(args, cmd...), nil
}

// sshDialer returns a dialer that tunnels the Docker API through
// `ssh host docker system dial-stdio`, like the docker CLI does.
func sshDialer(rawURL string) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	args, err := SSHArgs(rawURL, "docker", "system", "dial-stdio")
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Not CommandContext: the dial context ends once the connection is
		// established, but the process must live as long as the connection.
		cmd := exec.Command("ssh", args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		conn := &commandConn{cmd: cmd, stdin: stdin, stdout: stdout}
		cmd.Stderr = &conn.stderr
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("starting ssh: %w", err)
		}
		return conn, nil
	}, nil
}

// commandConn is a net.Conn over a subprocess's stdin and stdout.
type commandConn struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    bytes.Buffer
	closeOnce sync.Once
}

func (c *commandConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err == io.EOF && n == 0 {
		if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
			return 0, fmt.Errorf("ssh: %s", msg)
		}
	}
	return n, err
}

func (c *commandConn) Write(p []byte) (int, error) {
	return c.stdin.Write(p)
}

func (c *commandConn) Close() error {
	c.closeOnce.Do(func() {
		c.stdin.Close()
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
		c.cmd.Wait()
	})
	return nil
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "ssh" }
func (dummyAddr) String() string  { return "ssh" }

func (c *commandConn) LocalAddr() net.Addr                { return dummyAddr{} }
func (c *commandConn) RemoteAddr() net.Addr               { return dummyAddr{} }
func (c *commandConn) SetDeadline(t time.Time) error      { return nil }
func (c *commandConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *commandConn) SetWriteDeadline(t time.Time) error { return nil }
