package handlers

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/xuri/excelize/v2"
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

// Graduate marks a student as alumni (status_periode = Lulus).
func (h *StudentHandler) Graduate(c *fiber.Ctx) error {
	id := c.Params("id")
	var student models.Student

	if err := h.db.First(&student, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Student not found",
		})
	}

	statusLulus := "Lulus"
	student.StatusPeriode = &statusLulus
	student.KelasID = nil

	if err := h.db.Save(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to graduate student",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Student graduated successfully",
		"data":    student,
	})
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

// MassDelete soft-deletes students by id list.
func (h *StudentHandler) MassDelete(c *fiber.Ctx) error {
	type reqBody struct {
		IDs []uint `json:"ids"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}
	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "ids is required",
		})
	}

	res := h.db.Where("id IN ?", req.IDs).Delete(&models.Student{})
	if res.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to delete students",
		})
	}

	return c.JSON(fiber.Map{"message": "Students deleted successfully", "deleted": res.RowsAffected})
}

// ExportCSV exports students as XLSX with complete student components.
func (h *StudentHandler) ExportCSV(c *fiber.Ctx) error {
	var students []models.Student
	h.db.Preload("Kelas").Order("nama_lengkap asc").Find(&students)

	f := excelize.NewFile()
	sheet := "Data Santri"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"ID",
		"Nama Lengkap",
		"Jenis Kelamin",
		"Status Periode",
		"Jenjang Pendidikan Periode",
		"Tempat Lahir",
		"Tanggal Lahir",
		"NISN",
		"NIK",
		"No KK",
		"No Akte",
		"Alamat",
		"RT",
		"RW",
		"Desa/Kelurahan",
		"Kecamatan",
		"Kabupaten/Kota",
		"Provinsi",
		"Kode Pos",
		"No HP",
		"Anak Ke",
		"Dari Saudara",
		"Sekolah Asal",
		"Nama Ayah",
		"NIK Ayah",
		"Tahun Lahir Ayah",
		"Pekerjaan Ayah",
		"Pendidikan Ayah",
		"Nama Ibu",
		"NIK Ibu",
		"Tahun Lahir Ibu",
		"Pekerjaan Ibu",
		"Pendidikan Ibu",
		"File KTP Ortu",
		"File Kartu Keluarga",
		"File Ijazah",
		"File Surat Keterangan Lulus",
		"File Akta Kelahiran",
		"File Surat Pindah",
		"Foto Santri",
		"Kelas ID",
		"Kelas Nama",
		"Dibuat Pada",
		"Diubah Pada",
	}

	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F4E78"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	endHeaderCell, _ := excelize.CoordinatesToCellName(len(headers), 1)
	f.SetCellStyle(sheet, "A1", endHeaderCell, headerStyle)

	for i, s := range students {
		row := i + 2
		kelasID := ""
		if s.KelasID != nil {
			kelasID = fmt.Sprintf("%d", *s.KelasID)
		}
		kelasNama := ""
		if s.Kelas != nil {
			kelasNama = s.Kelas.Nama
		}

		values := []string{
			fmt.Sprintf("%d", s.ID),
			s.NamaLengkap,
			stringOrEmpty(s.JenisKelamin),
			stringOrEmpty(s.StatusPeriode),
			stringOrEmpty(s.JenjangPendidikanPeriode),
			stringOrEmpty(s.TempatLahir),
			stringOrEmpty(s.TanggalLahir),
			stringOrEmpty(s.NISN),
			stringOrEmpty(s.NIK),
			stringOrEmpty(s.NoKK),
			stringOrEmpty(s.NoAkte),
			stringOrEmpty(s.Alamat),
			stringOrEmpty(s.RT),
			stringOrEmpty(s.RW),
			stringOrEmpty(s.DesaKelurahan),
			stringOrEmpty(s.Kecamatan),
			stringOrEmpty(s.KabupatenKota),
			stringOrEmpty(s.Provinsi),
			stringOrEmpty(s.KodePos),
			stringOrEmpty(s.NoHP),
			intOrEmpty(s.AnakKe),
			intOrEmpty(s.DariSaudara),
			stringOrEmpty(s.SekolahAsal),
			stringOrEmpty(s.NamaAyah),
			stringOrEmpty(s.NIKAyah),
			stringOrEmpty(s.TahunLahirAyah),
			stringOrEmpty(s.PekerjaanAyah),
			stringOrEmpty(s.PendidikanAyah),
			stringOrEmpty(s.NamaIbu),
			stringOrEmpty(s.NIKIbu),
			stringOrEmpty(s.TahunLahirIbu),
			stringOrEmpty(s.PekerjaanIbu),
			stringOrEmpty(s.PendidikanIbu),
			stringOrEmpty(s.FileKTPOrtu),
			stringOrEmpty(s.FileKartuKeluarga),
			stringOrEmpty(s.FileIjazah),
			stringOrEmpty(s.FileSuratKeteranganLulus),
			stringOrEmpty(s.FileAktaKelahiran),
			stringOrEmpty(s.FileSuratPindah),
			stringOrEmpty(s.FotoSantri),
			kelasID,
			kelasNama,
			s.CreatedAt.Format("2006-01-02 15:04:05"),
			s.UpdatedAt.Format("2006-01-02 15:04:05"),
		}

		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			f.SetCellValue(sheet, cell, value)
		}
	}

	textStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 49})
	textCols := []int{8, 9, 10, 11, 13, 14, 19, 20, 25, 26, 30, 31, 41}
	if len(students) > 0 {
		for _, col := range textCols {
			colName, _ := excelize.ColumnNumberToName(col)
			startCell := fmt.Sprintf("%s2", colName)
			endCell := fmt.Sprintf("%s%d", colName, len(students)+1)
			f.SetCellStyle(sheet, startCell, endCell, textStyle)
		}
	}

	for i := 1; i <= len(headers); i++ {
		colName, _ := excelize.ColumnNumberToName(i)
		f.SetColWidth(sheet, colName, colName, 18)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=students-%s.xlsx", time.Now().Format("20060102-150405")))

	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to export students",
		})
	}

	return nil
}

// ExportTemplate downloads a CSV import template.
func (h *StudentHandler) ExportTemplate(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=students-template.csv")
	template := "nama_lengkap,nisn,jenis_kelamin,status_periode,nama_ayah,no_hp\nContoh Santri,1234567890,Laki-laki,Baru,Ayah Contoh,08123456789\n"
	return c.SendString(template)
}

// ImportCSV imports students from uploaded CSV file.
func (h *StudentHandler) ImportCSV(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "validation_error",
			Message: "CSV file is required (field name: file)",
		})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Failed to open uploaded file",
		})
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil || len(records) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid CSV file",
		})
	}

	created := 0
	skipped := 0
	for i, rec := range records {
		if i == 0 {
			continue // header
		}
		if len(rec) == 0 || strings.TrimSpace(rec[0]) == "" {
			skipped++
			continue
		}

		student := models.Student{
			NamaLengkap: rec[0],
		}
		if len(rec) > 1 && strings.TrimSpace(rec[1]) != "" {
			v := rec[1]
			student.NISN = &v
		}
		if len(rec) > 2 && strings.TrimSpace(rec[2]) != "" {
			v := rec[2]
			student.JenisKelamin = &v
		}
		if len(rec) > 3 && strings.TrimSpace(rec[3]) != "" {
			v := rec[3]
			student.StatusPeriode = &v
		}
		if len(rec) > 4 && strings.TrimSpace(rec[4]) != "" {
			v := rec[4]
			student.NamaAyah = &v
		}
		if len(rec) > 5 && strings.TrimSpace(rec[5]) != "" {
			v := rec[5]
			student.NoHP = &v
		}

		if err := h.db.Create(&student).Error; err != nil {
			skipped++
			continue
		}
		created++
	}

	return c.JSON(fiber.Map{
		"message": "Import completed",
		"created": created,
		"skipped": skipped,
	})
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func intOrEmpty(v *int) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}
