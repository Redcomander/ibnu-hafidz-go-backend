package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// setupTeacherStatisticsSchema creates minimal SQLite tables mirroring only the columns
// GetTeacherStatistics touches via raw SQL joins (never GORM preloading), so we don't have
// to pull in the full production schema (which relies on MySQL-only enum syntax).
func setupTeacherStatisticsSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, foto_guru TEXT, gender TEXT)`,
		`CREATE TABLE jadwal_formal (id INTEGER PRIMARY KEY, type TEXT DEFAULT 'normal', lesson_kelas_teacher_id INTEGER, hari TEXT, jam_mulai TEXT, jam_selesai TEXT, substitute_teacher_id INTEGER, substitute_date DATE, deleted_at DATETIME)`,
		`CREATE TABLE teacher_attendances (id INTEGER PRIMARY KEY, jadwal_formal_id INTEGER, jadwal_diniyyah_id INTEGER, user_id INTEGER, date DATE, status TEXT, notes TEXT, deleted_at DATETIME)`,
		`CREATE TABLE teacher_attendance_snapshots (id INTEGER PRIMARY KEY, teacher_attendance_id INTEGER, lesson TEXT, kelas TEXT)`,
		`CREATE TABLE substitute_logs (id INTEGER PRIMARY KEY, jadwal_formal_id INTEGER, jadwal_diniyyah_id INTEGER, original_teacher_id INTEGER, substitute_teacher_id INTEGER, date DATE, jam_mulai TEXT, jam_selesai TEXT, status TEXT, reason TEXT, deleted_at DATETIME)`,
		`CREATE TABLE substitute_log_snapshots (id INTEGER PRIMARY KEY, substitute_log_id INTEGER, lesson TEXT, kelas TEXT, jam_mulai TEXT, jam_selesai TEXT, original_teacher TEXT)`,
		`CREATE TABLE lesson_kelas_teachers (id INTEGER PRIMARY KEY, lesson_id INTEGER, kelas_id INTEGER)`,
		`CREATE TABLE lessons (id INTEGER PRIMARY KEY, nama TEXT)`,
		`CREATE TABLE kelas (id INTEGER PRIMARY KEY, nama TEXT, tingkat TEXT, gender TEXT)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table (%s): %v", stmt, err)
		}
	}
}

