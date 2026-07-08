package handlers

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type teachingJournalRecord struct {
	JadwalID     uint   `json:"jadwal_id"`
	Tanggal      string `json:"tanggal"`
	LessonName   string `json:"lesson_name"`
	KelasName    string `json:"kelas_name"`
	TeacherName  string `json:"teacher_name"`
	Materi       string `json:"materi"`
	Rangkuman    string `json:"rangkuman"`
	PresentCount int64  `json:"present_count"`
	StudentCount int64  `json:"student_count"`
	JournalKey   string `json:"journal_key"`
}

type teachingJournalItem struct {
	JadwalID uint   `json:"jadwal_id"`
	Tanggal  string `json:"tanggal"`
}

type absentStudentRow struct {
	StudentName string
	Status      string
	Reason      string
}

func canManageTeachingJournal(user *models.User) bool {
	return canManageTeacherAttendance(user)
}

func (h *AbsensiHandler) teachingJournalBaseQuery(typeStr, startDate, endExclusive, kelasID, search string) *gorm.DB {
	q := h.db.Table("absensis a").
		Joins("JOIN jadwal_formal jf ON jf.id = a.jadwal_formal_id").
		Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jf.lesson_kelas_teacher_id").
		Joins("LEFT JOIN lessons ls ON ls.id = lkt.lesson_id").
		Joins("LEFT JOIN kelas k ON k.id = lkt.kelas_id").
		Joins("LEFT JOIN users u ON u.id = lkt.user_id").
		Where("a.tanggal >= ? AND a.tanggal < ?", startDate, endExclusive).
		Where("a.deleted_at IS NULL")

	q = applyFormalScheduleTypeFilter(q, "jf", typeStr)

	if kelasID != "" {
		q = q.Where("k.id = ?", kelasID)
	}
	if search != "" {
		term := "%" + strings.ToLower(strings.TrimSpace(search)) + "%"
		q = q.Where("LOWER(COALESCE(ls.nama, '')) LIKE ? OR LOWER(COALESCE(k.nama, '')) LIKE ? OR LOWER(COALESCE(k.tingkat, '')) LIKE ? OR LOWER(COALESCE(u.name, '')) LIKE ? OR LOWER(COALESCE(a.materi, '')) LIKE ? OR LOWER(COALESCE(a.rangkuman, '')) LIKE ?", term, term, term, term, term, term)
	}

	return q
}

