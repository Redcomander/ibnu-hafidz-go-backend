package handlers

import (
	"strings"

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

func isKamarAdminUser(user *models.User) bool {
	if user == nil {
		return false
	}
	for _, role := range user.Roles {
		roleName := strings.ToLower(strings.TrimSpace(role.Name))
		if roleName == "super_admin" || roleName == "admin" || roleName == "administrator" {
			return true
		}
	}
	return false
}

func userCanManageKamar(user *models.User, kamar *models.Kamar) bool {
	if user == nil || kamar == nil {
		return false
	}

	if isKamarAdminUser(user) {
		return true
	}

	if user.ID != 0 && kamar.WaliKamarID == user.ID {
		return true
	}

	for _, wali := range kamar.SecondaryWalies {
		if wali != nil && wali.ID == user.ID {
			return true
		}
	}

	return false
}

func (h *KamarHandler) currentUser(c *fiber.Ctx) *models.User {
	if user, ok := c.Locals("user").(*models.User); ok && user != nil {
		return user
	}
	return nil
}

func (h *KamarHandler) enforceKamarAccess(c *fiber.Ctx, kamar *models.Kamar) error {
	user := h.currentUser(c)
	if user == nil {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "User context is missing",
		})
	}
	if !userCanManageKamar(user, kamar) {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "You are not assigned as the wali kamar or admin for this room",
		})
	}
	return nil
}

func (h *KamarHandler) enforceKamarDeleteAccess(c *fiber.Ctx) error {
	user := h.currentUser(c)
	if user == nil {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "User context is missing",
		})
	}
	if !isKamarAdminUser(user) {
		return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse{
			Error:   "forbidden",
			Message: "Only admins can delete a room",
		})
	}
	return nil
}

func (h *KamarHandler) scopeKamarQueryForUser(c *fiber.Ctx, query *gorm.DB) *gorm.DB {
	user := h.currentUser(c)
	if user == nil || isKamarAdminUser(user) {
		return query
	}

	return query.
		Joins("LEFT JOIN kamar_user ON kamar_user.kamar_id = kamars.id").
		Where("kamars.wali_kamar_id = ? OR kamar_user.user_id = ?", user.ID, user.ID).
		Distinct("kamars.id")
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
	query = h.scopeKamarQueryForUser(c, query)

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

	if err := h.enforceKamarAccess(c, &kamar); err != nil {
		return err
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

	if err := h.db.Preload("SecondaryWalies").First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	if err := h.enforceKamarAccess(c, &kamar); err != nil {
		return err
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

	if req.StudentID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Student is required",
		})
	}

	var student models.Student
	if err := h.db.First(&student, req.StudentID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	var existingCount int64
	if err := h.db.Table("kamar_siswa").Where("kamar_id = ? AND student_id = ?", kamar.ID, student.ID).Count(&existingCount).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to validate room membership",
		})
	}
	if existingCount > 0 {
		return c.Status(fiber.StatusConflict).JSON(models.ErrorResponse{
			Error:   "conflict",
			Message: "Student is already assigned to this room",
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

	if err := h.db.Preload("SecondaryWalies").First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	if err := h.enforceKamarAccess(c, &kamar); err != nil {
		return err
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

	if err := h.enforceKamarAccess(c, &kamar); err != nil {
		return err
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

	if req.SecondaryWaliIDs != nil {
		filtered := make([]uint, 0, len(req.SecondaryWaliIDs))
		seen := make(map[uint]struct{})
		for _, id := range req.SecondaryWaliIDs {
			if id == 0 || id == req.WaliKamarID {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			filtered = append(filtered, id)
		}

		var secondaryWalies []*models.User
		if len(filtered) > 0 {
			if err := h.db.Find(&secondaryWalies, filtered).Error; err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
					Error:   "validation_error",
					Message: "Invalid secondary wali IDs",
				})
			}
		}
		h.db.Model(&kamar).Association("SecondaryWalies").Replace(secondaryWalies)
	}

	h.db.Save(&kamar)

	h.db.Preload("WaliKamar").Preload("SecondaryWalies").First(&kamar, kamar.ID)

	return c.JSON(kamar)
}

// Delete removes a kamar
func (h *KamarHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var kamar models.Kamar

	if err := h.db.Preload("SecondaryWalies").First(&kamar, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kamar not found",
		})
	}

	if err := h.enforceKamarDeleteAccess(c); err != nil {
		return err
	}

	h.db.Delete(&kamar)
	return c.JSON(fiber.Map{"message": "Kamar deleted successfully"})
}
