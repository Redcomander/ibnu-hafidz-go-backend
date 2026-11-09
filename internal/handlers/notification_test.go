package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNotificationCountsPaginationAndReadOwnership(t *testing.T) {
	db := securityDB(t)
	db.Config.IgnoreRelationshipsWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&models.Notification{}))
	h := NewNotificationHandler(db, &config.Config{JWTSecret: "test"})
	for i := 0; i < 15; i++ {
		require.NoError(t, h.SendNotification(1, "Notice", "Message", "info", "/dashboard"))
	}
	require.NoError(t, h.SendNotification(2, "Other account", "Private", "info", "/dashboard"))
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(func(c *fiber.Ctx) error { c.Locals("userID", uint(1)); return c.Next() })
	app.Get("/notifications", h.List)
	app.Put("/notifications/:id/read", h.MarkRead)
	app.Post("/ticket", h.IssueSSETicket)
	call := func(method, path string, want int) map[string]any {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(method, path, nil))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, want, resp.StatusCode)
		data := map[string]any{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
		return data
	}
	data := call("GET", "/notifications?limit=10", 200)
	require.EqualValues(t, 15, data["unread_count"])
	require.Len(t, data["data"], 10)
	require.EqualValues(t, 2, data["last_page"])
	require.Len(t, call("GET", "/notifications?page=2&limit=10", 200)["data"], 5)
	for _, path := range []string{"/notifications?limit=0", "/notifications?limit=-1", "/notifications?page=0", "/notifications?limit=101"} {
		call("GET", path, 400)
	}
	call("PUT", "/notifications/16/read", 404)
	call("PUT", "/notifications/invalid/read", 400)
	require.EqualValues(t, 14, call("PUT", "/notifications/1/read", 200)["unread_count"])
	require.EqualValues(t, 0, call("PUT", "/notifications/all/read", 200)["unread_count"])
	var other models.Notification
	require.NoError(t, db.First(&other, uint(16)).Error)
	require.False(t, other.IsRead)
	ticket := call("POST", "/ticket", 200)["ticket"].(string)
	_, err := utils.ValidateToken(ticket, "test")
	require.Error(t, err, "stream ticket must never grant ordinary API access")
	user, ok := h.consumeTicket(ticket)
	require.True(t, ok)
	require.EqualValues(t, 1, user)
	_, ok = h.consumeTicket(ticket)
	require.False(t, ok, "ticket must be single-use")
	h.tickets["expired"] = sseTicket{UserID: 1, Expires: time.Now().Add(-time.Second)}
	_, ok = h.consumeTicket("expired")
	require.False(t, ok)
}

func TestBroadcastBuffersBurstAndIsolatesRecipients(t *testing.T) {
	h := NewNotificationHandler(nil, nil)
	first := make(chan string, 32)
	second := make(chan string, 32)
	h.clients[1] = map[chan string]bool{first: true}
	h.clients[2] = map[chan string]bool{second: true}
	for i := 0; i < 5; i++ {
		h.Broadcast(models.Notification{ID: uint(i + 1), UserID: 1, Title: "Notice"})
	}
	require.Len(t, first, 5)
	require.Empty(t, second)
	for i := 0; i < 100; i++ {
		h.Broadcast(models.Notification{UserID: 1})
	}
	require.Len(t, first, 32, "slow clients must not block request handlers")
}

func TestHalaqohNotificationsPersistWithSubmissionAndReview(t *testing.T) {
	for _, decision := range []string{"approve", "reject"} {
		t.Run(decision, func(t *testing.T) {
			db, app := halaqohMembershipApp(t)
			id := pendingMembershipID(t, membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`))
			var notices []models.Notification
			require.NoError(t, db.Order("id").Find(&notices).Error)
			require.Len(t, notices, 1)
			require.EqualValues(t, 3, notices[0].UserID)
			require.Equal(t, "/dashboard/halaqoh/validasi", *notices[0].ActionURL)
			require.Contains(t, notices[0].Message, fmt.Sprintf("#%d", id))
			url := "/requests/" + strconv.Itoa(int(id)) + "/" + decision
			require.Equal(t, 200, membershipRequest(t, app, 3, "POST", url, `{"reason":"Please revise"}`).StatusCode)
			require.NoError(t, db.Order("id").Find(&notices).Error)
			require.Len(t, notices, 2)
			require.EqualValues(t, 1, notices[1].UserID)
			require.Equal(t, "/dashboard/halaqoh-assignments", *notices[1].ActionURL)
			require.Equal(t, 409, membershipRequest(t, app, 3, "POST", url, `{}`).StatusCode)
			var count int64
			require.NoError(t, db.Model(&models.Notification{}).Count(&count).Error)
			require.EqualValues(t, 2, count, "repeated review must not duplicate notifications")
		})
	}
}

func TestMembershipRollsBackWhenNotificationPersistenceFails(t *testing.T) {
	db, app := halaqohMembershipApp(t)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_notification", func(tx *gorm.DB) {
		if tx.Statement.Table == "notifications" {
			tx.AddError(errors.New("notification storage failure"))
		}
	}))
	response := membershipRequest(t, app, 1, "POST", "/assignments", `{"user_id":1,"student_ids":[2]}`)
	require.Equal(t, 500, response.StatusCode)
	var count int64
	require.NoError(t, db.Model(&models.HalaqohAssignmentChangeRequest{}).Count(&count).Error)
	require.Zero(t, count, "a committed request must never silently lose its notification")
	require.Equal(t, []uint{1}, membershipIDs(t, db, 1))
}
