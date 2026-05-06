package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
)

// OCRProxyHandler proxies requests to the OCR microservice.
// Auth is already validated by the Go middleware — the OCR service runs with OCR_AUTH_MODE=none.
type OCRProxyHandler struct {
	cfg    *config.Config
	client *http.Client
}

func NewOCRProxyHandler(cfg *config.Config) *OCRProxyHandler {
	return &OCRProxyHandler{
		cfg: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second, // scans can take a while
		},
	}
}

// Proxy forwards any /api/ocr/* request to the OCR service, stripping the /api/ocr prefix.
// E.g. GET /api/ocr/health  →  GET http://localhost:3099/api/health
func (h *OCRProxyHandler) Proxy(c *fiber.Ctx) error {
	// Strip /api/ocr prefix to get downstream path
	upstreamPath := strings.TrimPrefix(string(c.Request().URI().Path()), "/api/ocr")
	if upstreamPath == "" {
		upstreamPath = "/"
	}

	// Preserve query string
	rawQuery := string(c.Request().URI().QueryString())
	targetURL := h.cfg.OCRBaseURL + upstreamPath
	if rawQuery != "" {
		targetURL = targetURL + "?" + rawQuery
	}

	// Stream the raw body (handles JSON, multipart/form-data, binary, etc.)
	rawBody := c.Body()
	var bodyReader io.Reader
	if len(rawBody) > 0 {
		bodyReader = bytes.NewReader(rawBody)
	}

	req, err := http.NewRequestWithContext(c.Context(), c.Method(), targetURL, bodyReader)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "ocr_proxy_error",
			"message": fmt.Sprintf("Failed to build upstream request: %v", err),
		})
	}

	// Forward Content-Type verbatim (multipart boundary must be preserved)
	if ct := c.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Accept", c.Get("Accept", "application/json"))
	if auth := c.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	// Pass optional API key if configured
	if h.cfg.OCRAPIKey != "" {
		req.Header.Set("X-OCR-API-Key", h.cfg.OCRAPIKey)
	}

	// Execute
	resp, err := h.client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "ocr_service_unavailable",
			"message": "OCR service is not reachable. Make sure ibnu-hafidz-ocr-service is running.",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "ocr_proxy_read_error",
			"message": "Failed to read OCR service response",
		})
	}

	// Mirror content-type and status
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	return c.Status(resp.StatusCode).Send(body)
}
