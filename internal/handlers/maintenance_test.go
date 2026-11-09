package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/maintenance"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMaintenanceAccessAndControls(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&models.CmsSetting{}, &models.User{}, &models.Role{}, &models.Permission{}))
	adminRole := models.Role{Name: "admin"}
	teacherRole := models.Role{Name: "teacher"}
	admin := models.User{Name: "Administrator", Email: "admin@test.invalid", Username: "admin", Password: "unchanged", Roles: []models.Role{adminRole}}
	teacher := models.User{Name: "Teacher", Email: "teacher@test.invalid", Username: "teacher", Password: "unchanged", Roles: []models.Role{teacherRole}}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&teacher).Error)
	cfg := &config.Config{JWTSecret: "maintenance-test-secret"}
	adminToken, err := utils.GenerateAccessToken(admin.ID, admin.Email, cfg.JWTSecret)
	require.NoError(t, err)
	teacherToken, err := utils.GenerateAccessToken(teacher.ID, teacher.Email, cfg.JWTSecret)
	require.NoError(t, err)
	app := fiber.New()
	app.Use(middleware.InjectDB(db))
	app.Use("/api", middleware.Maintenance(db, cfg))
	handler := NewMaintenanceHandler(db, cfg)
	app.Get("/api/maintenance", handler.Get)
	app.Put("/api/maintenance", middleware.Auth(cfg), handler.Update)
	for _, path := range []string{"/api/public/home", "/api/students", "/api/auth/refresh", "/health"} {
		app.Get(path, func(c *fiber.Ctx) error { return c.SendString("available") })
	}
	app.Post("/api/students", func(c *fiber.Ctx) error { return c.SendString("write executed") })
	cms := NewCmsHandler(db)
	app.Put("/cms/:key", cms.Upsert)
	app.Delete("/cms/:key", cms.Delete)
	request := func(method, path, token, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, want, resp.StatusCode, "%s %s", method, path)
		var data map[string]any
		if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
		}
		if want == 503 {
			require.Equal(t, "60", resp.Header.Get("Retry-After"))
			require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
		}
		return data
	}
	initial := request("GET", "/api/maintenance", "", "", 200)
	require.Equal(t, false, initial["enabled"])
	var count int64
	require.NoError(t, db.Model(&models.CmsSetting{}).Count(&count).Error)
	require.Zero(t, count, "GET must not create settings")
	request("PUT", "/api/maintenance", "", `{"enabled":true,"scope":"all"}`, 401)
	request("PUT", "/api/maintenance", teacherToken, `{"enabled":true,"scope":"all"}`, 403)
	request("PUT", "/api/maintenance", adminToken, `{"enabled":true,"scope":"invalid"}`, 400)
	request("PUT", "/api/maintenance", adminToken, `{"scope":"all"}`, 400)
	request("PUT", "/api/maintenance", adminToken, fmt.Sprintf(`{"enabled":true,"scope":"all","message":"%s"}`, strings.Repeat("a", 501)), 400)
	request("PUT", "/api/maintenance", adminToken, `{"enabled":true,"scope":"all","message":"Updating safely"}`, 200)
	require.Equal(t, "maintenance", request("GET", "/api/public/home", "", "", 503)["error"])
	request("GET", "/api/students", teacherToken, "", 503)
	request("POST", "/api/students", teacherToken, `{}`, 503)
	request("GET", "/api/students", "forged-token", "", 503)
	request("GET", "/api/students", adminToken, "", 200)
	request("GET", "/api/auth/refresh", "", "", 200)
	request("GET", "/health", "", "", 200)
	require.Equal(t, true, request("GET", "/api/maintenance", adminToken, "", 200)["can_bypass"])
	require.Equal(t, false, request("GET", "/api/maintenance", teacherToken, "", 200)["can_bypass"])
	request("PUT", "/cms/maintenance_mode", adminToken, `{"value":"{}"}`, 403)
	request("DELETE", "/cms/maintenance_mode", adminToken, "", 403)
	request("PUT", "/cms/MAINTENANCE_MODE", teacherToken, `{"value":"{}"}`, 403)
	request("PUT", "/api/maintenance", adminToken, `{"enabled":true,"scope":"public"}`, 200)
	request("GET", "/api/students", teacherToken, "", 200)
	request("GET", "/api/public/home", "", "", 503)
	request("PUT", "/api/maintenance", adminToken, `{"enabled":false,"scope":"all"}`, 200)
	request("GET", "/api/public/home", "", "", 200)
	var saved models.User
	require.NoError(t, db.First(&saved, admin.ID).Error)
	require.Equal(t, "unchanged", saved.Password)
	settings, err := maintenance.Read(db)
	require.NoError(t, err)
	require.False(t, settings.Enabled)
}
