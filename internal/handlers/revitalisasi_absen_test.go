package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

func TestCreateAbsenTukangWithMultipartForm(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiTukang{}, &models.RevitalisasiAbsenTukang{}); err != nil {
		t.Fatalf("auto migrate models: %v", err)
	}

	if err := db.Create(&models.RevitalisasiTukang{ID: 1, Name: "Tukang A", IsActive: true}).Error; err != nil {
		t.Fatalf("seed tukang: %v", err)
	}

	handler := NewRevitalisasiHandler(db, t.TempDir())
	app := fiber.New()
	app.Post("/absen-tukang", handler.CreateAbsenTukang)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tanggal", "2025-08-29"); err != nil {
		t.Fatalf("write tanggal field: %v", err)
	}
	if err := writer.WriteField("tukang_id", "1"); err != nil {
		t.Fatalf("write tukang_id field: %v", err)
	}
	if err := writer.WriteField("status", "hadir"); err != nil {
		t.Fatalf("write status field: %v", err)
	}
	if err := writer.WriteField("note", "ok"); err != nil {
		t.Fatalf("write note field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/absen-tukang", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		var payload map[string]any
		_ = json.NewDecoder(res.Body).Decode(&payload)
		t.Fatalf("expected 201, got %d with payload: %#v", res.StatusCode, payload)
	}
}
