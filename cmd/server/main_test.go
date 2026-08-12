package main

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSafeClientIPIgnoresSpoofedHeadersWhenNotTrustedProxy(t *testing.T) {
	app := fiber.New(fiber.Config{
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"127.0.0.1", "::1"},
	})

	app.Use(func(c *fiber.Ctx) error {
		c.Request().Header.Set("X-Forwarded-For", "203.0.113.10")
		c.Request().Header.Set("X-Real-IP", "203.0.113.10")
		got := safeClientIP(c)
		if got == "203.0.113.10" {
			t.Fatalf("trusted-check bypass: spoofed forwarded IP accepted")
		}
		if got == "" {
			t.Fatalf("expected a non-empty socket IP from the client connection")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if _, err := app.Test(req, -1); err != nil {
		t.Fatalf("unexpected app test error: %v", err)
	}
}
