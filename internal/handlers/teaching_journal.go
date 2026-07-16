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

type teachingJournalReportGroup struct {
	KelasName    string
	Rows         []teachingJournalRecord
	JournalCount int
	PresentCount int64
	StudentCount int64
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

	groups := make([]teachingJournalReportGroup, 0)
	groupIndex := map[string]int{}
	var totalPresent int64
	var totalStudents int64
	for _, row := range rows {
		key := strings.TrimSpace(row.KelasName)
		idx, ok := groupIndex[key]
		if !ok {
			groups = append(groups, teachingJournalReportGroup{KelasName: key})
			idx = len(groups) - 1
			groupIndex[key] = idx
		}
		groups[idx].Rows = append(groups[idx].Rows, row)
		groups[idx].JournalCount++
		groups[idx].PresentCount += row.PresentCount
		groups[idx].StudentCount += row.StudentCount
		totalPresent += row.PresentCount
		totalStudents += row.StudentCount
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 15)
	pdf.CellFormat(0, 8, "Rekap Jurnal Mengajar", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Dicetak pada %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	pdf.SetFillColor(245, 245, 245)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(38, 8, "Total Jurnal", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 8, "Total Hadir", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 8, "Total Santri", "1", 0, "L", true, 0, "")
	pdf.CellFormat(0, 8, "Rata-rata Kehadiran", "1", 1, "L", true, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(38, 8, fmt.Sprintf("%d", len(rows)), "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, fmt.Sprintf("%d", totalPresent), "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, fmt.Sprintf("%d", totalStudents), "1", 0, "C", false, 0, "")
	avgAttendance := "0%"
	if totalStudents > 0 {
		avgAttendance = fmt.Sprintf("%.1f%%", (float64(totalPresent)/float64(totalStudents))*100)
	}
	pdf.CellFormat(0, 8, avgAttendance, "1", 1, "C", false, 0, "")
	pdf.Ln(2)

	for i, group := range groups {
		if i > 0 {
			pdf.AddPage()
		}

		groupTitle := group.KelasName
		if groupTitle == "" {
			groupTitle = "Lainnya"
		}

		pdf.SetFont("Arial", "B", 12)
		pdf.CellFormat(0, 7, fmt.Sprintf("Kelas: %s", groupTitle), "0", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 6, fmt.Sprintf("Jurnal: %d | Hadir: %d | Santri: %d", group.JournalCount, group.PresentCount, group.StudentCount), "0", 1, "L", false, 0, "")
		pdf.Ln(1)

		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(18, 7, "Tanggal", "1", 0, "C", true, 0, "")
		pdf.CellFormat(28, 7, "Pelajaran", "1", 0, "C", true, 0, "")
		pdf.CellFormat(32, 7, "Pengajar", "1", 0, "C", true, 0, "")
		pdf.CellFormat(28, 7, "Hadir", "1", 0, "C", true, 0, "")
		pdf.CellFormat(32, 7, "Materi", "1", 0, "C", true, 0, "")
		pdf.CellFormat(0, 7, "Rangkuman", "1", 1, "C", true, 0, "")

		pdf.SetFont("Arial", "", 8)
		for _, row := range group.Rows {
			pdf.CellFormat(18, 7, row.Tanggal, "1", 0, "L", false, 0, "")
			pdf.CellFormat(28, 7, defaultString(row.LessonName, "-"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(32, 7, defaultString(row.TeacherName, "-"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(28, 7, fmt.Sprintf("%d / %d", row.PresentCount, row.StudentCount), "1", 0, "C", false, 0, "")
			pdf.CellFormat(32, 7, previewString(row.Materi, 35), "1", 0, "L", false, 0, "")
			pdf.MultiCell(0, 7, previewString(row.Rangkuman, 90), "1", "L", false)
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

func previewString(v string, limit int) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	runes := []rune(v)
	if len(runes) <= limit {
		return v
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
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
