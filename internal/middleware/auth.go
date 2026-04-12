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
		} else if c.Path() == "/api/notifications/stream" {
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

		// Load the full user once when DB is already available in the request context.
		if db, ok := c.Locals("db").(*gorm.DB); ok {
			var user models.User
			if err := db.Preload("Roles.Permissions").First(&user, claims.UserID).Error; err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "User not found")
			}
			c.Locals("user", &user)
		}

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

		if user, ok := c.Locals("user").(*models.User); ok && user != nil {
			if !user.HasPermission(permissionName) {
				return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
					Error:   "forbidden",
					Message: "You do not have permission to access this resource",
				})
			}

			return c.Next()
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

// PermissionAny checks if the authenticated user has at least one required permission.
func PermissionAny(permissionNames ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := c.Locals("userID").(uint)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Error:   "unauthorized",
				Message: "User not authenticated",
			})
		}

		db, ok := c.Locals("db").(*gorm.DB)
		if !ok {
			return c.Next()
		}

		if user, ok := c.Locals("user").(*models.User); ok && user != nil {
			for _, permissionName := range permissionNames {
				if user.HasPermission(permissionName) {
					return c.Next()
				}
			}

			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "You do not have permission to access this resource",
			})
		}

		var user models.User
		err := db.Preload("Roles.Permissions").First(&user, userID).Error
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "User not found",
			})
		}

		for _, permissionName := range permissionNames {
			if user.HasPermission(permissionName) {
				c.Locals("user", &user)
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to access this resource",
		})
	}
}

// InjectDB adds database instance to Fiber context for downstream middleware
func InjectDB(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("db", db)
		return c.Next()
	}
}

// ActivityLog stores authenticated request metadata for audit/activity page.
func ActivityLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip noisy endpoints
		path := c.Path()
		if path == "/api/notifications/stream" || strings.HasPrefix(path, "/health") {
			return c.Next()
		}

		err := c.Next()

		db, ok := c.Locals("db").(*gorm.DB)
		if !ok {
			return err
		}
		userID, ok := c.Locals("userID").(uint)
		if !ok || userID == 0 {
			return err
		}

		ip := c.IP()
		ua := c.Get("User-Agent")
		log := models.UserActivityLog{
			UserID:     userID,
			Method:     c.Method(),
			Path:       path,
			StatusCode: c.Response().StatusCode(),
			IPAddress:  &ip,
			UserAgent:  &ua,
		}

		_ = db.Create(&log).Error
		return err
	}
}
