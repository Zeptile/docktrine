package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	// ingressAPIAddress is where the ingress serves Traefik's API, inside its
	// own container only.
	ingressAPIAddress = "127.0.0.1:8081"
	// routeTimeout bounds the wait for the ingress to route to new containers.
	// Traefik normally picks up a container within about 2 seconds.
	routeTimeout = 30 * time.Second
)

// errNoIngressAPI means the node's ingress predates the API used to confirm
// routing; callers fall back to waiting a fixed time.
var errNoIngressAPI = errors.New("ingress API unavailable")

// waitRouted waits until the node ingress sends this environment's traffic
// to every one of the given containers. Without hostnames there is nothing
// to wait for; without an ingress on the node, hostnames can't work anyway
// and the deploy goes ahead as before.
func waitRouted(ctx context.Context, cli *client.Client, t target, ids []string, log *deployLog) error {
	if len(t.spec.Hostnames) == 0 || len(ids) == 0 {
		return nil
	}

	ingress, err := cli.ContainerInspect(ctx, IngressContainer)
	if errdefs.IsNotFound(err) {
		log.Printf("No ingress on %s, so hostnames won't route; install it with `docktrine servers ingress %s`",
			t.server.Name, t.server.Name)
		return nil
	}
	if err != nil {
		return err
	}
	if ingress.State == nil || !ingress.State.Running {
		return fmt.Errorf("the ingress on %s isn't running", t.server.Name)
	}

	want, names, err := backendURLs(ctx, cli, ids, t.spec.Port)
	if err != nil {
		return err
	}

	service := t.project() + "@docker"
	log.Printf("Waiting for the ingress to route to %s", strings.Join(names, ", "))
	deadline := time.Now().Add(routeTimeout)
	for {
		servers, err := ingressServers(ctx, cli, service)
		if errors.Is(err, errNoIngressAPI) {
			log.Printf("The ingress on %s can't report its routes; reinstall it with `docktrine servers ingress %s` for faster, verified swaps",
				t.server.Name, t.server.Name)
			return errNoIngressAPI
		}
		if err != nil {
			return err
		}
		if missing := missingURLs(want, servers); len(missing) == 0 {
			log.Printf("Ingress is routing to %s", strings.Join(names, ", "))
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("the ingress on %s didn't start routing to %s within %s",
				t.server.Name, strings.Join(names, ", "), routeTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// backendURLs are the addresses the ingress uses for the containers, as
// Traefik reports them: http://<ip on the docktrine network>:<port>.
func backendURLs(ctx context.Context, cli *client.Client, ids []string, port int) ([]string, []string, error) {
	urls, names := []string{}, []string{}
	for _, id := range ids {
		c, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, strings.TrimPrefix(c.Name, "/"))
		net, ok := c.NetworkSettings.Networks[Network]
		if !ok || net.IPAddress == "" {
			return nil, nil, fmt.Errorf("container %s isn't on the %s network", strings.TrimPrefix(c.Name, "/"), Network)
		}
		urls = append(urls, fmt.Sprintf("http://%s:%d", net.IPAddress, port))
	}
	sort.Strings(names)
	return urls, names, nil
}

func missingURLs(want, have []string) []string {
	set := map[string]bool{}
	for _, u := range have {
		set[u] = true
	}
	missing := []string{}
	for _, u := range want {
		if !set[u] {
			missing = append(missing, u)
		}
	}
	return missing
}

// ingressServers returns the backend URLs Traefik currently routes a
// service to; none while the service doesn't exist yet.
func ingressServers(ctx context.Context, cli *client.Client, service string) ([]string, error) {
	// wget exits 1 for HTTP errors (a service that doesn't exist yet is a
	// 404) and for refused connections; -S prints the status to tell them apart.
	out, errOut, code, err := execOutput(ctx, cli, IngressContainer,
		"wget", "-S", "-q", "-O", "-", "http://"+ingressAPIAddress+"/api/http/services/"+service)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		if strings.Contains(errOut, "404 Not Found") {
			return nil, nil
		}
		if strings.Contains(errOut, "Connection refused") || strings.Contains(errOut, "can't connect") {
			return nil, errNoIngressAPI
		}
		return nil, fmt.Errorf("querying the ingress: %s", strings.TrimSpace(errOut))
	}

	var body struct {
		LoadBalancer struct {
			Servers []struct {
				URL string `json:"url"`
			} `json:"servers"`
		} `json:"loadBalancer"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		return nil, fmt.Errorf("reading the ingress routes: %w", err)
	}
	urls := []string{}
	for _, s := range body.LoadBalancer.Servers {
		urls = append(urls, s.URL)
	}
	return urls, nil
}

// execOutput runs a command in a container and returns its stdout, stderr
// and exit code.
func execOutput(ctx context.Context, cli *client.Client, containerID string, cmd ...string) (string, string, int, error) {
	exec, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", "", 0, err
	}
	resp, err := cli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return "", "", 0, err
	}
	inspect, err := cli.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return "", "", 0, err
	}
	return stdout.String(), stderr.String(), inspect.ExitCode, nil
}
