package handlers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
)

// ── GetHistory returns a paginated list of individual attendance records ──

func (h *AbsensiHandler) GetHistory(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	gender := c.Query("gender")
	jenjang := c.Query("jenjang")
	status := c.Query("status")
	timeWindow := c.Query("time_window")
	search := c.Query("search")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Default to current month
	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	type HistoryRecord struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		Tanggal   string `json:"tanggal"`
		Catatan   string `json:"catatan"`
		KelasNama string `json:"kelas_nama"`
	}

	var records []HistoryRecord
	var total int64

	table := "absensis"
	if isDiniyyahAttendanceType(typeStr) {
		table = "absensi_diniyyahs"
	}

	baseQ := h.db.Table(table).
		Select(fmt.Sprintf("%s.id, students.nama_lengkap as name, %s.status, %s.tanggal, %s.catatan, COALESCE(kelas.nama,'') as kelas_nama", table, table, table, table)).
		Joins(fmt.Sprintf("JOIN students ON students.id = %s.student_id", table)).
		Joins("LEFT JOIN kelas ON kelas.id = students.kelas_id").
		Where(fmt.Sprintf("%s.tanggal >= ? AND %s.tanggal < ?", table, table), startDate, endExclusive).
		Where(fmt.Sprintf("%s.deleted_at IS NULL", table))
	if !isDiniyyahAttendanceType(typeStr) {
		baseQ = baseQ.Joins(fmt.Sprintf("JOIN jadwal_formal jf ON jf.id = %s.jadwal_formal_id", table))
		baseQ = applyFormalScheduleTypeFilter(baseQ, "jf", typeStr)
	}

	// Apply filters
	needKelas := kelasID != "" || gender != "" || jenjang != ""
	if needKelas {
		// kelas is already LEFT JOINed above, just add WHERE conditions
		if kelasID != "" {
			baseQ = baseQ.Where("students.kelas_id = ?", kelasID)
		}
		if gender != "" {
			baseQ = baseQ.Where("kelas.gender = ?", gender)
		}
		if jenjang != "" {
			switch jenjang {
			case "smp":
				baseQ = baseQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%7%", "%8%", "%9%")
			case "sma":
				baseQ = baseQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%10%", "%11%", "%12%")
			}
		}
	}
	if status != "" {
		baseQ = baseQ.Where(fmt.Sprintf("%s.status = ?", table), status)
	}
	if timeWindow != "" {
		baseQ = applyTimeWindowFilter(baseQ, table, timeWindow)
	}
	if search != "" {
		baseQ = baseQ.Where("students.nama_lengkap LIKE ?", "%"+search+"%")
	}

	// Count
	countQ := h.db.Table(table).
		Joins(fmt.Sprintf("JOIN students ON students.id = %s.student_id", table)).
		Where(fmt.Sprintf("%s.tanggal >= ? AND %s.tanggal < ?", table, table), startDate, endExclusive).
		Where(fmt.Sprintf("%s.deleted_at IS NULL", table))
	if !isDiniyyahAttendanceType(typeStr) {
		countQ = countQ.Joins(fmt.Sprintf("JOIN jadwal_formal jf ON jf.id = %s.jadwal_formal_id", table))
		countQ = applyFormalScheduleTypeFilter(countQ, "jf", typeStr)
	}

	if needKelas {
		countQ = countQ.Joins("LEFT JOIN kelas ON kelas.id = students.kelas_id")
		if kelasID != "" {
			countQ = countQ.Where("students.kelas_id = ?", kelasID)
		}
		if gender != "" {
			countQ = countQ.Where("kelas.gender = ?", gender)
		}
		if jenjang != "" {
			switch jenjang {
			case "smp":
				countQ = countQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%7%", "%8%", "%9%")
			case "sma":
				countQ = countQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%10%", "%11%", "%12%")
			}
		}
	}
	if status != "" {
		countQ = countQ.Where(fmt.Sprintf("%s.status = ?", table), status)
	}
	if timeWindow != "" {
		countQ = applyTimeWindowFilter(countQ, table, timeWindow)
	}
	if search != "" {
		countQ = countQ.Where("students.nama_lengkap LIKE ?", "%"+search+"%")
	}
	countQ.Count(&total)

	offset := (page - 1) * perPage
	baseQ.Order(fmt.Sprintf("%s.tanggal DESC, students.nama_lengkap ASC", table)).
		Offset(offset).Limit(perPage).Scan(&records)

	if records == nil {
		records = []HistoryRecord{}
	}

	return c.JSON(fiber.Map{
		"data":     records,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

type groupedStudentSummaryRow struct {
	Nama      string `json:"nama"`
	KelasNama string `json:"kelas_nama"`
	Tingkat   string `json:"tingkat"`
	Hadir     int    `json:"hadir"`
	Izin      int    `json:"izin"`
	Sakit     int    `json:"sakit"`
	Alpa      int    `json:"alpa"`
	Total     int    `json:"total"`
}

func (h *AbsensiHandler) getGroupedStudentSummaryRows(typeStr, startDate, endExclusive, kelasID, gender, jenjang, status, timeWindow string) ([]groupedStudentSummaryRow, error) {
	table := "absensis"
	if isDiniyyahAttendanceType(typeStr) {
		table = "absensi_diniyyahs"
	}

	q := h.db.Table(table+" a").
		Select("s.nama_lengkap as nama, COALESCE(k.nama, '') as kelas_nama, COALESCE(k.tingkat, '') as tingkat, SUM(CASE WHEN a.status = 'hadir' THEN 1 ELSE 0 END) as hadir, SUM(CASE WHEN a.status = 'izin' THEN 1 ELSE 0 END) as izin, SUM(CASE WHEN a.status = 'sakit' THEN 1 ELSE 0 END) as sakit, SUM(CASE WHEN a.status = 'alpa' THEN 1 ELSE 0 END) as alpa, COUNT(*) as total").
		Joins("JOIN students s ON s.id = a.student_id").
		Joins("LEFT JOIN kelas k ON k.id = s.kelas_id").
		Where("a.tanggal >= ? AND a.tanggal < ?", startDate, endExclusive).
		Where("a.deleted_at IS NULL")
	if !isDiniyyahAttendanceType(typeStr) {
		q = q.Joins("JOIN jadwal_formal jf ON jf.id = a.jadwal_formal_id")
		q = applyFormalScheduleTypeFilter(q, "jf", typeStr)
	}
	if kelasID != "" {
		q = q.Where("s.kelas_id = ?", kelasID)
	}
	if gender != "" {
		q = q.Where("k.gender = ?", gender)
	}
	if jenjang != "" {
		switch jenjang {
		case "smp":
			q = q.Where("(k.nama LIKE ? OR k.nama LIKE ? OR k.nama LIKE ? OR k.tingkat LIKE ? OR k.tingkat LIKE ? OR k.tingkat LIKE ?)", "%7%", "%8%", "%9%", "%7%", "%8%", "%9%")
		case "sma":
			q = q.Where("(k.nama LIKE ? OR k.nama LIKE ? OR k.nama LIKE ? OR k.tingkat LIKE ? OR k.tingkat LIKE ? OR k.tingkat LIKE ?)", "%10%", "%11%", "%12%", "%10%", "%11%", "%12%")
		}
	}
	if status != "" {
		q = q.Where("a.status = ?", status)
	}
	if timeWindow != "" {
		q = applyTimeWindowFilter(q, "a", timeWindow)
	}

	var rows []groupedStudentSummaryRow
	if err := q.Group("s.nama_lengkap, k.nama, k.tingkat").Order("k.nama ASC, k.tingkat ASC, s.nama_lengkap ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ── ExportStatisticsExcel exports attendance statistics to an .xlsx file ──

func (h *AbsensiHandler) ExportStatisticsExcel(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	gender := c.Query("gender")
	jenjang := c.Query("jenjang")
	status := c.Query("status")
	timeWindow := c.Query("time_window")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	rows, err := h.getGroupedStudentSummaryRows(typeStr, startDate, endExclusive, kelasID, gender, jenjang, status, timeWindow)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch grouped summary"})
	}
	if rows == nil {
		rows = []groupedStudentSummaryRow{}
	}

	f := excelize.NewFile()
	sheet := "Absensi"
	f.SetSheetName("Sheet1", sheet)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2E7D32"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "CCCCCC", Style: 1},
			{Type: "right", Color: "CCCCCC", Style: 1},
			{Type: "top", Color: "CCCCCC", Style: 1},
			{Type: "bottom", Color: "CCCCCC", Style: 1},
		},
	})

	f.SetCellValue(sheet, "A1", fmt.Sprintf("Laporan Absensi %s", strings.ToUpper(typeStr[:1])+typeStr[1:]))
	f.SetCellValue(sheet, "A2", fmt.Sprintf("Periode: %s s/d %s", startDate, endDate))
	f.SetCellValue(sheet, "A3", fmt.Sprintf("Filter: kelas=%s | gender=%s | jenjang=%s | status=%s | time_window=%s", attendanceValueOrDash(kelasID), attendanceValueOrDash(gender), attendanceValueOrDash(jenjang), attendanceValueOrDash(status), attendanceValueOrDash(timeWindow)))
	f.MergeCell(sheet, "A1", "I1")
	f.MergeCell(sheet, "A2", "I2")
	f.MergeCell(sheet, "A3", "I3")

	headers := []string{"No", "Nama Siswa", "Kelas", "Tingkat", "Hadir", "Izin", "Sakit", "Alpa", "Total"}
	for i, h := range headers {
		cell := fmt.Sprintf("%s5", string(rune('A'+i)))
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	for i, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		f.SetColWidth(sheet, col, col, []float64{6, 28, 18, 12, 10, 10, 10, 10, 10}[i])
	}

	totalRecords := 0
	for _, r := range rows {
		totalRecords += r.Total
	}

	groupedRows := map[string][]groupedStudentSummaryRow{}
	for _, r := range rows {
		key := strings.TrimSpace(r.KelasNama + " " + r.Tingkat)
		if key == "" {
			key = "Tanpa Kelas"
		}
		groupedRows[key] = append(groupedRows[key], r)
	}

	orderedKeys := make([]string, 0, len(groupedRows))
	for key := range groupedRows {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)

	currentRow := 6
	sectionStyleID, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "1F2937"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	for _, key := range orderedKeys {
		sectionTitle := key
		f.SetCellValue(sheet, fmt.Sprintf("A%d", currentRow), sectionTitle)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", currentRow), fmt.Sprintf("I%d", currentRow), sectionStyleID)
		f.MergeCell(sheet, fmt.Sprintf("A%d", currentRow), fmt.Sprintf("I%d", currentRow))
		currentRow++

		for i, r := range groupedRows[key] {
			row := currentRow + i
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.Nama)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), r.KelasNama)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), r.Tingkat)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), r.Hadir)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), r.Izin)
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), r.Sakit)
			f.SetCellValue(sheet, fmt.Sprintf("H%d", row), r.Alpa)
			f.SetCellValue(sheet, fmt.Sprintf("I%d", row), r.Total)
		}
		currentRow += len(groupedRows[key])
		currentRow++
	}

	footerRow := currentRow
	f.SetCellValue(sheet, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("Total keseluruhan: %d siswa", totalRecords))
	f.MergeCell(sheet, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("I%d", footerRow))

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=absensi_%s_%s_%s.xlsx", typeStr, startDate, endDate))
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel"})
	}
	return nil
}

