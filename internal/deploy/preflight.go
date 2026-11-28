package deploy

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// PreflightRequest is what the launch form knows before it launches.
type PreflightRequest struct {
	Node        string   `json:"node"`
	App         string   `json:"app"`
	Environment string   `json:"environment"`
	Hostnames   []string `json:"hostnames,omitempty"`
	// From inspecting the stack
	Requirements *Requirements `json:"requirements,omitempty"`
	// Published ports as the compose file writes them, e.g. "${SENTRY_BIND}"
	Ports []PreflightPort `json:"ports,omitempty"`
	// The values variables will have, for ports and requirement conditions;
	// secrets are left out
	Variables map[string]string `json:"variables,omitempty"`
}

type PreflightPort struct {
	Service   string `json:"service"`
	Published string `json:"published"`
}

const (
	CheckOK      = "ok"
	CheckInfo    = "info"
	CheckWarning = "warning"
	CheckFailed  = "failed"
)

type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type PreflightResult struct {
	Checks []Check `json:"checks"`
}

// Preflight checks that the node can run what is about to launch: enough
// memory and CPUs, free ports, and an ingress for its hostnames.
func (d *Deployer) Preflight(ctx context.Context, req PreflightRequest) (*PreflightResult, error) {
	server, err := d.db.ResolveServer(req.Node)
	if err != nil {
		return nil, err
	}
	res := &PreflightResult{Checks: []Check{}}
	add := func(c Check) { res.Checks = append(res.Checks, c) }

	cli, err := docker.Connect(server)
	if err == nil {
		defer cli.Close()
	}
	var info struct {
		version string
		cpus    int
		memMB   int
	}
	if err == nil {
		i, ierr := cli.Info(ctx)
		err = ierr
		info.version, info.cpus, info.memMB = i.ServerVersion, i.NCPU, int(i.MemTotal/(1<<20))
	}
	if err != nil {
		add(Check{ID: "server", Status: CheckFailed, Title: fmt.Sprintf("Can't reach %s", server.Name), Detail: err.Error()})
		return res, nil
	}

	req.Variables = withDefaults(req.Variables)
	resourceChecks(req, server.Name, info.cpus, info.memMB, info.version, add)
	if len(req.Ports) > 0 {
		add(d.portCheck(ctx, cli, req))
	}
	if len(req.Hostnames) > 0 {
		add(d.ingressCheck(ctx, cli, server))
	}
	return res, nil
}

func withDefaults(vars map[string]string) map[string]string {
	if vars == nil {
		vars = map[string]string{}
	}
	return vars
}

func resourceChecks(req PreflightRequest, node string, cpus, memMB int, version string, add func(Check)) {
	r := req.Requirements
	if r == nil {
		r = &Requirements{}
	}
	min := r.Applicable(req.Variables)
	why := "as its install script requires"
	if r.Source != "" {
		why = "as " + r.Source + " requires"
	}
	if min.When != nil {
		op := "is"
		if min.When.Not {
			op = "isn't"
		}
		why += fmt.Sprintf(" when %s %s %q", min.When.Variable, op, min.When.Value)
	}
	// Other minimums the script has, under other settings
	var alternatives []string
	for _, v := range r.Variants {
		if v == min || v.When == nil || v.MemoryMB >= min.MemoryMB {
			continue
		}
		op := "="
		if v.When.Not {
			op = "≠"
		}
		alternatives = append(alternatives, fmt.Sprintf("%s with %s%s%s", formatMemory(v.MemoryMB), v.When.Variable, op, v.When.Value))
	}

	needMem, needCPUs := min.MemoryMB, min.CPUs
	memWhy, cpuWhy := why, why
	if r.ReservedMemoryMB > needMem {
		needMem, memWhy = r.ReservedMemoryMB, "which its services reserve"
	}
	if r.ReservedCPUs > needCPUs {
		needCPUs, cpuWhy = r.ReservedCPUs, "which its services reserve"
	}

	switch {
	case needMem == 0:
		add(Check{ID: "memory", Status: CheckInfo, Title: fmt.Sprintf("%s has %s of memory", node, formatMemory(memMB)),
			Detail: "The stack doesn't say how much it needs."})
	case memMB >= needMem:
		add(Check{ID: "memory", Status: CheckOK, Title: fmt.Sprintf("%s of memory, of %s needed", formatMemory(memMB), formatMemory(needMem)),
			Detail: fmt.Sprintf("It needs %s, %s.", formatMemory(needMem), memWhy)})
	default:
		detail := fmt.Sprintf("It needs %s, %s; %s has %s.", formatMemory(needMem), memWhy, node, formatMemory(memMB))
		if len(alternatives) > 0 {
			detail += " It needs less as " + strings.Join(alternatives, ", ") + "."
		}
		add(Check{ID: "memory", Status: CheckFailed, Title: fmt.Sprintf("Not enough memory: %s of %s", formatMemory(memMB), formatMemory(needMem)), Detail: detail})
	}

	switch {
	case needCPUs == 0:
		add(Check{ID: "cpus", Status: CheckInfo, Title: fmt.Sprintf("%s has %d %s", node, cpus, plural(cpus, "CPU", "CPUs"))})
	case float64(cpus) >= needCPUs:
		add(Check{ID: "cpus", Status: CheckOK, Title: fmt.Sprintf("%d %s, of %s needed", cpus, plural(cpus, "CPU", "CPUs"), trimFloat(needCPUs)),
			Detail: fmt.Sprintf("It needs %s, %s.", trimFloat(needCPUs), cpuWhy)})
	default:
		add(Check{ID: "cpus", Status: CheckFailed, Title: fmt.Sprintf("Not enough CPUs: %d of %s", cpus, trimFloat(needCPUs)),
			Detail: fmt.Sprintf("It needs %s, %s; %s has %d.", trimFloat(needCPUs), cpuWhy, node, cpus)})
	}

	if r.DockerVersion != "" {
		if versionAtLeast(version, r.DockerVersion) {
			add(Check{ID: "docker", Status: CheckOK, Title: fmt.Sprintf("Docker %s, of %s needed", version, r.DockerVersion)})
		} else {
			add(Check{ID: "docker", Status: CheckFailed, Title: fmt.Sprintf("Docker %s is too old", version),
				Detail: fmt.Sprintf("It needs Docker %s or later, %s. Upgrade Docker on %s.", r.DockerVersion, why, node)})
		}
	}
}

