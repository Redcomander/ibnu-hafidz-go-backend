package handlers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/diagnostics"
	"github.com/ibnu-hafidz/web-v2/internal/maintenance"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type NotificationHandler struct {
	db      *gorm.DB
	cfg     *config.Config
	clients map[uint]map[chan string]bool // map[UserID]map[Channel]bool
	mu      sync.Mutex
	tickets map[string]sseTicket
}

func NewNotificationHandler(db *gorm.DB, cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{
		db:      db,
		cfg:     cfg,
		clients: make(map[uint]map[chan string]bool),
		tickets: make(map[string]sseTicket),
	}
}

type sseTicket struct {
	UserID  uint
	Expires time.Time
}

// Tickets are opaque, short-lived and consumed once. They cannot authenticate
// ordinary API calls and never expose a reusable access token in the URL.
func (h *NotificationHandler) IssueSSETicket(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return fiber.NewError(401, "Authentication required")
	}
	ticket := diagnostics.NewID()
	h.mu.Lock()
	for key, value := range h.tickets {
		if !value.Expires.After(time.Now()) {
			delete(h.tickets, key)
		}
	}
	h.tickets[ticket] = sseTicket{UserID: userID, Expires: time.Now().Add(30 * time.Second)}
	h.mu.Unlock()
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"ticket": ticket, "expires_in": 30})
}

func (h *NotificationHandler) consumeTicket(token string) (uint, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ticket, exists := h.tickets[token]
	delete(h.tickets, token)
	return ticket.UserID, exists && ticket.Expires.After(time.Now())
}

func (h *NotificationHandler) unread(userID uint) (int64, error) {
	var count int64
	err := h.db.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&count).Error
	return count, err
}

func (h *NotificationHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 || page > 1000000 {
		return fiber.NewError(400, "Invalid page")
	}
	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil || limit < 1 || limit > 100 {
		return fiber.NewError(400, "Limit must be between 1 and 100")
	}
	notifications := []models.Notification{}
	var total int64
	if err := h.db.Model(&models.Notification{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return err
	}
	if err := h.db.Where("user_id = ?", userID).Order("created_at desc, id desc").Limit(limit).Offset((page - 1) * limit).Find(&notifications).Error; err != nil {
		return err
	}
	unread, err := h.unread(userID)
	if err != nil {
		return err
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"data": notifications, "total": total, "unread_count": unread, "page": page, "last_page": (total + int64(limit) - 1) / int64(limit)})
}

func (h *NotificationHandler) MarkRead(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	id := c.Params("id")
	query := h.db.Model(&models.Notification{}).Where("user_id = ?", userID)
	if id == "all" {
		query = query.Where("is_read = ?", false)
	} else {
		parsed, err := strconv.ParseUint(id, 10, 64)
		if err != nil || parsed == 0 {
			return fiber.NewError(400, "Invalid notification ID")
		}
		var existing models.Notification
		if err := h.db.Where("id = ? AND user_id = ?", parsed, userID).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fiber.NewError(404, "Notification not found")
			}
			return err
		}
		query = query.Where("id = ?", parsed)
	}
	if err := query.Update("is_read", true).Error; err != nil {
		return err
	}
	count, err := h.unread(userID)
	if err != nil {
		return err
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"message": "Notifications marked as read", "unread_count": count})
}

// Stream handles Server-Sent Events (SSE)
func (h *NotificationHandler) Stream(c *fiber.Ctx) error {
	userID, valid := h.consumeTicket(c.Query("token"))
	if !valid {
		return fiber.NewError(401, "Invalid or expired stream ticket")
	}
	c.Locals("userID", userID)
	var user models.User
	if err := h.db.Preload("Roles").First(&user, userID).Error; err != nil {
		return fiber.NewError(401, "User unavailable")
	}
	settings, err := maintenance.Read(h.db)
	if err != nil {
		return fiber.NewError(503, "Maintenance settings unavailable")
	}
	if settings.Enabled && settings.Scope == "all" && !maintenance.IsAdmin(&user) {
		c.Set("Retry-After", "60")
		return c.Status(503).JSON(fiber.Map{"error": "maintenance", "message": settings.Message})
	}
	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	messageChan := make(chan string, 32)

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

		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
		lifetime := time.NewTimer(5 * time.Minute)
		defer lifetime.Stop()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-lifetime.C:
				return
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

	h.Broadcast(notif)
	return nil
}

// Broadcast happens only after the notification row's transaction commits.
func (h *NotificationHandler) Broadcast(notif models.Notification) {
	payload, err := json.Marshal(notif)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel := range h.clients[notif.UserID] {
		select {
		case channel <- string(payload):
		default:
		} // DB polling recovers overflow.
	}
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
		if err := h.SendNotification(user.ID, title, message, notifType, actionURL); err != nil {
			return err
		}
	}

	return nil
}
