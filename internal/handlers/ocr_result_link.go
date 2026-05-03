package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type OCRResultLinkHandler struct {
	db *gorm.DB
}

func NewOCRResultLinkHandler(db *gorm.DB) *OCRResultLinkHandler {
	return &OCRResultLinkHandler{db: db}
}

type createOCRResultLinkRequest struct {
	Source string `json:"source"`

	Filename *string  `json:"filename"`
	Score    *float64 `json:"score"`
	Correct  *int     `json:"correct"`
	Wrong    *int     `json:"wrong"`
	Blank    *int     `json:"blank"`

	RawResult interface{} `json:"raw_result"`

	LessonType *string `json:"lesson_type"`
	LessonID   *uint   `json:"lesson_id"`
	KelasID    *uint   `json:"kelas_id"`
	TeacherID  *uint   `json:"teacher_id"`
	StudentID  *uint   `json:"student_id"`

	AnswerKeyID *uint `json:"answer_key_id"`
}

func (h *OCRResultLinkHandler) Create(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
	}

	var req createOCRResultLinkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
	}

	if req.Source == "" {
		req.Source = "scan"
	}

	var rawResultStr *string
	if req.RawResult != nil {
		b, err := json.Marshal(req.RawResult)
		if err == nil {
			s := string(b)
			rawResultStr = &s
		}
	}

	record := models.OCRResultLink{
		Source:      req.Source,
		Filename:    req.Filename,
		Score:       req.Score,
		Correct:     req.Correct,
		Wrong:       req.Wrong,
		Blank:       req.Blank,
		RawResult:   rawResultStr,
		LessonType:  req.LessonType,
		LessonID:    req.LessonID,
		KelasID:     req.KelasID,
		TeacherID:   req.TeacherID,
		StudentID:   req.StudentID,
		AnswerKeyID: req.AnswerKeyID,
		ScannedByID: userID,
	}

	if err := h.db.Create(&record).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	if err := h.db.Preload("Lesson").
		Preload("Kelas.Students").
		Preload("Teacher").
		Preload("Student").
		Preload("ScannedBy").
		First(&record, record.ID).Error; err != nil {
		return c.Status(fiber.StatusCreated).JSON(record)
	}

	return c.Status(fiber.StatusCreated).JSON(record)
}

func (h *OCRResultLinkHandler) List(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}

	query := h.db.Model(&models.OCRResultLink{})

	if v := c.QueryInt("lesson_id", 0); v > 0 {
		query = query.Where("lesson_id = ?", v)
	}
	if v := c.QueryInt("kelas_id", 0); v > 0 {
		query = query.Where("kelas_id = ?", v)
	}
	if v := c.QueryInt("teacher_id", 0); v > 0 {
		query = query.Where("teacher_id = ?", v)
	}
	if v := c.QueryInt("student_id", 0); v > 0 {
		query = query.Where("student_id = ?", v)
	}
	if source := c.Query("source"); source != "" {
		query = query.Where("source = ?", source)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	offset := (page - 1) * perPage
	var rows []models.OCRResultLink
	if err := query.
		Preload("Lesson").
		Preload("Kelas.Students").
		Preload("Teacher").
		Preload("Student").
		Preload("ScannedBy").
		Order("created_at DESC").
		Offset(offset).
		Limit(perPage).
		Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	return c.JSON(fiber.Map{
		"data":        rows,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}