// listener is what holds a port on the node.
type listener struct {
	owner string
}

func (d *Deployer) portCheck(ctx context.Context, cli *client.Client, req PreflightRequest) Check {
	project := req.App + "-" + req.Environment
	held := map[int]listener{}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return Check{ID: "ports", Status: CheckWarning, Title: "Couldn't check the ports", Detail: err.Error()}
	}
	ours := map[int]bool{}
	for _, c := range containers {
		mine := c.Labels["com.docker.compose.project"] == project ||
			(c.Labels[LabelApp] == req.App && c.Labels[LabelEnv] == req.Environment)
		for _, p := range c.Ports {
			if p.PublicPort == 0 || p.Type != "tcp" {
				continue
			}
			port := int(p.PublicPort)
			if mine {
				ours[port] = true
				continue
			}
			held[port] = listener{owner: describeOwner(c.Labels, c.Names)}
		}
	}
	// Anything else listening on the node, like a web server outside Docker
	for _, port := range hostListeners(ctx, cli) {
		if _, ok := held[port]; !ok && !ours[port] {
			held[port] = listener{owner: "a process outside Docker"}
		}
	}

	var free []string
	var conflicts []string
	var unresolved []string
	seen := map[int]bool{}
	for _, p := range req.Ports {
		ports, ok := resolvePublished(p.Published, req.Variables)
		if !ok {
			unresolved = append(unresolved, p.Published)
			continue
		}
		for _, port := range ports {
			if seen[port] {
				continue
			}
			seen[port] = true
			if l, taken := held[port]; taken {
				conflicts = append(conflicts, fmt.Sprintf("%d (for %s) is taken by %s", port, p.Service, l.owner))
			} else {
				free = append(free, strconv.Itoa(port))
			}
		}
	}

	switch {
	case len(conflicts) > 0:
		return Check{ID: "ports", Status: CheckFailed, Title: plural(len(conflicts), "A port it publishes is taken", "Ports it publishes are taken"),
			Detail: "Port " + strings.Join(conflicts, "; port ") + ". Stop that, or change the port in Variables."}
	case len(unresolved) > 0:
		title := "Couldn't tell which ports it publishes"
		if len(free) > 0 {
			title = fmt.Sprintf("%s %s free, but not all could be checked", plural(len(free), "Port", "Ports"), strings.Join(free, ", "))
		}
		return Check{ID: "ports", Status: CheckWarning, Title: title,
			Detail: "Set " + strings.Join(unresolved, ", ") + " in Variables to check them."}
	default:
		return Check{ID: "ports", Status: CheckOK, Title: fmt.Sprintf("%s %s free", plural(len(free), "Port", "Ports"), strings.Join(free, ", ")),
			Detail: "The ports the stack publishes on the node."}
	}
}

