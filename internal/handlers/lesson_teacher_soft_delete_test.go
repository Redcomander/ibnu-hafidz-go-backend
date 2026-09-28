package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func TestLessonTeacherUnassignSoftDeletesAssignment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
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

	if err := db.Exec(`INSERT INTO lesson_kelas_teachers (id, lesson_id, kelas_id, user_id) VALUES (71, 1, 3, 9)`).Error; err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	if err := db.Exec(`INSERT INTO jadwal_formal (id, lesson_kelas_teacher_id, hari, jam_mulai, jam_selesai, type) VALUES (1, 71, 'Senin', '08:00:00', '09:00:00', 'normal')`).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	handler := NewLessonTeacherHandler(db)
	app := fiber.New()
	app.Delete("/lessons/assignments/:assignment_id", handler.Unassign)

	req := httptest.NewRequest(http.MethodDelete, "/lessons/assignments/71?type=formal", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var deletedCount int
	if err := db.Raw(`SELECT COUNT(*) FROM lesson_kelas_teachers WHERE id = 71 AND deleted_at IS NOT NULL`).Scan(&deletedCount).Error; err != nil {
		t.Fatalf("check soft delete: %v", err)
	}

	if deletedCount != 1 {
		t.Fatalf("expected assignment to be soft deleted, got deleted_count=%d", deletedCount)
	}
}
