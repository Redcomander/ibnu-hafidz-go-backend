package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type PermissionHandler struct {
	db *gorm.DB
}

func NewPermissionHandler(db *gorm.DB) *PermissionHandler {
	return &PermissionHandler{db: db}
}

// Create adds a new permission
func (h *PermissionHandler) Create(c *fiber.Ctx) error {
	if user, ok := c.Locals("user").(*models.User); ok && user != nil {
		if !(user.HasRole("super_admin") || user.HasRole("admin") || user.HasPermission("permissions.create")) {
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "You do not have permission to access this resource",
			})
		}
	}

	type CreatePermissionRequest struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}

	var req CreatePermissionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Permission name is required",
		})
	}

	perm := models.Permission{
		Name:        req.Name,
		DisplayName: &req.DisplayName,
		Description: &req.Description,
		Category:    &req.Category,
	}

	// Check if exists
	var existing models.Permission
	if err := h.db.Where("name = ?", req.Name).First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{
			Error:   "conflict",
			Message: "Permission with this name already exists",
		})
	}

	if err := h.db.Create(&perm).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create permission",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(perm)
}
