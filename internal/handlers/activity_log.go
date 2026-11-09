package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

type ActivityLogHandler struct {
	db            *gorm.DB
	countryLookup logCountryLookup
}

func NewActivityLogHandler(db *gorm.DB) *ActivityLogHandler {
	return &ActivityLogHandler{db: db, countryLookup: utils.LookupIPCountries}
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
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + search + "%"
		query = query.Where("user_activity_logs.path LIKE ? OR user_activity_logs.method LIKE ?", like, like)
	}

	query.Count(&total)

	paginatedQuery, page, perPage := PaginateQuery(c, query, []string{"created_at", "path", "method"})
	if err := paginatedQuery.Order("created_at DESC").Preload("User").Find(&logs).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to load request logs")
	}
	enrichLogCountries(c.Context(), h.db, logs, h.countryLookup)

	return c.JSON(BuildPaginatedResponse(logs, total, page, perPage))
}
