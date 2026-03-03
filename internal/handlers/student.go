package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type StudentHandler struct {
	db *gorm.DB
}

func NewStudentHandler(db *gorm.DB) *StudentHandler {
	return &StudentHandler{db: db}
}

// List returns paginated students with search/filter/sort
func (h *StudentHandler) List(c *fiber.Ctx) error {
	var students []models.Student
	var total int64

	// Count total (before pagination)
	query := h.db.Model(&models.Student{})

	// Apply filters — using Laravel column names
	if gender := c.Query("jenis_kelamin"); gender != "" {
		query = query.Where("jenis_kelamin = ?", gender)
	}
	if status := c.Query("status_periode"); status != "" {
		query = query.Where("status_periode = ?", status)
	}

	query.Count(&total)

	// Apply search, sort, pagination — using Laravel column names
	paginatedQuery, page, perPage := PaginateQuery(c, query, []string{"nama_lengkap", "nisn", "nama_ayah", "alamat"})
	paginatedQuery.Find(&students)

	return c.JSON(BuildPaginatedResponse(students, total, page, perPage))
}

// Get returns a single student by ID
func (h *StudentHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var student models.Student

	if err := h.db.First(&student, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	return c.JSON(student)
}

// Create adds a new student
func (h *StudentHandler) Create(c *fiber.Ctx) error {
	var student models.Student
	if err := c.BodyParser(&student); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if student.NamaLengkap == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Nama lengkap santri harus diisi",
		})
	}

	if err := h.db.Create(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create student",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(student)
}

// Update modifies an existing student
func (h *StudentHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var student models.Student

	if err := h.db.First(&student, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	if err := c.BodyParser(&student); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	h.db.Save(&student)
	return c.JSON(student)
}

// Delete soft-deletes a student
func (h *StudentHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.Student{}, id)

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	return c.JSON(fiber.Map{"message": "Student deleted successfully"})
}
