package deploy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

const (
	healthTimeout   = 2 * time.Minute
	stableWindow    = 10 * time.Second
	stopGracePeriod = 10
	// drainPeriod covers the ingress noticing container changes; Traefik
	// batches Docker events for ~2s before applying them.
	drainPeriod = 5 * time.Second
)

// deployImage starts the new container next to the old one, waits for it to
// be healthy, then removes the previous containers. If the new container
// never becomes healthy it is removed and the old one keeps serving.
func (d *Deployer) deployImage(ctx context.Context, t target, log *deployLog) error {
	cli, err := docker.Connect(t.server)
	if err != nil {
		return err
	}
	defer cli.Close()

	log.Printf("Pulling %s", t.spec.Image)
	if err := pullImage(ctx, cli, t.spec.Image, d.registryAuth(t.spec.Image)); err != nil {
		return fmt.Errorf("pulling %s: %w", t.spec.Image, err)
	}

	if err := ensureNetwork(ctx, cli); err != nil {
		return err
	}

	binds, err := ensureVolumes(ctx, cli, t)
	if err != nil {
		return err
	}

	labels := t.labels()
	for k, v := range t.routingLabels() {
		labels[k] = v
	}

	config := &container.Config{
		Image:  t.spec.Image,
		Env:    envList(t.env),
		Labels: labels,
	}
	if len(t.spec.Command) > 0 {
		config.Cmd = t.spec.Command
	}
	if t.spec.Healthcheck != "" {
		url := fmt.Sprintf("http://127.0.0.1:%d%s", t.spec.Port, t.spec.Healthcheck)
		config.Healthcheck = &container.HealthConfig{
			Test: []string{"CMD-SHELL", fmt.Sprintf(
				"wget -q -O /dev/null %[1]s || curl -fsS -o /dev/null %[1]s", url)},
			Interval:    5 * time.Second,
			Timeout:     3 * time.Second,
			StartPeriod: 10 * time.Second,
			Retries:     3,
		}
	}

	hostConfig := &container.HostConfig{
		Binds:         binds,
		NetworkMode:   container.NetworkMode(Network),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}

	name := fmt.Sprintf("%s-%d", t.project(), t.deploymentID)
	log.Printf("Creating container %s", name)
	created, err := cli.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, name)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		removeContainer(cli, created.ID)
		return fmt.Errorf("starting container: %w", err)
	}

	if err := waitHealthy(ctx, cli, created.ID, t.spec.Healthcheck != "", log); err != nil {
		log.Printf("Last log lines from %s:", name)
		for _, line := range tailLogs(cli, created.ID, 30) {
			log.Printf("  | %s", line)
		}
		removeContainer(cli, created.ID)
		return err
	}

	// Only stop the old containers once the new one gets traffic; if it
	// never does, the old ones keep serving.
	routed := true
	if err := waitRouted(ctx, cli, t, []string{created.ID}, log); errors.Is(err, errNoIngressAPI) {
		routed = false
	} else if err != nil {
		removeContainer(cli, created.ID)
		return err
	}

	return removePrevious(ctx, cli, t, log, routed)
}

func pullImage(ctx context.Context, cli *client.Client, ref, auth string) error {
	reader, err := cli.ImagePull(ctx, ref, image.PullOptions{RegistryAuth: auth})
	if err != nil {
		return err
	}
	defer reader.Close()

	// The pull only completes once the progress stream is drained; errors
	// (e.g. auth or manifest not found) arrive inside the stream.
	decoder := json.NewDecoder(reader)
	for {
		var msg struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&msg); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if msg.Error != "" {
			return fmt.Errorf("%s", msg.Error)
		}
	}
}

func ensureNetwork(ctx context.Context, cli *client.Client) error {
	_, err := cli.NetworkInspect(ctx, Network, network.InspectOptions{})
	if err == nil {
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return err
	}
	_, err = cli.NetworkCreate(ctx, Network, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{LabelRole: "network"},
	})
	if errdefs.IsConflict(err) {
		return nil
	}
	return err
}

// ensureVolumes turns spec volumes into bind strings. Named volumes are
// scoped to the environment ("data" becomes "<app>-<env>-data") and labelled
// so destroy --purge can find them; absolute paths are bind mounts.
func ensureVolumes(ctx context.Context, cli *client.Client, t target) ([]string, error) {
	binds := []string{}
	for _, spec := range t.spec.Volumes {
		source, rest, ok := strings.Cut(spec, ":")
		if !ok || source == "" || rest == "" {
			return nil, fmt.Errorf("invalid volume %q, expected name:/path", spec)
		}
		if !strings.HasPrefix(source, "/") {
			source = t.project() + "-" + source
			_, err := cli.VolumeCreate(ctx, volume.CreateOptions{
				Name: source,
				Labels: map[string]string{
					LabelApp: t.spec.App,
					LabelEnv: t.spec.Environment,
				},
			})
			if err != nil {
				return nil, fmt.Errorf("creating volume %s: %w", source, err)
			}
		}
		binds = append(binds, source+":"+rest)
	}
	return binds, nil
}

