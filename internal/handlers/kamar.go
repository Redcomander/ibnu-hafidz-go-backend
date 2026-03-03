package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type KamarHandler struct {
	db *gorm.DB
}

func NewKamarHandler(db *gorm.DB) *KamarHandler {
	return &KamarHandler{db: db}
}

// List returns all kamar with pagination and search
func (h *KamarHandler) List(c *fiber.Ctx) error {
	var kamars []models.Kamar
	var total int64

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 10)
	search := c.Query("search")
	sort := c.Query("sort", "created_at")
	order := c.Query("order", "desc")
	// Preload WaliKamar and SecondaryWalies
	query := h.db.Model(&models.Kamar{}).Preload("WaliKamar").Preload("SecondaryWalies")

	if search != "" {
		query = query.Where("nama_kamar LIKE ? OR keterangan LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	offset := (page - 1) * perPage
	query.Order(sort + " " + order).Limit(perPage).Offset(offset).Find(&kamars)

	return c.JSON(fiber.Map{
		"data":        kamars,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Get returns a single kamar
func (h *KamarHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var kamar models.Kamar

	if err := h.db.Preload("WaliKamar").Preload("SecondaryWalies").Preload("Students").First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	return c.JSON(kamar)
}

// Create adds a new kamar
func (h *KamarHandler) Create(c *fiber.Ctx) error {
	type CreateKamarRequest struct {
		NamaKamar        string `json:"nama_kamar"`
		Kapasitas        int    `json:"kapasitas"`
		Keterangan       string `json:"keterangan"`
		WaliKamarID      uint   `json:"wali_kamar_id"`
		SecondaryWaliIDs []uint `json:"secondary_wali_ids"`
	}

	var req CreateKamarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.NamaKamar == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Nama kamar is required",
		})
	}

	// WaliKamarID is required in DB schema
	if req.WaliKamarID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Wali Kamar is required",
		})
	}

	kamar := models.Kamar{
		NamaKamar:   req.NamaKamar,
		Kapasitas:   req.Kapasitas,
		Keterangan:  &req.Keterangan,
		WaliKamarID: req.WaliKamarID,
	}

	// Handle Secondary Walies
	if len(req.SecondaryWaliIDs) > 0 {
		var secondaryWalies []*models.User
		if err := h.db.Find(&secondaryWalies, req.SecondaryWaliIDs).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "validation_error",
				Message: "Invalid secondary wali IDs",
			})
		}
		kamar.SecondaryWalies = secondaryWalies
	}

	if err := h.db.Create(&kamar).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create kamar",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(kamar)
}

// AddStudent adds a student to a room
func (h *KamarHandler) AddStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	var kamar models.Kamar

	if err := h.db.First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	type AddStudentRequest struct {
		StudentID uint `json:"student_id"`
	}

	var req AddStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	var student models.Student
	if err := h.db.First(&student, req.StudentID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	if err := h.db.Model(&kamar).Association("Students").Append(&student); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to add student to kamar",
		})
	}

	return c.JSON(fiber.Map{"message": "Student added successfully"})
}

// RemoveStudent removes a student from a room
func (h *KamarHandler) RemoveStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	studentID := c.Params("student_id")
	var kamar models.Kamar

	if err := h.db.First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	var student models.Student
	if err := h.db.First(&student, studentID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	if err := h.db.Model(&kamar).Association("Students").Delete(&student); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to remove student from kamar",
		})
	}

	return c.JSON(fiber.Map{"message": "Student removed successfully"})
}

// Update modifies a kamar
func (h *KamarHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var kamar models.Kamar

	if err := h.db.Preload("SecondaryWalies").First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	type UpdateKamarRequest struct {
		NamaKamar        string `json:"nama_kamar"`
		Kapasitas        int    `json:"kapasitas"`
		Keterangan       string `json:"keterangan"`
		WaliKamarID      uint   `json:"wali_kamar_id"`
		SecondaryWaliIDs []uint `json:"secondary_wali_ids"`
	}

	var req UpdateKamarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.NamaKamar != "" {
		kamar.NamaKamar = req.NamaKamar
	}
	if req.Kapasitas != 0 {
		kamar.Kapasitas = req.Kapasitas
	}
	if req.Keterangan != "" {
		kamar.Keterangan = &req.Keterangan
	}
	if req.WaliKamarID != 0 {
		kamar.WaliKamarID = req.WaliKamarID
	}

	// Update Secondary Walies if provided (can be empty slice to clear)
	// We check if the field was present in JSON by pointer or convention,
	// but here we just assume if the client sends the key, it updates.
	// For simplicity in this handler structure, we'll update association if request is parsed.
	// Ideally we'd valid partial updates more strictly.
	// Assuming prompt wants "can assign multiple user", implied full update on save.

	if req.SecondaryWaliIDs != nil {
		var secondaryWalies []*models.User
		if len(req.SecondaryWaliIDs) > 0 {
			if err := h.db.Find(&secondaryWalies, req.SecondaryWaliIDs).Error; err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
					Error:   "validation_error",
					Message: "Invalid secondary wali IDs",
				})
			}
		}
		// Replace associations
		h.db.Model(&kamar).Association("SecondaryWalies").Replace(secondaryWalies)
	}

	h.db.Save(&kamar)

	// Reload for response
	h.db.Preload("WaliKamar").Preload("SecondaryWalies").First(&kamar, kamar.ID)

	return c.JSON(kamar)
}

// Delete removes a kamar
func (h *KamarHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var kamar models.Kamar

	if err := h.db.First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	h.db.Delete(&kamar)
	return c.JSON(fiber.Map{"message": "Kamar deleted successfully"})
}
