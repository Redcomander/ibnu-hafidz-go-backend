package handlers

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeHalaqohGenderFilter(t *testing.T) {
	cases := map[string]string{
		"banin":     "banin",
		"Banin":     "banin",
		"putra":     "banin",
		"L":         "banin",
		"lakilaki":  "banin",
		"banat":     "banat",
		"Putri":     "banat",
		"P":         "banat",
		"perempuan": "banat",
		"female":    "banat",
		"":          "",
		"unknown":   "",
	}

	for input, want := range cases {
		if got := normalizeHalaqohGenderFilter(input); got != want {
			t.Fatalf("normalizeHalaqohGenderFilter(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStudentGenderFilterValues(t *testing.T) {
	if got := studentGenderFilterValues("banin"); len(got) == 0 {
		t.Fatal("studentGenderFilterValues('banin') should not be empty")
	}

	if got := studentGenderFilterValues("banat"); len(got) == 0 {
		t.Fatal("studentGenderFilterValues('banat') should not be empty")
	}

	if got := studentGenderFilterValues("unknown"); len(got) != 0 {
		t.Fatalf("studentGenderFilterValues('unknown') should be empty, got %#v", got)
	}
}

func TestHalaqohDutyStatusLabels(t *testing.T) {
	for _, raw := range []string{"Dinas", "Dinas Luar", "dinas luar", " dinas "} {
		if got := capitalize(raw); got != "Dinas" {
			t.Fatalf("%q => %q", raw, got)
		}
	}
}

func TestHalaqohDinasReportAndExportDoNotRewriteHistory(t *testing.T) {
	db := securityDB(t)
	db.Config.IgnoreRelationshipsWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&models.HalaqohTeacherAttendance{}))
	require.NoError(t, db.Exec(`CREATE TABLE halaqoh_substitute_logs (id INTEGER PRIMARY KEY, original_teacher_id INTEGER, substitute_teacher_id INTEGER, date DATE, session TEXT, status TEXT, reason TEXT)`).Error)
	require.NoError(t, db.Create(&models.User{ID: 1, Name: "Guru"}).Error)
	require.NoError(t, db.Create(&models.HalaqohTeacherAttendance{UserID: 1, Date: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), Session: "Shubuh", Status: "Dinas Luar"}).Error)
	handler := NewHalaqohStatsHandler(db)
	app := fiber.New()
	app.Get("/stats", handler.TeacherStatistics)
	res, err := app.Test(httptest.NewRequest("GET", "/stats?start_date=2026-11-01&end_date=2026-11-30", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var body struct {
		Stats    map[string]int `json:"stats"`
		Teachers []struct {
			Dinas int `json:"dinas"`
		} `json:"teachers"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, 1, body.Stats["Dinas"])
	require.Len(t, body.Teachers, 1)
	require.Equal(t, 1, body.Teachers[0].Dinas)
	exported, _, err := handler.fetchTeacherData("2026-11-01", "2026-11-30", "", "")
	require.NoError(t, err)
	require.Len(t, exported, 1)
	require.Equal(t, 1, exported[0].Dinas)
	var saved models.HalaqohTeacherAttendance
	require.NoError(t, db.First(&saved).Error)
	require.Equal(t, "Dinas Luar", saved.Status)
}
