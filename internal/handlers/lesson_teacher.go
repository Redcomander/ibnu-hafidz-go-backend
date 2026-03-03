package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

func parseUint(s string) uint {
	val, _ := strconv.ParseUint(s, 10, 32)
	return uint(val)
}

type LessonTeacherHandler struct {
	db *gorm.DB
}

func NewLessonTeacherHandler(db *gorm.DB) *LessonTeacherHandler {
	return &LessonTeacherHandler{db: db}
}

// ListByClass returns all teacher assignments for a specific class
func (h *LessonTeacherHandler) ListByClass(c *fiber.Ctx) error {
	classID := c.Params("id")
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		var assignments []models.DiniyyahLessonTeacher
		if err := h.db.Preload("DiniyyahLesson").Preload("Teacher").
			Where("kelas_id = ?", classID).Find(&assignments).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch assignments"})
		}
		return c.JSON(assignments)
	}

	// Formal
	var assignments []models.LessonTeacher
	if err := h.db.Preload("Lesson").Preload("Teacher").
		Where("kelas_id = ?", classID).Find(&assignments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch assignments"})
	}
	return c.JSON(assignments)
}

// Assign links a teacher to a lesson for a class
func (h *LessonTeacherHandler) Assign(c *fiber.Ctx) error {
	classID := c.Params("id")
	type Input struct {
		LessonID  uint   `json:"lesson_id"`
		TeacherID uint   `json:"teacher_id"`
		Type      string `json:"type"` // "formal" or "diniyyah"
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		assignment := models.DiniyyahLessonTeacher{
			KelasID:          uint(parseUint(classID)),
			DiniyyahLessonID: input.LessonID,
			UserID:           input.TeacherID,
		}
		// Check duplicate
		var count int64
		h.db.Model(&models.DiniyyahLessonTeacher{}).
			Where("kelas_id = ? AND diniyyah_lesson_id = ?", assignment.KelasID, assignment.DiniyyahLessonID).
			Count(&count)
		if count > 0 {
			var existing models.DiniyyahLessonTeacher
			h.db.Where("kelas_id = ? AND diniyyah_lesson_id = ?", assignment.KelasID, assignment.DiniyyahLessonID).
				First(&existing)
			existing.UserID = input.TeacherID
			h.db.Save(&existing)
			// Return updated with preloads
			h.db.Preload("DiniyyahLesson").Preload("Teacher").First(&existing, existing.ID)
			return c.JSON(existing)
		}

		if err := h.db.Create(&assignment).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign teacher"})
		}

		// Reload to get details
		h.db.Preload("DiniyyahLesson").Preload("Teacher").First(&assignment, assignment.ID)
		return c.Status(fiber.StatusCreated).JSON(assignment)
	}

	// Formal
	assignment := models.LessonTeacher{
		KelasID:  uint(parseUint(classID)),
		LessonID: input.LessonID,
		UserID:   input.TeacherID,
	}

	// Check duplicate/Update
	var count int64
	h.db.Model(&models.LessonTeacher{}).
		Where("kelas_id = ? AND lesson_id = ?", assignment.KelasID, assignment.LessonID).
		Count(&count)
	if count > 0 {
		var existing models.LessonTeacher
		h.db.Where("kelas_id = ? AND lesson_id = ?", assignment.KelasID, assignment.LessonID).
			First(&existing)
		existing.UserID = input.TeacherID
		h.db.Save(&existing)
		// Return updated with preloads
		h.db.Preload("Lesson").Preload("Teacher").First(&existing, existing.ID)
		return c.JSON(existing)
	}

	if err := h.db.Create(&assignment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign teacher"})
	}

	// Reload
	h.db.Preload("Lesson").Preload("Teacher").First(&assignment, assignment.ID)
	return c.Status(fiber.StatusCreated).JSON(assignment)
}

