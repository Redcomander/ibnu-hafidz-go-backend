package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type KelasHandler struct {
	db *gorm.DB
}

func NewKelasHandler(db *gorm.DB) *KelasHandler {
	return &KelasHandler{db: db}
}

// List returns paginated classes with search/filter/sort
func (h *KelasHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	search := c.Query("search")
	sort := c.Query("sort", "created_at")
	order := c.Query("order", "desc")

	// Preload Walis & Students
	query := h.db.Model(&models.Kelas{}).Preload("WaliKelas").Preload("WaliKelasDiniyyah").Preload("Students")

	if search != "" {
		query = query.Where("nama LIKE ? OR tingkat LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// Filter by gender if provided
	if gender := c.Query("gender"); gender != "" {
		query = query.Where("gender = ?", gender)
	}

	var total int64
	query.Count(&total)

	var classes []models.Kelas
	offset := (page - 1) * perPage
	query.Order(sort + " " + order).Limit(perPage).Offset(offset).Find(&classes)

	return c.JSON(fiber.Map{
		"data":        classes,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

// Get returns a single class
func (h *KelasHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.Preload("WaliKelas").Preload("WaliKelasDiniyyah").Preload("Students").First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kelas not found",
		})
	}

	return c.JSON(kelas)
}

// GetStats returns statistics for cards
func (h *KelasHandler) GetStats(c *fiber.Ctx) error {
	var totalKelas int64
	var totalSantri int64
	var activeWalis int64

	h.db.Model(&models.Kelas{}).Count(&totalKelas)
	h.db.Model(&models.Student{}).Count(&totalSantri)
	h.db.Model(&models.Kelas{}).Distinct("wali_kelas_id").Where("wali_kelas_id IS NOT NULL").Count(&activeWalis)

	return c.JSON(fiber.Map{
		"total_kelas":  totalKelas,
		"total_santri": totalSantri,
		"active_walis": activeWalis,
	})
}

// Create adds a new class
func (h *KelasHandler) Create(c *fiber.Ctx) error {
	type CreateKelasRequest struct {
		Nama                string  `json:"nama"`
		Tingkat             string  `json:"tingkat"`
		Gender              *string `json:"gender"`
		WaliKelasID         *uint   `json:"wali_kelas_id"`
		WaliKelasDiniyyahID *uint   `json:"wali_kelas_diniyyah_id"`
	}

	var req CreateKelasRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Nama == "" || req.Tingkat == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "Nama dan Tingkat harus diisi",
		})
	}

	kelas := models.Kelas{
		Nama:                req.Nama,
		Tingkat:             req.Tingkat,
		Gender:              req.Gender,
		WaliKelasID:         req.WaliKelasID,
		WaliKelasDiniyyahID: req.WaliKelasDiniyyahID,
	}

	if err := h.db.Create(&kelas).Error; err != nil {
		log.Printf("Failed to create kelas: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to create kelas",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(kelas)
}

// Update modifies a class
func (h *KelasHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kelas not found",
		})
	}

	type UpdateKelasRequest struct {
		Nama                string  `json:"nama"`
		Tingkat             string  `json:"tingkat"`
		Gender              *string `json:"gender"`
		WaliKelasID         *uint   `json:"wali_kelas_id"`
		WaliKelasDiniyyahID *uint   `json:"wali_kelas_diniyyah_id"`
	}

	var req UpdateKelasRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	if req.Nama != "" {
		kelas.Nama = req.Nama
	}
	if req.Tingkat != "" {
		kelas.Tingkat = req.Tingkat
	}
	// Allow clearing gender/walis if sent (logic depends on frontend, assuming partial updates for now)
	// Or simplistic override if provided
	if req.Gender != nil {
		kelas.Gender = req.Gender
	}
	if req.WaliKelasID != nil {
		kelas.WaliKelasID = req.WaliKelasID
	}
	if req.WaliKelasDiniyyahID != nil {
		kelas.WaliKelasDiniyyahID = req.WaliKelasDiniyyahID
	}

	if err := h.db.Save(&kelas).Error; err != nil {
		log.Printf("Failed to update kelas: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to update kelas",
		})
	}

	// Reload with relations
	h.db.Preload("WaliKelas").Preload("WaliKelasDiniyyah").First(&kelas, kelas.ID)

	return c.JSON(kelas)
}

// Delete soft-deletes a class
func (h *KelasHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.Kelas{}, id)

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kelas not found",
		})
	}

	return c.JSON(fiber.Map{"message": "Kelas deleted successfully"})
}

// AddStudent adds a student to a class
func (h *KelasHandler) AddStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kelas not found",
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

	// Update student's kelas_id
	student.KelasID = &kelas.ID
	if err := h.db.Save(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to add student to kelas",
		})
	}

	return c.JSON(fiber.Map{"message": "Student added successfully"})
}

// RemoveStudent removes a student from a class
func (h *KelasHandler) RemoveStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	studentID := c.Params("student_id")
	var kelas models.Kelas

	if err := h.db.First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kelas not found",
		})
	}

	var student models.Student
	if err := h.db.First(&student, studentID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	// Check if student belongs to this kelas
	if student.KelasID == nil || *student.KelasID != kelas.ID {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Student does not belong to this kelas",
		})
	}

	// Remove student from kelas
	student.KelasID = nil
	if err := h.db.Save(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to remove student from kelas",
		})
	}

	return c.JSON(fiber.Map{"message": "Student removed successfully"})
}

// ExportExcel exports kelas students as CSV (excel-friendly).
func (h *KelasHandler) ExportExcel(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.Preload("Students").First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kelas not found"})
	}

	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=kelas-%d-%s.csv", kelas.ID, time.Now().Format("20060102-150405")))

	var b strings.Builder
	b.WriteString("id,nama_lengkap,nisn,jenis_kelamin,status_periode,nama_ayah\n")
	for _, s := range kelas.Students {
		b.WriteString(fmt.Sprintf("%d,\"%s\",\"%s\",\"%s\",\"%s\",\"%s\"\n",
			s.ID,
			escapeCSV(s.NamaLengkap),
			escapeCSV(ptrVal(s.NISN)),
			escapeCSV(ptrVal(s.JenisKelamin)),
			escapeCSV(ptrVal(s.StatusPeriode)),
			escapeCSV(ptrVal(s.NamaAyah)),
		))
	}

	return c.SendString(b.String())
}

// ExportPDF exports a printable plain-text summary.
func (h *KelasHandler) ExportPDF(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.Preload("Students").First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kelas not found"})
	}

	c.Set("Content-Type", "text/plain; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=kelas-%d-%s.txt", kelas.ID, time.Now().Format("20060102-150405")))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("LAPORAN KELAS: %s %s\n", kelas.Nama, kelas.Tingkat))
	b.WriteString("====================================\n")
	for i, s := range kelas.Students {
		b.WriteString(fmt.Sprintf("%d. %s | NISN: %s | Gender: %s\n", i+1, s.NamaLengkap, ptrVal(s.NISN), ptrVal(s.JenisKelamin)))
	}

	return c.SendString(b.String())
}

func ptrVal(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func escapeCSV(v string) string {
	return strings.ReplaceAll(v, "\"", "\"\"")
}
