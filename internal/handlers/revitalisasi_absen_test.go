package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestUpdateAbsenTukangWithPhotoRemovalAndAppend(t *testing.T) {
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

	uploadRoot := t.TempDir()
	handler := NewRevitalisasiHandler(db, uploadRoot)
	app := fiber.New()
	app.Put("/absen-tukang/:id", handler.UpdateAbsenTukang)

	oldPath1 := filepath.ToSlash(filepath.Join("revitalisasi", "absen", "old-1.jpg"))
	oldPath2 := filepath.ToSlash(filepath.Join("revitalisasi", "absen", "old-2.jpg"))
	for _, rel := range []string{oldPath1, oldPath2} {
		fullPath := filepath.Join(uploadRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(fullPath, []byte("fake-jpg"), 0644); err != nil {
			t.Fatalf("write fixture %s: %v", rel, err)
		}
	}

	item := models.RevitalisasiAbsenTukang{
		Tanggal:   mustDate(t, "2025-08-28"),
		TukangID:  1,
		Status:    "hadir",
		Note:      "before",
		PhotoPath: stringPtr(strings.Join([]string{oldPath1, oldPath2}, ";")),
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed attendance: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("tanggal", "2025-08-29"); err != nil {
		t.Fatalf("write tanggal field: %v", err)
	}
	if err := writer.WriteField("tukang_id", "1"); err != nil {
		t.Fatalf("write tukang_id field: %v", err)
	}
	if err := writer.WriteField("status", "izin"); err != nil {
		t.Fatalf("write status field: %v", err)
	}
	if err := writer.WriteField("note", "updated"); err != nil {
		t.Fatalf("write note field: %v", err)
	}
	if err := writer.WriteField("remove_photo", oldPath1); err != nil {
		t.Fatalf("write remove_photo field: %v", err)
	}
	newPhoto, err := writer.CreateFormFile("photo", "new.jpg")
	if err != nil {
		t.Fatalf("create new photo form file: %v", err)
	}
	jpegBytes := mustJPEGBytes(t, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	if _, err := newPhoto.Write(jpegBytes); err != nil {
		t.Fatalf("write new photo bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/absen-tukang/1", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		var payload map[string]any
		_ = json.NewDecoder(res.Body).Decode(&payload)
		t.Fatalf("expected 200, got %d with payload: %#v", res.StatusCode, payload)
	}

	var updated models.RevitalisasiAbsenTukang
	if err := db.First(&updated, item.ID).Error; err != nil {
		t.Fatalf("load updated record: %v", err)
	}
	if updated.PhotoPath == nil || !strings.Contains(*updated.PhotoPath, oldPath2) || strings.Contains(*updated.PhotoPath, oldPath1) {
		t.Fatalf("expected remaining photo list to keep oldPath2 and remove oldPath1, got: %#v", updated.PhotoPath)
	}
	if updated.Note != "updated" {
		t.Fatalf("expected note updated, got: %q", updated.Note)
	}
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return parsed
}

func stringPtr(value string) *string {
	return &value
}

func mustJPEGBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}
