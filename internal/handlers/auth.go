package handlers

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
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
	err := h.db.Preload("Roles.Permissions").Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		// Dummy check to prevent timing side-channel attack (prevents user enumeration via response time)
		utils.CheckPassword(req.Password, "$2a$10$vI8aWBnW3fID.ZQ4/zo1G.q1lRps.9cGLcZEiGDMVr5yUP1KUOYTa")
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

	ip := strings.TrimSpace(c.Get("CF-Connecting-IP"))
	if ip == "" {
		ip = strings.TrimSpace(c.Get("X-Forwarded-For"))
		if idx := strings.Index(ip, ","); idx >= 0 {
			ip = strings.TrimSpace(ip[:idx])
		}
	}
	if ip == "" {
		ip = strings.TrimSpace(c.Get("X-Real-IP"))
	}
	if ip == "" {
		ip = strings.TrimSpace(c.Get("True-Client-IP"))
	}
	if ip == "" {
		ip = strings.TrimSpace(c.IP())
	}

	country := strings.TrimSpace(c.Get("CF-IPCountry"))
	if country == "" {
		country = strings.TrimSpace(c.Get("X-Country-Code"))
	}
	if country == "" {
		country = strings.TrimSpace(c.Get("X-Vercel-IP-Country"))
	}
	country = strings.ToUpper(country)

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

func buildLoginDeviceLabel(userAgent string) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return "Perangkat tidak diketahui"
	}

	browser := "Browser"
	switch {
	case strings.Contains(ua, "edg/") || strings.Contains(ua, "edge/"):
		browser = "Edge"
	case strings.Contains(ua, "opr/"):
		browser = "Opera"
	case strings.Contains(ua, "firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "chrome/") && !strings.Contains(ua, "crios"):
		browser = "Chrome"
	case strings.Contains(ua, "safari/"):
		browser = "Safari"
	}

	osName := "Perangkat"
	switch {
	case strings.Contains(ua, "windows nt"):
		osName = "Windows"
	case strings.Contains(ua, "android"):
		osName = "Android"
	case strings.Contains(ua, "iphone"):
		osName = "iPhone"
	case strings.Contains(ua, "ipad"):
		osName = "iPad"
	case strings.Contains(ua, "mac os"):
		osName = "macOS"
	case strings.Contains(ua, "linux"):
		osName = "Linux"
	}

	return fmt.Sprintf("%s di %s", browser, osName)
}

// GetDeviceLoginHistory returns recent successful login events for the current user.
func (h *AuthHandler) GetDeviceLoginHistory(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uint)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
	}

	var logs []models.UserActivityLog
	if err := h.db.Where("user_id = ? AND method = ? AND path = ?", userID, "POST", "/api/auth/login").Order("created_at desc").Limit(10).Find(&logs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Gagal mengambil riwayat login perangkat",
		})
	}

	history := make([]fiber.Map, 0, len(logs))
	for i, log := range logs {
		agent := ""
		if log.UserAgent != nil {
			agent = *log.UserAgent
		}

		countryCode := ""
		if log.CountryCode != nil {
			countryCode = *log.CountryCode
		}

		history = append(history, fiber.Map{
			"id":           log.ID,
			"created_at":   log.CreatedAt,
			"ip_address":   log.IPAddress,
			"user_agent":   agent,
			"device_name":  log.DeviceName,
			"country_code": countryCode,
			"is_current":   i == 0,
		})
	}

	return c.JSON(fiber.Map{"data": history})
}

