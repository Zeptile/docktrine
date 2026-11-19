package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
)

const (
	IngressContainer = "docktrine-ingress"
	IngressImage     = "traefik:v3.1"
)

func connect(server *database.Server) (*client.Client, error) {
	return docker.Connect(server)
}

// syncProxyHosts makes NPM forward each hostname of the environment to the
// node's ingress, and removes hosts the environment no longer declares.
func (d *Deployer) syncProxyHosts(env *database.Environment, spec *manifest.Spec, server *database.Server, log *deployLog) error {
	existing, err := d.db.ListProxyHosts(env.ID)
	if err != nil {
		return err
	}
	if len(spec.Hostnames) == 0 && len(existing) == 0 {
		return nil
	}
	if d.cfg.NPM == nil {
		return fmt.Errorf("Nginx Proxy Manager is not configured (DOCKTRINE_NPM_URL)")
	}
	if server.IngressHost == "" || server.IngressPort == 0 {
		return fmt.Errorf("server %s has no ingress_host/ingress_port for the gateway to forward to", server.Name)
	}

	wanted := map[string]bool{}
	for _, host := range spec.Hostnames {
		wanted[strings.ToLower(host)] = true
	}
	known := map[string]int64{}
	for _, h := range existing {
		known[h.Domain] = h.NpmID
	}

	for domain := range wanted {
		if id, ok := known[domain]; ok {
			if err := d.cfg.NPM.UpdateForward(id, server.IngressHost, server.IngressPort); err == nil {
				continue
			} else if !strings.Contains(err.Error(), "404") {
				return fmt.Errorf("updating %s: %w", domain, err)
			}
			// Deleted in NPM by hand; fall through and recreate it.
		}

		current, err := d.cfg.NPM.FindProxyHost(domain)
		if err != nil {
			return err
		}
		var id int64
		if current != nil {
			log.Printf("Adopting existing proxy host for %s", domain)
			id = current.ID
			if err := d.cfg.NPM.UpdateForward(id, server.IngressHost, server.IngressPort); err != nil {
				return fmt.Errorf("updating %s: %w", domain, err)
			}
		} else {
			log.Printf("Creating proxy host for %s", domain)
			id, err = d.cfg.NPM.CreateProxyHost(domain, server.IngressHost, server.IngressPort)
			if err != nil {
				return fmt.Errorf("creating %s: %w", domain, err)
			}
		}
		if err := d.db.SaveProxyHost(env.ID, domain, id); err != nil {
			return err
		}
	}

	for _, h := range existing {
		if wanted[h.Domain] {
			continue
		}
		log.Printf("Removing proxy host for %s", h.Domain)
		if err := d.cfg.NPM.DeleteProxyHost(h.NpmID); err != nil {
			return fmt.Errorf("removing %s: %w", h.Domain, err)
		}
		if err := d.db.DeleteProxyHost(h.Domain); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployer) removeProxyHosts(env *database.Environment) error {
	hosts, err := d.db.ListProxyHosts(env.ID)
	if err != nil || len(hosts) == 0 {
		return err
	}
	if d.cfg.NPM == nil {
		return fmt.Errorf("cannot remove proxy hosts: Nginx Proxy Manager is not configured")
	}
	for _, h := range hosts {
		if err := d.cfg.NPM.DeleteProxyHost(h.NpmID); err != nil {
			return fmt.Errorf("removing %s: %w", h.Domain, err)
		}
		if err := d.db.DeleteProxyHost(h.Domain); err != nil {
			return err
		}
	}
	return nil
}

type IngressOptions struct {
	// Port the ingress listens on, on the node. The gateway (or its SSH
	// tunnel) must reach this port.
	Port int `json:"port"`
	// Bind restricts the published port to one address, e.g. 127.0.0.1 when
	// the node is only reached through an SSH tunnel. Defaults to all.
	Bind string `json:"bind"`
}

// InstallIngress (re)creates the Traefik container that routes requests
// arriving from the gateway to app containers by Host header.
func (d *Deployer) InstallIngress(server *database.Server, opts IngressOptions) error {
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("a valid port is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli, err := connect(server)
	if err != nil {
		return err
	}
	defer cli.Close()

	if err := pullImage(ctx, cli, IngressImage, ""); err != nil {
		return fmt.Errorf("pulling %s: %w", IngressImage, err)
	}
	if err := ensureNetwork(ctx, cli); err != nil {
		return err
	}

	if err := cli.ContainerRemove(ctx, IngressContainer, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	config := &container.Config{
		Image: IngressImage,
		Cmd: []string{
			"--providers.docker=true",
			"--providers.docker.exposedbydefault=false",
			"--providers.docker.network=" + Network,
			"--entrypoints.web.address=:80",
			// TLS ends at the gateway; trust its X-Forwarded-* headers.
			"--entrypoints.web.forwardedheaders.insecure=true",
			"--ping=true",
			// A container that just stopped leaves an unreachable IP behind
			// until Traefik drops it; fail fast so the retry middleware moves
			// the request to a live container instead of hanging.
			"--serverstransport.forwardingtimeouts.dialtimeout=1s",
			// Read-only API on the container's loopback, out of reach of app
			// containers; deploys query it to confirm a new container is
			// routed before reporting success or stopping the old one.
			"--api=true",
			"--api.insecure=true",
			"--api.dashboard=false",
			"--entrypoints.traefik.address=" + ingressAPIAddress,
		},
		Labels:       map[string]string{LabelRole: "ingress"},
		ExposedPorts: nat.PortSet{"80/tcp": struct{}{}},
	}
	hostConfig := &container.HostConfig{
		NetworkMode:   container.NetworkMode(Network),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
		Binds:         []string{"/var/run/docker.sock:/var/run/docker.sock:ro"},
		PortBindings: nat.PortMap{
			"80/tcp": []nat.PortBinding{{HostIP: opts.Bind, HostPort: fmt.Sprint(opts.Port)}},
		},
	}

	created, err := cli.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, IngressContainer)
	if err != nil {
		return err
	}
	return cli.ContainerStart(ctx, created.ID, container.StartOptions{})
}
