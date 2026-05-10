package handlers

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

// OcrAnswerKeyHandler handles CRUD for answer keys stored in the database.
// It is called from two route groups:
//   - /api/ocr-answer-keys  (JWT auth — Vue frontend)
//   - /api/ocr-service/answer-keys  (service token auth — OCR microservice)
type OcrAnswerKeyHandler struct {
	db *gorm.DB
}

func NewOcrAnswerKeyHandler(db *gorm.DB) *OcrAnswerKeyHandler {
	return &OcrAnswerKeyHandler{db: db}
}

// List returns a summary list of all answer keys.
func (h *OcrAnswerKeyHandler) List(c *fiber.Ctx) error {
	var keys []models.OcrAnswerKey
	if err := h.db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	summaries := make([]fiber.Map, 0, len(keys))
	for _, k := range keys {
		summaries = append(summaries, ocrAnswerKeySummary(k))
	}
	return c.JSON(fiber.Map{"keys": summaries})
}

// Get returns a single answer key with its full answers array.
func (h *OcrAnswerKeyHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid ID",
		})
	}

	var key models.OcrAnswerKey
	if err := h.db.First(&key, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Answer key not found",
		})
	}

	var answerMap map[string]string
	_ = json.Unmarshal([]byte(key.Answers), &answerMap)

	total := key.TotalCount
	if total == 0 {
		total = len(answerMap)
	}
	answerArr := make([]string, total)
	for i := 1; i <= total; i++ {
		answerArr[i-1] = answerMap[strconv.Itoa(i)]
	}

	return c.JSON(fiber.Map{
		"id":         strconv.FormatUint(uint64(key.ID), 10),
		"name":       key.Name,
		"answers":    answerArr,
		"key_map":    answerMap,
		"count":      key.TotalCount,
		"created_at": key.CreatedAt,
		"updated_at": key.UpdatedAt,
	})
}

// Create stores a new answer key.
func (h *OcrAnswerKeyHandler) Create(c *fiber.Ctx) error {
	var body map[string]interface{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	name := strings.TrimSpace(ocrStringVal(body["name"]))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Nama kunci jawaban wajib diisi",
		})
	}

	keyMap, total := ocrNormalizeAnswers(body["answers"], ocrIntVal(body["total"]))
	if len(keyMap) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Jawaban tidak boleh kosong",
		})
	}

	answersJSON, _ := json.Marshal(keyMap)
	record := models.OcrAnswerKey{
		Name:       name,
		Answers:    string(answersJSON),
		TotalCount: total,
	}
	if err := h.db.Create(&record).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"key":     ocrAnswerKeySummary(record),
	})
}

// Update modifies an existing answer key.
func (h *OcrAnswerKeyHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid ID",
		})
	}

	var key models.OcrAnswerKey
	if err := h.db.First(&key, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Answer key not found",
		})
	}

	var body map[string]interface{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if name := strings.TrimSpace(ocrStringVal(body["name"])); name != "" {
		key.Name = name
	}

	if body["answers"] != nil {
		keyMap, total := ocrNormalizeAnswers(body["answers"], ocrIntVal(body["total"]))
		answersJSON, _ := json.Marshal(keyMap)
		key.Answers = string(answersJSON)
		key.TotalCount = total
	}

	if err := h.db.Save(&key).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"key":     ocrAnswerKeySummary(key),
	})
}

// Delete soft-deletes an answer key.
func (h *OcrAnswerKeyHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid ID",
		})
	}

	result := h.db.Delete(&models.OcrAnswerKey{}, id)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: result.Error.Error(),
		})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Answer key not found",
		})
	}

	return c.JSON(fiber.Map{"success": true})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ocrAnswerKeySummary(k models.OcrAnswerKey) fiber.Map {
	var answerMap map[string]string
	_ = json.Unmarshal([]byte(k.Answers), &answerMap)

	preview := make(map[string]string)
	i := 0
	for q, a := range answerMap {
		if i >= 5 {
			break
		}
		preview[q] = a
		i++
	}

	return fiber.Map{
		"id":         strconv.FormatUint(uint64(k.ID), 10),
		"name":       k.Name,
		"count":      k.TotalCount,
		"preview":    preview,
		"key_map":    answerMap,
		"created_at": k.CreatedAt,
		"updated_at": k.UpdatedAt,
	}
}

// ocrNormalizeAnswers converts the answers payload (object or array) to a canonical map.
func ocrNormalizeAnswers(answers interface{}, totalHint int) (map[string]string, int) {
	keyMap := make(map[string]string)

	switch v := answers.(type) {
	case []interface{}:
		for i, val := range v {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				keyMap[strconv.Itoa(i+1)] = strings.ToUpper(strings.TrimSpace(s))
			}
		}
	case map[string]interface{}:
		for k, val := range v {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				keyMap[k] = strings.ToUpper(strings.TrimSpace(s))
			}
		}
	}

	total := totalHint
	if total == 0 {
		total = len(keyMap)
	}
	return keyMap, total
}

func ocrStringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func ocrIntVal(v interface{}) int {
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}
