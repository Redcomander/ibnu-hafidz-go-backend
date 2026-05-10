package handlers

import (
	"encoding/json"
	"strconv"
	"strings"

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
	Source         string  `json:"source"`
	IdempotencyKey *string `json:"idempotency_key"`

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

type updateOCRResultLinkStudentRequest struct {
	StudentID *uint `json:"student_id"`
}

type updateOCRResultLinkRequest struct {
	Score       *float64    `json:"score"`
	Correct     *int        `json:"correct"`
	Wrong       *int        `json:"wrong"`
	Blank       *int        `json:"blank"`
	RawResult   interface{} `json:"raw_result"`
	AnswerKeyID *uint       `json:"answer_key_id"`
}

func parseStringPtr(value interface{}) *string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		return &trimmed
	case float64:
		trimmed := strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
		if trimmed == "" {
			return nil
		}
		return &trimmed
	case int:
		trimmed := strconv.Itoa(v)
		return &trimmed
	default:
		return nil
	}
}

func parseFloat64Ptr(value interface{}) *float64 {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case float64:
		return &v
	case int:
		n := float64(v)
		return &n
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		n, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil
		}
		return &n
	default:
		return nil
	}
}

func parseIntPtr(value interface{}) *int {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case float64:
		n := int(v)
		if float64(n) != v {
			return nil
		}
		return &n
	case int:
		return &v
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		n64, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil
		}
		n := int(n64)
		return &n
	default:
		return nil
	}
}

func parseUintPtr(value interface{}) *uint {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return nil
		}
		n := uint(v)
		if float64(n) != v {
			return nil
		}
		return &n
	case int:
		if v <= 0 {
			return nil
		}
		n := uint(v)
		return &n
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		n64, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil || n64 == 0 {
			return nil
		}
		n := uint(n64)
		return &n
	default:
		return nil
	}
}

func (h *OCRResultLinkHandler) Create(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "User not authenticated",
		})
	}

	var body map[string]interface{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
	}

	req := createOCRResultLinkRequest{
		Source:         "",
		IdempotencyKey: parseStringPtr(body["idempotency_key"]),
		Filename:       parseStringPtr(body["filename"]),
		Score:          parseFloat64Ptr(body["score"]),
		Correct:        parseIntPtr(body["correct"]),
		Wrong:          parseIntPtr(body["wrong"]),
		Blank:          parseIntPtr(body["blank"]),
		RawResult:      body["raw_result"],
		LessonType:     parseStringPtr(body["lesson_type"]),
		LessonID:       parseUintPtr(body["lesson_id"]),
		KelasID:        parseUintPtr(body["kelas_id"]),
		TeacherID:      parseUintPtr(body["teacher_id"]),
		StudentID:      parseUintPtr(body["student_id"]),
		AnswerKeyID:    parseUintPtr(body["answer_key_id"]),
	}

	if source := parseStringPtr(body["source"]); source != nil {
		req.Source = *source
	}

	if req.Source == "" {
		req.Source = "scan"
	}

	if req.IdempotencyKey != nil {
		trimmed := strings.TrimSpace(*req.IdempotencyKey)
		if trimmed == "" {
			req.IdempotencyKey = nil
		} else {
			req.IdempotencyKey = &trimmed

			var existing models.OCRResultLink
			err := h.db.Where("scanned_by_id = ? AND idempotency_key = ?", userID, trimmed).
				Preload("Lesson").
				Preload("Kelas.Students").
				Preload("Teacher").
				Preload("Student").
				Preload("ScannedBy").
				First(&existing).Error
			if err == nil {
				return c.JSON(existing)
			}
			if err != nil && err != gorm.ErrRecordNotFound {
				return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
					Error:   "server_error",
					Message: err.Error(),
				})
			}
		}
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
		Source:         req.Source,
		IdempotencyKey: req.IdempotencyKey,
		Filename:       req.Filename,
		Score:          req.Score,
		Correct:        req.Correct,
		Wrong:          req.Wrong,
		Blank:          req.Blank,
		RawResult:      rawResultStr,
		LessonType:     req.LessonType,
		LessonID:       req.LessonID,
		KelasID:        req.KelasID,
		TeacherID:      req.TeacherID,
		StudentID:      req.StudentID,
		AnswerKeyID:    req.AnswerKeyID,
		ScannedByID:    userID,
	}

	if err := h.db.Create(&record).Error; err != nil {
		if req.IdempotencyKey != nil {
			var existing models.OCRResultLink
			findErr := h.db.Where("scanned_by_id = ? AND idempotency_key = ?", userID, *req.IdempotencyKey).
				Preload("Lesson").
				Preload("Kelas.Students").
				Preload("Teacher").
				Preload("Student").
				Preload("ScannedBy").
				First(&existing).Error
			if findErr == nil {
				return c.JSON(existing)
			}
		}

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

func (h *OCRResultLinkHandler) UpdateStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid result link id",
		})
	}

	var req updateOCRResultLinkStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
	}

	if req.StudentID != nil && *req.StudentID == 0 {
		req.StudentID = nil
	}

	var record models.OCRResultLink
	if err := h.db.First(&record, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
				Error:   "not_found",
				Message: "OCR result link not found",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	if req.StudentID != nil {
		var student models.Student
		if err := h.db.Select("id", "kelas_id").First(&student, *req.StudentID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
					Error:   "validation_error",
					Message: "Student not found",
				})
			}

			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "server_error",
				Message: err.Error(),
			})
		}

		if record.KelasID != nil {
			if student.KelasID == nil || *student.KelasID != *record.KelasID {
				return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
					Error:   "validation_error",
					Message: "Selected student is not in this class",
				})
			}
		}
	}

	if err := h.db.Model(&record).Update("student_id", req.StudentID).Error; err != nil {
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
		return c.JSON(record)
	}

	return c.JSON(record)
}

func (h *OCRResultLinkHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid result link id",
		})
	}

	if err := h.db.Delete(&models.OCRResultLink{}, id).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{"success": true})
}

func (h *OCRResultLinkHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid result link id",
		})
	}

	var body map[string]interface{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
	}

	var record models.OCRResultLink
	if err := h.db.First(&record, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
				Error:   "not_found",
				Message: "OCR result link not found",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	updates := map[string]interface{}{}

	if _, ok := body["score"]; ok {
		updates["score"] = parseFloat64Ptr(body["score"])
	}
	if _, ok := body["correct"]; ok {
		updates["correct_count"] = parseIntPtr(body["correct"])
	}
	if _, ok := body["wrong"]; ok {
		updates["wrong_count"] = parseIntPtr(body["wrong"])
	}
	if _, ok := body["blank"]; ok {
		updates["blank_count"] = parseIntPtr(body["blank"])
	}
	if _, ok := body["answer_key_id"]; ok {
		updates["answer_key_id"] = parseUintPtr(body["answer_key_id"])
	}
	if raw, ok := body["raw_result"]; ok {
		if raw == nil {
			updates["raw_result"] = nil
		} else {
			b, err := json.Marshal(raw)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
					Error:   "validation_error",
					Message: "Invalid raw_result payload",
				})
			}
			s := string(b)
			updates["raw_result"] = &s
		}
	}

	if len(updates) == 0 {
		return c.JSON(record)
	}

	if err := h.db.Model(&record).Updates(updates).Error; err != nil {
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
		return c.JSON(record)
	}

	return c.JSON(record)
}
