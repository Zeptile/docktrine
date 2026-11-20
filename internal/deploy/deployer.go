// Package deploy turns a manifest.Spec into running containers on a node,
// routes its hostnames through Nginx Proxy Manager, and cleans up after it.
package deploy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/Zeptile/docktrine/internal/npm"
	"github.com/Zeptile/docktrine/internal/secrets"
	"github.com/docker/docker/api/types/registry"
)

const (
	// Network is the Docker network shared by app containers and the node ingress.
	Network = "docktrine"

	LabelApp        = "docktrine.app"
	LabelEnv        = "docktrine.env"
	LabelDeployment = "docktrine.deployment"
	LabelRole       = "docktrine.role"

	deployTimeout = 15 * time.Minute
)

type Config struct {
	// Registry is the central registry host (optionally with a namespace),
	// e.g. "registry.zeptile.software" or "ghcr.io/zeptile".
	Registry         string      `json:"registry"`
	RegistryUser     string      `json:"-"`
	RegistryPassword string      `json:"-"`
	NPM              *npm.Client `json:"-"`
}

// ConfigFromEnv reads the deployer settings from DOCKTRINE_* variables.
func ConfigFromEnv() Config {
	cfg := Config{
		Registry:         strings.TrimRight(os.Getenv("DOCKTRINE_REGISTRY"), "/"),
		RegistryUser:     os.Getenv("DOCKTRINE_REGISTRY_USER"),
		RegistryPassword: os.Getenv("DOCKTRINE_REGISTRY_PASSWORD"),
	}

	if url := os.Getenv("DOCKTRINE_NPM_URL"); url != "" {
		cfg.NPM = npm.New(url, os.Getenv("DOCKTRINE_NPM_EMAIL"), os.Getenv("DOCKTRINE_NPM_PASSWORD"))
		cfg.NPM.LetsEncryptEmail = os.Getenv("DOCKTRINE_LETSENCRYPT_EMAIL")
		if id, err := strconv.ParseInt(os.Getenv("DOCKTRINE_NPM_CERTIFICATE_ID"), 10, 64); err == nil {
			cfg.NPM.CertificateID = id
		}
	}
	return cfg
}

type Deployer struct {
	db  *database.DB
	box *secrets.Box
	cfg Config
	mu  sync.Mutex
}

func New(db *database.DB, box *secrets.Box, cfg Config) *Deployer {
	return &Deployer{db: db, box: box, cfg: cfg}
}

func (d *Deployer) Config() Config {
	return d.cfg
}

// Start records a deployment and runs it in the background. Progress is
// appended to the deployment's log, which clients poll.
func (d *Deployer) Start(spec *manifest.Spec, createdBy string) (*database.Deployment, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if _, err := d.db.ResolveServer(spec.Node); err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	env, err := d.db.UpsertEnvironment(spec)
	if err != nil {
		return nil, err
	}
	active, err := d.db.HasActiveDeployment(env.ID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("a deployment of %s/%s is already in progress", spec.App, spec.Environment)
	}

	dep, err := d.db.CreateDeployment(env, spec, createdBy)
	if err != nil {
		return nil, err
	}

	go d.run(env, dep)
	return dep, nil
}

// Rollback redeploys the spec of an earlier deployment, by default the last
// successful one before the current deployment.
func (d *Deployer) Rollback(env *database.Environment, toID int64, createdBy string) (*database.Deployment, error) {
	var target *database.Deployment

	if toID != 0 {
		dep, err := d.db.GetDeployment(toID)
		if err != nil {
			return nil, err
		}
		if dep == nil || dep.EnvironmentID != env.ID {
			return nil, fmt.Errorf("deployment %d not found for %s/%s", toID, env.App, env.Name)
		}
		target = dep
	} else {
		history, err := d.db.ListDeployments(env.ID, 50)
		if err != nil {
			return nil, err
		}
		for i := range history {
			dep := &history[i]
			isCurrent := env.CurrentDeploymentID != nil && dep.ID == *env.CurrentDeploymentID
			if dep.Status == database.DeploymentSucceeded && !isCurrent {
				target = dep
				break
			}
		}
		if target == nil {
			return nil, fmt.Errorf("no earlier successful deployment to roll back to")
		}
	}

	return d.Start(target.Spec, createdBy+fmt.Sprintf(" (rollback to #%d)", target.ID))
}

type deployLog struct {
	db *database.DB
	id int64
}

func (l *deployLog) Printf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	logger.Info(fmt.Sprintf("deployment #%d: %s", l.id, line))
	if err := l.db.AppendDeploymentLog(l.id, line); err != nil {
		logger.Error(err, "Failed to append deployment log")
	}
}

func (d *Deployer) run(env *database.Environment, dep *database.Deployment) {
	log := &deployLog{db: d.db, id: dep.ID}
	spec := dep.Spec

	fail := func(err error) {
		log.Printf("FAILED: %v", err)
		if dbErr := d.db.SetDeploymentStatus(dep.ID, database.DeploymentFailed, err.Error()); dbErr != nil {
			logger.Error(dbErr, "Failed to update deployment status")
		}
	}

	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Errorf("internal error: %v", r))
		}
	}()

	if err := d.db.SetDeploymentStatus(dep.ID, database.DeploymentRunning, ""); err != nil {
		logger.Error(err, "Failed to update deployment status")
	}

	server, err := d.db.ResolveServer(spec.Node)
	if err != nil {
		fail(err)
		return
	}

	vars, err := d.environmentVariables(spec)
	if err != nil {
		fail(err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), deployTimeout)
	defer cancel()

	target := target{server: server, spec: spec, deploymentID: dep.ID, env: vars}
	log.Printf("Deploying %s/%s to %s", spec.App, spec.Environment, server.Name)

	if spec.Compose != "" {
		err = d.deployCompose(ctx, target, log)
	} else {
		err = d.deployImage(ctx, target, log)
	}
	if err != nil {
		fail(err)
		return
	}

	if err := d.db.SetCurrentDeployment(env.ID, dep.ID); err != nil {
		fail(err)
		return
	}

	message := ""
	if err := d.syncProxyHosts(env, spec, server, log); err != nil {
		message = "deployed, but hostnames were not configured: " + err.Error()
		log.Printf("WARNING: %s", message)
	}

	log.Printf("Deployment succeeded")
	if err := d.db.SetDeploymentStatus(dep.ID, database.DeploymentSucceeded, message); err != nil {
		logger.Error(err, "Failed to update deployment status")
	}
}

