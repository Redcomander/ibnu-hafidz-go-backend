package handlers

import (
	"fmt"
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

// ── ExportStatisticsExcel exports attendance statistics to an .xlsx file ──

func (h *AbsensiHandler) ExportStatisticsExcel(c *fiber.Ctx) error {
	typeStr := c.Query("type", "formal")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	kelasID := c.Query("kelas_id")
	gender := c.Query("gender")
	jenjang := c.Query("jenjang")
	status := c.Query("status")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	table := "absensis"
	if isDiniyyahAttendanceType(typeStr) {
		table = "absensi_diniyyahs"
	}

	type Row struct {
		Name      string `json:"name"`
		Status    string `json:"status"`
		Tanggal   string `json:"tanggal"`
		Catatan   string `json:"catatan"`
		KelasNama string `json:"kelas_nama"`
	}

	q := h.db.Table(table).
		Select(fmt.Sprintf("students.nama_lengkap as name, %s.status, %s.tanggal, %s.catatan, COALESCE(kelas.nama,'') as kelas_nama", table, table, table)).
		Joins(fmt.Sprintf("JOIN students ON students.id = %s.student_id", table)).
		Joins("LEFT JOIN kelas ON kelas.id = students.kelas_id").
		Where(fmt.Sprintf("%s.tanggal >= ? AND %s.tanggal < ?", table, table), startDate, endExclusive).
		Where(fmt.Sprintf("%s.deleted_at IS NULL", table)).
		Order(fmt.Sprintf("%s.tanggal DESC, students.nama_lengkap ASC", table))
	if !isDiniyyahAttendanceType(typeStr) {
		q = q.Joins(fmt.Sprintf("JOIN jadwal_formal jf ON jf.id = %s.jadwal_formal_id", table))
		q = applyFormalScheduleTypeFilter(q, "jf", typeStr)
	}

	// Apply filters
	if kelasID != "" {
		q = q.Where("students.kelas_id = ?", kelasID)
	}
	if gender != "" {
		q = q.Where("kelas.gender = ?", gender)
	}
	if jenjang != "" {
		switch jenjang {
		case "smp":
			q = q.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%7%", "%8%", "%9%")
		case "sma":
			q = q.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%10%", "%11%", "%12%")
		}
	}
	if status != "" {
		q = q.Where(fmt.Sprintf("%s.status = ?", table), status)
	}

	var rows []Row
	q.Scan(&rows)
	if rows == nil {
		rows = []Row{}
	}

	// Build Excel
	f := excelize.NewFile()
	sheet := "Absensi"
	f.SetSheetName("Sheet1", sheet)

	// Header style
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

	// Title
	f.SetCellValue(sheet, "A1", fmt.Sprintf("Laporan Absensi %s", strings.ToUpper(typeStr[:1])+typeStr[1:]))
	f.SetCellValue(sheet, "A2", fmt.Sprintf("Periode: %s s/d %s", startDate, endDate))
	f.MergeCell(sheet, "A1", "E1")
	f.MergeCell(sheet, "A2", "E2")

	// Headers
	headers := []string{"No", "Nama Siswa", "Kelas", "Tanggal", "Status", "Catatan"}
	for i, h := range headers {
		cell := fmt.Sprintf("%s4", string(rune('A'+i)))
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Column widths
	f.SetColWidth(sheet, "A", "A", 5)
	f.SetColWidth(sheet, "B", "B", 30)
	f.SetColWidth(sheet, "C", "C", 15)
	f.SetColWidth(sheet, "D", "D", 15)
	f.SetColWidth(sheet, "E", "E", 10)
	f.SetColWidth(sheet, "F", "F", 30)

	// Data rows
	for i, r := range rows {
		row := i + 5
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.Name)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), r.KelasNama)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), r.Tanggal)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), r.Status)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), r.Catatan)
	}

	// Summary row
	summaryRow := len(rows) + 6
	f.SetCellValue(sheet, fmt.Sprintf("A%d", summaryRow), fmt.Sprintf("Total: %d records", len(rows)))

	// Write to response
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

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	endT, _ := time.Parse("2006-01-02", endDate)
	endExclusive := endT.AddDate(0, 0, 1).Format("2006-01-02")

	table := "absensis"
	if isDiniyyahAttendanceType(typeStr) {
		table = "absensi_diniyyahs"
	}

	// --- Get counts ---
	type SC struct {
		Status string
		Count  int
	}
	var counts []SC
	cntQ := h.db.Table(table).
		Select("status, count(*) as count").
		Joins(fmt.Sprintf("JOIN students ON students.id = %s.student_id", table)).
		Where(fmt.Sprintf("%s.tanggal >= ? AND %s.tanggal < ?", table, table), startDate, endExclusive).
		Where("deleted_at IS NULL").
		Group("status")
	if !isDiniyyahAttendanceType(typeStr) {
		cntQ = cntQ.Joins(fmt.Sprintf("JOIN jadwal_formal jf ON jf.id = %s.jadwal_formal_id", table))
		cntQ = applyFormalScheduleTypeFilter(cntQ, "jf", typeStr)
	}

	needKelas := kelasID != "" || gender != "" || jenjang != ""
	if needKelas {
		cntQ = cntQ.Joins("JOIN kelas ON kelas.id = students.kelas_id")
		if kelasID != "" {
			cntQ = cntQ.Where("students.kelas_id = ?", kelasID)
		}
		if gender != "" {
			cntQ = cntQ.Where("kelas.gender = ?", gender)
		}
		if jenjang != "" {
			switch jenjang {
			case "smp":
				cntQ = cntQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%7%", "%8%", "%9%")
			case "sma":
				cntQ = cntQ.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%10%", "%11%", "%12%")
			}
		}
	}
	if status != "" {
		cntQ = cntQ.Where(fmt.Sprintf("%s.status = ?", table), status)
	}
	cntQ.Scan(&counts)

	statusMap := map[string]int{"hadir": 0, "izin": 0, "sakit": 0, "alpa": 0}
	for _, sc := range counts {
		statusMap[sc.Status] = sc.Count
	}

	// --- Get detail rows ---
	type Row struct {
		Name    string
		Status  string
		Tanggal string
		Catatan string
	}
	var rows []Row
	q := h.db.Table(table).
		Select(fmt.Sprintf("students.nama_lengkap as name, %s.status, %s.tanggal, %s.catatan", table, table, table)).
		Joins(fmt.Sprintf("JOIN students ON students.id = %s.student_id", table)).
		Where(fmt.Sprintf("%s.tanggal >= ? AND %s.tanggal < ?", table, table), startDate, endExclusive).
		Where(fmt.Sprintf("%s.deleted_at IS NULL", table)).
		Order(fmt.Sprintf("%s.tanggal DESC", table)).
		Limit(500)
	if !isDiniyyahAttendanceType(typeStr) {
		q = q.Joins(fmt.Sprintf("JOIN jadwal_formal jf ON jf.id = %s.jadwal_formal_id", table))
		q = applyFormalScheduleTypeFilter(q, "jf", typeStr)
	}

	if needKelas {
		q = q.Joins("JOIN kelas ON kelas.id = students.kelas_id")
		if kelasID != "" {
			q = q.Where("students.kelas_id = ?", kelasID)
		}
		if gender != "" {
			q = q.Where("kelas.gender = ?", gender)
		}
		if jenjang != "" {
			switch jenjang {
			case "smp":
				q = q.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%7%", "%8%", "%9%")
			case "sma":
				q = q.Where("(kelas.nama LIKE ? OR kelas.nama LIKE ? OR kelas.nama LIKE ?)", "%10%", "%11%", "%12%")
			}
		}
	}
	if status != "" {
		q = q.Where(fmt.Sprintf("%s.status = ?", table), status)
	}
	q.Scan(&rows)

	// --- Build PDF ---
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	// Title
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, fmt.Sprintf("Laporan Absensi %s", strings.ToUpper(typeStr[:1])+typeStr[1:]), "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s s/d %s", startDate, endDate), "", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Summary cards
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Ringkasan Statistik", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)

	summaryItems := []struct {
		Label string
		Count int
	}{
		{"Hadir", statusMap["hadir"]},
		{"Izin", statusMap["izin"]},
		{"Sakit", statusMap["sakit"]},
		{"Alpa", statusMap["alpa"]},
	}
	colW := 65.0
	for _, item := range summaryItems {
		pdf.CellFormat(colW, 8, fmt.Sprintf("  %s: %d", item.Label, item.Count), "1", 0, "L", false, 0, "")
	}
	pdf.Ln(12)

	// Table header
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(46, 125, 50)
	pdf.SetTextColor(255, 255, 255)
	pdfHeaders := []float64{10, 70, 30, 25, 125} // No, Nama, Tanggal, Status, Catatan
	headerLabels := []string{"No", "Nama Siswa", "Tanggal", "Status", "Catatan"}
	for i, label := range headerLabels {
		pdf.CellFormat(pdfHeaders[i], 8, label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(245, 245, 245)

	for i, r := range rows {
		fill := i%2 == 1
		pdf.CellFormat(pdfHeaders[0], 7, fmt.Sprintf("%d", i+1), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(pdfHeaders[1], 7, truncStr(r.Name, 40), "1", 0, "L", fill, 0, "")
		pdf.CellFormat(pdfHeaders[2], 7, r.Tanggal, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(pdfHeaders[3], 7, r.Status, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(pdfHeaders[4], 7, truncStr(r.Catatan, 70), "1", 0, "L", fill, 0, "")
		pdf.Ln(-1)
	}

	// Footer
	pdf.Ln(5)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 5, fmt.Sprintf("Digenerate pada: %s | Total: %d records", time.Now().Format("02/01/2006 15:04"), len(rows)), "", 1, "R", false, 0, "")

	// Write to response
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=absensi_%s_%s_%s.pdf", typeStr, startDate, endDate))

	if err := pdf.Output(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate PDF"})
	}
	return nil
}

// truncStr truncates a string to maxLen characters.
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
	type TeacherSummaryEntry struct {
		ID         uint
		Name       string
		Avatar     string
		Hadir      int
		Izin       int
		Sakit      int
		Alpha      int
		Substitute int
	}
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
		ID     uint
		Name   string
		Avatar string
		Count  int
	}
	var subCounts []SubCount
	subQ := h.db.Table("substitute_logs").
		Select("substitute_logs.substitute_teacher_id as id, u.name, u.foto_guru as avatar, count(*) as count").
		Joins("JOIN users u ON u.id = substitute_logs.substitute_teacher_id").
		Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
		Where("substitute_logs.deleted_at IS NULL")

	if isDiniyyahAttendanceType(typeStr) {
		subQ = subQ.Where("substitute_logs.jadwal_diniyyah_id IS NOT NULL")
		if kelasID != "" {
			subQ = subQ.Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs.jadwal_diniyyah_id").
				Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
				Where("dkt.kelas_id = ?", kelasID)
		}
	} else {
		subQ = subQ.Where("substitute_logs.jadwal_formal_id IS NOT NULL").Joins("JOIN jadwal_formal jf ON jf.id = substitute_logs.jadwal_formal_id")
		subQ = applyFormalScheduleTypeFilter(subQ, "jf", typeStr)
	}

	if teacherID != "" {
		subQ = subQ.Where("substitute_logs.substitute_teacher_id = ?", teacherID)
	}
	if gender != "" {
		subQ = subQ.Where("u.gender = ?", gender)
	}

	subQ.Group("substitute_logs.substitute_teacher_id, u.name, u.foto_guru").Scan(&subCounts)

	inSummaryMap := make(map[uint]int)
	for i, t := range teacherSummary {
		inSummaryMap[t.ID] = i
	}

	for _, sc := range subCounts {
		if idx, exists := inSummaryMap[sc.ID]; exists {
			teacherSummary[idx].Substitute = sc.Count
		} else {
			teacherSummary = append(teacherSummary, TeacherSummaryEntry{
				ID:         sc.ID,
				Name:       sc.Name,
				Avatar:     sc.Avatar,
				Hadir:      0,
				Izin:       0,
				Sakit:      0,
				Alpha:      0,
				Substitute: sc.Count,
			})
		}
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
		subHistQ := h.db.Table("substitute_logs").
			Select("substitute_logs.date, diniyyah_lessons.nama as lesson, CONCAT(kelas.nama, ' ', kelas.tingkat) as kelas, "+
				"original.name as original_teacher, substitute_logs.status as original_status, "+
				"substitute.name as substitute_teacher, substitute_logs.reason").
			Joins("JOIN jadwal_diniyyahs jd ON jd.id = substitute_logs.jadwal_diniyyah_id").
			Joins("JOIN diniyyah_kelas_teachers dkt ON dkt.id = jd.diniyyah_kelas_teacher_id").
			Joins("JOIN diniyyah_lessons ON diniyyah_lessons.id = dkt.diniyyah_lesson_id").
			Joins("JOIN kelas ON kelas.id = dkt.kelas_id").
			Joins("JOIN users original ON original.id = substitute_logs.original_teacher_id").
			Joins("JOIN users substitute ON substitute.id = substitute_logs.substitute_teacher_id").
			Where("substitute_logs.date >= ? AND substitute_logs.date < ?", startDate, endExclusive).
			Where("substitute_logs.deleted_at IS NULL")
		if teacherID != "" {
			subHistQ = subHistQ.Where("(substitute_logs.original_teacher_id = ? OR substitute_logs.substitute_teacher_id = ?)", teacherID, teacherID)
		}
		if gender != "" {
			subHistQ = subHistQ.Where("(original.gender = ? OR substitute.gender = ?)", gender, gender)
		}
		if kelasID != "" {
			subHistQ = subHistQ.Where("dkt.kelas_id = ?", kelasID)
		}
		subHistQ.Order("substitute_logs.date DESC").Scan(&substituteHistory)
	}

	// Calculate Global Counts
	teacherCountsMap := map[string]int{"Hadir": 0, "Izin": 0, "Sakit": 0, "Alpha": 0, "Substitute": 0}
	for _, t := range teacherSummary {
		teacherCountsMap["Hadir"] += t.Hadir
		teacherCountsMap["Izin"] += t.Izin
		teacherCountsMap["Sakit"] += t.Sakit
		teacherCountsMap["Alpha"] += t.Alpha
	}

	totalSub := 0
	for _, sc := range subCounts {
		if teacherID != "" {
			tid := uint(0)
			fmt.Sscanf(teacherID, "%d", &tid)
			if sc.ID == tid {
				totalSub += sc.Count
			}
		} else {
			totalSub += sc.Count
		}
	}
	teacherCountsMap["Substitute"] = totalSub

	// Build PDF
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Rekapan Kehadiran Guru ("+strings.ToUpper(typeStr)+")", "", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, "Periode: "+startDate+" s/d "+endDate, "", 1, "C", false, 0, "")
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
	filename := fmt.Sprintf("Rekapan_Absensi_Guru_%s_%s_%s.pdf", typeStr, startDate, endDate)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}