func describeOwner(labels map[string]string, names []string) string {
	if app, env := labels[LabelApp], labels[LabelEnv]; app != "" && env != "" {
		return app + "/" + env
	}
	if labels[LabelRole] == "ingress" || (len(names) > 0 && strings.TrimPrefix(names[0], "/") == IngressContainer) {
		return "the ingress"
	}
	if p := labels["com.docker.compose.project"]; p != "" {
		return "the compose project " + p
	}
	if len(names) > 0 {
		return "the container " + strings.TrimPrefix(names[0], "/")
	}
	return "a container"
}

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?-([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// resolvePublished expands variables in a published port and returns the
// ports it covers: "9000", "${PORT:-80}", "8000-8002", "127.0.0.1:9000".
func resolvePublished(published string, vars map[string]string) ([]int, bool) {
	ok := true
	s := varRef.ReplaceAllStringFunc(published, func(m string) string {
		sub := varRef.FindStringSubmatch(m)
		name := sub[1] + sub[3]
		if v, set := vars[name]; set && v != "" {
			return v
		}
		if strings.Contains(m, "-") && sub[1] != "" {
			return sub[2]
		}
		ok = false
		return ""
	})
	if !ok {
		return nil, false
	}
	// A host address in front of the port
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		// Compose picks a free port
		return nil, true
	}
	lo, hi, isRange := strings.Cut(s, "-")
	a, err := strconv.Atoi(lo)
	if err != nil || a < 1 || a > 65535 {
		return nil, false
	}
	b := a
	if isRange {
		if b, err = strconv.Atoi(hi); err != nil || b < a || b > 65535 || b-a > 1000 {
			return nil, false
		}
	}
	ports := make([]int, 0, b-a+1)
	for p := a; p <= b; p++ {
		ports = append(ports, p)
	}
	return ports, true
}

// hostListeners lists the TCP ports listening on the node, from a helper
// container in the host's network. It returns none if that can't run.
func hostListeners(ctx context.Context, cli *client.Client) []int {
	if err := ensureImage(ctx, cli, helperImage); err != nil {
		return nil
	}
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:  helperImage,
			Cmd:    []string{"netstat", "-tln"},
			Labels: map[string]string{LabelRole: "helper"},
		},
		&container.HostConfig{NetworkMode: "host"},
		nil, nil, "")
	if err != nil {
		return nil
	}
	defer removeContainer(cli, created.ID)

	wait, errs := cli.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return nil
	}
	select {
	case <-wait:
	case <-errs:
		return nil
	case <-ctx.Done():
		return nil
	}
	logs, err := cli.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true})
	if err != nil {
		return nil
	}
	defer logs.Close()
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(demux(pw, logs)) }()
	out, _ := io.ReadAll(pr)
	return parseNetstat(string(out))
}

// parseNetstat reads the local ports of `netstat -tln` lines like
// "tcp  0  0 0.0.0.0:9000  0.0.0.0:*  LISTEN".
func parseNetstat(out string) []int {
	seen := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "tcp") {
			continue
		}
		local := fields[3]
		port, err := strconv.Atoi(local[strings.LastIndex(local, ":")+1:])
		if err != nil {
			continue
		}
		// Loopback-only listeners on the node still collide with a
		// published port on all addresses
		seen[port] = true
	}
	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

func (d *Deployer) ingressCheck(ctx context.Context, cli *client.Client, server *database.Server) Check {
	state := "not installed"
	if inspect, err := cli.ContainerInspect(ctx, IngressContainer); err == nil {
		state = inspect.State.Status
	}
	switch {
	case d.cfg.NPM == nil:
		return Check{ID: "ingress", Status: CheckWarning, Title: "Hostnames won't be set up on the gateway",
			Detail: "Nginx Proxy Manager isn't configured on this Docktrine server (DOCKTRINE_NPM_URL). The app still runs."}
	case state != "running":
		return Check{ID: "ingress", Status: CheckWarning, Title: fmt.Sprintf("The ingress on %s is %s", server.Name, state),
			Detail: "Hostnames route through it. Install it from Servers; the app still runs without it."}
	case server.IngressHost == "" || server.IngressPort == 0:
		return Check{ID: "ingress", Status: CheckWarning, Title: "The gateway doesn't know where to reach " + server.Name,
			Detail: "Set the server's ingress host and port in Servers so hostnames can be forwarded to it."}
	}
	return Check{ID: "ingress", Status: CheckOK, Title: "The ingress is running",
		Detail: fmt.Sprintf("The gateway forwards hostnames to %s:%d.", server.IngressHost, server.IngressPort)}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