// TestGetTeacherStatisticsCountsSubstituteAndIzinFromLogOnly reproduces the reported bug:
// a formal substitute assignment (recorded only in substitute_logs, with no matching
// teacher_attendances row) must still show up as a non-zero Substitute count for the
// substitute teacher and a non-zero Izin count for the original absent teacher, for the
// exact date range requested by the frontend.
func TestGetTeacherStatisticsCountsDinasLuarAsDinas(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}
	setupTeacherStatisticsSchema(t, db)

	if err := db.Exec(`INSERT INTO users (id, name) VALUES (1, 'Original Teacher')`).Error; err != nil {
		t.Fatalf("seed original teacher: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (id, name) VALUES (2, 'Substitute Teacher')`).Error; err != nil {
		t.Fatalf("seed substitute teacher: %v", err)
	}
	if err := db.Exec(`INSERT INTO jadwal_formal (id, lesson_kelas_teacher_id, hari, jam_mulai, jam_selesai) VALUES (10, 1, 'Senin', '08:00:00', '09:30:00')`).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	if err := db.Exec(`INSERT INTO substitute_logs (jadwal_formal_id, original_teacher_id, substitute_teacher_id, date, status, reason) VALUES (10, 1, 2, '2026-09-03', 'Dinas Luar', 'Kegiatan dinas')`).Error; err != nil {
		t.Fatalf("seed substitute log: %v", err)
	}

	handler := NewAbsensiHandler(db)
	app := fiber.New()
	app.Get("/attendance/teacher-statistics", handler.GetTeacherStatistics)

	req := httptest.NewRequest(http.MethodGet, "/attendance/teacher-statistics?type=formal&start_date=2026-09-01&end_date=2026-09-30", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var payload struct {
		TeacherCounts map[string]int `json:"teacher_counts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.TeacherCounts["Dinas"] != 2 {
		t.Fatalf("expected Dinas count 2, got %d (payload: %+v)", payload.TeacherCounts["Dinas"], payload.TeacherCounts)
	}
	if payload.TeacherCounts["Izin"] != 0 {
		t.Fatalf("expected Izin count 0 when status is Dinas Luar, got %d (payload: %+v)", payload.TeacherCounts["Izin"], payload.TeacherCounts)
	}
	if payload.TeacherCounts["Substitute"] != 2 {
		t.Fatalf("expected Substitute count 2, got %d (payload: %+v)", payload.TeacherCounts["Substitute"], payload.TeacherCounts)
	}
}

func TestGetTeacherStatisticsCountsSubstituteAndIzinFromLogOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory db: %v", err)
	}
	setupTeacherStatisticsSchema(t, db)

	if err := db.Exec(`INSERT INTO users (id, name) VALUES (1, 'Original Teacher')`).Error; err != nil {
		t.Fatalf("seed original teacher: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (id, name) VALUES (2, 'Substitute Teacher')`).Error; err != nil {
		t.Fatalf("seed substitute teacher: %v", err)
	}
	// Schedule spans 08:00-09:30 (>80 minutes), so both the absence and the substitute
	// assignment must be counted as 2 session units per the formal counting rule.
	if err := db.Exec(`INSERT INTO jadwal_formal (id, lesson_kelas_teacher_id, hari, jam_mulai, jam_selesai) VALUES (10, 1, 'Senin', '08:00:00', '09:30:00')`).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	if err := db.Exec(`INSERT INTO substitute_logs (jadwal_formal_id, original_teacher_id, substitute_teacher_id, date, status, reason) VALUES (10, 1, 2, '2026-09-03', 'Izin', 'Sakit mendadak')`).Error; err != nil {
		t.Fatalf("seed substitute log: %v", err)
	}

	handler := NewAbsensiHandler(db)
	app := fiber.New()
	app.Get("/attendance/teacher-statistics", handler.GetTeacherStatistics)

	req := httptest.NewRequest(http.MethodGet, "/attendance/teacher-statistics?type=formal&start_date=2026-09-01&end_date=2026-09-30", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var payload struct {
		TeacherCounts  map[string]int `json:"teacher_counts"`
		TeacherSummary []struct {
			ID         uint   `json:"id"`
			Name       string `json:"name"`
			Izin       int    `json:"izin"`
			Substitute int    `json:"substitute"`
		} `json:"teacher_summary"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.TeacherCounts["Substitute"] != 2 {
		t.Fatalf("expected global Substitute count 2, got %d (payload: %+v)", payload.TeacherCounts["Substitute"], payload.TeacherCounts)
	}
	if payload.TeacherCounts["Izin"] != 2 {
		t.Fatalf("expected global Izin count 2, got %d (payload: %+v)", payload.TeacherCounts["Izin"], payload.TeacherCounts)
	}

	var foundSubstitute, foundOriginal bool
	for _, entry := range payload.TeacherSummary {
		if entry.ID == 2 {
			foundSubstitute = true
			if entry.Substitute != 2 {
				t.Fatalf("expected substitute teacher Substitute=2, got %d", entry.Substitute)
			}
		}
		if entry.ID == 1 {
			foundOriginal = true
			if entry.Izin != 2 {
				t.Fatalf("expected original teacher Izin=2, got %d", entry.Izin)
			}
		}
	}
	if !foundSubstitute {
		t.Fatalf("substitute teacher entry missing from teacher_summary: %+v", payload.TeacherSummary)
	}
	if !foundOriginal {
		t.Fatalf("original teacher entry missing from teacher_summary: %+v", payload.TeacherSummary)
	}
}
