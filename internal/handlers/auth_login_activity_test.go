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

func TestLoginStoresDeviceAndCountryInActivityLog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
