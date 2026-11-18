package handlers

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/gofiber/fiber/v2"
)

func createdBy(c *fiber.Ctx) string {
	if desc, ok := c.Locals("api_key_name").(string); ok && desc != "" {
		return desc
	}
	return "api"
}

func (h *Handler) environmentParam(c *fiber.Ctx) (*database.Environment, error) {
	env, err := h.db.GetEnvironment(c.Params("app"), c.Params("env"))
	if err != nil {
		return nil, c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if env == nil {
		return nil, c.Status(404).JSON(fiber.Map{
			"error": fmt.Sprintf("environment %s/%s not found", c.Params("app"), c.Params("env")),
		})
	}
	return env, nil
}

// GetConfig godoc
// @Summary Get deployer configuration
// @Description Settings the CLI needs to build and push images
// @Tags deployments
// @Produce json
// @Success 200 {object} interface{}
// @Router /config [get]
func (h *Handler) GetConfig(c *fiber.Ctx) error {
	cfg := h.deployer.Config()
	return c.JSON(fiber.Map{
		"registry":       cfg.Registry,
		"npm_configured": cfg.NPM != nil,
	})
}

// CreateDeployment godoc
// @Summary Deploy an environment
// @Description Start a deployment from a resolved spec. Poll GET /deployments/{id} for progress.
// @Tags deployments
// @Accept json
// @Produce json
// @Param spec body manifest.Spec true "Resolved environment spec"
// @Success 202 {object} database.Deployment
// @Failure 400 {object} interface{}
// @Failure 409 {object} interface{}
// @Router /deployments [post]
func (h *Handler) CreateDeployment(c *fiber.Ctx) error {
	var spec manifest.Spec
	if err := c.BodyParser(&spec); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	dep, err := h.deployer.Start(&spec, createdBy(c))
	if err != nil {
		logger.Warn(fmt.Sprintf("Deployment of %s/%s rejected: %v", spec.App, spec.Environment, err))
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(202).JSON(dep)
}

// ListDeployments godoc
// @Summary List recent deployments
// @Tags deployments
// @Produce json
// @Param limit query int false "Max results (default 50)"
// @Success 200 {array} database.Deployment
// @Router /deployments [get]
func (h *Handler) ListDeployments(c *fiber.Ctx) error {
	deployments, err := h.db.ListDeployments(0, c.QueryInt("limit", 50))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(deployments)
}

// GetDeployment godoc
// @Summary Get a deployment and its log
// @Tags deployments
// @Produce json
// @Param id path int true "Deployment ID"
// @Success 200 {object} database.Deployment
// @Failure 404 {object} interface{}
// @Router /deployments/{id} [get]
func (h *Handler) GetDeployment(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid deployment id"})
	}
	dep, err := h.db.GetDeployment(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if dep == nil {
		return c.Status(404).JSON(fiber.Map{"error": "deployment not found"})
	}
	return c.JSON(dep)
}

// ListApps godoc
// @Summary List app environments
// @Description Every deployed environment with its latest deployment
// @Tags apps
// @Produce json
// @Success 200 {array} interface{}
// @Router /apps [get]
func (h *Handler) ListApps(c *fiber.Ctx) error {
	envs, err := h.db.ListEnvironments()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	result := []fiber.Map{}
	for i := range envs {
		env := &envs[i]
		latest, err := h.db.ListDeployments(env.ID, 1)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		item := fiber.Map{"environment": env}
		if len(latest) > 0 {
			item["latest_deployment"] = latest[0]
		}
		if env.CurrentDeploymentID != nil {
			if current, err := h.db.GetDeployment(*env.CurrentDeploymentID); err == nil && current != nil {
				current.Log = ""
				item["current_deployment"] = current
			}
		}
		result = append(result, item)
	}
	return c.JSON(result)
}

// GetAppEnvironment godoc
// @Summary Get an environment
// @Description Environment details, live containers and deployment history
// @Tags apps
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 200 {object} interface{}
// @Failure 404 {object} interface{}
// @Router /apps/{app}/{env} [get]
func (h *Handler) GetAppEnvironment(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}

	deployments, err := h.db.ListDeployments(env.ID, 25)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	hosts, err := h.db.ListProxyHosts(env.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	domains := []string{}
	for _, host := range hosts {
		domains = append(domains, host.Domain)
	}

	result := fiber.Map{
		"environment": env,
		"deployments": deployments,
		"hostnames":   domains,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	containers, err := h.deployer.Containers(ctx, env)
	if err != nil {
		result["containers_error"] = err.Error()
	} else {
		result["containers"] = containers
	}
	return c.JSON(result)
}

// GetAppLogs godoc
// @Summary Stream environment logs
// @Description Plain-text logs of every container of the environment
// @Tags apps
// @Produce plain
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Param tail query int false "Lines of history (default 200)"
// @Param follow query bool false "Keep streaming new lines"
// @Router /apps/{app}/{env}/logs [get]
func (h *Handler) GetAppLogs(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	tail := c.QueryInt("tail", 200)
	follow := c.QueryBool("follow", false)

	c.Set("Content-Type", "text/plain; charset=utf-8")
	c.Set("Cache-Control", "no-cache")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := &cancelOnError{w: w, cancel: cancel}
		if err := h.deployer.Logs(ctx, env, tail, follow, out); err != nil && ctx.Err() == nil {
			fmt.Fprintf(w, "error: %v\n", err)
			w.Flush()
		}
	})
	return nil
}

// cancelOnError stops log streaming once the client has gone away.
type cancelOnError struct {
	w      *bufio.Writer
	cancel context.CancelFunc
}

func (c *cancelOnError) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *cancelOnError) Flush() error {
	err := c.w.Flush()
	if err != nil {
		c.cancel()
	}
	return err
}

