package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CmsHandler struct {
	db *gorm.DB
}

func NewCmsHandler(db *gorm.DB) *CmsHandler {
	return &CmsHandler{db: db}
}

// List returns all CMS settings (key + value).
func (h *CmsHandler) List(c *fiber.Ctx) error {
	var settings []models.CmsSetting
	h.db.Order("key asc").Find(&settings)
	return c.JSON(fiber.Map{
		"data": settings,
	})
}

// Get returns a single CMS setting by key.
func (h *CmsHandler) Get(c *fiber.Ctx) error {
	key := c.Params("key")
	var setting models.CmsSetting
	if err := h.db.Where("`key` = ?", key).First(&setting).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "CMS setting not found",
		})
	}
	return c.JSON(fiber.Map{"data": setting})
}

// Upsert creates or updates a CMS setting by key.
func (h *CmsHandler) Upsert(c *fiber.Ctx) error {
	key := c.Params("key")

	var body struct {
		Value string `json:"value"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "invalid_body",
			Message: "Invalid request body",
		})
	}
	if body.Value == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "value is required",
		})
	}

	setting := models.CmsSetting{
		Key:   key,
		Value: body.Value,
	}

	// ON DUPLICATE KEY UPDATE value = VALUES(value)
	result := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&setting)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "db_error",
			Message: "Failed to save CMS setting",
		})
	}

	return c.JSON(fiber.Map{
		"message": "CMS setting saved",
		"data":    setting,
	})
}

// Delete removes a CMS setting, restoring the hardcoded default.
func (h *CmsHandler) Delete(c *fiber.Ctx) error {
	key := c.Params("key")
	h.db.Where("`key` = ?", key).Delete(&models.CmsSetting{})
	return c.JSON(fiber.Map{"message": "CMS setting deleted — hardcoded default restored"})
}
