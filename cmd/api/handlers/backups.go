package handlers

import (
	"fmt"
	"strconv"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// backupParam finds the backup in the path, of the environment in the path.
func (h *Handler) backupParam(c *fiber.Ctx) (*database.Backup, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return nil, c.Status(400).JSON(fiber.Map{"error": "invalid backup id"})
	}
	b, err := h.db.GetBackup(id)
	if err != nil {
		return nil, c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if b == nil || b.App != c.Params("app") || b.Environment != c.Params("env") {
		return nil, c.Status(404).JSON(fiber.Map{"error": "backup not found"})
	}
	return b, nil
}

// ListBackups godoc
// @Summary List an environment's backups
// @Description Backups of its volumes and stack directory, newest first, with where they are kept and how many are kept. Backups outlive a destroyed environment, so this works for one that no longer exists.
// @Tags backups
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 200 {object} interface{}
// @Router /apps/{app}/{env}/backups [get]
func (h *Handler) ListBackups(c *fiber.Ctx) error {
	app, env := c.Params("app"), c.Params("env")
	backups, err := h.db.ListBackups(app, env)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"backups": backups,
		"dir":     deploy.BackupsDir() + "/" + app + "-" + env,
		"keep":    deploy.DefaultBackupsKept(),
		"busy":    h.deployer.Busy(app, env),
	})
}

// CreateBackup godoc
// @Summary Back up an environment now
// @Description Start a backup of the environment's volumes, and its stack directory for stacks from Git. Its containers stop while it runs. Poll the list for its status.
// @Tags backups
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Success 202 {object} database.Backup
// @Failure 409 {object} interface{}
// @Router /apps/{app}/{env}/backups [post]
func (h *Handler) CreateBackup(c *fiber.Ctx) error {
	env, err := h.environmentParam(c)
	if env == nil {
		return err
	}
	b, err := h.deployer.BackupNow(env, createdBy(c))
	if err != nil {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(fmt.Sprintf("Backing up %s/%s as backup #%d", env.App, env.Name, b.ID))
	return c.Status(202).JSON(b)
}

// RestoreBackup godoc
// @Summary Restore a backup
// @Description Stop the environment and replace its volumes, and its stack directory, with the backup's. With redeploy, deploy again what ran when the backup was taken, so code and data match; otherwise its containers start again on the restored data.
// @Tags backups
// @Accept json
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Param id path int true "Backup ID"
// @Success 200 {object} deploy.RestoreResult
// @Failure 409 {object} interface{}
// @Router /apps/{app}/{env}/backups/{id}/restore [post]
func (h *Handler) RestoreBackup(c *fiber.Ctx) error {
	b, err := h.backupParam(c)
	if b == nil {
		return err
	}
	var body struct {
		Redeploy bool `json:"redeploy"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
	}
	res, err := h.deployer.Restore(b, body.Redeploy, createdBy(c))
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to restore backup #%d of %s/%s", b.ID, b.App, b.Environment))
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(res.Message)
	return c.JSON(res)
}

// DeleteBackup godoc
// @Summary Delete a backup
// @Description Delete a backup's files from its node, and its record
// @Tags backups
// @Produce json
// @Param app path string true "App name"
// @Param env path string true "Environment name"
// @Param id path int true "Backup ID"
// @Success 200 {object} interface{}
// @Router /apps/{app}/{env}/backups/{id} [delete]
func (h *Handler) DeleteBackup(c *fiber.Ctx) error {
	b, err := h.backupParam(c)
	if b == nil {
		return err
	}
	if err := h.deployer.DeleteBackup(b); err != nil {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": fmt.Sprintf("backup #%d deleted", b.ID)})
}