// RollbackEnvironment godoc
// @Summary Roll back an environment
// @Description Redeploy an earlier deployment (default: the previous successful one)
// @Tags apps
// @Accept json
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 202 {object} database.Deployment
// @Router /apps/{app}/{env}/rollback [post]
func (h *Handler) RollbackEnvironment(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	var body struct {
		DeploymentID int64 `json:"deployment_id"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
	}

	dep, err := h.deployer.Rollback(env, body.DeploymentID, createdBy(c))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(202).JSON(dep)
}

// DestroyEnvironment godoc
// @Summary Destroy an environment
// @Description Remove its containers and hostnames; purge=true also deletes volumes
// @Tags apps
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Param purge query bool false "Also delete volumes"
// @Success 200 {object} interface{}
// @Router /apps/{app}/{env} [delete]
func (h *Handler) DestroyEnvironment(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	active, err := h.db.HasActiveDeployment(env.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if active {
		return c.Status(409).JSON(fiber.Map{"error": "a deployment is in progress"})
	}

	if err := h.deployer.Destroy(env, c.QueryBool("purge", false)); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to destroy %s/%s", env.App, env.Name))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(fmt.Sprintf("Destroyed %s/%s", env.App, env.Name))
	return c.JSON(fiber.Map{"message": fmt.Sprintf("%s/%s destroyed", env.App, env.Name)})
}

var secretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ListSecrets godoc
// @Summary List secret names
// @Description Names only; values are never returned
// @Tags secrets
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name (use 'preview' for previews)"
// @Success 200 {array} database.SecretInfo
// @Router /apps/{app}/{env}/secrets [get]
func (h *Handler) ListSecrets(c *fiber.Ctx) error {
	list, err := h.db.ListSecrets(c.Params("app"), c.Params("env"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

// SetSecrets godoc
// @Summary Set secrets
// @Description Create or replace secret values; applied on the next deploy
// @Tags secrets
// @Accept json
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name (use 'preview' for previews)"
// @Success 200 {object} interface{}
// @Router /apps/{app}/{env}/secrets [put]
func (h *Handler) SetSecrets(c *fiber.Ctx) error {
	var body struct {
		Values map[string]string `json:"values"`
	}
	if err := c.BodyParser(&body); err != nil || len(body.Values) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "body must be {\"values\": {\"KEY\": \"value\"}}"})
	}

	for key, value := range body.Values {
		if !secretKeyPattern.MatchString(key) {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("invalid secret name %q", key)})
		}
		encrypted, err := h.box.Encrypt(value)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if err := h.db.SetSecret(c.Params("app"), c.Params("env"), key, encrypted); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"message": fmt.Sprintf("%d secret(s) saved, redeploy to apply", len(body.Values))})
}

// DeleteSecret godoc
// @Summary Delete a secret
// @Tags secrets
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Param key path string true "Secret name"
// @Success 200 {object} interface{}
// @Router /apps/{app}/{env}/secrets/{key} [delete]
func (h *Handler) DeleteSecret(c *fiber.Ctx) error {
	deleted, err := h.db.DeleteSecret(c.Params("app"), c.Params("env"), c.Params("key"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !deleted {
		return c.Status(404).JSON(fiber.Map{"error": "secret not found"})
	}
	return c.JSON(fiber.Map{"message": "secret deleted"})
}
