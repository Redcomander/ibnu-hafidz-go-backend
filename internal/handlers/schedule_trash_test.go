package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func TestScheduleListTrashedAndLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}

	if err := db.Exec(`CREATE TABLE lessons (
        id INTEGER PRIMARY KEY,
        nama TEXT,
        created_at DATETIME,
        updated_at DATETIME,
        deleted_at DATETIME
    )`).Error; err != nil {
		t.Fatalf("create lessons: %v", err)
	}

	if err := db.Exec(`CREATE TABLE users (
        id INTEGER PRIMARY KEY,
        name TEXT,
        created_at DATETIME,
        updated_at DATETIME,
        deleted_at DATETIME
    )`).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}

	if err := db.Exec(`CREATE TABLE kelas (
        id INTEGER PRIMARY KEY,
        nama TEXT,
        tingkat TEXT,
        gender TEXT,
        created_at DATETIME,
        updated_at DATETIME,
        deleted_at DATETIME
    )`).Error; err != nil {
		t.Fatalf("create kelas: %v", err)
	}

	if err := db.Exec(`CREATE TABLE lesson_kelas_teachers (
        id INTEGER PRIMARY KEY,
        lesson_id INTEGER,
        kelas_id INTEGER,
        user_id INTEGER,
        created_at DATETIME,
        updated_at DATETIME,
        deleted_at DATETIME
    )`).Error; err != nil {
		t.Fatalf("create lesson_kelas_teachers: %v", err)
	}

	if err := db.Exec(`CREATE TABLE jadwal_formal (
        id INTEGER PRIMARY KEY,
        lesson_kelas_teacher_id INTEGER,
        hari TEXT,
        jam_mulai TEXT,
        jam_selesai TEXT,
        type TEXT,
        created_at DATETIME,
        updated_at DATETIME,
        deleted_at DATETIME
    )`).Error; err != nil {
		t.Fatalf("create jadwal_formal: %v", err)
	}

	if err := db.Exec(`INSERT INTO lessons (id, nama) VALUES (1, 'Matematika')`).Error; err != nil {
		t.Fatalf("seed lesson: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (id, name) VALUES (9, 'Guru A')`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Exec(`INSERT INTO kelas (id, nama, tingkat) VALUES (3, 'A', '1')`).Error; err != nil {
		t.Fatalf("seed class: %v", err)
	}
	if err := db.Exec(`INSERT INTO lesson_kelas_teachers (id, lesson_id, kelas_id, user_id) VALUES (71, 1, 3, 9)`).Error; err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	softDeleteTime := time.Now().Add(-time.Hour)
	if err := db.Exec(`INSERT INTO jadwal_formal (id, lesson_kelas_teacher_id, hari, jam_mulai, jam_selesai, type, deleted_at)
        VALUES (12, 71, 'Senin', '08:00:00', '09:00:00', 'normal', ?)`, softDeleteTime).Error; err != nil {
		t.Fatalf("seed soft-deleted schedule: %v", err)
	}

	handler := NewScheduleHandler(db)
	app := fiber.New()
	app.Get("/schedules/trashed", handler.ListTrashed)
	app.Put("/schedules/:id/restore", handler.Restore)
	app.Delete("/schedules/:id/force", handler.ForceDelete)

	listReq := httptest.NewRequest(http.MethodGet, "/schedules/trashed?type=formal", nil)
	listRes, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("perform list request: %v", err)
	}
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listRes.StatusCode)
	}

	restoreReq := httptest.NewRequest(http.MethodPut, "/schedules/12/restore?type=formal", nil)
	restoreRes, err := app.Test(restoreReq)
	if err != nil {
		t.Fatalf("perform restore request: %v", err)
	}
	if restoreRes.StatusCode != http.StatusOK {
		t.Fatalf("expected restore status 200, got %d", restoreRes.StatusCode)
	}

	var restoredDeletedCount int
	if err := db.Raw(`SELECT COUNT(*) FROM jadwal_formal WHERE id = 12 AND deleted_at IS NULL`).Scan(&restoredDeletedCount).Error; err != nil {
		t.Fatalf("query restored deleted_at: %v", err)
	}
	if restoredDeletedCount != 1 {
		t.Fatalf("expected schedule to be restored, deleted_at should be null")
	}

	forceReq := httptest.NewRequest(http.MethodDelete, "/schedules/12/force?type=formal", nil)
	forceRes, err := app.Test(forceReq)
	if err != nil {
		t.Fatalf("perform force delete request: %v", err)
	}
	if forceRes.StatusCode != http.StatusOK {
		t.Fatalf("expected force delete status 200, got %d", forceRes.StatusCode)
	}

	var remainingCount int
	if err := db.Raw(`SELECT COUNT(*) FROM jadwal_formal WHERE id = 12`).Scan(&remainingCount).Error; err != nil {
		t.Fatalf("query remaining schedule: %v", err)
	}
	if remainingCount != 0 {
		t.Fatalf("expected schedule to be permanently deleted, got count=%d", remainingCount)
	}
}
