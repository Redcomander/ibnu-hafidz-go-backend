package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type ActivityLogHandler struct {
	db *gorm.DB
}

func NewActivityLogHandler(db *gorm.DB) *ActivityLogHandler {
	return &ActivityLogHandler{db: db}
}

// List returns paginated user activity logs.
func (h *ActivityLogHandler) List(c *fiber.Ctx) error {
	var logs []models.UserActivityLog
	var total int64

	query := h.db.Model(&models.UserActivityLog{})

	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if userName := strings.TrimSpace(c.Query("user_name")); userName != "" {
		query = query.Joins("LEFT JOIN users ON users.id = user_activity_logs.user_id").Where("users.name LIKE ?", "%"+userName+"%")
	}
	if method := c.Query("method"); method != "" {
		query = query.Where("method = ?", method)
	}

	query.Count(&total)

	paginatedQuery, page, perPage := PaginateQuery(c, query, []string{"path", "method"})
	paginatedQuery.Preload("User").Find(&logs)

	return c.JSON(BuildPaginatedResponse(logs, total, page, perPage))
}
