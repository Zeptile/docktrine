package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

type ContainerStatus struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Service    string `json:"service,omitempty"`
	Image      string `json:"image"`
	State      string `json:"state"`
	Status     string `json:"status"`
	Deployment int64  `json:"deployment"`
}

// Containers lists what currently runs for an environment on its node.
func (d *Deployer) Containers(ctx context.Context, env *database.Environment) ([]ContainerStatus, error) {
	server, err := d.db.ResolveServer(env.Server)
	if err != nil {
		return nil, err
	}
	cli, err := connect(server)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	containers, err := listWorkload(ctx, cli, env.App, env.Name)
	if err != nil {
		return nil, err
	}

	statuses := []ContainerStatus{}
	for _, c := range containers {
		deployment, _ := strconv.ParseInt(c.Labels[LabelDeployment], 10, 64)
		statuses = append(statuses, ContainerStatus{
			ID:         c.ID[:12],
			Name:       strings.TrimPrefix(firstName(c.Names), "/"),
			Service:    c.Labels["com.docker.compose.service"],
			Image:      c.Image,
			State:      c.State,
			Status:     c.Status,
			Deployment: deployment,
		})
	}
	return statuses, nil
}

// Logs streams the logs of every container of an environment to w. With
// several containers (compose), each line is prefixed by its service name.
func (d *Deployer) Logs(ctx context.Context, env *database.Environment, tail int, follow bool, w io.Writer) error {
	server, err := d.db.ResolveServer(env.Server)
	if err != nil {
		return err
	}
	cli, err := connect(server)
	if err != nil {
		return err
	}
	defer cli.Close()

	containers, err := listWorkload(ctx, cli, env.App, env.Name)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("no containers running for %s/%s", env.App, env.Name)
	}

	out := &lockedWriter{w: w}
	var wg sync.WaitGroup
	errs := make(chan error, len(containers))
	for _, c := range containers {
		prefix := ""
		if len(containers) > 1 {
			name := c.Labels["com.docker.compose.service"]
			if name == "" {
				name = strings.TrimPrefix(firstName(c.Names), "/")
			}
			prefix = name + " | "
		}

		reader, err := cli.ContainerLogs(ctx, c.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     follow,
			Tail:       strconv.Itoa(tail),
		})
		if err != nil {
			return err
		}

		wg.Add(1)
		go func(reader io.ReadCloser, prefix string) {
			defer wg.Done()
			defer reader.Close()

			pr, pw := io.Pipe()
			go func() { pw.CloseWithError(demux(pw, reader)) }()

			scanner := bufio.NewScanner(pr)
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			for scanner.Scan() {
				if err := out.WriteLine(prefix + scanner.Text()); err != nil {
					errs <- err
					return
				}
			}
		}(reader, prefix)
	}

	wg.Wait()
	close(errs)
	return <-errs
}

// demux splits Docker's multiplexed stdout/stderr stream into one writer.
func demux(w io.Writer, r io.Reader) error {
	_, err := stdcopy.StdCopy(w, w, r)
	return err
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) WriteLine(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := io.WriteString(l.w, line+"\n"); err != nil {
		return err
	}
	if f, ok := l.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}
