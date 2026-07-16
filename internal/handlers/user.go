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

// ExportCredentialsExcel exports user credential sheet (username + default password).
func (h *UserHandler) ExportCredentialsExcel(c *fiber.Ctx) error {
	var users []models.User
	if err := h.buildUserExportQuery(c).Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to export users"})
	}

	f := excelize.NewFile()
	sheet := "User Credentials"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No", "Nama Lengkap", "Username", "Email", "Password Default"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	for i, user := range users {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), strings.TrimSpace(user.Name))
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), user.Username)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), user.Email)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), "12345678")
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
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, "Data Login Pengguna", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(0, 6, fmt.Sprintf("Dicetak: %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(10, 7, "No", "1", 0, "C", false, 0, "")
	pdf.CellFormat(58, 7, "Nama Lengkap", "1", 0, "L", false, 0, "")
	pdf.CellFormat(40, 7, "Username", "1", 0, "L", false, 0, "")
	pdf.CellFormat(60, 7, "Email", "1", 0, "L", false, 0, "")
	pdf.CellFormat(22, 7, "Password", "1", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 8)
	for i, user := range users {
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = "-"
		}
		email := user.Email
		if len(name) > 35 {
			name = name[:32] + "..."
		}
		if len(email) > 38 {
			email = email[:35] + "..."
		}

		pdf.CellFormat(10, 6, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		pdf.CellFormat(58, 6, name, "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, user.Username, "1", 0, "L", false, 0, "")
		pdf.CellFormat(60, 6, email, "1", 0, "L", false, 0, "")
		pdf.CellFormat(22, 6, "12345678", "1", 1, "C", false, 0, "")
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
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		RoleIDs  []uint `json:"role_ids"`
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
		Name:     req.Name,
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
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
