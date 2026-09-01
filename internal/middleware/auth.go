package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

func extractTokenFromRequest(c *fiber.Ctx) string {
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	if tokenString := c.Query("token"); tokenString != "" {
		return tokenString
	}

	if tokenString := c.Query("ticket"); tokenString != "" {
		return tokenString
	}

	return ""
}

// Auth validates JWT access token from Authorization header or query token.
// Some browser-based downloads (excel/pdf exports) cannot send the Authorization header
// via a plain anchor navigation, so a signed query token is accepted there as well.
func Auth(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenString := extractTokenFromRequest(c)
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

func hasTeacherScopedHalaqohAccess(user *models.User, permissionName string) bool {
	if user == nil {
		return false
	}

	if user.HasPermission(permissionName) {
		return true
	}

	allowedPermissions := map[string]struct{}{
		"students.view":              {},
		"users.view":                 {},
		"halaqoh-assignments.create": {},
		"halaqoh-assignments.edit":   {},
		"halaqoh-assignments.delete": {},
		"halaqoh.view_all":           {},
	}
	if _, ok := allowedPermissions[permissionName]; !ok {
		return false
	}

	for _, role := range user.Roles {
		roleName := strings.ToLower(strings.TrimSpace(role.Name))
		if roleName == "teacher" || roleName == "guru" || roleName == "musyrif" || roleName == "tim_presensi" || roleName == "staff" {
			return true
		}
	}

	return false
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
			if !hasTeacherScopedHalaqohAccess(user, permissionName) {
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
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "Permission check unavailable",
			})
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
		if !hasTeacherScopedHalaqohAccess(&user, permissionName) {
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
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
				Error:   "forbidden",
				Message: "Permission check unavailable",
			})
		}

		if user, ok := c.Locals("user").(*models.User); ok && user != nil {
			for _, permissionName := range permissionNames {
				if hasTeacherScopedHalaqohAccess(user, permissionName) {
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
			if hasTeacherScopedHalaqohAccess(&user, permissionName) {
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

// ServiceToken validates service-to-service requests using the X-Service-Token header.
// Used by internal microservices (e.g. OCR service) that do not have a user JWT.
func ServiceToken(token string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token == "" {
			return fiber.NewError(fiber.StatusServiceUnavailable, "Service token not configured on this server")
		}
		provided := strings.TrimSpace(c.Get("X-Service-Token"))
		if provided == "" || provided != token {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid or missing service token")
		}
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

// ActivityLog stores authenticated request metadata for audit/activity page.
func ActivityLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip noisy endpoints
		path := c.Path()
		if path == "/api/notifications/stream" || path == "/api/activity-logs" || strings.HasPrefix(path, "/health") {
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
