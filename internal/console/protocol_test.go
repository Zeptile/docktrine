package console

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestParseControl(t *testing.T) {
	cases := []struct {
		in      string
		ok      bool
		cols    int
		rows    int
		comment string
	}{
		{`{"type":"resize","cols":120,"rows":40}`, true, 120, 40, "resize"},
		{`{"type":"resize","cols":1,"rows":1}`, true, 1, 1, "smallest"},
		{`{"type":"resize","cols":1000,"rows":500}`, true, 1000, 500, "largest"},
		{`{"type":"resize","cols":0,"rows":40}`, false, 0, 0, "zero columns"},
		{`{"type":"resize","cols":120,"rows":-3}`, false, 0, 0, "negative rows"},
		{`{"type":"resize","cols":5000,"rows":40}`, false, 0, 0, "too wide"},
		{`{"type":"resize"}`, false, 0, 0, "no size"},
		{`{"type":"input","data":"ls"}`, false, 0, 0, "input goes in binary messages"},
		{`{"cols":80,"rows":24}`, false, 0, 0, "no type"},
		{`resize 80 24`, false, 0, 0, "not JSON"},
	}
	for _, c := range cases {
		m, err := ParseControl([]byte(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%s: ParseControl(%s) error = %v, want ok=%v", c.comment, c.in, err, c.ok)
			continue
		}
		if c.ok && (m.Cols != c.cols || m.Rows != c.rows) {
			t.Errorf("%s: got %dx%d, want %dx%d", c.comment, m.Cols, m.Rows, c.cols, c.rows)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		cols, rows string
		wc, wr     uint
	}{
		{"132", "43", 132, 43},
		{"", "", 80, 24},
		{"abc", "24", 80, 24},
		{"0", "24", 80, 24},
		{"99999", "24", 80, 24},
	}
	for _, c := range cases {
		gc, gr := ParseSize(c.cols, c.rows)
		if gc != c.wc || gr != c.wr {
			t.Errorf("ParseSize(%q, %q) = %d, %d; want %d, %d", c.cols, c.rows, gc, gr, c.wc, c.wr)
		}
	}
}

// feedAll runs chunks through a scanner and joins what it passes on.
func feedAll(chunks ...string) (string, *Marker) {
	var s markerScanner
	var out bytes.Buffer
	for _, c := range chunks {
		out.Write(s.Feed([]byte(c)))
	}
	// What's still held back would be flushed by the next read or EOF
	out.Write(s.buf)
	return out.String(), s.marker
}

func TestMarkerScanner(t *testing.T) {
	marker := "\x1b]6973;42;bash\a"

	cases := []struct {
		name   string
		chunks []string
		out    string
		pid    int
		shell  string
	}{
		{"whole", []string{marker + "root@box:/# "}, "root@box:/# ", 42, "bash"},
		{"split in the prefix", []string{"\x1b]69", "73;42;bash\aprompt"}, "prompt", 42, "bash"},
		{"split in the body", []string{"\x1b]6973;4", "2;ba", "sh\a", "$ "}, "$ ", 42, "bash"},
		{"one byte at a time", splitBytes(marker + "$ "), "$ ", 42, "bash"},
		{"sh", []string{"\x1b]6973;7;sh\a# "}, "# ", 7, "sh"},
		{"no marker", []string{"OCI runtime exec failed: sh not found\r\n"}, "OCI runtime exec failed: sh not found\r\n", 0, ""},
		{"another escape first", []string{"\x1b[?2004h$ "}, "\x1b[?2004h$ ", 0, ""},
		{"a bad pid passes through", []string{"\x1b]6973;x;bash\a$ "}, "\x1b]6973;x;bash\a$ ", 0, ""},
		{"an unknown shell passes through", []string{"\x1b]6973;5;zsh\a$ "}, "\x1b]6973;5;zsh\a$ ", 0, ""},
		{"only the first marker counts", []string{marker, marker}, marker, 42, "bash"},
	}
	for _, c := range cases {
		out, m := feedAll(c.chunks...)
		if out != c.out {
			t.Errorf("%s: output %q, want %q", c.name, out, c.out)
		}
		switch {
		case c.pid == 0 && m != nil:
			t.Errorf("%s: found marker %+v, want none", c.name, *m)
		case c.pid != 0 && (m == nil || m.PID != c.pid || m.Shell != c.shell):
			t.Errorf("%s: marker %+v, want pid %d shell %s", c.name, m, c.pid, c.shell)
		}
	}
}

func TestMarkerScannerGivesUp(t *testing.T) {
	// A prefix with no end must not hold output back forever
	var s markerScanner
	long := "\x1b]6973;" + string(bytes.Repeat([]byte("1"), markerLimit))
	if out := s.Feed([]byte(long)); string(out) != long {
		t.Fatalf("held back %d bytes past the limit", len(long)-len(out))
	}
	if out := s.Feed([]byte("more")); string(out) != "more" {
		t.Fatalf("after giving up, got %q", out)
	}
}

func splitBytes(s string) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i : i+1]
	}
	return out
}

// The wrapper is shell code; run it with the local sh to check it prints
// the marker the scanner expects, then execs the shell it named.
func TestWrapperScript(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	cmd := exec.Command("sh", "-c", wrapperScript("-c 'echo hello'"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("wrapper failed: %v", err)
	}
	var s markerScanner
	rest := string(s.Feed(out))
	if s.marker == nil {
		t.Fatalf("no marker in %q", out)
	}
	if s.marker.Shell != "bash" && s.marker.Shell != "sh" {
		t.Errorf("shell %q", s.marker.Shell)
	}
	if rest != "hello\n" {
		t.Errorf("output after the marker %q, want %q", rest, "hello\n")
	}
}
