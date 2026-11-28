package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// updateNotice returns what to show about updates for env, or nil.
func (h *Handler) updateNotice(env *database.Environment, current *database.Deployment) *deploy.UpdateInfo {
	if current == nil || !deploy.UpdatesCheckable(env, current.Spec) {
		return nil
	}
	check, err := h.db.GetUpdateCheck(env.ID)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to read the update check of %s/%s: %v", env.App, env.Name, err))
		return nil
	}
	return deploy.UpdateNotice(env, check, current.Spec)
}

// CheckUpdates godoc
// @Summary Check an environment for updates
// @Description Look for a newer commit on the Git ref of a stack, and newer images behind the tags its containers run, now rather than at the next background check
// @Tags apps
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 200 {object} deploy.UpdateInfo
// @Failure 422 {object} interface{}
// @Router /apps/{app}/{env}/update/check [post]
func (h *Handler) CheckUpdates(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	check, err := h.deployer.CheckUpdates(ctx, env)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if check == nil {
		return c.Status(422).JSON(fiber.Map{"error": fmt.Sprintf("%s/%s runs nothing that can be checked for updates", env.App, env.Name)})
	}
	current, err := h.db.GetDeployment(check.DeploymentID)
	if err != nil || current == nil {
		return c.Status(500).JSON(fiber.Map{"error": "the running deployment went away"})
	}
	return c.JSON(deploy.UpdateNotice(env, check, current.Spec))
}

// UpdateEnvironment godoc
// @Summary Update an environment
// @Description Deploy the running spec again at the newest version: the head of its Git ref, and its images pulled again by tag. A stack's setup command runs again when the commit changes.
// @Tags apps
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 202 {object} database.Deployment
// @Failure 400 {object} interface{}
// @Router /apps/{app}/{env}/update [post]
func (h *Handler) UpdateEnvironment(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	dep, err := h.deployer.Update(env, createdBy(c))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(202).JSON(dep)
}
