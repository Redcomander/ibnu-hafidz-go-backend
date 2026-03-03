package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
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
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
		RoleIDs  []uint `json:"role_ids"`
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

	h.db.Save(&user)

	// Update roles if provided
	if len(req.RoleIDs) > 0 {
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
