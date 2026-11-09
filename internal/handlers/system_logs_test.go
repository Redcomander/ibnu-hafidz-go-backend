package handlers

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/diagnostics"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsCaptureAndPrivacy(t *testing.T) {
	store, err := diagnostics.New(t.TempDir())
	require.NoError(t, err)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(middleware.Diagnostics(store))
	app.Get("/api/fail/:token", func(c *fiber.Ctx) error {
		return errors.New("password=private-token alice@example.test SELECT * FROM users")
	})
	app.Get("/api/direct", func(c *fiber.Ctx) error { return c.Status(500).JSON(fiber.Map{"message": "secret sql password"}) })
	app.Get("/api/panic", func(c *fiber.Ctx) error { panic("private-panic-value") })
	app.Get("/api/reject", func(c *fiber.Ctx) error { return fiber.NewError(403, "Denied") })
	run := func(path string, status int) string {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Request-ID", "forged")
		req.Header.Set("Authorization", "Bearer secret")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, status, resp.StatusCode)
		id := resp.Header.Get("X-Request-ID")
		require.Len(t, id, 32)
		require.NotEqual(t, "forged", id)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		if status >= 500 {
			require.Contains(t, string(data), id)
			require.NotContains(t, string(data), "private-token")
			require.NotContains(t, string(data), "secret sql")
		}
		return id
	}
	id := run("/api/fail/private-path-token?password=secret-query", 500)
	records := store.Snapshot()
	require.Len(t, records, 1)
	require.Equal(t, id, records[0].RequestID)
	require.Equal(t, "/api/fail/:token", records[0].Route)
	require.NotEmpty(t, records[0].Handler)
	run("/api/direct", 500)
	run("/api/panic", 500)
	run("/api/reject", 403)
	require.Len(t, store.Snapshot(), 3)
	require.NoError(t, store.SetDebug(true))
	run("/api/reject", 403)
	require.Len(t, store.Snapshot(), 4)
	for _, event := range store.Snapshot() {
		text := event.Summary + event.Route + strings.Join(event.Frames, " ")
		for _, secret := range []string{"private-path-token", "secret-query", "private-panic-value", "alice@example.test", "SELECT *"} {
			require.NotContains(t, text, secret)
		}
		if event.Kind == "panic" {
			require.NotEmpty(t, event.Frames)
		}
	}
}

func TestSystemLogAuthorizationAndManagement(t *testing.T) {
	store, err := diagnostics.New(t.TempDir())
	require.NoError(t, err)
	id := diagnostics.NewID()
	require.NoError(t, store.Add(diagnostics.Event{ID: id, RequestID: id, CreatedAt: time.Now(), Status: 500, Severity: "error", Route: "/api/students", Fingerprint: "group"}))
	h := NewSystemLogHandler(store)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(func(c *fiber.Ctx) error {
		role := c.Get("Test-Role")
		if role != "" {
			c.Locals("user", &models.User{Roles: []models.Role{{Name: role}}})
			c.Locals("userID", uint(9))
		}
		return c.Next()
	})
	group := app.Group("/api/system-logs", SuperAdminLogs)
	group.Get("/", h.List)
	group.Put("/debug", h.Debug)
	group.Put("/:id/resolution", h.Resolve)
	request := func(method, path, role, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Test-Role", role)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, want, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(data)
	}
	request("GET", "/api/system-logs", "", "", 403)
	request("PUT", "/api/system-logs/debug", "admin", `{"enabled":true}`, 403)
	request("GET", "/api/system-logs", "teacher", "", 403)
	require.Contains(t, request("GET", "/api/system-logs?search=group&status=500", "super_admin", "", 200), id)
	require.NotContains(t, request("GET", "/api/system-logs?status=404", "super_admin", "", 200), id)
	request("GET", "/api/system-logs?status=bad", "super_admin", "", 400)
	request("PUT", "/api/system-logs/debug", "super_admin", `{}`, 400)
	request("PUT", "/api/system-logs/debug", "super_admin", `{"enabled":true}`, 200)
	require.True(t, store.DebugEnabled())
	request("PUT", "/api/system-logs/"+id+"/resolution", "super_admin", `{"resolved":true}`, 200)
	require.True(t, store.Snapshot()[0].Resolved)
	request("PUT", "/api/system-logs/invalid/resolution", "super_admin", `{"resolved":true}`, 404)
}
