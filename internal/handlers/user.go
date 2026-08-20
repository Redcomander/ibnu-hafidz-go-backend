package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type UserHandler struct {
	db *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// List returns paginated users with search/filter/sort
func (h *UserHandler) List(c *fiber.Ctx) error {
	var users []models.User
	var total int64

	query := h.db.Model(&models.User{}).Preload("Roles")
	query.Count(&total)

	paginatedQuery, page, perPage := PaginateQuery(c, query, []string{"name", "email"})
	paginatedQuery.Find(&users)

	return c.JSON(BuildPaginatedResponse(users, total, page, perPage))
}

func (h *UserHandler) buildUserExportQuery(c *fiber.Ctx) *gorm.DB {
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.User{}).Order("name ASC")

	if search != "" {
		term := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(username) LIKE ?",
			term, term, term,
		)
	}

	return query
}

func userExportText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func userExportTruncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

// ExportCredentialsExcel exports user credential sheet (username + default password).
func (h *UserHandler) ExportCredentialsExcel(c *fiber.Ctx) error {
	var users []models.User
	if err := h.buildUserExportQuery(c).Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to export users"})
	}

	f := excelize.NewFile()
	sheet := "User Credentials"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No", "Nama Lengkap", "Username", "Email", "NIK", "Tempat Lahir", "Tanggal Lahir", "Password Default"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellStr(sheet, cell, header)
	}

	for i, user := range users {
		row := i + 2
		f.SetCellInt(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellStr(sheet, fmt.Sprintf("B%d", row), strings.TrimSpace(user.Name))
		f.SetCellStr(sheet, fmt.Sprintf("C%d", row), user.Username)
		f.SetCellStr(sheet, fmt.Sprintf("D%d", row), user.Email)
		f.SetCellStr(sheet, fmt.Sprintf("E%d", row), userExportText(user.NIK))
		f.SetCellStr(sheet, fmt.Sprintf("F%d", row), userExportText(user.TempatLahir))
		f.SetCellStr(sheet, fmt.Sprintf("G%d", row), userExportText(user.TanggalLahir))
		f.SetCellStr(sheet, fmt.Sprintf("H%d", row), "12345678")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("user_credentials_%s.xlsx", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

// ExportCredentialsPDF exports user credential sheet (username + default password).
func (h *UserHandler) ExportCredentialsPDF(c *fiber.Ctx) error {
	var users []models.User
	if err := h.buildUserExportQuery(c).Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to export users"})
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(true, 10)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Data Login Pengguna", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 8)
	pdf.CellFormat(0, 5, fmt.Sprintf("Dicetak: %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("Arial", "B", 7)
	colSpecs := []struct {
		width  float64
		header string
	}{
		{10, "No"},
		{28, "Nama"},
		{24, "Username"},
		{26, "Email"},
		{24, "NIK"},
		{22, "Tempat Lahir"},
		{20, "Tanggal Lahir"},
		{14, "Password"},
	}

	currentX := 8.0
	for _, spec := range colSpecs {
		pdf.CellFormat(spec.width, 6, spec.header, "1", 0, "C", false, 0, "")
		currentX += spec.width
	}
	pdf.Ln(6)

	pdf.SetFont("Arial", "", 6.5)
	for i, user := range users {
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = "-"
		}
		email := user.Email
		if len(email) > 22 {
			email = email[:19] + "..."
		}
		valueNIK := userExportText(user.NIK)
		if len(valueNIK) > 18 {
			valueNIK = valueNIK[:15] + "..."
		}
		place := userExportText(user.TempatLahir)
		if len(place) > 16 {
			place = place[:13] + "..."
		}
		birthDate := userExportText(user.TanggalLahir)
		if len(birthDate) > 14 {
			birthDate = birthDate[:11] + "..."
		}

		pdf.CellFormat(10, 6, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		pdf.CellFormat(28, 6, userExportTruncate(name, 18), "1", 0, "L", false, 0, "")
		pdf.CellFormat(24, 6, userExportTruncate(user.Username, 15), "1", 0, "L", false, 0, "")
		pdf.CellFormat(26, 6, userExportTruncate(email, 24), "1", 0, "L", false, 0, "")
		pdf.CellFormat(24, 6, userExportTruncate(valueNIK, 18), "1", 0, "L", false, 0, "")
		pdf.CellFormat(22, 6, userExportTruncate(place, 16), "1", 0, "L", false, 0, "")
		pdf.CellFormat(20, 6, userExportTruncate(birthDate, 14), "1", 0, "L", false, 0, "")
		pdf.CellFormat(14, 6, "12345678", "1", 1, "C", false, 0, "")
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("user_credentials_%s.pdf", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

// Get returns a single user by ID with roles
func (h *UserHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var user models.User

	if err := h.db.Preload("Roles.Permissions").First(&user, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	return c.JSON(user)
}

// Create adds a new user (teacher)
func (h *UserHandler) Create(c *fiber.Ctx) error {
	type CreateUserRequest struct {
		Name         string `json:"name"`
		Username     string `json:"username"`
		Email        string `json:"email"`
		NIK          string `json:"nik"`
		TempatLahir  string `json:"tempat_lahir"`
		TanggalLahir string `json:"tanggal_lahir"`
		Password     string `json:"password"`
		RoleIDs      []uint `json:"role_ids"`
	}

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Name, username, email, and password are required",
		})
	}

	// Check if email or username already exists
	var existing models.User
	if h.db.Where("email = ? OR username = ?", req.Email, req.Username).First(&existing).Error == nil {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{
			Error:   "conflict",
			Message: "Email or username already exists",
		})
	}

	// Hash password (bcrypt — same as Laravel)
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to hash password",
		})
	}

	user := models.User{
		Name:         req.Name,
		Username:     req.Username,
		Email:        req.Email,
		NIK:          normalizeOptionalString(req.NIK),
		TempatLahir:  normalizeOptionalString(req.TempatLahir),
		TanggalLahir: normalizeOptionalString(req.TanggalLahir),
		Password:     hashedPassword,
	}

	if err := h.db.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create user",
		})
	}

	// Assign roles
	if len(req.RoleIDs) > 0 {
		var roles []models.Role
		h.db.Where("id IN ?", req.RoleIDs).Find(&roles)
		h.db.Model(&user).Association("Roles").Replace(roles)
	}

	h.db.Preload("Roles").First(&user, user.ID)
	return c.Status(fiber.StatusCreated).JSON(user)
}

