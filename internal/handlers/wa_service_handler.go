package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type WAServiceHandler struct {
	cfg    *config.Config
	db     *gorm.DB
	client *http.Client
}

func NewWAServiceHandler(cfg *config.Config, db *gorm.DB) *WAServiceHandler {
	return &WAServiceHandler{
		cfg:    cfg,
		db:     db,
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (h *WAServiceHandler) requireUserID(c *fiber.Ctx) (uint, error) {
	userID, _ := c.Locals("userID").(uint)
	if userID == 0 {
		return 0, fiber.ErrUnauthorized
	}
	return userID, nil
}

func (h *WAServiceHandler) Status(c *fiber.Ctx) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "missing_user_id",
			Message: "Authenticated user ID is required for WA session access",
		})
	}
	resp, err := h.doRequest("GET", "/api/wa/status", nil, userID)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(models.ErrorResponse{
			Error:   "wa_service_unavailable",
			Message: "WhatsApp service is not reachable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return c.Status(resp.StatusCode).Send(body)
	}
	return c.Status(resp.StatusCode).Send(body)
}

func (h *WAServiceHandler) QR(c *fiber.Ctx) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "missing_user_id",
			Message: "Authenticated user ID is required for WA session access",
		})
	}
	resp, err := h.doRequest("GET", "/api/wa/qr", nil, userID)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(models.ErrorResponse{
			Error:   "wa_service_unavailable",
			Message: "WhatsApp service is not reachable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return c.Status(resp.StatusCode).Send(body)
	}
	return c.Status(resp.StatusCode).Send(body)
}

func (h *WAServiceHandler) Disconnect(c *fiber.Ctx) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "missing_user_id",
			Message: "Authenticated user ID is required for WA session access",
		})
	}
	resp, err := h.doRequest("POST", "/api/wa/session/disconnect", nil, userID)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(models.ErrorResponse{
			Error:   "wa_service_unavailable",
			Message: "WhatsApp service is not reachable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return c.Status(resp.StatusCode).Send(body)
	}
	return c.Status(resp.StatusCode).Send(body)
}

func (h *WAServiceHandler) Send(c *fiber.Ctx) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "missing_user_id",
			Message: "Authenticated user ID is required for WA session access",
		})
	}

	type requestBody struct {
		KontakID   uint   `json:"kontak_id"`
		Number     string `json:"number"`
		Message    string `json:"message"`
		TemplateID *uint  `json:"template_id,omitempty"`
	}

	var req requestBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.KontakID != 0 {
		var kontak models.Kontak
		if err := h.db.First(&kontak, req.KontakID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
				Error:   "not_found",
				Message: "Kontak not found",
			})
		}
		if strings.TrimSpace(req.Number) == "" {
			req.Number = kontak.NoWhatsapp
		}
		if strings.TrimSpace(req.Message) == "" {
			if req.TemplateID != nil {
				var tmpl models.TemplatePesan
				if tErr := h.db.First(&tmpl, *req.TemplateID).Error; tErr == nil {
					req.Message = renderTemplateMessage(tmpl.Konten, kontak)
				}
			}
		}
		if strings.TrimSpace(req.Message) == "" {
			req.Message = fmt.Sprintf("Assalamu'alaikum, kami dari tim PPDB ingin menindaklanjuti data %s.", kontak.Nama)
		}
	}

	if strings.TrimSpace(req.Number) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Nomor WhatsApp wajib diisi",
		})
	}
	if strings.TrimSpace(req.Message) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Pesan wajib diisi",
		})
	}

	payload := map[string]string{
		"number": req.Number,
		"text":   req.Message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to encode WA payload",
		})
	}

	resp, err := h.doRequest("POST", "/api/wa/send", bytes.NewReader(body), userID)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(models.ErrorResponse{
			Error:   "wa_service_unavailable",
			Message: "WhatsApp service is not reachable",
		})
	}
	defer resp.Body.Close()

	resultBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return c.Status(resp.StatusCode).Send(resultBody)
	}

	if req.KontakID != 0 {
		now := time.Now()
		var kontak models.Kontak
		if err := h.db.First(&kontak, req.KontakID).Error; err == nil {
			kontak.LastContactAt = &now
			userID, _ := c.Locals("userID").(uint)
			if userID != 0 {
				kontak.HandlerID = &userID
			}
			_ = h.db.Save(&kontak).Error

			entry := models.RiwayatKontak{KontakID: kontak.ID, PesanFinal: &req.Message, DikirimVia: strPtr("whatsapp")}
			if req.TemplateID != nil {
				entry.TemplatePesanID = req.TemplateID
			}
			if userID != 0 {
				entry.UserID = &userID
			}
			_ = h.db.Create(&entry).Error
		}
	}

	return c.Status(resp.StatusCode).Send(resultBody)
}

func (h *WAServiceHandler) doRequest(method, path string, body io.Reader, userID uint) (*http.Response, error) {
	if userID == 0 {
		return nil, fmt.Errorf("missing_user_id")
	}

	url := strings.TrimRight(h.cfg.WAServiceURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if h.cfg.WAServiceToken != "" && h.cfg.WAServiceToken != "change-me" {
		req.Header.Set("X-WA-Service-Token", h.cfg.WAServiceToken)
	}
	req.Header.Set("X-User-ID", strconv.FormatUint(uint64(userID), 10))

	return h.client.Do(req)
}

func normalizeWAServiceNumber(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	trimmed = strings.ReplaceAll(trimmed, "(", "")
	trimmed = strings.ReplaceAll(trimmed, ")", "")
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "0") {
		return "62" + trimmed[1:]
	}
	if strings.HasPrefix(trimmed, "+62") {
		return strings.TrimPrefix(trimmed, "+")
	}
	return trimmed
}

func isValidWAServicePayload(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSpace(v))
	return err == nil
}
