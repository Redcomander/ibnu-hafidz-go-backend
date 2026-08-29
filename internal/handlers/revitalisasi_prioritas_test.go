package handlers

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

func TestRevitalisasiPrioritasCRUD(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&models.RevitalisasiPrioritas{}); err != nil {
		t.Fatalf("auto migrate prioritas model: %v", err)
	}

	item := models.RevitalisasiPrioritas{
		Judul:     "Pengerjaan atap",
		Deskripsi: "Prioritas utama untuk area utama",
		Tingkat:   "high",
		Urutan:    1,
		IsActive:  true,
	}

	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create prioritas: %v", err)
	}

	var count int64
	if err := db.Model(&models.RevitalisasiPrioritas{}).Count(&count).Error; err != nil {
		t.Fatalf("count prioritas: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 prioritas row, got %d", count)
	}
}
