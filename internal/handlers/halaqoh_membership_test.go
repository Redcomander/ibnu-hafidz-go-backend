package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/middleware"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func halaqohMembershipApp(t *testing.T) (*gorm.DB, *fiber.App) {
	t.Helper()
	db := securityDB(t)
	// Related classroom models use MySQL ENUM; these tests only need the
	// membership tables, not automatic migration of their entire relation graph.
	db.Config.IgnoreRelationshipsWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&models.Student{}, &models.HalaqohAssignment{}, &models.HalaqohAssignmentChangeRequest{}))
	teacherRole := models.Role{Name: "guru"}
	adminRole := models.Role{Name: "admin"}
	require.NoError(t, db.Create(&teacherRole).Error)
	require.NoError(t, db.Create(&adminRole).Error)
	users := []models.User{
		{ID: 1, Name: "Owner", Username: "owner", Email: "owner@test", Password: "hash", Roles: []models.Role{teacherRole}},
		{ID: 2, Name: "Other", Username: "other", Email: "other@test", Password: "hash", Roles: []models.Role{teacherRole}},
		{ID: 3, Name: "Admin", Username: "admin", Email: "admin@test", Password: "hash", Roles: []models.Role{adminRole}},
	}
	for _, u := range users {
		require.NoError(t, db.Create(&u).Error)
	}
	secret := "PRIVATE-STUDENT-FIELD"
	for i := uint(1); i <= 5; i++ {
		require.NoError(t, db.Create(&models.Student{ID: i, NamaLengkap: "Student " + strconv.Itoa(int(i)), NIK: &secret, Alamat: &secret}).Error)
	}
	require.NoError(t, db.Create(&models.HalaqohAssignment{UserID: 1, StudentID: 1, Active: true}).Error)
	require.NoError(t, db.Create(&models.HalaqohAssignment{UserID: 2, StudentID: 3, Active: true}).Error)
	handler := NewHalaqohHandler(db)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(middleware.InjectDB(db))
	app.Use(func(c *fiber.Ctx) error {
		id, _ := strconv.ParseUint(c.Get("X-Test-User"), 10, 64)
		var user models.User
		if err := db.Preload("Roles.Permissions").First(&user, uint(id)).Error; err != nil {
			return fiber.NewError(401)
		}
		c.Locals("userID", user.ID)
		c.Locals("user", &user)
		return c.Next()
	})
	app.Get("/students", middleware.Permission("students.view"), NewStudentHandler(db).List)
	app.Get("/options", handler.StudentOptions)
	app.Post("/assignments", handler.CreateAssignment)
	app.Put("/assignments/:teacher_id", handler.UpdateAssignment)
	app.Delete("/assignments/:id", handler.DeleteAssignment)
	app.Delete("/assignments/teacher/:teacher_id", handler.DeleteAssignmentsByTeacher)
	app.Get("/requests", handler.ListAssignmentChangeRequests)
	app.Post("/requests/:id/approve", handler.ApproveAssignmentChangeRequest)
	app.Post("/requests/:id/reject", handler.RejectAssignmentChangeRequest)
	return db, app
}

func membershipRequest(t *testing.T, app *fiber.App, user uint, method, target, body string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", strconv.Itoa(int(user)))
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func membershipIDs(t *testing.T, db *gorm.DB, teacher uint) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&models.HalaqohAssignment{}).Where("user_id = ? AND active = ?", teacher, true).Order("student_id").Pluck("student_id", &ids).Error)
	return ids
}

