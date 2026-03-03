package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

// Auth validates JWT access token from Authorization header
func Auth(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get token from Authorization header or Query param (for SSE)
		authHeader := c.Get("Authorization")
		tokenString := ""

		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				tokenString = parts[1]
			}
		} else {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "Missing or invalid authorization")
		}

		// Validate token
		claims, err := utils.ValidateToken(tokenString, cfg.JWTSecret)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid or expired token")
		}

		// Store user info in context
		c.Locals("userID", claims.UserID)
		c.Locals("email", claims.Email)

		return c.Next()
	}
}

// Permission checks if the authenticated user has the required permission
func Permission(permissionName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := c.Locals("userID").(uint)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Error:   "unauthorized",
				Message: "User not authenticated",
			})
		}

		// Get DB from context (we store it during request)
		db, ok := c.Locals("db").(*gorm.DB)
		if !ok {
			// Fallback: permission check skipped if DB not in context
			return c.Next()
		}

		// Load user with roles and permissions
		var user models.User
		err := db.Preload("Roles.Permissions").First(&user, userID).Error
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "User not found",
			})
		}

		// Check permission
		if !user.HasPermission(permissionName) {
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "You do not have permission to access this resource",
			})
		}

		// Store user in context for handlers
		c.Locals("user", &user)

		return c.Next()
	}
}

// InjectDB adds database instance to Fiber context for downstream middleware
func InjectDB(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("db", db)
		return c.Next()
	}
}
