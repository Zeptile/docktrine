package handlers

import (
	"context"
	"time"

	"github.com/Zeptile/docktrine/internal/catalog"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/gofiber/fiber/v2"
)

// InspectStack godoc
// @Summary Inspect a compose stack
// @Description Read a compose stack, from a public Git repository or pasted compose text, to fill in the launch form: its services, ports and the variables it reads
// @Tags launch
// @Accept json
// @Produce json
// @Success 200 {object} deploy.StackInfo
// @Failure 422 {object} interface{}
// @Router /stacks/inspect [post]
func (h *Handler) InspectStack(c *fiber.Ctx) error {
	var body struct {
		Source  *manifest.GitSource `json:"source"`
		Compose string              `json:"compose"`
	}
	if err := c.BodyParser(&body); err != nil || (body.Source == nil) == (body.Compose == "") {
		return c.Status(400).JSON(fiber.Map{"error": "body must have either source or compose"})
	}
	if body.Source != nil {
		// Inspecting always looks at the latest of the ref
		body.Source.Commit = ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	info, err := deploy.InspectStack(ctx, body.Source, body.Compose)
	if err != nil {
		return c.Status(422).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}

// InspectImage godoc
// @Summary Inspect an image
// @Description Read the ports and volumes an image declares, from its registry, without pulling it
// @Tags launch
// @Produce json
// @Param image query string true "Image reference, e.g. postgres:16"
// @Success 200 {object} deploy.ImageInfo
// @Failure 422 {object} interface{}
// @Router /images/inspect [get]
func (h *Handler) InspectImage(c *fiber.Ctx) error {
	ref := c.Query("image")
	if ref == "" {
		return c.Status(400).JSON(fiber.Map{"error": "image is required"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := h.deployer.InspectImage(ctx, ref)
	if err != nil {
		return c.Status(422).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}

// Preflight godoc
// @Summary Check a launch before it starts
// @Description Check that a node can run what is about to launch: memory, CPUs and Docker version against the stack's requirements, free ports, and an ingress for its hostnames
// @Tags launch
// @Accept json
// @Produce json
// @Param body body deploy.PreflightRequest true "What is about to launch"
// @Success 200 {object} deploy.PreflightResult
// @Failure 404 {object} interface{}
// @Router /launch/preflight [post]
func (h *Handler) Preflight(c *fiber.Ctx) error {
	var req deploy.PreflightRequest
	if err := c.BodyParser(&req); err != nil || req.Node == "" {
		return c.Status(400).JSON(fiber.Map{"error": "body must name a node"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := h.deployer.Preflight(ctx, req)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// ListCatalog godoc
// @Summary List known apps
// @Description The catalog of presets for the launch form: popular self-hosted apps, pinned to tested versions, with the variables and settings they need
// @Tags launch
// @Produce json
// @Success 200 {array} catalog.Preset
// @Failure 500 {object} interface{}
// @Router /catalog [get]
func (h *Handler) ListCatalog(c *fiber.Ctx) error {
	presets, err := catalog.All()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(presets)
}
