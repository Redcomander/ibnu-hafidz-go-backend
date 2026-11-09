package handlers

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func TestDinasAndLegacyRecordsShareTotalsAndLabels(t *testing.T) {
	// Mirror the MySQL CONCAT used by the production history queries.
	if err := sqlitedriver.RegisterScalarFunction("CONCAT", -1, func(_ *sqlitedriver.FunctionContext, args []driver.Value) (driver.Value, error) {
		var result strings.Builder
		for _, arg := range args {
			if arg == nil {
				return nil, nil
			}
			result.WriteString(fmt.Sprint(arg))
		}
		return result.String(), nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, attendanceType := range []string{"formal", "ramadhan", "diniyyah"} {
		t.Run(attendanceType, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			setupTeacherStatisticsSchema(t, db)
			statements := []string{
				`CREATE TABLE jadwal_diniyyahs (id INTEGER PRIMARY KEY, diniyyah_kelas_teacher_id INTEGER, jam_mulai TEXT, jam_selesai TEXT)`,
				`CREATE TABLE diniyyah_kelas_teachers (id INTEGER PRIMARY KEY, diniyyah_lesson_id INTEGER, kelas_id INTEGER)`,
				`CREATE TABLE diniyyah_lessons (id INTEGER PRIMARY KEY, nama TEXT)`,
				`CREATE TABLE substitute_logs_diniyyah (id INTEGER PRIMARY KEY, jadwal_diniyyah_id INTEGER, original_teacher_id INTEGER, substitute_teacher_id INTEGER, date DATE, status TEXT, reason TEXT, jam_mulai TEXT, jam_selesai TEXT)`,
				`CREATE TABLE substitute_diniyyah_log_snapshots (id INTEGER PRIMARY KEY, substitute_diniyyah_log_id INTEGER, lesson TEXT, kelas TEXT, jam_mulai TEXT, jam_selesai TEXT, original_teacher TEXT)`,
				`INSERT INTO users (id, name) VALUES (1, 'Original Teacher'), (2, 'Substitute Teacher')`,
				`INSERT INTO kelas (id, nama, tingkat) VALUES (1, '7', 'A')`,
				`INSERT INTO lessons (id, nama) VALUES (1, 'Matematika')`,
				`INSERT INTO lesson_kelas_teachers (id, lesson_id, kelas_id) VALUES (1, 1, 1)`,
				`INSERT INTO diniyyah_lessons (id, nama) VALUES (1, 'Fiqih')`,
				`INSERT INTO diniyyah_kelas_teachers (id, diniyyah_lesson_id, kelas_id) VALUES (1, 1, 1)`,
				`INSERT INTO jadwal_diniyyahs (id, diniyyah_kelas_teacher_id, jam_mulai, jam_selesai) VALUES (10, 1, '13:00:00', '14:00:00')`,
				`INSERT INTO jadwal_formal (id, lesson_kelas_teacher_id, type, jam_mulai, jam_selesai) VALUES (10, 1, 'normal', '08:00:00', '09:30:00')`,
			}
			for _, statement := range statements {
				if err := db.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			column, logTable := "jadwal_formal_id", "substitute_logs"
			if attendanceType == "ramadhan" {
				db.Exec(`UPDATE jadwal_formal SET type = 'ramadhan'`)
			}
			if attendanceType == "diniyyah" {
				column, logTable = "jadwal_diniyyah_id", "substitute_logs_diniyyah"
			}
			for i, status := range []string{"Dinas", "Dinas Luar"} {
				if err := db.Exec(fmt.Sprintf(`INSERT INTO teacher_attendances (%s, user_id, date, status) VALUES (10, 1, ?, ?)`, column), fmt.Sprintf("2026-09-%02d", i+1), status).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Exec(fmt.Sprintf(`INSERT INTO %s (%s, original_teacher_id, substitute_teacher_id, date, status) VALUES (10, 1, 2, ?, ?)`, logTable, column), fmt.Sprintf("2026-09-%02d", i+3), status).Error; err != nil {
					t.Fatal(err)
				}
			}
			handler := NewAbsensiHandler(db)
			app := fiber.New()
			app.Get("/stats", handler.GetTeacherStatistics)
			app.Get("/excel", handler.ExportTeacherStatisticsExcel)
			query := "?type=" + attendanceType + "&start_date=2026-09-01&end_date=2026-09-30"
			res, err := app.Test(httptest.NewRequest("GET", "/stats"+query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var payload struct {
				Counts      map[string]int            `json:"teacher_counts"`
				Summary     []TeacherSummaryEntry     `json:"teacher_summary"`
				Absences    []struct{ Status string } `json:"absence_history"`
				Substitutes []struct {
					Status string `json:"original_status"`
				} `json:"substitute_history"`
			}
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			want := 8
			if attendanceType == "diniyyah" {
				want = 4
			}
			if payload.Counts["Dinas"] != want {
				t.Fatalf("Dinas = %d, want %d", payload.Counts["Dinas"], want)
			}
			var summaryTotal int
			for _, row := range payload.Summary {
				summaryTotal += row.Dinas
			}
			if summaryTotal != want {
				t.Fatalf("summary Dinas = %d, want %d", summaryTotal, want)
			}
			if len(payload.Absences) != 4 || len(payload.Substitutes) != 2 {
				t.Fatalf("missing history: %+v", payload)
			}
			for _, row := range payload.Absences {
				if row.Status != "Dinas" {
					t.Fatalf("old absence label: %q", row.Status)
				}
			}
			for _, row := range payload.Substitutes {
				if row.Status != "Dinas" {
					t.Fatalf("old substitute label: %q", row.Status)
				}
			}
			res, err = app.Test(httptest.NewRequest("GET", "/excel"+query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			book, err := excelize.OpenReader(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()
			value, err := book.GetCellValue(book.GetSheetName(0), "H6")
			if err != nil || value != fmt.Sprint(want) {
				t.Fatalf("Excel Dinas total = %q, want %d: %v", value, want, err)
			}
			for _, sheet := range book.GetSheetList() {
				rows, err := book.GetRows(sheet)
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					for _, cell := range row {
						if cell == "Dinas Luar" {
							t.Fatal("Excel retains old label")
						}
					}
				}
			}
			var legacy int64
			db.Table("teacher_attendances").Where("status = ?", "Dinas Luar").Count(&legacy)
			if legacy != 1 {
				t.Fatal("historical database value was rewritten")
			}
		})
	}
}

func TestTeacherAttendanceUpdateAcceptsDinasAndLegacyAlias(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	setupTeacherStatisticsSchema(t, db)
	db.Exec(`ALTER TABLE teacher_attendances ADD COLUMN updated_at DATETIME`)
	db.Exec(`INSERT INTO teacher_attendances (id, user_id, date, status) VALUES (1, 1, '2026-09-01', 'Izin')`)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{ID: 1, Roles: []models.Role{{Name: "admin"}}})
		return c.Next()
	})
	app.Put("/attendance/:id", NewAbsensiHandler(db).UpdateTeacherAttendanceRecord)
	for _, status := range []string{"Dinas", "Dinas Luar"} {
		req := httptest.NewRequest("PUT", "/attendance/1", strings.NewReader(fmt.Sprintf(`{"status":%q,"notes":"Duty"}`, status)))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s rejected: %d", status, res.StatusCode)
		}
		var stored string
		db.Table("teacher_attendances").Select("status").Where("id = 1").Scan(&stored)
		if stored != "Dinas" {
			t.Fatalf("saved %q, want Dinas", stored)
		}
	}
}
