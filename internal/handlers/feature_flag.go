package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type FeatureFlagHandler struct {
	db *gorm.DB
}

func NewFeatureFlagHandler(db *gorm.DB) *FeatureFlagHandler {
	return &FeatureFlagHandler{db: db}
}

func (h *FeatureFlagHandler) GetRamadhan(c *fiber.Ctx) error {
	var flag models.FeatureFlag
	if err := h.db.Where("`key` = ?", "ramadhan_mode").First(&flag).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			flag = models.FeatureFlag{Key: "ramadhan_mode", IsEnabled: false}
			h.db.Create(&flag)
			return c.JSON(flag)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to load feature flag"})
	}
	return c.JSON(flag)
}

func (h *FeatureFlagHandler) SetRamadhan(c *fiber.Ctx) error {
	type reqBody struct {
		IsEnabled bool `json:"is_enabled"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}

	var flag models.FeatureFlag
	err := h.db.Where("`key` = ?", "ramadhan_mode").First(&flag).Error
	if err == gorm.ErrRecordNotFound {
		flag = models.FeatureFlag{Key: "ramadhan_mode", IsEnabled: req.IsEnabled}
		if err := h.db.Create(&flag).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to create feature flag"})
		}
		return c.JSON(flag)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update feature flag"})
	}

	flag.IsEnabled = req.IsEnabled
	if err := h.db.Save(&flag).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update feature flag"})
	}
	return c.JSON(flag)
}
