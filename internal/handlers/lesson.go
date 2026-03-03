package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type LessonHandler struct {
	db *gorm.DB
}

func NewLessonHandler(db *gorm.DB) *LessonHandler {
	return &LessonHandler{db: db}
}

// List returns all lessons, filtered by type (formal/diniyyah)
func (h *LessonHandler) List(c *fiber.Ctx) error {
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		var lessons []models.DiniyyahLesson
		if err := h.db.Preload("Assignments.Kelas").Preload("Assignments.Teacher").Find(&lessons).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch diniyyah lessons"})
		}

		type Response struct {
			models.DiniyyahLesson
			Type string `json:"type"`
		}
		var response []Response
		for _, l := range lessons {
			response = append(response, Response{DiniyyahLesson: l, Type: "diniyyah"})
		}
		return c.JSON(response)
	}

	// Default: Formal
	var lessons []models.Lesson
	if err := h.db.Preload("Assignments.Kelas").Preload("Assignments.Teacher").Find(&lessons).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch formal lessons"})
	}

	type Response struct {
		models.Lesson
		Type string `json:"type"`
	}
	var response []Response
	for _, l := range lessons {
		l.Type = models.LessonTypeFormal
		response = append(response, Response{Lesson: l, Type: "formal"})
	}
	return c.JSON(response)
}

// Create adds a new lesson
func (h *LessonHandler) Create(c *fiber.Ctx) error {
	type Input struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		Description string `json:"description"`
		Type        string `json:"type"` // "formal" or "diniyyah"
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		lesson := models.DiniyyahLesson{
			Name: input.Name,
		}
		if err := h.db.Create(&lesson).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create diniyyah lesson"})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"id": lesson.ID, "name": lesson.Name, "type": "diniyyah",
		})
	}

	// Formal
	lesson := models.Lesson{
		Name:        input.Name,
		Type:        models.LessonTypeFormal, // Explicitly set
		Description: input.Description,
	}
	if input.Code != "" {
		lesson.Code = &input.Code
	}
	if err := h.db.Create(&lesson).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create formal lesson"})
	}
	return c.Status(fiber.StatusCreated).JSON(lesson)
}

// Get returns a single lesson
func (h *LessonHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		var lesson models.DiniyyahLesson
		if err := h.db.First(&lesson, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Diniyyah lesson not found"})
		}
		return c.JSON(fiber.Map{
			"id": lesson.ID, "name": lesson.Name, "type": "diniyyah",
		})
	}

	var lesson models.Lesson
	if err := h.db.First(&lesson, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Formal lesson not found"})
	}
	return c.JSON(lesson)
}

// Update modifies an existing lesson
func (h *LessonHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	type Input struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		var lesson models.DiniyyahLesson
		if err := h.db.First(&lesson, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Lesson not found"})
		}
		lesson.Name = input.Name
		h.db.Save(&lesson)
		return c.JSON(fiber.Map{
			"id": lesson.ID, "name": lesson.Name, "type": "diniyyah",
		})
	}

	// Formal
	var lesson models.Lesson
	if err := h.db.First(&lesson, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Lesson not found"})
	}
	lesson.Name = input.Name
	lesson.Code = nil
	if input.Code != "" {
		lesson.Code = &input.Code
	}
	lesson.Description = input.Description
	h.db.Save(&lesson)
	return c.JSON(lesson)
}

// Delete removes a lesson (soft delete)
func (h *LessonHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		if err := h.db.Delete(&models.DiniyyahLesson{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete diniyyah lesson"})
		}
	} else {
		if err := h.db.Delete(&models.Lesson{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete formal lesson"})
		}
	}

	return c.JSON(fiber.Map{"message": "Lesson deleted successfully"})
}