// target is everything a deploy strategy needs.
type target struct {
	server       *database.Server
	spec         *manifest.Spec
	deploymentID int64
	env          map[string]string
}

func (t target) project() string {
	return t.spec.App + "-" + t.spec.Environment
}

func (t target) labels() map[string]string {
	return map[string]string{
		LabelApp:        t.spec.App,
		LabelEnv:        t.spec.Environment,
		LabelDeployment: strconv.FormatInt(t.deploymentID, 10),
	}
}

// routingLabels tells the node ingress (Traefik) which hostnames go to a container.
func (t target) routingLabels() map[string]string {
	if len(t.spec.Hostnames) == 0 {
		return map[string]string{}
	}
	rules := []string{}
	for _, host := range t.spec.Hostnames {
		rules = append(rules, fmt.Sprintf("Host(`%s`)", host))
	}
	router := t.project()
	return map[string]string{
		"traefik.enable":                                                "true",
		"traefik.docker.network":                                        Network,
		"traefik.http.routers." + router + ".rule":                      strings.Join(rules, " || "),
		"traefik.http.routers." + router + ".entrypoints":               "web",
		"traefik.http.routers." + router + ".middlewares":               router + "-retry",
		"traefik.http.middlewares." + router + "-retry.retry.attempts":  "3",
		"traefik.http.services." + router + ".loadbalancer.server.port": strconv.Itoa(t.spec.Port),
	}
}

// secretScope is the environment name secrets are stored under. All preview
// instances of an app share the secrets set for "preview".
func secretScope(spec *manifest.Spec) string {
	if spec.Preview {
		return manifest.PreviewEnvironment
	}
	return spec.Environment
}

func (d *Deployer) environmentVariables(spec *manifest.Spec) (map[string]string, error) {
	values := map[string]string{}
	for k, v := range spec.Env {
		values[k] = v
	}
	if len(spec.Secrets) == 0 {
		return values, nil
	}

	stored, err := d.db.GetSecretValues(spec.App, secretScope(spec))
	if err != nil {
		return nil, err
	}

	missing := []string{}
	for _, key := range spec.Secrets {
		encrypted, ok := stored[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		value, err := d.box.Decrypt(encrypted)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing secrets for %s/%s: %s (set them with `docktrine secrets set`)",
			spec.App, secretScope(spec), strings.Join(missing, ", "))
	}
	return values, nil
}

// registryAuth returns the encoded credentials for images in the central registry.
func (d *Deployer) registryAuth(image string) string {
	host := d.registryHost()
	if host == "" || d.cfg.RegistryUser == "" || !strings.HasPrefix(image, host+"/") {
		return ""
	}
	auth, err := registry.EncodeAuthConfig(registry.AuthConfig{
		Username:      d.cfg.RegistryUser,
		Password:      d.cfg.RegistryPassword,
		ServerAddress: host,
	})
	if err != nil {
		return ""
	}
	return auth
}

func (d *Deployer) registryHost() string {
	host, _, _ := strings.Cut(d.cfg.Registry, "/")
	return host
}

// dockerConfigJSON is a docker CLI config.json holding the registry login,
// used when shelling out to docker compose.
func (d *Deployer) dockerConfigJSON() []byte {
	auths := map[string]any{}
	if host := d.registryHost(); host != "" && d.cfg.RegistryUser != "" {
		token := base64.StdEncoding.EncodeToString([]byte(d.cfg.RegistryUser + ":" + d.cfg.RegistryPassword))
		auths[host] = map[string]string{"auth": token}
	}
	data, _ := json.Marshal(map[string]any{"auths": auths})
	return data
}

// Destroy removes everything an environment runs: containers, its NPM hosts,
// and with purge also its volumes. The environment record is deleted.
func (d *Deployer) Destroy(env *database.Environment, purge bool) error {
	server, err := d.db.ResolveServer(env.Server)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := d.removeWorkload(ctx, server, env.App, env.Name, purge); err != nil {
		return err
	}
	if err := d.removeProxyHosts(env); err != nil {
		return err
	}
	return d.db.DeleteEnvironment(env.ID)
}

// StartJanitor periodically destroys preview environments past their TTL.
func (d *Deployer) StartJanitor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			d.reapPreviews()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (d *Deployer) reapPreviews() {
	expired, err := d.db.ListExpiredPreviews(time.Now())
	if err != nil {
		logger.Error(err, "Failed to list expired previews")
		return
	}
	for i := range expired {
		env := &expired[i]
		active, err := d.db.HasActiveDeployment(env.ID)
		if err != nil || active {
			continue
		}
		logger.Info(fmt.Sprintf("Preview %s/%s expired, destroying", env.App, env.Name))
		if err := d.Destroy(env, true); err != nil {
			logger.Error(err, fmt.Sprintf("Failed to destroy preview %s/%s", env.App, env.Name))
		}
	}
}
