package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

type NotificationHandler struct {
	db      *gorm.DB
	cfg     *config.Config
	clients map[uint]map[chan string]bool // map[UserID]map[Channel]bool
	mu      sync.Mutex
}

func NewNotificationHandler(db *gorm.DB, cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{
		db:      db,
		cfg:     cfg,
		clients: make(map[uint]map[chan string]bool),
	}
}

// IssueSSETicket issues a short-lived (30-second) JWT for SSE authentication.
// The client uses this token in the SSE URL query param (?token=...) so that
// the long-lived access token never appears in server logs or browser history.
func (h *NotificationHandler) IssueSSETicket(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	email, _ := c.Locals("email").(string)

	token, err := utils.GenerateSSEToken(userID, email, h.cfg.JWTSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "token_error",
			Message: "Failed to generate SSE ticket",
		})
	}

	return c.JSON(fiber.Map{
		"ticket":     token,
		"expires_in": 30,
	})
}

// List returns notifications for the current user
func (h *NotificationHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)
	offset := (page - 1) * limit

	var notifications []models.Notification
	var total int64

	h.db.Model(&models.Notification{}).
		Where("user_id = ?", userID).
		Count(&total)

	h.db.Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&notifications)

	// Count unread
	var unreadCount int64
	h.db.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&unreadCount)

	return c.JSON(fiber.Map{
		"data":         notifications,
		"total":        total,
		"unread_count": unreadCount,
		"page":         page,
		"last_page":    (int(total) + limit - 1) / limit,
	})
}

// MarkRead marks notifications as read
func (h *NotificationHandler) MarkRead(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	idStr := c.Params("id")

	query := h.db.Model(&models.Notification{}).Where("user_id = ?", userID)

	if idStr == "all" {
		query.Where("is_read = ?", false).Update("is_read", true)
	} else {
		query.Where("id = ?", idStr).Update("is_read", true)
	}

	return c.JSON(fiber.Map{"message": "Notifications marked as read"})
}

// Stream handles Server-Sent Events (SSE)
func (h *NotificationHandler) Stream(c *fiber.Ctx) error {
	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	userID := c.Locals("userID").(uint)
	messageChan := make(chan string)

	// Register client
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan string]bool)
	}
	h.clients[userID][messageChan] = true
	h.mu.Unlock()

	// Keep connection open and write events.
	// NOTE: cleanup must happen INSIDE the writer closure — the defer in the
	// outer handler runs on "return nil" (before the writer goroutine starts),
	// which would close messageChan immediately and cause a 0s disconnect loop.
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			h.mu.Lock()
			delete(h.clients[userID], messageChan)
			if len(h.clients[userID]) == 0 {
				delete(h.clients, userID)
			}
			h.mu.Unlock()
			close(messageChan)
		}()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-messageChan:
				if !ok {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}

			case <-ticker.C:
				// SSE comment heartbeat to keep idle connections alive.
				// Client disconnect is detected by the Flush error below.
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}

// SendNotification saves to DB and broadcasts to SSE clients
func (h *NotificationHandler) SendNotification(userID uint, title, message, notifType, actionURL string) error {
	// 1. Save to DB
	notif := models.Notification{
		UserID:    userID,
		Title:     title,
		Message:   message,
		Type:      notifType,
		IsRead:    false,
		ActionURL: &actionURL,
	}

	if err := h.db.Create(&notif).Error; err != nil {
		return err
	}

	// 2. Broadcast to connected clients
	payload, _ := json.Marshal(notif)
	msg := string(payload)

	h.mu.Lock()
	if clients, ok := h.clients[userID]; ok {
		for clientChan := range clients {
			select {
			case clientChan <- msg:
			default:
				// Skip if channel is blocked
			}
		}
	}
	h.mu.Unlock()

	return nil
}

// SendToRole sends a notification to all users with a specific role
func (h *NotificationHandler) SendToRole(roleName, title, message, notifType, actionURL string) error {
	var users []models.User
	if err := h.db.Joins("JOIN role_user ON role_user.user_id = users.id").
		Joins("JOIN roles ON roles.id = role_user.role_id").
		Where("roles.name = ?", roleName).
		Find(&users).Error; err != nil {
		return err
	}

	for _, user := range users {
		// Run in goroutine to not block
		go h.SendNotification(user.ID, title, message, notifType, actionURL)
	}

	return nil
}
