package handlers

import (
	"testing"

	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestHalaqohTimPresensiSubstituteAccess(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	require.NoError(t, db.Exec(`CREATE TABLE halaqoh_substitute_logs (
 id INTEGER PRIMARY KEY, halaqoh_assignment_id INTEGER, original_teacher_id INTEGER,
 substitute_teacher_id INTEGER, date DATE, session TEXT, status TEXT, reason TEXT,
 is_active BOOLEAN, created_at DATETIME, updated_at DATETIME)`).Error)
	role := models.Role{Name: "tim_presensi"}
	require.NoError(t, db.Create(&role).Error)
	user := models.User{ID: 4, Name: "Tim", Username: "tim", Email: "tim@test", Password: "hash", Roles: []models.Role{role}}
	require.NoError(t, db.Create(&user).Error)
	require.True(t, canSubmitHalaqohTeacherAttendance(&user))
	require.True(t, canAssignHalaqohSubstitute(&user))
	require.False(t, isHalaqohAdmin(&user), "attendance role must not become a membership administrator")
	handler := NewHalaqohHandler(db)
	app.Post("/assignments/:id/substitute", handler.AssignSubstitute)
	app.Delete("/assignments/:id/substitute", handler.UnassignSubstitute)
	body := `{"substitute_teacher_id":2,"substitute_date":"2026-11-11","sessions":["Shubuh"],"substitute_status":"Izin"}`
	for _, method := range []string{"POST", "DELETE"} {
		response := membershipRequest(t, app, 1, method, "/assignments/1/substitute", body)
		response.Body.Close()
		require.Equal(t, 403, response.StatusCode, "owning a halaqoh does not grant substitute management")
	}
	response := membershipRequest(t, app, 4, "POST", "/assignments/1/substitute", body)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	var count int64
	require.NoError(t, db.Model(&models.HalaqohSubstituteLog{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// SQLite stores Go dates with a time component; MariaDB DATE stores only the date.
	require.NoError(t, db.Exec("UPDATE halaqoh_substitute_logs SET date = '2026-11-11'").Error)
	response = membershipRequest(t, app, 4, "DELETE", "/assignments/1/substitute", `{"date":"2026-11-11","sessions":["Shubuh"]}`)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, db.Model(&models.HalaqohSubstituteLog{}).Count(&count).Error)
	require.Zero(t, count)
}
