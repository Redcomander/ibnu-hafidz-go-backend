package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRequestLogCountryFallbackPersistsAndPreservesHeaders(t *testing.T) {
	db := securityDB(t)
	require.NoError(t, db.AutoMigrate(&models.UserActivityLog{}))
	ip, local, existing := "8.8.8.8", "192.168.1.2", "SG"
	rows := []models.UserActivityLog{
		{Method: "GET", Path: "/one", IPAddress: &ip},
		{Method: "GET", Path: "/two", IPAddress: &ip},
		{Method: "GET", Path: "/header", IPAddress: &ip, CountryCode: &existing},
		{Method: "GET", Path: "/local", IPAddress: &local},
	}
	require.NoError(t, db.Create(&rows).Error)
	handler := NewActivityLogHandler(db)
	handler.countryLookup = func(_ context.Context, ips []string) map[string]string {
		return map[string]string{"8.8.8.8": "ID"}
	}
	app := fiber.New()
	app.Get("/logs", handler.List)
	response, err := app.Test(httptest.NewRequest("GET", "/logs", nil))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	var payload struct {
		Data []models.UserActivityLog `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Len(t, payload.Data, 4)
	var stored []models.UserActivityLog
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Equal(t, "ID", *stored[0].CountryCode)
	require.Equal(t, "ID", *stored[1].CountryCode)
	require.Equal(t, "SG", *stored[2].CountryCode)
	require.Nil(t, stored[3].CountryCode)
}

func TestRequestLogCountryFailureStillReturnsLogs(t *testing.T) {
	db := securityDB(t)
	require.NoError(t, db.AutoMigrate(&models.UserActivityLog{}))
	ip := "8.8.8.8"
	require.NoError(t, db.Create(&models.UserActivityLog{Method: "GET", Path: "/test", IPAddress: &ip}).Error)
	handler := NewActivityLogHandler(db)
	handler.countryLookup = func(context.Context, []string) map[string]string { return nil }
	app := fiber.New()
	app.Get("/logs", handler.List)
	response, err := app.Test(httptest.NewRequest("GET", "/logs", nil))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
}
