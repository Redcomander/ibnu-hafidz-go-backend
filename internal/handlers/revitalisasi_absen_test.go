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

func TestGeneratePayrollReportRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiTukang{}, &models.RevitalisasiAbsenTukang{}); err != nil {
		t.Fatalf("auto migrate models: %v", err)
	}

	handler := NewRevitalisasiHandler(db, t.TempDir())
	if err := db.Create(&models.RevitalisasiTukang{
		ID:         1,
		Jenis:      "sma",
		Name:       "Tukang A",
		Divisi:     "Bangunan",
		Area:       "Ruang Utama",
		GajiHarian: 150000,
		Kasbon:     50000,
		CaraPotong: "langsung",
		IsActive:   true,
	}).Error; err != nil {
		t.Fatalf("seed tukang: %v", err)
	}

	for _, item := range []struct {
		date   string
		status string
	}{
		{date: "2025-08-04", status: "hadir"},
		{date: "2025-08-05", status: "hadir"},
		{date: "2025-08-06", status: "izin"},
	} {
		if err := db.Create(&models.RevitalisasiAbsenTukang{
			Jenis:    "sma",
			Tanggal:  mustDate(t, item.date),
			TukangID: 1,
			Status:   normalizeStatus(item.status),
		}).Error; err != nil {
			t.Fatalf("seed attendance for %s: %v", item.date, err)
		}
	}

	rows, err := handler.generatePayrollReportRows("sma", "2025-08-01", "2025-08-31")
	if err != nil {
		t.Fatalf("generate payroll rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HariHadir != 2 {
		t.Fatalf("expected 2 hadir days, got %d", rows[0].HariHadir)
	}
	if rows[0].TotalGaji != 300000 {
		t.Fatalf("expected total gaji 300000, got %v", rows[0].TotalGaji)
	}
	if rows[0].Kasbon != 50000 {
		t.Fatalf("expected kasbon 50000, got %v", rows[0].Kasbon)
	}
	if rows[0].TotalSetelahKasbon != 250000 {
		t.Fatalf("expected total after kasbon 250000, got %v", rows[0].TotalSetelahKasbon)
	}
}

func TestRevitalisasiKasbonLedgerBalance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiTukang{}, &models.RevitalisasiKasbon{}); err != nil {
		t.Fatalf("auto migrate kasbon models: %v", err)
	}

	handler := NewRevitalisasiHandler(db, t.TempDir())
	if err := db.Create(&models.RevitalisasiTukang{ID: 1, Jenis: "sma", Name: "Tukang A", GajiHarian: 150000, Kasbon: 0, IsActive: true}).Error; err != nil {
		t.Fatalf("seed tukang: %v", err)
	}

	if err := handler.syncTukangKasbonBalance(1); err != nil {
		t.Fatalf("sync initial kasbon: %v", err)
	}

	if err := db.Create(&models.RevitalisasiKasbon{Jenis: "penambahan", TukangID: 1, Tanggal: mustDate(t, "2025-08-01"), Jumlah: 50000, Metode: "langsung", Keterangan: "Pinjaman awal"}).Error; err != nil {
		t.Fatalf("create add kasbon: %v", err)
	}
	if err := db.Create(&models.RevitalisasiKasbon{Jenis: "pelunasan", TukangID: 1, Tanggal: mustDate(t, "2025-08-05"), Jumlah: 20000, Metode: "angsuran", Keterangan: "Bayar cicilan"}).Error; err != nil {
		t.Fatalf("create pay kasbon: %v", err)
	}

	if err := handler.syncTukangKasbonBalance(1); err != nil {
		t.Fatalf("sync adjusted kasbon: %v", err)
	}

	var tukang models.RevitalisasiTukang
	if err := db.First(&tukang, 1).Error; err != nil {
		t.Fatalf("load tukang: %v", err)
	}
	if tukang.Kasbon != 30000 {
		t.Fatalf("expected kasbon balance 30000, got %v", tukang.Kasbon)
	}
}

