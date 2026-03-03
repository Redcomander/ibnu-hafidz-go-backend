package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type RoleHandler struct {
	db *gorm.DB
}

func NewRoleHandler(db *gorm.DB) *RoleHandler {
	return &RoleHandler{db: db}
}

// List returns all roles with their permissions
func (h *RoleHandler) List(c *fiber.Ctx) error {
	var roles []models.Role
	h.db.Preload("Permissions").
		Select("roles.*, (SELECT count(*) FROM role_user JOIN users ON users.id = role_user.user_id WHERE role_user.role_id = roles.id AND users.deleted_at IS NULL) as users_count").
		Order("name asc").
		Find(&roles)

	// Also return all available permissions grouped by category
	var permissions []models.Permission
	h.db.Order("category asc, display_name asc").Find(&permissions)

	return c.JSON(fiber.Map{
		"roles":       roles,
		"permissions": permissions,
	})
}

// Get returns a single role by ID
func (h *RoleHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var role models.Role

	if err := h.db.Preload("Permissions").First(&role, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Role not found",
		})
	}

	return c.JSON(role)
}

// Create adds a new role with permissions
func (h *RoleHandler) Create(c *fiber.Ctx) error {
	type CreateRoleRequest struct {
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		Description   string `json:"description"`
		Color         string `json:"color"`
		IsSystemRole  bool   `json:"is_system_role"`
		PermissionIDs []uint `json:"permission_ids"`
	}

	var req CreateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Role name is required",
		})
	}

	role := models.Role{
		Name:         req.Name,
		DisplayName:  &req.DisplayName,
		Description:  &req.Description,
		Color:        &req.Color,
		IsSystemRole: req.IsSystemRole,
	}

	if err := h.db.Create(&role).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create role",
		})
	}

	// Assign permissions
	if len(req.PermissionIDs) > 0 {
		var permissions []models.Permission
		h.db.Where("id IN ?", req.PermissionIDs).Find(&permissions)
		h.db.Model(&role).Association("Permissions").Replace(permissions)
	}

	h.db.Preload("Permissions").First(&role, role.ID)
	return c.Status(fiber.StatusCreated).JSON(role)
}

// Update modifies a role and its permissions
func (h *RoleHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var role models.Role

	if err := h.db.First(&role, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Role not found",
		})
	}

	type UpdateRoleRequest struct {
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		Description   string `json:"description"`
		Color         string `json:"color"`
		IsSystemRole  bool   `json:"is_system_role"`
		PermissionIDs []uint `json:"permission_ids"`
	}

	var req UpdateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	if req.DisplayName != "" {
		role.DisplayName = &req.DisplayName
	}
	if req.Description != "" {
		role.Description = &req.Description
	}
	if req.Color != "" {
		role.Color = &req.Color
	}
	role.IsSystemRole = req.IsSystemRole

	h.db.Save(&role)

	// Update permissions
	if len(req.PermissionIDs) > 0 {
		var permissions []models.Permission
		h.db.Where("id IN ?", req.PermissionIDs).Find(&permissions)
		h.db.Model(&role).Association("Permissions").Replace(permissions)
	}

	h.db.Preload("Permissions").First(&role, role.ID)
	return c.JSON(role)
}

// Delete removes a role
func (h *RoleHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var role models.Role

	if err := h.db.First(&role, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Role not found",
		})
	}

	// Remove permission associations first
	h.db.Model(&role).Association("Permissions").Clear()
	// Remove user associations
	h.db.Model(&role).Association("Users").Clear()
	// Delete role
	h.db.Delete(&role)

	return c.JSON(fiber.Map{"message": "Role deleted successfully"})
}
