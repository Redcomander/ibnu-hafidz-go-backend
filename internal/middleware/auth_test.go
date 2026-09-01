package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

func TestHasTeacherScopedHalaqohAccess_AllowsTeacherRolesForStudentsView(t *testing.T) {
	user := &models.User{
		Roles: []models.Role{{Name: "guru"}},
	}

	if !hasTeacherScopedHalaqohAccess(user, "students.view") {
		t.Fatal("expected guru role to be allowed for students.view in Halaqoh assignment access")
	}
}

func TestPermissionAny_AllowsTeacherRoleForStudentsView(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", uint(1))
		c.Locals("db", &gorm.DB{})
		c.Locals("user", &models.User{Roles: []models.Role{{Name: "guru"}}})
		return c.Next()
	})
	app.Get("/ok", PermissionAny("students.view", "users.view"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/ok", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d but got %d", fiber.StatusOK, resp.StatusCode)
	}
}

func TestAuth_AcceptsAccessTokenFromQueryParam(t *testing.T) {
	cfg := &config.Config{JWTSecret: "test-secret"}
	token, err := utils.GenerateAccessToken(42, "demo@example.com", cfg.JWTSecret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	app := fiber.New()
	app.Get("/download", Auth(cfg), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/download?token="+token, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d but got %d", fiber.StatusOK, resp.StatusCode)
	}
}