func envList(values map[string]string) []string {
	list := make([]string, 0, len(values))
	for k, v := range values {
		list = append(list, k+"="+v)
	}
	sort.Strings(list)
	return list
}

// waitHealthy waits for the Docker health check to pass, or, without one,
// for the container to stay up without restarting for a short window.
func waitHealthy(ctx context.Context, cli *client.Client, id string, hasHealthcheck bool, log *deployLog) error {
	if hasHealthcheck {
		log.Printf("Waiting for health check to pass")
	} else {
		log.Printf("Waiting %s for the container to stay up (no healthcheck configured)", stableWindow)
	}

	deadline := time.Now().Add(healthTimeout)
	startedAt := time.Now()
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}

		inspect, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			return err
		}
		state := inspect.State
		if state.Restarting || inspect.RestartCount > 0 {
			return fmt.Errorf("container keeps crashing (restarted %d times)", inspect.RestartCount)
		}
		if !state.Running && state.Status == "exited" {
			return fmt.Errorf("container exited with code %d", state.ExitCode)
		}

		if hasHealthcheck {
			if state.Health != nil {
				switch state.Health.Status {
				case "healthy":
					log.Printf("Container is healthy")
					return nil
				case "unhealthy":
					return fmt.Errorf("container is unhealthy")
				}
			}
		} else if state.Running && time.Since(startedAt) >= stableWindow {
			log.Printf("Container is running")
			return nil
		}
	}
	return fmt.Errorf("container did not become healthy within %s", healthTimeout)
}

func tailLogs(cli *client.Client, id string, lines int) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reader, err := cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(lines),
	})
	if err != nil {
		return nil
	}
	defer reader.Close()

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(demux(pw, reader))
	}()

	out := []string{}
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	return out
}

func removeContainer(cli *client.Client, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

// removePrevious stops and removes containers of this environment that
// belong to older deployments. routed says the ingress is confirmed to send
// traffic to the new container; otherwise it waits for that to happen.
func removePrevious(ctx context.Context, cli *client.Client, t target, log *deployLog, routed bool) error {
	containers, err := listWorkload(ctx, cli, t.spec.App, t.spec.Environment)
	if err != nil {
		return err
	}

	current := strconv.FormatInt(t.deploymentID, 10)
	previous := []types.Container{}
	for _, c := range containers {
		if c.Labels[LabelDeployment] != current {
			previous = append(previous, c)
		}
	}
	if len(previous) == 0 {
		return nil
	}

	// Requests still sent to a stopping container are retried on the new one
	// by the router's retry middleware. Without confirmation from the
	// ingress, give it time to start routing to the new container first.
	if len(t.spec.Hostnames) > 0 && !routed {
		time.Sleep(drainPeriod)
	}

	timeout := stopGracePeriod
	for _, c := range previous {
		log.Printf("Removing previous container %s", strings.TrimPrefix(firstName(c.Names), "/"))
		cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout})
		if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("removing previous container: %w", err)
		}
	}
	return nil
}

func listWorkload(ctx context.Context, cli *client.Client, app, env string) ([]types.Container, error) {
	return cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", LabelApp+"="+app),
			filters.Arg("label", LabelEnv+"="+env),
		),
	})
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// removeWorkload deletes all containers of an environment, plus compose
// networks and, with purge, its volumes.
func (d *Deployer) removeWorkload(ctx context.Context, server *database.Server, app, env string, purge, stackFiles bool) error {
	cli, err := docker.Connect(server)
	if err != nil {
		return err
	}
	defer cli.Close()

	containers, err := listWorkload(ctx, cli, app, env)
	if err != nil {
		return err
	}
	timeout := stopGracePeriod
	for _, c := range containers {
		cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout})
		// Purging also takes the anonymous volumes of images that declare
		// VOLUME, which have no labels to find them by later
		if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true, RemoveVolumes: purge}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}

	project := app + "-" + env
	networks, err := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project)),
	})
	if err == nil {
		for _, n := range networks {
			cli.NetworkRemove(ctx, n.ID)
		}
	}

	if !purge {
		return nil
	}
	for _, f := range []filters.Args{
		filters.NewArgs(filters.Arg("label", LabelApp+"="+app), filters.Arg("label", LabelEnv+"="+env)),
		filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project)),
	} {
		volumes, err := cli.VolumeList(ctx, volume.ListOptions{Filters: f})
		if err != nil {
			return err
		}
		for _, v := range volumes.Volumes {
			if err := cli.VolumeRemove(ctx, v.Name, true); err != nil && !errdefs.IsNotFound(err) {
				return fmt.Errorf("removing volume %s: %w", v.Name, err)
			}
		}
	}
	if stackFiles {
		if err := removeStackFiles(ctx, cli, project); err != nil {
			return fmt.Errorf("removing the stack's files: %w", err)
		}
	}
	return nil
}