func (h *AbsensiHandler) GetTeachingJournals(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	search := c.Query("search")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "15"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 15
	}

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	baseQ := h.teachingJournalBaseQuery(typeStr, startDate, endExclusive, kelasID, search)
	groupedQ := baseQ.Session(&gorm.Session{}).
		Select("a.jadwal_formal_id, DATE(a.tanggal) as tanggal").
		Group("a.jadwal_formal_id, DATE(a.tanggal)")

	var total int64
	if err := h.db.Table("(?) as teaching_journals", groupedQ).Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghitung jurnal mengajar"})
	}

	var rows []teachingJournalRecord
	offset := (page - 1) * perPage
	if err := baseQ.
		Select("a.jadwal_formal_id as jadwal_id, DATE_FORMAT(MIN(a.tanggal), '%Y-%m-%d') as tanggal, COALESCE(ls.nama, '-') as lesson_name, TRIM(CONCAT(COALESCE(k.nama, ''), ' ', COALESCE(k.tingkat, ''))) as kelas_name, COALESCE(u.name, '-') as teacher_name, MAX(COALESCE(a.materi, '')) as materi, MAX(COALESCE(a.rangkuman, '')) as rangkuman, SUM(CASE WHEN a.status = 'hadir' THEN 1 ELSE 0 END) as present_count, COUNT(*) as student_count").
		Group("a.jadwal_formal_id, DATE(a.tanggal), ls.nama, k.nama, k.tingkat, u.name").
		Order("DATE(a.tanggal) DESC, ls.nama ASC, k.nama ASC").
		Offset(offset).Limit(perPage).
		Scan(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal memuat jurnal mengajar"})
	}

	for i := range rows {
		rows[i].Materi = strings.TrimSpace(rows[i].Materi)
		rows[i].Rangkuman = strings.TrimSpace(rows[i].Rangkuman)
		rows[i].KelasName = strings.TrimSpace(rows[i].KelasName)
		rows[i].JournalKey = fmt.Sprintf("%d__%s", rows[i].JadwalID, rows[i].Tanggal)
	}

	if rows == nil {
		rows = []teachingJournalRecord{}
	}

	return c.JSON(fiber.Map{
		"data":     rows,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *AbsensiHandler) UpdateTeachingJournal(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !canManageTeachingJournal(user) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk mengubah jurnal mengajar"})
	}

	jadwalID, err := strconv.ParseUint(c.Params("jadwalID"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Jadwal tidak valid"})
	}
	tanggal := c.Params("tanggal")
	if _, err := time.Parse("2006-01-02", tanggal); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tanggal tidak valid"})
	}

	var req struct {
		Materi    *string `json:"materi"`
		Rangkuman *string `json:"rangkuman"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body request tidak valid"})
	}

	updates := map[string]interface{}{}
	if req.Materi != nil {
		updates["materi"] = strings.TrimSpace(*req.Materi)
	}
	if req.Rangkuman != nil {
		updates["rangkuman"] = strings.TrimSpace(*req.Rangkuman)
	}
	if len(updates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tidak ada data yang diperbarui"})
	}
	start, end := dateRange(tanggal)

	result := h.db.Model(&models.Absensi{}).
		Where("jadwal_formal_id = ? AND tanggal >= ? AND tanggal < ?", uint(jadwalID), start, end).
		Updates(updates)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal memperbarui jurnal mengajar"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Jurnal mengajar tidak ditemukan"})
	}

	return c.JSON(fiber.Map{"message": "Jurnal mengajar diperbarui"})
}

func (h *AbsensiHandler) DeleteTeachingJournal(c *fiber.Ctx) error {
	user, err := h.getUserFromContext(c)
	if err != nil {
		return err
	}
	if !canManageTeachingJournal(user) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Anda tidak memiliki akses untuk menghapus jurnal mengajar"})
	}

	jadwalID, err := strconv.ParseUint(c.Params("jadwalID"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Jadwal tidak valid"})
	}
	tanggal := c.Params("tanggal")
	if _, err := time.Parse("2006-01-02", tanggal); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Tanggal tidak valid"})
	}
	start, end := dateRange(tanggal)

	result := h.db.Where("jadwal_formal_id = ? AND tanggal >= ? AND tanggal < ?", uint(jadwalID), start, end).Delete(&models.Absensi{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal menghapus jurnal mengajar"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Jurnal mengajar tidak ditemukan"})
	}

	return c.JSON(fiber.Map{"message": "Jurnal mengajar dihapus"})
}

func (h *AbsensiHandler) ExportTeachingJournalsPDF(c *fiber.Ctx) error {
	var req struct {
		Items []teachingJournalItem `json:"items"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body request tidak valid"})
	}
	if len(req.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Pilih minimal satu jurnal mengajar"})
	}

	rows := make([]teachingJournalRecord, 0, len(req.Items))
	for _, item := range req.Items {
		if item.JadwalID == 0 {
			continue
		}
		if _, err := time.Parse("2006-01-02", item.Tanggal); err != nil {
			continue
		}
		start, end := dateRange(item.Tanggal)

		var row teachingJournalRecord
		err := h.db.Table("absensis a").
			Select("a.jadwal_formal_id as jadwal_id, DATE_FORMAT(MIN(a.tanggal), '%Y-%m-%d') as tanggal, COALESCE(ls.nama, '-') as lesson_name, TRIM(CONCAT(COALESCE(k.nama, ''), ' ', COALESCE(k.tingkat, ''))) as kelas_name, COALESCE(u.name, '-') as teacher_name, MAX(COALESCE(a.materi, '')) as materi, MAX(COALESCE(a.rangkuman, '')) as rangkuman, SUM(CASE WHEN a.status = 'hadir' THEN 1 ELSE 0 END) as present_count, COUNT(*) as student_count").
			Joins("JOIN jadwal_formal jf ON jf.id = a.jadwal_formal_id").
			Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jf.lesson_kelas_teacher_id").
			Joins("LEFT JOIN lessons ls ON ls.id = lkt.lesson_id").
			Joins("LEFT JOIN kelas k ON k.id = lkt.kelas_id").
			Joins("LEFT JOIN users u ON u.id = lkt.user_id").
			Where("a.jadwal_formal_id = ? AND a.tanggal >= ? AND a.tanggal < ?", item.JadwalID, start, end).
			Where("a.deleted_at IS NULL").
			Group("a.jadwal_formal_id, DATE(a.tanggal), ls.nama, k.nama, k.tingkat, u.name").
			Take(&row).Error
		if err == nil {
			row.Materi = strings.TrimSpace(row.Materi)
			row.Rangkuman = strings.TrimSpace(row.Rangkuman)
			row.KelasName = strings.TrimSpace(row.KelasName)
			row.JournalKey = fmt.Sprintf("%d__%s", row.JadwalID, row.Tanggal)
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Tidak ada jurnal mengajar yang dapat diunduh"})
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 15)
	pdf.CellFormat(0, 8, "Jurnal Mengajar", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Dicetak pada %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	for idx, row := range rows {
		if idx > 0 {
			pdf.Ln(3)
		}

		start, end := dateRange(row.Tanggal)
		var absentRows []absentStudentRow
		h.db.Table("absensis a").
			Select("COALESCE(s.nama_lengkap, '-') as student_name, a.status, COALESCE(a.catatan, '') as reason").
			Joins("LEFT JOIN students s ON s.id = a.student_id").
			Where("a.jadwal_formal_id = ? AND a.tanggal >= ? AND a.tanggal < ?", row.JadwalID, start, end).
			Where("a.deleted_at IS NULL").
			Where("a.status IN ?", []string{"izin", "sakit", "alpa"}).
			Order("FIELD(a.status, 'izin', 'sakit', 'alpa'), s.nama_lengkap ASC").
			Scan(&absentRows)

		pdf.SetFillColor(234, 247, 236)
		pdf.SetDrawColor(183, 225, 188)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(0, 8, fmt.Sprintf("%s - %s", row.LessonName, row.Tanggal), "1", 1, "L", true, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(40, 7, "Kelas", "1", 0, "L", false, 0, "")
		pdf.CellFormat(0, 7, row.KelasName, "1", 1, "L", false, 0, "")
		pdf.CellFormat(40, 7, "Pengajar", "1", 0, "L", false, 0, "")
		pdf.CellFormat(0, 7, row.TeacherName, "1", 1, "L", false, 0, "")
		pdf.CellFormat(40, 7, "Jumlah Hadir", "1", 0, "L", false, 0, "")
		pdf.CellFormat(0, 7, fmt.Sprintf("%d dari %d santri", row.PresentCount, row.StudentCount), "1", 1, "L", false, 0, "")
		pdf.CellFormat(40, 7, "Materi", "1", 0, "L", false, 0, "")
		x, y := pdf.GetXY()
		pdf.MultiCell(0, 7, defaultString(row.Materi, "-"), "1", "L", false)
		if pdf.GetY() == y {
			pdf.SetXY(x, y)
		}

		pdf.CellFormat(40, 7, "Rangkuman", "1", 0, "L", false, 0, "")
		xSummary, ySummary := pdf.GetXY()
		pdf.MultiCell(0, 7, defaultString(row.Rangkuman, "-"), "1", "L", false)
		if pdf.GetY() == ySummary {
			pdf.SetXY(xSummary, ySummary)
		}

		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 7, "Siswa Tidak Masuk", "1", 1, "L", false, 0, "")
		if len(absentRows) == 0 {
			pdf.SetFont("Arial", "", 10)
			pdf.CellFormat(0, 7, "-", "1", 1, "L", false, 0, "")
			continue
		}

		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(70, 7, "Nama", "1", 0, "L", false, 0, "")
		pdf.CellFormat(28, 7, "Status", "1", 0, "L", false, 0, "")
		pdf.CellFormat(0, 7, "Alasan", "1", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 9)
		for _, absent := range absentRows {
			pdf.CellFormat(70, 7, defaultString(strings.TrimSpace(absent.StudentName), "-"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(28, 7, absenceStatusLabel(absent.Status), "1", 0, "L", false, 0, "")
			xReason, yReason := pdf.GetXY()
			pdf.MultiCell(0, 7, defaultString(strings.TrimSpace(absent.Reason), "-"), "1", "L", false)
			if pdf.GetY() == yReason {
				pdf.SetXY(xReason, yReason)
			}
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal membuat file jurnal mengajar"})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=jurnal_mengajar.pdf")
	return c.Send(buf.Bytes())
}

func defaultString(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func absenceStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "izin":
		return "Izin"
	case "sakit":
		return "Sakit"
	case "alpa", "alpha":
		return "Alpha"
	default:
		return "-"
	}
}
