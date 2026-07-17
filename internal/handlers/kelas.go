package handlers

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/xuri/excelize/v2"
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

// ExportExcel exports kelas students as an .xlsx roster.
func (h *KelasHandler) ExportExcel(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.Preload("WaliKelas").Preload("WaliKelasDiniyyah").Preload("Students").First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kelas not found"})
	}

	f := excelize.NewFile()
	sheet := "Daftar Santri"
	f.SetSheetName("Sheet1", sheet)

	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14, Color: "1F2937"}})
	metaStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Size: 10, Color: "374151"}})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4F46E5"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	cellStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "111827"},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border: []excelize.Border{{Type: "left", Color: "E5E7EB", Style: 1}, {Type: "right", Color: "E5E7EB", Style: 1}, {Type: "top", Color: "E5E7EB", Style: 1}, {Type: "bottom", Color: "E5E7EB", Style: 1}},
	})

	f.SetCellValue(sheet, "A1", "DAFTAR SANTRI")
	f.SetCellValue(sheet, "A2", fmt.Sprintf("Kelas: %s %s", kelas.Nama, kelas.Tingkat))
	f.SetCellValue(sheet, "A3", fmt.Sprintf("Dicetak pada: %s", time.Now().Format("02 Jan 2006 15:04")))
	f.MergeCell(sheet, "A1", "H1")
	f.MergeCell(sheet, "A2", "H2")
	f.MergeCell(sheet, "A3", "H3")
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	f.SetCellStyle(sheet, "A2", "A3", metaStyle)

	f.SetCellValue(sheet, "A5", "Nama Kelas")
	f.SetCellValue(sheet, "B5", kelas.Nama)
	f.SetCellValue(sheet, "A6", "Tingkat")
	f.SetCellValue(sheet, "B6", kelas.Tingkat)
	f.SetCellValue(sheet, "A7", "Gender")
	f.SetCellValue(sheet, "B7", strings.Title(strings.ToLower(ptrVal(kelas.Gender))))
	f.SetCellValue(sheet, "A8", "Wali Kelas")
	f.SetCellValue(sheet, "B8", ptrUserName(kelas.WaliKelas))
	f.SetCellValue(sheet, "A9", "Wali Kelas Diniyyah")
	f.SetCellValue(sheet, "B9", ptrUserName(kelas.WaliKelasDiniyyah))
	f.SetCellValue(sheet, "A10", "Jumlah Santri")
	f.SetCellValue(sheet, "B10", len(kelas.Students))
	for row := 5; row <= 10; row++ {
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), metaStyle)
		f.SetCellStyle(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), metaStyle)
	}

	headers := []string{"No", "NISN", "Nama Lengkap", "Jenis Kelamin", "Tempat Lahir", "Tanggal Lahir", "Alamat", "No. HP"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 12)
		f.SetCellValue(sheet, cell, header)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	for i, student := range kelas.Students {
		row := 13 + i
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ptrVal(student.NISN))
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), student.NamaLengkap)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), ptrVal(student.JenisKelamin))
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), ptrVal(student.TempatLahir))
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), ptrVal(student.TanggalLahir))
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), ptrVal(student.Alamat))
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), ptrVal(student.NoHP))
		for col := 1; col <= len(headers); col++ {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			f.SetCellStyle(sheet, cell, cell, cellStyle)
		}
	}

	if len(kelas.Students) == 0 {
		f.SetCellValue(sheet, "A13", "Belum ada santri yang terdaftar di kelas ini.")
	}

	for _, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		_ = f.SetColWidth(sheet, col, col, map[string]float64{"A": 6, "B": 18, "C": 28, "D": 14, "E": 20, "F": 16, "G": 34, "H": 18}[col])
	}

	fileName := fmt.Sprintf("Daftar_Santri_%s_%s.xlsx", safeKelasFileName(kelas), time.Now().Format("20060102-150405"))
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to generate kelas excel"})
	}

	return c.Send(buf.Bytes())
}

// ExportPDF exports kelas students as a printable PDF roster.
func (h *KelasHandler) ExportPDF(c *fiber.Ctx) error {
	id := c.Params("id")
	var kelas models.Kelas

	if err := h.db.Preload("WaliKelas").Preload("WaliKelasDiniyyah").Preload("Students").First(&kelas, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kelas not found"})
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "DAFTAR SANTRI", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("Kelas: %s %s", kelas.Nama, kelas.Tingkat), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 7, fmt.Sprintf("Dicetak pada: %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 8, "Informasi Kelas", "1", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(45, 8, "Nama Kelas", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, kelas.Nama, "1", 1, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Tingkat", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, kelas.Tingkat, "1", 1, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Gender", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, strings.Title(strings.ToLower(ptrVal(kelas.Gender))), "1", 1, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Wali Kelas", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, ptrUserName(kelas.WaliKelas), "1", 1, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Wali Kelas Diniyyah", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, ptrUserName(kelas.WaliKelasDiniyyah), "1", 1, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Jumlah Santri", "1", 0, "L", false, 0, "")
	pdf.CellFormat(0, 8, fmt.Sprintf("%d orang", len(kelas.Students)), "1", 1, "L", false, 0, "")
	pdf.Ln(4)

	if len(kelas.Students) == 0 {
		pdf.CellFormat(0, 8, "Belum ada santri yang terdaftar di kelas ini.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(10, 8, "No", "1", 0, "C", false, 0, "")
		pdf.CellFormat(25, 8, "NISN", "1", 0, "C", false, 0, "")
		pdf.CellFormat(65, 8, "Nama Lengkap", "1", 0, "C", false, 0, "")
		pdf.CellFormat(20, 8, "L/P", "1", 0, "C", false, 0, "")
		pdf.CellFormat(30, 8, "Tempat Lahir", "1", 0, "C", false, 0, "")
		pdf.CellFormat(25, 8, "Tanggal Lahir", "1", 0, "C", false, 0, "")
		pdf.CellFormat(0, 8, "No. HP", "1", 1, "C", false, 0, "")

		pdf.SetFont("Arial", "", 9)
		for i, student := range kelas.Students {
			pdf.CellFormat(10, 8, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
			pdf.CellFormat(25, 8, truncKelasPDF(ptrVal(student.NISN), 18), "1", 0, "L", false, 0, "")
			pdf.CellFormat(65, 8, truncKelasPDF(student.NamaLengkap, 35), "1", 0, "L", false, 0, "")
			pdf.CellFormat(20, 8, strings.ToUpper(ptrVal(student.JenisKelamin)), "1", 0, "C", false, 0, "")
			pdf.CellFormat(30, 8, truncKelasPDF(ptrVal(student.TempatLahir), 18), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 8, truncKelasPDF(ptrVal(student.TanggalLahir), 12), "1", 0, "C", false, 0, "")
			pdf.CellFormat(0, 8, truncKelasPDF(ptrVal(student.NoHP), 20), "1", 1, "L", false, 0, "")
		}
	}

	fileName := fmt.Sprintf("Daftar_Santri_%s_%s.pdf", safeKelasFileName(kelas), time.Now().Format("20060102-150405"))
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to generate kelas pdf"})
	}

	return c.Send(buf.Bytes())
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

func ptrUserName(u *models.User) string {
	if u == nil || strings.TrimSpace(u.Name) == "" {
		return "Belum Ditentukan"
	}
	return u.Name
}

func safeKelasFileName(kelas models.Kelas) string {
	base := strings.TrimSpace(kelas.Nama + "_" + kelas.Tingkat)
	if base == "" {
		base = "kelas"
	}
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")
	return base
}

func truncKelasPDF(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