func TestGeneratePayrollReportRowsAngsuranCutsDailyInstallment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiTukang{}, &models.RevitalisasiAbsenTukang{}); err != nil {
		t.Fatalf("auto migrate payroll models: %v", err)
	}

	handler := NewRevitalisasiHandler(db, t.TempDir())
	if err := db.Create(&models.RevitalisasiTukang{
		ID:         1,
		Jenis:      "sma",
		Name:       "Tukang B",
		Divisi:     "Bangunan",
		Area:       "Area A",
		GajiHarian: 150000,
		Kasbon:     50000,
		CaraPotong: "angsuran",
		IsActive:   true,
	}).Error; err != nil {
		t.Fatalf("seed tukang: %v", err)
	}

	for _, item := range []struct {
		date   string
		status string
	}{
		{date: "2025-08-04", status: "hadir"},
		{date: "2025-08-05", status: "hadir"},
	} {
		if err := db.Create(&models.RevitalisasiAbsenTukang{
			Jenis:    "sma",
			Tanggal:  mustDate(t, item.date),
			TukangID: 1,
			Status:   normalizeStatus(item.status),
		}).Error; err != nil {
			t.Fatalf("seed attendance for %s: %v", item.date, err)
		}
	}

	rows, err := handler.generatePayrollReportRows("sma", "2025-08-01", "2025-08-31")
	if err != nil {
		t.Fatalf("generate payroll rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].TotalSetelahKasbon != 275000 {
		t.Fatalf("expected angsuran deduction to cut 25000 from salary total, got %v", rows[0].TotalSetelahKasbon)
	}
}

func TestForceDeleteTukangRemovesRelatedRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiTukang{}, &models.RevitalisasiKasbon{}, &models.RevitalisasiAbsenTukang{}); err != nil {
		t.Fatalf("auto migrate related models: %v", err)
	}

	handler := NewRevitalisasiHandler(db, t.TempDir())
	if err := db.Create(&models.RevitalisasiTukang{
		ID:         1,
		Jenis:      "sma",
		Name:       "Tukang C",
		GajiHarian: 100000,
		Kasbon:     20000,
		IsActive:   true,
	}).Error; err != nil {
		t.Fatalf("seed tukang: %v", err)
	}
	if err := db.Create(&models.RevitalisasiKasbon{Jenis: "penambahan", TukangID: 1, Tanggal: mustDate(t, "2025-08-01"), Jumlah: 20000, Metode: "langsung"}).Error; err != nil {
		t.Fatalf("seed kasbon: %v", err)
	}
	if err := db.Create(&models.RevitalisasiAbsenTukang{Jenis: "sma", Tanggal: mustDate(t, "2025-08-04"), TukangID: 1, Status: "hadir"}).Error; err != nil {
		t.Fatalf("seed attendance: %v", err)
	}

	app := fiber.New()
	app.Delete("/tukang/:id/force", handler.ForceDeleteTukang)
	res, err := app.Test(httptest.NewRequest(http.MethodDelete, "/tukang/1/force", nil))
	if err != nil {
		t.Fatalf("perform force delete request: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected force delete status 200, got %d", res.StatusCode)
	}

	var tukangCount int64
	if err := db.Unscoped().Model(&models.RevitalisasiTukang{}).Count(&tukangCount).Error; err != nil {
		t.Fatalf("count tukang: %v", err)
	}
	if tukangCount != 0 {
		t.Fatalf("expected tukang to be permanently removed, got count=%d", tukangCount)
	}

	var absenCount int64
	if err := db.Unscoped().Model(&models.RevitalisasiAbsenTukang{}).Count(&absenCount).Error; err != nil {
		t.Fatalf("count absen: %v", err)
	}
	if absenCount != 0 {
		t.Fatalf("expected absen to be removed, got count=%d", absenCount)
	}

	var kasbonCount int64
	if err := db.Unscoped().Model(&models.RevitalisasiKasbon{}).Count(&kasbonCount).Error; err != nil {
		t.Fatalf("count kasbon: %v", err)
	}
	if kasbonCount != 0 {
		t.Fatalf("expected kasbon to be removed, got count=%d", kasbonCount)
	}
}

func mustJPEGBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}
