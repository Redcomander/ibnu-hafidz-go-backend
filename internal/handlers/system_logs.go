package handlers

import (
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/diagnostics"
	"github.com/ibnu-hafidz/web-v2/internal/models"
)

type SystemLogHandler struct{ store *diagnostics.Store }

func NewSystemLogHandler(store *diagnostics.Store) *SystemLogHandler {
	return &SystemLogHandler{store: store}
}

func SuperAdminLogs(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(*models.User)
	if user == nil || !user.HasRole("super_admin") {
		return fiber.NewError(403, "Only super administrators can manage production diagnostics")
	}
	return c.Next()
}

func (h *SystemLogHandler) List(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		return fiber.NewError(400, "Invalid page")
	}
	if page > 1000000 {
		return fiber.NewError(400, "Invalid page")
	}
	if severity := c.Query("severity"); severity != "" && severity != "error" && severity != "warning" {
		return fiber.NewError(400, "Invalid severity")
	}
	perPage := 20
	status := 0
	if c.Query("status") != "" {
		status, err = strconv.Atoi(c.Query("status"))
		if err != nil || status < 100 || status > 599 {
			return fiber.NewError(400, "Invalid status")
		}
	}
	resolved := c.Query("resolved")
	if resolved != "" && resolved != "true" && resolved != "false" {
		return fiber.NewError(400, "Invalid resolved filter")
	}
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	events := h.store.Snapshot()
	matched := []diagnostics.Event{}
	errors, warnings, unresolved := 0, 0, 0
	for _, event := range events {
		if event.Severity == "error" {
			errors++
		} else {
			warnings++
		}
		if !event.Resolved {
			unresolved++
		}
		if status != 0 && event.Status != status {
			continue
		}
		if resolved != "" && event.Resolved != (resolved == "true") {
			continue
		}
		if c.Query("severity") != "" && c.Query("severity") != event.Severity {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(event.RequestID+" "+event.Route+" "+event.Fingerprint+" "+event.Handler), search) {
			continue
		}
		matched = append(matched, event)
	}
	total := len(matched)
	start := (page - 1) * perPage
	if start < 0 || start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return c.JSON(fiber.Map{"data": matched[start:end], "total": total, "page": page, "total_pages": (total + perPage - 1) / perPage,
		"summary": fiber.Map{"errors": errors, "warnings": warnings, "unresolved": unresolved}, "settings": h.store.Settings()})
}

func (h *SystemLogHandler) Resolve(c *fiber.Ctx) error {
	var body struct {
		Resolved *bool `json:"resolved"`
	}
	if c.BodyParser(&body) != nil || body.Resolved == nil {
		return fiber.NewError(400, "resolved must be a boolean")
	}
	actor, _ := c.Locals("userID").(uint)
	if err := h.store.Resolve(c.Params("id"), *body.Resolved, actor); err != nil {
		if os.IsNotExist(err) {
			return fiber.NewError(404, "Diagnostic event not found")
		}
		return fiber.NewError(503, "Diagnostic storage unavailable")
	}
	return c.JSON(fiber.Map{"resolved": *body.Resolved})
}

func (h *SystemLogHandler) Debug(c *fiber.Ctx) error {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if c.BodyParser(&body) != nil || body.Enabled == nil {
		return fiber.NewError(400, "enabled must be a boolean")
	}
	if h.store.SetDebug(*body.Enabled) != nil {
		return fiber.NewError(503, "Diagnostic storage unavailable")
	}
	return c.JSON(h.store.Settings())
}