// ListByLesson returns all teacher assignments for a specific lesson
func (h *LessonTeacherHandler) ListByLesson(c *fiber.Ctx) error {
	lessonID := c.Params("id")
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		var assignments []models.DiniyyahLessonTeacher
		if err := h.db.Preload("Kelas").Preload("Teacher").
			Where("diniyyah_lesson_id = ?", lessonID).Find(&assignments).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch assignments"})
		}
		return c.JSON(assignments)
	}

	// Formal
	var assignments []models.LessonTeacher
	if err := h.db.Preload("Kelas").Preload("Teacher").
		Where("lesson_id = ?", lessonID).Find(&assignments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch assignments"})
	}
	return c.JSON(assignments)
}

// AssignToLesson links a teacher and a class to a lesson
func (h *LessonTeacherHandler) AssignToLesson(c *fiber.Ctx) error {
	lessonID := c.Params("id")
	type Input struct {
		KelasID   uint   `json:"kelas_id"`
		TeacherID uint   `json:"teacher_id"`
		Type      string `json:"type"` // "formal" or "diniyyah"
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Type == "diniyyah" {
		assignment := models.DiniyyahLessonTeacher{
			DiniyyahLessonID: uint(parseUint(lessonID)),
			KelasID:          input.KelasID,
			UserID:           input.TeacherID,
		}

		// Check duplicate (same lesson, same class)
		var count int64
		h.db.Model(&models.DiniyyahLessonTeacher{}).
			Where("diniyyah_lesson_id = ? AND kelas_id = ?", assignment.DiniyyahLessonID, assignment.KelasID).
			Count(&count)

		if count > 0 {
			// Update existing teacher for this lesson-class combo
			var existing models.DiniyyahLessonTeacher
			h.db.Where("diniyyah_lesson_id = ? AND kelas_id = ?", assignment.DiniyyahLessonID, assignment.KelasID).
				First(&existing)
			existing.UserID = input.TeacherID
			h.db.Save(&existing)
			h.db.Preload("Kelas").Preload("Teacher").First(&existing, existing.ID)
			return c.JSON(existing)
		}

		if err := h.db.Create(&assignment).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign teacher"})
		}

		h.db.Preload("Kelas").Preload("Teacher").First(&assignment, assignment.ID)
		return c.Status(fiber.StatusCreated).JSON(assignment)
	}

	// Formal
	assignment := models.LessonTeacher{
		LessonID: uint(parseUint(lessonID)),
		KelasID:  input.KelasID,
		UserID:   input.TeacherID,
	}

	// Check duplicate
	var count int64
	h.db.Model(&models.LessonTeacher{}).
		Where("lesson_id = ? AND kelas_id = ?", assignment.LessonID, assignment.KelasID).
		Count(&count)

	if count > 0 {
		var existing models.LessonTeacher
		h.db.Where("lesson_id = ? AND kelas_id = ?", assignment.LessonID, assignment.KelasID).
			First(&existing)
		existing.UserID = input.TeacherID
		h.db.Save(&existing)
		h.db.Preload("Kelas").Preload("Teacher").First(&existing, existing.ID)
		return c.JSON(existing)
	}

	if err := h.db.Create(&assignment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign teacher"})
	}

	h.db.Preload("Kelas").Preload("Teacher").First(&assignment, assignment.ID)
	return c.Status(fiber.StatusCreated).JSON(assignment)
}

// Unassign removes a link
func (h *LessonTeacherHandler) Unassign(c *fiber.Ctx) error {
	id := c.Params("assignment_id")
	lessonType := c.Query("type", "formal")

	if lessonType == "diniyyah" {
		if err := h.db.Delete(&models.DiniyyahLessonTeacher{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unassign"})
		}
	} else {
		if err := h.db.Delete(&models.LessonTeacher{}, id).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unassign"})
		}
	}

	return c.JSON(fiber.Map{"message": "Unassigned successfully"})
}

// Helper (Assuming strConv helper exists or I duplicate it here for safety if utils pkg not handy)
// But wait, there is no shared parseUint in this file scope. I should import strconv.
// Start of file needs strconv.