// UpdateProfile updates authenticated user's own profile.
func (h *AuthHandler) UpdateProfile(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uint)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	type UpdateProfileRequest struct {
		Name            string `json:"name"`
		Username        string `json:"username"`
		Email           string `json:"email"`
		NIK             string `json:"nik"`
		TempatLahir     string `json:"tempat_lahir"`
		TanggalLahir    string `json:"tanggal_lahir"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Email) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Name, username, and email are required",
		})
	}

	// Ensure username/email remain unique (excluding current user).
	var existing models.User
	if err := h.db.Where("(email = ? OR username = ?) AND id <> ?", req.Email, req.Username, userID).First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{
			Error:   "conflict",
			Message: "Email or username already exists",
		})
	}

	user.Name = req.Name
	user.Username = req.Username
	user.Email = req.Email
	user.NIK = normalizeOptionalString(req.NIK)
	user.TempatLahir = normalizeOptionalString(req.TempatLahir)
	user.TanggalLahir = normalizeOptionalString(req.TanggalLahir)

	if strings.TrimSpace(req.NewPassword) != "" {
		if len(req.NewPassword) < 6 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "validation_error",
				Message: "New password must be at least 6 characters",
			})
		}
		if strings.TrimSpace(req.CurrentPassword) == "" || !utils.CheckPassword(req.CurrentPassword, user.Password) {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Current password is incorrect",
			})
		}

		hashedPassword, err := utils.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: "Failed to hash password",
			})
		}
		user.Password = hashedPassword
	}

	if err := h.db.Save(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update profile",
		})
	}

	h.db.Preload("Roles.Permissions").First(&user, userID)
	return c.JSON(user)
}

func normalizeOptionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeStoredUploadPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.ReplaceAll(filepath.ToSlash(trimmed), "\\", "/")
	trimmed = strings.TrimPrefix(trimmed, "./")
	trimmed = strings.TrimPrefix(trimmed, "/")

	switch {
	case strings.HasPrefix(trimmed, "uploads/"):
		return "/" + trimmed
	case strings.HasPrefix(trimmed, "avatars/"):
		return "/uploads/" + trimmed
	case strings.HasPrefix(trimmed, "gallery/"):
		return "/uploads/" + trimmed
	default:
		return "/uploads/" + trimmed
	}
}

func resolveStoredUploadDiskPath(basePath, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.ReplaceAll(filepath.ToSlash(trimmed), "\\", "/")
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "uploads/")
	trimmed = strings.TrimPrefix(trimmed, "avatars/")
	trimmed = strings.TrimPrefix(trimmed, "gallery/")
	trimmed = strings.TrimPrefix(trimmed, "./")

	return filepath.Join(basePath, filepath.FromSlash(trimmed))
}

// UploadProfileAvatar uploads and updates authenticated user's avatar.
func (h *AuthHandler) UploadProfileAvatar(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uint)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	file, err := c.FormFile("avatar")
	if err != nil || file == nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Avatar file is required",
		})
	}

	if file.Size > 2*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Avatar size must be <= 2MB",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
	}
	if !allowed[ext] {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Avatar format must be jpg, png, or webp",
		})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid avatar file",
		})
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	mimeType := http.DetectContentType(buf[:n])
	allowedMimes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}
	if !allowedMimes[mimeType] {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Avatar content must be a valid image (jpg, png, webp)",
		})
	}

	uploadPath := "./uploads"
	if os.Getenv("UPLOAD_PATH") != "" {
		uploadPath = os.Getenv("UPLOAD_PATH")
	}

	avatarDir := filepath.Join(uploadPath, "avatars")
	if err := os.MkdirAll(avatarDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to prepare upload directory",
		})
	}

	filename := fmt.Sprintf("user_%d_%d%s", userID, time.Now().UnixNano(), ext)
	relPath := filepath.ToSlash(filepath.Join("avatars", filename))
	storedPath := normalizeStoredUploadPath(relPath)
	absPath := filepath.Join(uploadPath, filepath.FromSlash(strings.TrimPrefix(storedPath, "/uploads/")))

	if err := c.SaveFile(file, absPath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to save avatar",
		})
	}

	if user.FotoGuru != nil && *user.FotoGuru != "" {
		oldPath := resolveStoredUploadDiskPath(uploadPath, *user.FotoGuru)
		if oldPath != absPath {
			_ = os.Remove(oldPath)
		}
	}

	user.FotoGuru = &storedPath
	if err := h.db.Save(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update avatar",
		})
	}

	h.db.Preload("Roles.Permissions").First(&user, userID)
	return c.JSON(user)
}

// DeleteProfile soft deletes authenticated user's own account.
func (h *AuthHandler) DeleteProfile(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(uint)

	type DeleteProfileRequest struct {
		CurrentPassword string `json:"current_password"`
	}

	var req DeleteProfileRequest
	_ = c.BodyParser(&req)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	if strings.TrimSpace(req.CurrentPassword) == "" || !utils.CheckPassword(req.CurrentPassword, user.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Current password is required and must be correct",
		})
	}

	if err := h.db.Delete(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to delete account",
		})
	}

	// Clear refresh cookie so active browser session ends immediately.
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   h.cfg.Environment == "production",
		SameSite: "Lax",
		Path:     "/api/auth",
	})

	return c.JSON(fiber.Map{"message": "Account deleted successfully"})
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
