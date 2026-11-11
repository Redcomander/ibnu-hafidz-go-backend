package handlers

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestStudentProgressPermissionAndModuleBoundaries(t *testing.T) {
	db := securityDB(t)
	db.Config.IgnoreRelationshipsWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&models.Student{}))
	require.NoError(t, db.Exec("CREATE TABLE absensis (id INTEGER PRIMARY KEY, student_id INTEGER, tanggal DATETIME, status TEXT, catatan TEXT, deleted_at DATETIME)").Error)
	require.NoError(t, db.Create(&models.Student{ID: 1, NamaLengkap: "Ahmad"}).Error)
	for _, row := range []string{"('2026-11-01','hadir')", "('2026-11-02','alpa')", "('2026-11-03','alpa')", "('2026-11-04','alpha')"} {
		require.NoError(t, db.Exec("INSERT INTO absensis(student_id,tanggal,status,catatan) SELECT 1,column1,column2,'Catatan guru' FROM (VALUES "+row+")").Error)
	}
	user := &models.User{ID: 5, Roles: []models.Role{{Name: "teacher", Permissions: []models.Permission{{Name: "students.view"}, {Name: "absensi.view"}}}}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", user); return c.Next() })
	h := NewStudentHandler(db)
	app.Get("/students/signals", h.AttendanceSignals)
	app.Get("/students/:id/progress", h.Progress)
	res, err := app.Test(httptest.NewRequest("GET", "/students/1/progress?start_date=2026-11-01&end_date=2026-11-30", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var body struct {
		Modules []string         `json:"modules"`
		Counts  []progressCount  `json:"counts"`
		History []progressRecord `json:"history"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, []string{"Formal"}, body.Modules)
	require.Len(t, body.Counts, 4)
	require.Len(t, body.History, 4)
	require.Equal(t, "Catatan guru", body.History[0].Notes)
	res, err = app.Test(httptest.NewRequest("GET", "/students/signals?start_date=2026-11-01&end_date=2026-11-30", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var signals struct {
		Data []struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&signals))
	require.Len(t, signals.Data, 1)
	require.Equal(t, 3, signals.Data[0].Count)
	for _, path := range []string{"/students/signals?module=Halaqoh", "/students/1/progress?start_date=invalid", "/students/1/progress?start_date=2020-01-01&end_date=2026-11-30"} {
		res, err = app.Test(httptest.NewRequest("GET", path, nil))
		require.NoError(t, err)
		require.Contains(t, []int{400, 403}, res.StatusCode)
		res.Body.Close()
	}
	user.Roles = nil
	res, err = app.Test(httptest.NewRequest("GET", "/students/1/progress", nil))
	require.NoError(t, err)
	require.Equal(t, 403, res.StatusCode)
	res.Body.Close()
	var count int64
	require.NoError(t, db.Table("absensis").Count(&count).Error)
	require.EqualValues(t, 4, count, "read endpoints must preserve records")
}
