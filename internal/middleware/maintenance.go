package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/maintenance"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

// MaintenanceAdmin checks a signed token and CURRENT database roles, never a
// client-provided bypass header/cookie or an unverified JWT role claim.
func MaintenanceAdmin(c *fiber.Ctx, db *gorm.DB, cfg *config.Config) bool {
	token := extractTokenFromRequest(c)
	if token == "" {
		return false
	}
	claims, err := utils.ValidateToken(token, cfg.JWTSecret)
	if err != nil {
		return false
	}
	var user models.User
	if err := db.Preload("Roles").First(&user, claims.UserID).Error; err != nil {
		return false
	}
	return maintenance.IsAdmin(&user)
}

func Maintenance(db *gorm.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := strings.TrimRight(c.Path(), "/")
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		// Keep session recovery and the authenticated control endpoint reachable.
		switch path {
		case "/api/maintenance", "/api/auth/login", "/api/auth/refresh", "/api/auth/logout", "/api/auth/me":
			return c.Next()
		}
		settings, err := maintenance.Read(db)
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "service_unavailable", "message": "Maintenance settings unavailable"})
		}
		isPublic := path == "/api/public" || strings.HasPrefix(path, "/api/public/")
		if !settings.Enabled || (settings.Scope == "public" && !isPublic) || MaintenanceAdmin(c, db, cfg) {
			return c.Next()
		}
		c.Set("Retry-After", "60")
		c.Set("Cache-Control", "no-store")
		return c.Status(503).JSON(fiber.Map{"error": "maintenance", "message": settings.Message, "scope": settings.Scope})
	}
}
