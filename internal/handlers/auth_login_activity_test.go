package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLoginStoresCountryFromGeoHeaders(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&models.User{}, &models.UserActivityLog{}))

	hash, err := utils.HashPassword("secret123")
	require.NoError(t, err)

	user := models.User{
		Name:     "Geo User",
		Username: "geouser",
		Email:    "geo@example.com",
		Password: hash,
	}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{
		JWTSecret:        "test-secret",
		JWTRefreshSecret: "test-refresh-secret",
		Environment:      "development",
	}

	handler := NewAuthHandler(db, cfg)
	app := fiber.New()
	app.Use(middleware.InjectDB(db))
	app.Use(middleware.ActivityLog())
	app.Post("/api/auth/login", handler.Login)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"geouser","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("X-Geo-Country", "SG")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var logs []models.UserActivityLog
	require.NoError(t, db.Order("created_at desc").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].CountryCode)
	assert.Equal(t, "SG", *logs[0].CountryCode)
}

func TestUnauthorizedProtectedRouteIsLoggedAs401(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.UserActivityLog{}))

	cfg := &config.Config{JWTSecret: "test-secret", JWTRefreshSecret: "test-refresh-secret", Environment: "development"}

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(middleware.InjectDB(db))
	app.Use(middleware.ActivityLog())
	app.Get("/protected", middleware.Auth(cfg), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var logs []models.UserActivityLog
	require.NoError(t, db.Order("created_at desc").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "/protected", logs[0].Path)
	assert.Equal(t, 401, logs[0].StatusCode)
	assert.EqualValues(t, 0, logs[0].UserID)
}

func TestAnonymousPublicAnd404RequestsAreLogged(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.UserActivityLog{}))

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(middleware.InjectDB(db))
	app.Use(middleware.InjectConfig(&config.Config{JWTSecret: "test-secret"}))
	app.Use(middleware.ActivityLog())
	app.Get("/public/home", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	publicReq := httptest.NewRequest(http.MethodGet, "/public/home", nil)
	publicResp, err := app.Test(publicReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, publicResp.StatusCode)

	missingReq := httptest.NewRequest(http.MethodGet, "/missing-page", nil)
	missingResp, err := app.Test(missingReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, missingResp.StatusCode)

	var logs []models.UserActivityLog
	require.NoError(t, db.Order("created_at desc").Find(&logs).Error)
	require.Len(t, logs, 2)

	seen := map[string]int{}
	for _, log := range logs {
		seen[log.Path] = log.StatusCode
		assert.EqualValues(t, 0, log.UserID)
	}

	assert.Equal(t, 200, seen["/public/home"])
	assert.Equal(t, 404, seen["/missing-page"])
}

func TestAuthorizedPublicRequestKeepsUserIdentityInActivityLog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.UserActivityLog{}))

	hash, err := utils.HashPassword("secret123")
	require.NoError(t, err)

	user := models.User{
		Name:     "Requester",
		Username: "requester",
		Email:    "requester@example.com",
		Password: hash,
	}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{JWTSecret: "test-secret", JWTRefreshSecret: "test-refresh-secret", Environment: "development"}
	token, err := utils.GenerateAccessToken(user.ID, user.Email, cfg.JWTSecret)
	require.NoError(t, err)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(middleware.InjectDB(db))
	app.Use(middleware.InjectConfig(cfg))
	app.Use(middleware.ActivityLog())
	app.Get("/public/home", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/public/home", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var logs []models.UserActivityLog
	require.NoError(t, db.Order("created_at desc").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.EqualValues(t, user.ID, logs[0].UserID)
	assert.Equal(t, "/public/home", logs[0].Path)
}

func TestLoginStoresDeviceAndCountryInActivityLog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&models.User{}, &models.UserActivityLog{}))

	hash, err := utils.HashPassword("secret123")
	require.NoError(t, err)

	user := models.User{
		Name:     "Test User",
		Username: "tester",
		Email:    "tester@example.com",
		Password: hash,
	}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{
		JWTSecret:        "test-secret",
		JWTRefreshSecret: "test-refresh-secret",
		Environment:      "development",
	}

	handler := NewAuthHandler(db, cfg)
	app := fiber.New()
	app.Use(middleware.InjectDB(db))
	app.Use(middleware.ActivityLog())
	app.Post("/api/auth/login", handler.Login)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"tester","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Mobile Safari/537.36")
	req.Header.Set("CF-IPCountry", "ID")
	req.Header.Set("X-Forwarded-For", "203.0.113.12, 10.0.0.1")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var logs []models.UserActivityLog
	require.NoError(t, db.Order("created_at desc").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.EqualValues(t, user.ID, logs[0].UserID)
	assert.Equal(t, "POST", logs[0].Method)
	assert.Equal(t, "/api/auth/login", logs[0].Path)
	assert.Equal(t, 200, logs[0].StatusCode)
	assert.NotNil(t, logs[0].DeviceName)
	assert.Equal(t, "Chrome di Android", *logs[0].DeviceName)
	assert.NotNil(t, logs[0].CountryCode)
	assert.Equal(t, "ID", *logs[0].CountryCode)
	assert.NotNil(t, logs[0].IPAddress)
	assert.Equal(t, "203.0.113.12", *logs[0].IPAddress)
}
