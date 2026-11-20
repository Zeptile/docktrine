package handlers

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Zeptile/docktrine/cmd/api/middleware"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

type apiKeyResponse struct {
	database.APIKey
	// Current marks the key that made this request
	Current bool `json:"current"`
}

type apiKeyNameRequest struct {
	Name string `json:"name"`
}

// ownKeysOnly is the user whose keys a non-admin may see and change, or
// nil for admins, who manage every key.
func ownKeysOnly(c *fiber.Ctx) *int64 {
	p := middleware.GetPrincipal(c)
	if database.RoleAtLeast(p.Role(), database.RoleAdmin) {
		return nil
	}
	return p.UserID()
}

// keyParam loads the key in the path, if the principal may manage it.
func (h *Handler) keyParam(c *fiber.Ctx) (*database.APIKey, error) {
	id, err := keyIDParam(c)
	if id == 0 {
		return nil, err
	}
	key, err := h.db.GetAPIKey(id)
	if err != nil {
		return nil, c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if own := ownKeysOnly(c); key == nil || (own != nil && (key.UserID == nil || *key.UserID != *own)) {
		return nil, c.Status(404).JSON(fiber.Map{"error": "API key not found"})
	}
	return key, nil
}

func keyIDParam(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, c.Status(400).JSON(fiber.Map{"error": "invalid API key id"})
	}
	return id, nil
}

// ListAPIKeys godoc
// @Summary List API keys
// @Description Names and prefixes only; keys are never returned after creation. Admins see every key, others their own.
// @Tags keys
// @Produce json
// @Success 200 {array} apiKeyResponse
// @Router /keys [get]
func (h *Handler) ListAPIKeys(c *fiber.Ctx) error {
	keys, err := h.db.ListAPIKeys(ownKeysOnly(c))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var currentID int64
	if p := middleware.GetPrincipal(c); p.Key != nil {
		currentID = p.Key.ID
	}
	result := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		result[i] = apiKeyResponse{APIKey: k, Current: k.ID == currentID}
	}
	return c.JSON(result)
}

// CreateAPIKey godoc
// @Summary Create an API key
// @Description The key is in the response only; store it right away. It acts with its creator's role.
// @Tags keys
// @Accept json
// @Produce json
// @Param body body apiKeyNameRequest true "Who or what uses the key, e.g. alex-laptop or ci"
// @Success 201 {object} database.NewAPIKey
// @Failure 409 {object} interface{}
// @Router /keys [post]
func (h *Handler) CreateAPIKey(c *fiber.Ctx) error {
	var body apiKeyNameRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	key, err := h.db.CreateAPIKey(body.Name, middleware.GetPrincipal(c).UserID())
	if errors.Is(err, database.ErrAPIKeyNameTaken) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(fmt.Sprintf("API key %q (%s) created by %s", key.Name, key.Prefix, createdBy(c)))
	return c.Status(201).JSON(key)
}

// RenameAPIKey godoc
// @Summary Rename an API key
// @Description Deployments already made keep the old name in their history
// @Tags keys
// @Accept json
// @Produce json
// @Param id path int true "API key id"
// @Param body body apiKeyNameRequest true "New name"
// @Success 200 {object} interface{}
// @Router /keys/{id} [patch]
func (h *Handler) RenameAPIKey(c *fiber.Ctx) error {
	key, err := h.keyParam(c)
	if key == nil {
		return err
	}
	id := key.ID
	var body apiKeyNameRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	found, err := h.db.RenameAPIKey(id, body.Name)
	if errors.Is(err, database.ErrAPIKeyNameTaken) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if !found {
		return c.Status(404).JSON(fiber.Map{"error": "API key not found"})
	}
	return c.JSON(fiber.Map{"message": "API key renamed"})
}

// RevokeAPIKey godoc
// @Summary Revoke an API key
// @Description Requests with the key fail from now on. While nobody can sign in with a password, the last key can't be revoked.
// @Tags keys
// @Produce json
// @Param id path int true "API key id"
// @Success 200 {object} interface{}
// @Failure 409 {object} interface{}
// @Router /keys/{id} [delete]
func (h *Handler) RevokeAPIKey(c *fiber.Ctx) error {
	key, err := h.keyParam(c)
	if key == nil {
		return err
	}
	id := key.ID

	found, err := h.db.RevokeAPIKey(id)
	if errors.Is(err, database.ErrLastAPIKey) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !found {
		return c.Status(404).JSON(fiber.Map{"error": "API key not found"})
	}
	logger.Info(fmt.Sprintf("API key %d revoked by %s", id, createdBy(c)))
	return c.JSON(fiber.Map{"message": "API key revoked"})
}