func pendingMembershipID(t *testing.T, response *http.Response) uint {
	t.Helper()
	require.Equal(t, 202, response.StatusCode)
	var payload struct {
		ID uint `json:"request_id"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.NotZero(t, payload.ID)
	return payload.ID
}

func TestHalaqohRestrictedPickerAndOwnership(t *testing.T) {
	_, app := halaqohMembershipApp(t)
	require.Equal(t, 403, membershipRequest(t, app, 1, "GET", "/students", "").StatusCode)
	response := membershipRequest(t, app, 1, "GET", "/options", "")
	require.Equal(t, 200, response.StatusCode)
	var payload struct {
		Data    []map[string]interface{} `json:"data"`
		PerPage int                      `json:"per_page"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	ids := []uint{}
	for _, student := range payload.Data {
		ids = append(ids, uint(student["id"].(float64)))
		require.NotContains(t, student, "nik")
		require.NotContains(t, student, "alamat")
	}
	require.Equal(t, []uint{1}, ids, "no search must expose only the own roster")
	response = membershipRequest(t, app, 1, "GET", "/options?search=Student&per_page=1000", "")
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Len(t, payload.Data, 5, "meaningful search can find any student")
	require.Equal(t, 20, payload.PerPage)
	for _, student := range payload.Data {
		require.NotContains(t, student, "nik")
		require.NotContains(t, student, "alamat")
	}
	response = membershipRequest(t, app, 1, "GET", "/options?search=%25%25", "")
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Empty(t, payload.Data, "wildcard-only input must not enumerate the directory")
	require.Equal(t, 403, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":2,"student_ids":[2]}`).StatusCode)
	require.Equal(t, 403, membershipRequest(t, app, 1, "PUT", "/assignments/2", `{"user_id":2,"student_ids":[2]}`).StatusCode)
	require.Equal(t, 403, membershipRequest(t, app, 1, "DELETE", "/assignments/2", "").StatusCode)
	require.Equal(t, 400, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[999]}`).StatusCode)
}

func TestHalaqohTransferRequiresExplicitAdminApproval(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	id := pendingMembershipID(t, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[3]}`))
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	require.Equal(t, []uint{3}, membershipIDs(t, db, 2))
	target := "/requests/" + strconv.Itoa(int(id)) + "/approve"
	require.Equal(t, 403, membershipRequest(t, app, 1, "POST", target, `{"confirm_transfers":true}`).StatusCode)
	require.Equal(t, 409, membershipRequest(t, app, 3, "POST", target, "").StatusCode)
	require.Equal(t, []uint{3}, membershipIDs(t, db, 2))
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", target, `{"confirm_transfers":true}`).StatusCode)
	require.Equal(t, []uint{1, 3}, membershipIDs(t, db, 1))
	require.Empty(t, membershipIDs(t, db, 2))
	var original models.HalaqohAssignment
	require.NoError(t, db.First(&original, uint(2)).Error)
	require.EqualValues(t, 2, original.UserID, "transfer must preserve the source teacher on historical assignments")
	require.False(t, original.Active)
}

