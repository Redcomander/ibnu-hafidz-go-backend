package handlers

import (
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/maintenance"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type MaintenanceHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewMaintenanceHandler(db *gorm.DB, cfg *config.Config) *MaintenanceHandler {
	return &MaintenanceHandler{db: db, cfg: cfg}
}

func (h *MaintenanceHandler) Get(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	settings, err := maintenance.Read(h.db)
	if err != nil {
		return fiber.NewError(503, "Maintenance settings unavailable")
	}
	return c.JSON(fiber.Map{"enabled": settings.Enabled, "scope": settings.Scope, "message": settings.Message, "can_bypass": middleware.MaintenanceAdmin(c, h.db, h.cfg)})
}

func (h *MaintenanceHandler) Update(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(*models.User)
	if !maintenance.IsAdmin(user) {
		return fiber.NewError(403, "Only administrators can change maintenance mode")
	}
	var input struct {
		Enabled *bool  `json:"enabled"`
		Scope   string `json:"scope"`
		Message string `json:"message"`
	}
	if err := c.BodyParser(&input); err != nil || input.Enabled == nil {
		return fiber.NewError(400, "enabled must be a boolean")
	}
	if input.Scope != "all" && input.Scope != "public" {
		return fiber.NewError(400, "scope must be all or public")
	}
	input.Message = strings.TrimSpace(input.Message)
	if utf8.RuneCountInString(input.Message) > 500 {
		return fiber.NewError(400, "Message must not exceed 500 characters")
	}
	if input.Message == "" {
		input.Message = maintenance.DefaultMessage
	}
	settings := maintenance.Settings{Enabled: *input.Enabled, Scope: input.Scope, Message: input.Message}
	if err := maintenance.Save(h.db, settings); err != nil {
		return fiber.NewError(500, "Failed to save maintenance mode")
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"enabled": settings.Enabled, "scope": settings.Scope, "message": settings.Message, "can_bypass": true})
}