// ── ExportStatisticsPDF exports attendance statistics to a PDF file ──

func (h *AbsensiHandler) ExportStatisticsPDF(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	gender := c.Query("gender")
	jenjang := c.Query("jenjang")
	status := c.Query("status")
	timeWindow := c.Query("time_window")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	rows, err := h.getGroupedStudentSummaryRows(typeStr, startDate, endExclusive, kelasID, gender, jenjang, status, timeWindow)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch grouped summary"})
	}
	if rows == nil {
		rows = []groupedStudentSummaryRow{}
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, fmt.Sprintf("Laporan Absensi %s", strings.ToUpper(typeStr[:1])+typeStr[1:]), "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s s/d %s", startDate, endDate), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 7, fmt.Sprintf("Filter: kelas=%s | gender=%s | jenjang=%s | status=%s | time_window=%s", attendanceValueOrDash(kelasID), attendanceValueOrDash(gender), attendanceValueOrDash(jenjang), attendanceValueOrDash(status), attendanceValueOrDash(timeWindow)), "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Ringkasan Per Siswa", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(46, 125, 50)
	pdf.SetTextColor(255, 255, 255)
	pdfHeaders := []float64{10, 48, 24, 18, 18, 18, 18, 18, 18}
	headerLabels := []string{"No", "Nama Siswa", "Kelas", "Tingkat", "Hadir", "Izin", "Sakit", "Alpa", "Total"}
	for i, label := range headerLabels {
		pdf.CellFormat(pdfHeaders[i], 8, label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	groupedRows := map[string][]groupedStudentSummaryRow{}
	for _, r := range rows {
		key := strings.TrimSpace(r.KelasNama + " " + r.Tingkat)
		if key == "" {
			key = "Tanpa Kelas"
		}
		groupedRows[key] = append(groupedRows[key], r)
	}

	orderedKeys := make([]string, 0, len(groupedRows))
	for key := range groupedRows {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(245, 245, 245)
	for _, groupKey := range orderedKeys {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 8, fmt.Sprintf("Kelas: %s", groupKey), "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(46, 125, 50)
		pdf.SetTextColor(255, 255, 255)
		for i, label := range headerLabels {
			pdf.CellFormat(pdfHeaders[i], 8, label, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFillColor(245, 245, 245)
		for i, r := range groupedRows[groupKey] {
			fill := i%2 == 1
			vals := []string{fmt.Sprintf("%d", i+1), r.Nama, r.KelasNama, r.Tingkat, fmt.Sprintf("%d", r.Hadir), fmt.Sprintf("%d", r.Izin), fmt.Sprintf("%d", r.Sakit), fmt.Sprintf("%d", r.Alpa), fmt.Sprintf("%d", r.Total)}
			for j, v := range vals {
				pdf.CellFormat(pdfHeaders[j], 7, v, "1", 0, "C", fill, 0, "")
			}
			pdf.Ln(-1)
		}
		pdf.Ln(3)
	}

	pdf.Ln(5)
	pdf.SetFont("Helvetica", "I", 8)
	totalStudents := 0
	for _, r := range rows {
		totalStudents += r.Total
	}
	pdf.CellFormat(0, 5, fmt.Sprintf("Digenerate pada: %s | Total: %d siswa", time.Now().Format("02/01/2006 15:04"), totalStudents), "", 1, "R", false, 0, "")

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=absensi_%s_%s_%s.pdf", typeStr, startDate, endDate))
	if err := pdf.Output(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate PDF"})
	}
	return nil
}

func attendanceValueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

// truncStr truncates a string to maxLen characters.
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

type missingTeacherAttendanceRow struct {
	Date       time.Time
	Teacher    string
	Lesson     string
	Kelas      string
	Status     string
	Attendance string
}

func (h *AbsensiHandler) getMissingTeacherAttendanceRows(typeStr, startDate, endExclusive, teacherID, gender, kelasID string) ([]missingTeacherAttendanceRow, error) {
	rows := make([]missingTeacherAttendanceRow, 0)

	if isDiniyyahAttendanceType(typeStr) {
		q := h.db.Table("teacher_attendances ta").
			Select("ta.date, u.name as teacher, COALESCE(dl.nama, '-') as lesson, TRIM(CONCAT(COALESCE(k.nama, ''), ' ', COALESCE(k.tingkat, ''))) as kelas, ta.status, 'Belum isi absensi santri' as attendance").
			Joins("JOIN users u ON u.id = ta.user_id").
			Joins("JOIN jadwal_diniyyahs jd ON jd.id = ta.jadwal_diniyyah_id").
			Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
			Joins("LEFT JOIN diniyyah_lessons dl ON dl.id = dkt.diniyyah_lesson_id").
			Joins("LEFT JOIN kelas k ON k.id = dkt.kelas_id").
			Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
			Where("ta.deleted_at IS NULL").
			Where("ta.jadwal_diniyyah_id IS NOT NULL").
			Where("LOWER(COALESCE(ta.status, '')) = ?", "hadir").
			Where("NOT EXISTS (SELECT 1 FROM absensi_diniyyahs ad WHERE ad.deleted_at IS NULL AND ad.jadwal_diniyyah_id = ta.jadwal_diniyyah_id AND DATE(ad.tanggal) = DATE(ta.date))")

		if teacherID != "" {
			q = q.Where("ta.user_id = ?", teacherID)
		}
		if gender != "" {
			q = q.Where("u.gender = ?", gender)
		}
		if kelasID != "" {
			q = q.Where("dkt.kelas_id = ?", kelasID)
		}

		err := q.Order("ta.date DESC, u.name ASC").Scan(&rows).Error
		return rows, err
	}

	q := h.db.Table("teacher_attendances ta").
		Select("ta.date, u.name as teacher, COALESCE(ls.nama, '-') as lesson, TRIM(CONCAT(COALESCE(k.nama, ''), ' ', COALESCE(k.tingkat, ''))) as kelas, ta.status, 'Belum isi absensi santri' as attendance").
		Joins("JOIN users u ON u.id = ta.user_id").
		Joins("JOIN jadwal_formal jf ON jf.id = ta.jadwal_formal_id").
		Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jf.lesson_kelas_teacher_id").
		Joins("LEFT JOIN lessons ls ON ls.id = lkt.lesson_id").
		Joins("LEFT JOIN kelas k ON k.id = lkt.kelas_id").
		Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
		Where("ta.deleted_at IS NULL").
		Where("ta.jadwal_formal_id IS NOT NULL").
		Where("LOWER(COALESCE(ta.status, '')) = ?", "hadir").
		Where("NOT EXISTS (SELECT 1 FROM absensis a WHERE a.deleted_at IS NULL AND a.jadwal_formal_id = ta.jadwal_formal_id AND DATE(a.tanggal) = DATE(ta.date))")

	q = applyFormalScheduleTypeFilter(q, "jf", typeStr)

	if teacherID != "" {
		q = q.Where("ta.user_id = ?", teacherID)
	}
	if gender != "" {
		q = q.Where("u.gender = ?", gender)
	}
	if kelasID != "" {
		q = q.Where("lkt.kelas_id = ?", kelasID)
	}

	err := q.Order("ta.date DESC, u.name ASC").Scan(&rows).Error
	return rows, err
}

// ── ExportTeacherStatisticsPDF exports teacher statistics to a PDF file ──
func (h *AbsensiHandler) ExportTeacherStatisticsPDF(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")
	kelasID := c.Query("kelas_id")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	// 1. Fetch Teacher Summary
	var teacherSummary []TeacherSummaryEntry

	summaryQ := h.db.Table("teacher_attendances ta").
		Select("u.id, u.name, u.foto_guru as avatar, "+
			"SUM(CASE WHEN ta.status = 'Hadir' THEN 1 ELSE 0 END) as hadir, "+
			"SUM(CASE WHEN ta.status = 'Izin' THEN 1 ELSE 0 END) as izin, "+
			"SUM(CASE WHEN ta.status = 'Sakit' THEN 1 ELSE 0 END) as sakit, "+
			"SUM(CASE WHEN ta.status = 'Alpha' THEN 1 ELSE 0 END) as alpha").
		Joins("JOIN users u ON u.id = ta.user_id").
		Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
		Where("ta.deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		summaryQ = summaryQ.Where("ta.jadwal_diniyyah_id IS NOT NULL")
		if kelasID != "" {
			summaryQ = summaryQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = ta.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
	} else {
		summaryQ = summaryQ.Where("ta.jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = ta.jadwal_formal_id")
		summaryQ = applyFormalScheduleTypeFilter(summaryQ, "jf", typeStr)
	}
	if teacherID != "" {
		summaryQ = summaryQ.Where("ta.user_id = ?", teacherID)
	}
	if gender != "" {
		summaryQ = summaryQ.Where("u.gender = ?", gender)
	}

	summaryQ.Group("u.id, u.name, u.foto_guru").Scan(&teacherSummary)

	// Substitute Counts
	type SubCount struct {
		ID         uint
		Name       string
		Avatar     string
		JamMulai   string
		JamSelesai string
	}
	var subCounts []SubCount
	if isDiniyyahAttendanceType(typeStr) {
		subQ := h.db.Table("substitute_logs_diniyyah").
			Select("substitute_logs_diniyyah.substitute_teacher_id as id, u.name, u.foto_guru as avatar, substitute_logs_diniyyah.jam_mulai as jam_mulai, substitute_logs_diniyyah.jam_selesai as jam_selesai").
			Joins("JOIN users u ON u.id = substitute_logs_diniyyah.substitute_teacher_id").
			Where("substitute_logs_diniyyah.date >= ? AND substitute_logs_diniyyah.date < ?", startDate, endExclusive)
		if kelasID != "" {
			subQ = subQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs_diniyyah.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
		if teacherID != "" {
			subQ = subQ.Where("substitute_logs_diniyyah.substitute_teacher_id = ?", teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("u.gender = ?", gender)
		}
		subQ.Scan(&subCounts)
	} else {
		subQ := h.db.Table("substitute_logs").
			Select("substitute_logs.substitute_teacher_id as id, u.name, u.foto_guru as avatar, COALESCE(substitute_logs.jam_mulai, jadwal_formal.jam_mulai) as jam_mulai, COALESCE(substitute_logs.jam_selesai, jadwal_formal.jam_selesai) as jam_selesai").
			Joins("JOIN users u ON u.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL").
			Where("substitute_logs.jadwal_formal_id IS NOT NULL").
			Joins("JOIN jadwal_formal jf ON jf.id = substitute_logs.jadwal_formal_id")
		subQ = applyFormalScheduleTypeFilter(subQ, "jf", typeStr)
		if teacherID != "" {
			subQ = subQ.Where("substitute_logs.substitute_teacher_id = ?", teacherID)
		}
		if gender != "" {
			subQ = subQ.Where("u.gender = ?", gender)
		}
		subQ.Scan(&subCounts)
	}

	for _, sc := range subCounts {
		count := 1
		if !isDiniyyahAttendanceType(typeStr) {
			count = substituteSessionCount(sc.JamMulai, sc.JamSelesai)
		}
		teacherSummary = applySubstituteTeacherCounts(teacherSummary, sc.ID, sc.Name, sc.Avatar, count)
	}

	// Original teacher absences logged via substitute records should be counted against the original teacher only for absence statuses.
	type OriginalSubStatusCount struct {
		ID         uint
		Name       string
		Avatar     string
		Status     string
		JamMulai   string
		JamSelesai string
		Count      int
	}
	var originalSubStatusCounts []OriginalSubStatusCount
	if isDiniyyahAttendanceType(typeStr) {
		origQ := h.db.Table("substitute_logs_diniyyah").
			Select("substitute_logs_diniyyah.original_teacher_id as id, u.name, u.foto_guru as avatar, substitute_logs_diniyyah.status, substitute_logs_diniyyah.jam_mulai as jam_mulai, substitute_logs_diniyyah.jam_selesai as jam_selesai").
			Joins("JOIN users u ON u.id = substitute_logs_diniyyah.original_teacher_id").
			Where("substitute_logs_diniyyah.date >= ? AND substitute_logs_diniyyah.date < ?", startDate, endExclusive)
		if kelasID != "" {
			origQ = origQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs_diniyyah.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
		if teacherID != "" {
			origQ = origQ.Where("substitute_logs_diniyyah.original_teacher_id = ?", teacherID)
		}
		if gender != "" {
			origQ = origQ.Where("u.gender = ?", gender)
		}
		origQ.Scan(&originalSubStatusCounts)
	} else {
		origQ := h.db.Table("substitute_logs").
			Select("substitute_logs.original_teacher_id as id, u.name, u.foto_guru as avatar, substitute_logs.status, COALESCE(substitute_logs.jam_mulai, jadwal_formal.jam_mulai) as jam_mulai, COALESCE(substitute_logs.jam_selesai, jadwal_formal.jam_selesai) as jam_selesai").
			Joins("JOIN users u ON u.id = substitute_logs.original_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL").
			Where("substitute_logs.jadwal_formal_id IS NOT NULL").
			Joins("JOIN jadwal_formal jf ON jf.id = substitute_logs.jadwal_formal_id")
		origQ = applyFormalScheduleTypeFilter(origQ, "jf", typeStr)
		if teacherID != "" {
			origQ = origQ.Where("substitute_logs.original_teacher_id = ?", teacherID)
		}
		if gender != "" {
			origQ = origQ.Where("u.gender = ?", gender)
		}
		origQ.Scan(&originalSubStatusCounts)
	}

	for _, oc := range originalSubStatusCounts {
		status := strings.TrimSpace(strings.ToLower(oc.Status))
		if status != "izin" && status != "sakit" && status != "alpha" {
			continue
		}
		count := 1
		if !isDiniyyahAttendanceType(typeStr) {
			count = substituteSessionCount(oc.JamMulai, oc.JamSelesai)
		}
		teacherSummary = applyOriginalTeacherStatus(teacherSummary, oc.ID, oc.Name, oc.Avatar, status, count)
	}

	// 2. Fetch Substitute History
	type SubHistoryEntry struct {
		Date              time.Time
		Lesson            string
		Kelas             string
		OriginalTeacher   string
		OriginalStatus    string
		SubstituteTeacher string
		Reason            string
	}
	var substituteHistory []SubHistoryEntry

	if !isDiniyyahAttendanceType(typeStr) {
		subHistQ := h.db.Table("substitute_logs").
			Select("substitute_logs.date, lessons.nama as lesson, CONCAT(kelas.nama, ' ', kelas.tingkat) as kelas, "+
				"original.name as original_teacher, substitute_logs.status as original_status, "+
				"substitute.name as substitute_teacher, substitute_logs.reason").
			Joins("JOIN jadwal_formal ON jadwal_formal.id = substitute_logs.jadwal_formal_id").
			Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jadwal_formal.lesson_kelas_teacher_id").
			Joins("JOIN lessons ON lessons.id = lkt.lesson_id").
			Joins("JOIN kelas ON kelas.id = lkt.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL")
		subHistQ = applyFormalScheduleTypeFilter(subHistQ, "jadwal_formal", typeStr)
		if teacherID != "" {
			subHistQ = subHistQ.Where("(substitute_logs.original_teacher_id = ? OR substitute_logs.substitute_teacher_id = ?)", teacherID, teacherID)
		}
		if gender != "" {
			subHistQ = subHistQ.Where("(original.gender = ? OR substitute.gender = ?)", gender, gender)
		}
		subHistQ.Order("substitute_logs.date DESC").Scan(&substituteHistory)
	} else if isDiniyyahAttendanceType(typeStr) {
		subHistQ := h.db.Table("substitute_logs_diniyyah").
			Select("substitute_logs_diniyyah.date, COALESCE(sdls.lesson, diniyyah_lessons.nama, '-') as lesson, COALESCE(sdls.kelas, CONCAT(kelas.nama, ' ', kelas.tingkat), '-') as kelas, "+
				"COALESCE(sdls.original_teacher, original.name, '-') as original_teacher, substitute_logs_diniyyah.status as original_status, "+
				"substitute.name as substitute_teacher, substitute_logs_diniyyah.reason").
			Joins("LEFT JOIN substitute_diniyyah_log_snapshots sdls ON sdls.substitute_diniyyah_log_id = substitute_logs_diniyyah.id").
			Joins("LEFT JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs_diniyyah.jadwal_diniyyah_id").
			Joins("LEFT JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
			Joins("LEFT JOIN diniyyah_lessons ON diniyyah_lessons.id = dkt.diniyyah_lesson_id").
			Joins("LEFT JOIN kelas ON kelas.id = dkt.kelas_id").
			Joins("LEFT JOIN users original ON original.id = substitute_logs_diniyyah.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs_diniyyah.substitute_teacher_id").
			Where("substitute_logs_diniyyah.date >= ? AND substitute_logs_diniyyah.date < ?", startDate, endExclusive)
		if teacherID != "" {
			subHistQ = subHistQ.Where("(substitute_logs_diniyyah.original_teacher_id = ? OR substitute_logs_diniyyah.substitute_teacher_id = ?)", teacherID, teacherID)
		}
		if gender != "" {
			subHistQ = subHistQ.Where("(original.gender = ? OR substitute.gender = ?)", gender, gender)
		}
		if kelasID != "" {
			subHistQ = subHistQ.Where("dkt.kelas_id = ?", kelasID)
		}
		subHistQ.Order("substitute_logs_diniyyah.date DESC").Scan(&substituteHistory)
	}

	// Calculate Global Counts
	teacherCountsMap := map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0, "Substitute": 0}
	for _, t := range teacherSummary {
		teacherCountsMap["Hadir"] += t.Hadir
		teacherCountsMap["Izin"] += t.Izin
		teacherCountsMap["Sakit"] += t.Sakit
		teacherCountsMap["Alpha"] += t.Alpha
	}

	if !isDiniyyahAttendanceType(typeStr) {
		var formalStatusRows []struct {
			ID         uint
			Status     string
			JamMulai   string
			JamSelesai string
			Count      int
		}
		rawStatusQ := h.db.Table("teacher_attendances ta").
			Select("ta.user_id as id, ta.status, COALESCE(jf.jam_mulai, '-') as jam_mulai, COALESCE(jf.jam_selesai, '-') as jam_selesai, count(*) as count").
			Joins("JOIN jadwal_formal jf ON jf.id = ta.jadwal_formal_id").
			Where("ta.date >= ? AND ta.date < ?", startDate, endExclusive).
			Where("ta.deleted_at IS NULL").
			Where("ta.jadwal_formal_id IS NOT NULL").
			Where("ta.status IN ?", []string{"Izin", "Sakit", "Alpha"})
		rawStatusQ = applyFormalScheduleTypeFilter(rawStatusQ, "jf", typeStr)
		if teacherID != "" {
			rawStatusQ = rawStatusQ.Where("ta.user_id = ?", teacherID)
		}
		if gender != "" {
			rawStatusQ = rawStatusQ.Joins("JOIN users u ON u.id = ta.user_id").Where("u.gender = ?", gender)
		}
		if kelasID != "" {
			rawStatusQ = rawStatusQ.Joins("JOIN lesson_kelas_teachers lkt ON lkt.id = jf.lesson_kelas_teacher_id").Where("lkt.kelas_id = ?", kelasID)
		}
		rawStatusQ.Group("ta.user_id, ta.status, COALESCE(jf.jam_mulai, '-'), COALESCE(jf.jam_selesai, '-')").Scan(&formalStatusRows)
		for _, row := range formalStatusRows {
			normCount := normalizeFormalTeacherStatusCount(row.Status, row.Count, row.JamMulai, row.JamSelesai)
			if normCount <= 0 {
				continue
			}
			for i := range teacherSummary {
				if teacherSummary[i].ID == row.ID {
					switch strings.ToLower(row.Status) {
					case "izin":
						teacherSummary[i].Izin = normCount
					case "sakit":
						teacherSummary[i].Sakit = normCount
					case "alpha":
						teacherSummary[i].Alpha = normCount
					}
					break
				}
			}
		}
		teacherCountsMap["Izin"] = 0
		teacherCountsMap["Sakit"] = 0
		teacherCountsMap["Alpha"] = 0
		for _, t := range teacherSummary {
			teacherCountsMap["Izin"] += t.Izin
			teacherCountsMap["Sakit"] += t.Sakit
			teacherCountsMap["Alpha"] += t.Alpha
		}
	}

	totalSub := 0
	for _, sc := range subCounts {
		count := 1
		if !isDiniyyahAttendanceType(typeStr) {
			count = substituteSessionCount(sc.JamMulai, sc.JamSelesai)
		}
		if teacherID != "" {
			tid := uint(0)
			fmt.Sscanf(teacherID, "%d", &tid)
			if sc.ID == tid {
				totalSub += count
			}
		} else {
			totalSub += count
		}
	}
	teacherCountsMap["Substitute"] = totalSub

	exportTimestamp := time.Now().Format("2006-01-02 15:04:05")

	// Build PDF
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Rekapan Kehadiran Guru ("+strings.ToUpper(typeStr)+")", "", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, "Periode: "+startDate+" s/d "+endDate, "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 8, "Tanggal Export: "+exportTimestamp, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Global Counts
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 10, "Ringkasan Total", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Hadir: %d | Izin: %d | Sakit: %d | Alpha: %d | Substitute: %d",
		teacherCountsMap["Hadir"], teacherCountsMap["Izin"], teacherCountsMap["Sakit"],
		teacherCountsMap["Alpha"], teacherCountsMap["Substitute"]), "", 1, "L", false, 0, "")
	pdf.Ln(5)

	// Summary Table
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 10, "Rekapan Per Guru", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 10)
	// Headers: No | Nama Guru | H | I | S | A | Sub
	pdf.CellFormat(10, 8, "No", "1", 0, "C", false, 0, "")
	pdf.CellFormat(90, 8, "Nama Guru", "1", 0, "L", false, 0, "")
	pdf.CellFormat(15, 8, "Hadir", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Izin", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Sakit", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Alpha", "1", 0, "C", false, 0, "")
	pdf.CellFormat(20, 8, "Substitute", "1", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	for i, t := range teacherSummary {
		pdf.CellFormat(10, 8, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		pdf.CellFormat(90, 8, truncStr(t.Name, 40), "1", 0, "L", false, 0, "")
		pdf.CellFormat(15, 8, fmt.Sprintf("%d", t.Hadir), "1", 0, "C", false, 0, "")
		pdf.CellFormat(15, 8, fmt.Sprintf("%d", t.Izin), "1", 0, "C", false, 0, "")
		pdf.CellFormat(15, 8, fmt.Sprintf("%d", t.Sakit), "1", 0, "C", false, 0, "")
		pdf.CellFormat(15, 8, fmt.Sprintf("%d", t.Alpha), "1", 0, "C", false, 0, "")
		pdf.CellFormat(20, 8, fmt.Sprintf("%d", t.Substitute), "1", 1, "C", false, 0, "")
	}

	pdf.Ln(10)

	// Substitute Logs Table
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 10, "Log Guru Pengganti (Substitute)", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(30, 8, "Tanggal", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Pelajaran/Kelas", "1", 0, "L", false, 0, "")
	pdf.CellFormat(40, 8, "Guru Asli", "1", 0, "L", false, 0, "")
	pdf.CellFormat(30, 8, "Status (Asli)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Pengganti", "1", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	for _, log := range substituteHistory {
		pdf.CellFormat(30, 8, log.Date.Format("02/01/2006"), "1", 0, "C", false, 0, "")

		lessonKelasStr := fmt.Sprintf("%s / %s", truncStr(log.Lesson, 15), truncStr(log.Kelas, 10))
		pdf.CellFormat(40, 8, lessonKelasStr, "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 8, truncStr(log.OriginalTeacher, 20), "1", 0, "L", false, 0, "")
		pdf.CellFormat(30, 8, truncStr(log.OriginalStatus, 15), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, truncStr(log.SubstituteTeacher, 20), "1", 1, "L", false, 0, "")
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekapan_Absensi_Guru_%s_%s_%s_%s.pdf", typeStr, startDate, endDate, time.Now().Format("20060102_150405"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

func (h *AbsensiHandler) ExportTeacherMissingAttendancePDF(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")
	kelasID := c.Query("kelas_id")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	missingTeacherRows, err := h.getMissingTeacherAttendanceRows(typeStr, startDate, endExclusive, teacherID, gender, kelasID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Gagal memuat guru yang belum mengisi absensi santri"})
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	exportTimestamp := time.Now().Format("2006-01-02 15:04:05")

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Laporan Guru Belum Isi Absensi Santri", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("Tipe: %s | Periode: %s s/d %s", strings.ToUpper(typeStr), startDate, endDate), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 7, "Tanggal Export: "+exportTimestamp, "", 1, "C", false, 0, "")
	pdf.Ln(3)

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total data: %d", len(missingTeacherRows)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(239, 68, 68)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(25, 8, "Tanggal", "1", 0, "C", true, 0, "")
	pdf.CellFormat(45, 8, "Guru", "1", 0, "L", true, 0, "")
	pdf.CellFormat(50, 8, "Pelajaran", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 8, "Kelas", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 8, "Status", "1", 0, "C", true, 0, "")
	pdf.CellFormat(0, 8, "Keterangan", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(0, 0, 0)
	if len(missingTeacherRows) == 0 {
		pdf.CellFormat(0, 8, "Tidak ada guru yang memenuhi kriteria pada periode ini.", "1", 1, "L", false, 0, "")
	} else {
		for _, row := range missingTeacherRows {
			pdf.CellFormat(25, 8, row.Date.Format("02/01/2006"), "1", 0, "C", false, 0, "")
			pdf.CellFormat(45, 8, truncStr(row.Teacher, 24), "1", 0, "L", false, 0, "")
			pdf.CellFormat(50, 8, truncStr(row.Lesson, 28), "1", 0, "L", false, 0, "")
			pdf.CellFormat(40, 8, truncStr(row.Kelas, 22), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 8, truncStr(row.Status, 12), "1", 0, "C", false, 0, "")
			pdf.CellFormat(0, 8, row.Attendance, "1", 1, "L", false, 0, "")
		}
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Laporan_Guru_Belum_Isi_Absensi_%s_%s_%s_%s.pdf", typeStr, startDate, endDate, time.Now().Format("20060102_150405"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}