// Update modifies an existing user
func (h *UserHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var user models.User

	if err := h.db.First(&user, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	type UpdateUserRequest struct {
		Name                 string `json:"name"`
		Username             string `json:"username"`
		Email                string `json:"email"`
		NIK                  string `json:"nik"`
		TempatLahir          string `json:"tempat_lahir"`
		TanggalLahir         string `json:"tanggal_lahir"`
		Password             string `json:"password"`
		PasswordConfirmation string `json:"password_confirmation"`
		RoleIDs              []uint `json:"role_ids"`
	}

	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.NIK != "" || req.TempatLahir != "" || req.TanggalLahir != "" {
		user.NIK = normalizeOptionalString(req.NIK)
		user.TempatLahir = normalizeOptionalString(req.TempatLahir)
		user.TanggalLahir = normalizeOptionalString(req.TanggalLahir)
	}
	if req.Password != "" {
		if len(req.Password) < 6 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "validation_error",
				Message: "Password minimal 6 karakter",
			})
		}
		if req.PasswordConfirmation != "" && req.Password != req.PasswordConfirmation {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "validation_error",
				Message: "Konfirmasi password tidak sama",
			})
		}

		hashedPassword, err := utils.HashPassword(req.Password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: "Failed to hash password",
			})
		}
		user.Password = hashedPassword
	}

	h.db.Save(&user)

	// Update roles if field is provided (including empty array to clear roles).
	if req.RoleIDs != nil {
		var roles []models.Role
		h.db.Where("id IN ?", req.RoleIDs).Find(&roles)
		h.db.Model(&user).Association("Roles").Replace(roles)
	}

	h.db.Preload("Roles").First(&user, user.ID)
	return c.JSON(user)
}

// Delete soft-deletes a user
func (h *UserHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.User{}, id)

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "User not found",
		})
	}

	return c.JSON(fiber.Map{"message": "User deleted successfully"})
}

// GetTeachers returns all users with 'teacher' role
func (h *UserHandler) GetTeachers(c *fiber.Ctx) error {
	var teachers []models.User

	// Assuming there is a Role 'teacher' or 'Guru'
	// Linking via UserRoles table
	if err := h.db.Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.name LIKE ?", "%teacher%"). // or 'guru'? Safe bet to check both or standard 'teacher'
		Or("roles.name LIKE ?", "%guru%").
		Preload("Roles").
		Find(&teachers).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch teachers"})
	}

	return c.JSON(teachers)
}