func TestHalaqohAddRequiresAdminAndPreservesExistingStudents(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	id := pendingMembershipID(t, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`))
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	require.Equal(t, 409, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[4]}`).StatusCode)
	target := "/requests/" + strconv.Itoa(int(id)) + "/approve"
	require.Equal(t, 403, membershipRequest(t, app, 1, "POST", target, "").StatusCode)
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", target, "").StatusCode)
	require.Equal(t, []uint{1, 2}, membershipIDs(t, db, 1))
	require.Equal(t, []uint{3}, membershipIDs(t, db, 2))
	require.Equal(t, 409, membershipRequest(t, app, 3, "POST", target, "").StatusCode)
	require.Equal(t, 409, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/reject", `{"reason":"late"}`).StatusCode)
	response := membershipRequest(t, app, 2, "GET", "/requests", "")
	var payload struct {
		Data []models.HalaqohAssignmentChangeRequest `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Empty(t, payload.Data)
	response = membershipRequest(t, app, 1, "GET", "/requests", "")
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Len(t, payload.Data, 1)
	require.Len(t, payload.Data[0].StudentDetails, 2)
	require.Len(t, payload.Data[0].PreviousStudentDetails, 1)
	require.Empty(t, payload.Data[0].Reviewer.Email, "reviewer metadata must not expose private user fields")
	for _, student := range payload.Data[0].StudentDetails {
		require.Nil(t, student.NIK)
		require.Nil(t, student.Alamat)
	}
}

func TestHalaqohEditAndDeleteStayPendingAndRejectionPreservesRoster(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	id := pendingMembershipID(t, membershipRequest(t, app, 1, "PUT", "/assignments/1", `{"user_id":1,"student_ids":[2]}`))
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/reject", `{"reason":"Keep current roster"}`).StatusCode)
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	id = pendingMembershipID(t, membershipRequest(t, app, 1, "DELETE", "/assignments/1", ""))
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/approve", "").StatusCode)
	require.Empty(t, membershipIDs(t, db, 1))
	var total int64
	require.NoError(t, db.Model(&models.Student{}).Count(&total).Error)
	require.EqualValues(t, 5, total, "membership deletion must not delete master students")
}

func TestHalaqohApprovalRejectsStaleAndTransferredStudents(t *testing.T) {
	for _, kind := range []string{"roster", "student-transfer"} {
		t.Run(kind, func(t *testing.T) {
			db, app := halaqohMembershipApp(t)
			id := pendingMembershipID(t, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`))
			if kind == "roster" {
				require.NoError(t, db.Create(&models.HalaqohAssignment{UserID: 1, StudentID: 4, Active: true}).Error)
			} else {
				require.NoError(t, db.Create(&models.HalaqohAssignment{UserID: 2, StudentID: 2, Active: true}).Error)
			}
			before := membershipIDs(t, db, 1)
			require.Equal(t, 409, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/approve", "").StatusCode)
			require.Equal(t, before, membershipIDs(t, db, 1))
			var request models.HalaqohAssignmentChangeRequest
			require.NoError(t, db.First(&request, id).Error)
			require.Equal(t, "pending", request.Status)
		})
	}
}

func TestHalaqohApprovalRollsBackOnFailure(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	id := pendingMembershipID(t, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test_membership_failure", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "HalaqohAssignment" {
			tx.AddError(errors.New("simulated assignment persistence failure"))
		}
	}))
	require.Equal(t, 500, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/approve", "").StatusCode)
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
	var request models.HalaqohAssignmentChangeRequest
	require.NoError(t, db.First(&request, id).Error)
	require.Equal(t, "pending", request.Status)
}

func TestHalaqohEditAndDeleteAllApproval(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	id := pendingMembershipID(t, membershipRequest(t, app, 1, "PUT", "/assignments/1", `{"user_id":1,"student_ids":[1,2]}`))
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/approve", "").StatusCode)
	require.Equal(t, []uint{1, 2}, membershipIDs(t, db, 1))
	id = pendingMembershipID(t, membershipRequest(t, app, 1, "DELETE", "/assignments/teacher/1", ""))
	require.Equal(t, []uint{1, 2}, membershipIDs(t, db, 1))
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", "/requests/"+strconv.Itoa(int(id))+"/approve", "").StatusCode)
	require.Empty(t, membershipIDs(t, db, 1))
	require.Equal(t, []uint{3}, membershipIDs(t, db, 2))
}

func TestHalaqohAdminCanStillManageRosterDirectly(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	require.Equal(t, 200, membershipRequest(t, app, 3, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`).StatusCode)
	require.Equal(t, []uint{1, 2}, membershipIDs(t, db, 1))
	require.Equal(t, 200, membershipRequest(t, app, 3, "PUT", "/assignments/1", `{"user_id":1,"student_ids":[2]}`).StatusCode)
	require.Equal(t, []uint{2}, membershipIDs(t, db, 1))
	var pending int64
	require.NoError(t, db.Model(&models.HalaqohAssignmentChangeRequest{}).Count(&pending).Error)
	require.Zero(t, pending)
	var assignment models.HalaqohAssignment
	require.NoError(t, db.Where("user_id = ? AND active = ?", 1, true).First(&assignment).Error)
	require.Equal(t, 200, membershipRequest(t, app, 3, "DELETE", "/assignments/"+strconv.Itoa(int(assignment.ID)), "").StatusCode)
	require.Empty(t, membershipIDs(t, db, 1))
}
