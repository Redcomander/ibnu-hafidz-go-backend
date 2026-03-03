package handlers

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
}

// Login authenticates a user and returns JWT tokens
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if req.Username == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Username and password are required",
		})
	}

	// Find user by username column
	var user models.User
	if err := h.db.Preload("Roles.Permissions").Where("username = ?", req.Username).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Invalid username or password",
		})
	}

	// Verify password (bcrypt — compatible with Laravel hashes)
	if !utils.CheckPassword(req.Password, user.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Invalid username or password",
		})
	}

	// Generate access token
	accessToken, err := utils.GenerateAccessToken(user.ID, user.Email, h.cfg.JWTSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to generate token",
		})
	}

	// Generate refresh token
	refreshToken, err := utils.GenerateRefreshToken(user.ID, user.Email, h.cfg.JWTRefreshSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to generate refresh token",
		})
	}

	// Set refresh token as HttpOnly cookie
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HTTPOnly: true,
		Secure:   h.cfg.Environment == "production",
		SameSite: "Lax",
		Path:     "/api/auth",
	})

	return c.JSON(models.AuthResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   900, // 15 minutes in seconds
		User:        user,
	})
}

// Refresh generates a new access token using the refresh token cookie
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	refreshToken := c.Cookies("refresh_token")
	if refreshToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "No refresh token provided",
		})
	}

	// Validate refresh token
	claims, err := utils.ValidateToken(refreshToken, h.cfg.JWTRefreshSecret)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Invalid or expired refresh token",
		})
	}

	// Verify user still exists
	var user models.User
	if err := h.db.Preload("Roles.Permissions").First(&user, claims.UserID).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User no longer exists",
		})
	}

	// Generate new access token
	accessToken, err := utils.GenerateAccessToken(user.ID, user.Email, h.cfg.JWTSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to generate token",
		})
	}

	return c.JSON(models.AuthResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   900,
		User:        user,
	})
}

// Logout clears the refresh token cookie
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   h.cfg.Environment == "production",
		SameSite: "Lax",
		Path:     "/api/auth",
	})

	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// Me returns the authenticated user's profile
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uint)

	var user models.User
	if err := h.db.Preload("Roles.Permissions").First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	return c.JSON(user)
}

// ErrorHandler is the global Fiber error handler
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		msg = e.Message
	} else {
		msg = err.Error()
	}

	// Determine error type based on status code
	errorType := "server_error"
	switch code {
	case fiber.StatusBadRequest:
		errorType = "bad_request"
	case fiber.StatusUnauthorized:
		errorType = "unauthorized"
	case fiber.StatusForbidden:
		errorType = "forbidden"
	case fiber.StatusNotFound:
		errorType = "not_found"
	}

	return c.Status(code).JSON(models.ErrorResponse{
		Error:   errorType,
		Message: msg,
	})
}

// ---- Pagination helper used by all list handlers ----

// PaginateQuery applies search, filter, sort, and pagination to a GORM query
func PaginateQuery(c *fiber.Ctx, db *gorm.DB, searchFields []string) (*gorm.DB, int, int) {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	search := c.Query("search", "")
	sortBy := c.Query("sort", "created_at")
	order := c.Query("order", "desc")

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 1000 {
		perPage = 1000
	}

	query := db

	// Search across multiple fields
	if search != "" && len(searchFields) > 0 {
		var conditions []string
		var args []interface{}
		for _, field := range searchFields {
			conditions = append(conditions, field+" LIKE ?")
			args = append(args, "%"+search+"%")
		}
		query = query.Where(strings.Join(conditions, " OR "), args...)
	}

	// Sort
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	query = query.Order(sortBy + " " + order)

	// Pagination
	offset := (page - 1) * perPage
	query = query.Offset(offset).Limit(perPage)

	return query, page, perPage
}

// BuildPaginatedResponse creates a standardized paginated response
func BuildPaginatedResponse(data interface{}, total int64, page, perPage int) models.PaginatedResponse {
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	return models.PaginatedResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}
}
