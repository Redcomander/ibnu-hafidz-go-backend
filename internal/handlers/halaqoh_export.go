package handlers

import (
	"fmt"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/xuri/excelize/v2"
)

// Helper structs for export
type teacherExportSummary struct {
	ID         uint
	Name       string
	Sesi       string
	Hadir      int
	Izin       int
	Sakit      int
	Alpha      int
	Substitute int
}

type substituteExportHistory struct {
	ID         uint
	Date       time.Time
	Sesi       string
	Original   string
	Status     string
	Substitute string
}

func (h *HalaqohStatsHandler) fetchTeacherData(startDate, endDate, teacherIDStr, gender string) ([]teacherExportSummary, []substituteExportHistory, error) {
	attQuery := h.db.Model(&models.HalaqohTeacherAttendance{}).Preload("Teacher")
	if startDate != "" && endDate != "" {
		attQuery = attQuery.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherIDStr != "" {
		attQuery = attQuery.Where("user_id = ?", teacherIDStr)
	}
	if gender != "" {
		attQuery = attQuery.Joins("JOIN users ON users.id = halaqoh_teacher_attendances.user_id").
			Where("users.gender = ?", gender)
	}

	var attendances []models.HalaqohTeacherAttendance
	if err := attQuery.Find(&attendances).Error; err != nil {
		return nil, nil, err
	}

	subQuery := h.db.Model(&models.HalaqohSubstituteLog{}).
		Preload("OriginalTeacher").
		Preload("SubstituteTeacher")
	if startDate != "" && endDate != "" {
		subQuery = subQuery.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherIDStr != "" {
		subQuery = subQuery.Where("original_teacher_id = ? OR substitute_teacher_id = ?", teacherIDStr, teacherIDStr)
	}
	if gender != "" {
		subQuery = subQuery.Joins("LEFT JOIN users as orig ON orig.id = halaqoh_substitute_logs.original_teacher_id").
			Joins("LEFT JOIN users as sub ON sub.id = halaqoh_substitute_logs.substitute_teacher_id").
			Where("orig.gender = ? OR sub.gender = ?", gender, gender)
	}

	var subLogs []models.HalaqohSubstituteLog
	if err := subQuery.Find(&subLogs).Error; err != nil {
		return nil, nil, err
	}

	teacherMap := make(map[string]*teacherExportSummary)
	getTeacherSession := func(id uint, name string, session string) *teacherExportSummary {
		key := fmt.Sprintf("%d-%s", id, session)
		if _, exists := teacherMap[key]; !exists {
			teacherMap[key] = &teacherExportSummary{ID: id, Name: name, Sesi: session}
		}
		return teacherMap[key]
	}

	for _, a := range attendances {
		if a.Teacher.ID != 0 {
			ts := getTeacherSession(a.Teacher.ID, a.Teacher.Name, a.Session)
			switch capitalize(a.Status) {
			case "Hadir":
				ts.Hadir++
			case "Izin":
				ts.Izin++
			case "Sakit":
				ts.Sakit++
			case "Alpha":
				ts.Alpha++
			}
		}
	}

	var history []substituteExportHistory
	for _, log := range subLogs {
		if log.OriginalTeacher.ID != 0 {
			sess := "-"
			if log.Session != nil {
				sess = *log.Session
			}
			ts := getTeacherSession(log.OriginalTeacher.ID, log.OriginalTeacher.Name, sess)
			st := ""
			if log.Status != nil {
				st = capitalize(*log.Status)
			}
			switch st {
			case "Hadir":
				ts.Hadir++
			case "Izin":
				ts.Izin++
			case "Sakit":
				ts.Sakit++
			case "Alpha":
				ts.Alpha++
			}
		}

		penggantiName := "-"
		sess := "-"
		if log.Session != nil {
			sess = *log.Session
		}

		if log.SubstituteTeacher.ID != 0 {
			ts := getTeacherSession(log.SubstituteTeacher.ID, log.SubstituteTeacher.Name, sess)
			ts.Substitute++
			penggantiName = log.SubstituteTeacher.Name
		}

		statusLabel := "-"
		if log.Status != nil {
			statusLabel = *log.Status
		}

		history = append(history, substituteExportHistory{
			ID:         log.ID,
			Date:       log.Date,
			Sesi:       sess,
			Original:   log.OriginalTeacher.Name,
			Status:     statusLabel,
			Substitute: penggantiName,
		})
	}

	var summaries []teacherExportSummary
	for _, ts := range teacherMap {
		summaries = append(summaries, *ts)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Name == summaries[j].Name {
			return summaries[i].Sesi < summaries[j].Sesi
		}
		return summaries[i].Name < summaries[j].Name
	})

	return summaries, history, nil
}

func (h *HalaqohStatsHandler) ExportHalaqohTeacherPDF(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	summaries, history, err := h.fetchTeacherData(startDate, endDate, teacherID, gender)
	if err != nil {
		return c.Status(500).SendString("Database error")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Rekapan Kehadiran Halaqoh Guru", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, "Periode: "+startDate+" s/d "+endDate, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 10, "Rekapan Per Guru", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(10, 8, "No", "1", 0, "C", false, 0, "")
	pdf.CellFormat(70, 8, "Nama Guru", "1", 0, "L", false, 0, "")
	pdf.CellFormat(30, 8, "Sesi", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Hadir", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Izin", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Sakit", "1", 0, "C", false, 0, "")
	pdf.CellFormat(15, 8, "Alpha", "1", 0, "C", false, 0, "")
	pdf.CellFormat(20, 8, "Substitute", "1", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetFont("Arial", "", 10)

	type Group struct {
		Name    string
		Entries []teacherExportSummary
	}
	var groups []*Group
	for _, s := range summaries {
		if len(groups) == 0 || groups[len(groups)-1].Name != s.Name {
			groups = append(groups, &Group{Name: s.Name})
		}
		groups[len(groups)-1].Entries = append(groups[len(groups)-1].Entries, s)
	}

	for i, g := range groups {
		h := 8.0 * float64(len(g.Entries))

		_, pageH := pdf.GetPageSize()
		_, _, _, bMargin := pdf.GetMargins()
		if pdf.GetY()+h > pageH-bMargin {
			pdf.AddPage()
		}

		xStart := pdf.GetX()
		yStart := pdf.GetY()

		pdf.CellFormat(10, h, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		name := g.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		pdf.CellFormat(70, h, name, "1", 0, "L", false, 0, "")

		for j, s := range g.Entries {
			if j > 0 {
				pdf.SetXY(xStart+80, yStart+float64(j)*8.0)
			}
			pdf.CellFormat(30, 8, s.Sesi, "1", 0, "C", false, 0, "")
			pdf.CellFormat(15, 8, fmt.Sprintf("%d", s.Hadir), "1", 0, "C", false, 0, "")
			pdf.CellFormat(15, 8, fmt.Sprintf("%d", s.Izin), "1", 0, "C", false, 0, "")
			pdf.CellFormat(15, 8, fmt.Sprintf("%d", s.Sakit), "1", 0, "C", false, 0, "")
			pdf.CellFormat(15, 8, fmt.Sprintf("%d", s.Alpha), "1", 0, "C", false, 0, "")
			pdf.CellFormat(20, 8, fmt.Sprintf("%d", s.Substitute), "1", 1, "C", false, 0, "")
		}
	}

	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 10, "Log Guru Pengganti (Substitute)", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(30, 8, "Tanggal", "1", 0, "C", false, 0, "")
	pdf.CellFormat(30, 8, "Sesi", "1", 0, "C", false, 0, "")
	pdf.CellFormat(50, 8, "Guru Asli", "1", 0, "L", false, 0, "")
	pdf.CellFormat(30, 8, "Status (Asli)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(50, 8, "Pengganti", "1", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	for _, log := range history {
		pdf.CellFormat(30, 8, log.Date.Format("02/01/2006"), "1", 0, "C", false, 0, "")
		pdf.CellFormat(30, 8, log.Sesi, "1", 0, "C", false, 0, "")

		orig := log.Original
		if len(orig) > 25 {
			orig = orig[:22] + "..."
		}
		pdf.CellFormat(50, 8, orig, "1", 0, "L", false, 0, "")

		pdf.CellFormat(30, 8, log.Status, "1", 0, "C", false, 0, "")

		sub := log.Substitute
		if len(sub) > 25 {
			sub = sub[:22] + "..."
		}
		pdf.CellFormat(50, 8, sub, "1", 1, "L", false, 0, "")
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekapan_Halaqoh_Guru_%s_%s.pdf", startDate, endDate)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

func (h *HalaqohStatsHandler) ExportHalaqohTeacherExcel(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	summaries, history, err := h.fetchTeacherData(startDate, endDate, teacherID, gender)
	if err != nil {
		return c.Status(500).SendString("Database error")
	}

	f := excelize.NewFile()
	sheet1 := "Ringkasan Guru"
	f.SetSheetName("Sheet1", sheet1)

	headers1 := []string{"No", "Nama Guru", "Sesi", "Hadir", "Izin", "Sakit", "Alpha", "Substitute"}
	for i, header := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet1, cell, header)
	}
	styleCenter, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center"},
	})
	styleLeft, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "left"},
	})

	type Group struct {
		Name    string
		Entries []teacherExportSummary
	}
	var groups []*Group
	for _, s := range summaries {
		if len(groups) == 0 || groups[len(groups)-1].Name != s.Name {
			groups = append(groups, &Group{Name: s.Name})
		}
		groups[len(groups)-1].Entries = append(groups[len(groups)-1].Entries, s)
	}

	row := 2
	for i, g := range groups {
		startRow := row
		for _, s := range g.Entries {
			f.SetCellValue(sheet1, fmt.Sprintf("C%d", row), s.Sesi)
			f.SetCellValue(sheet1, fmt.Sprintf("D%d", row), s.Hadir)
			f.SetCellValue(sheet1, fmt.Sprintf("E%d", row), s.Izin)
			f.SetCellValue(sheet1, fmt.Sprintf("F%d", row), s.Sakit)
			f.SetCellValue(sheet1, fmt.Sprintf("G%d", row), s.Alpha)
			f.SetCellValue(sheet1, fmt.Sprintf("H%d", row), s.Substitute)
			row++
		}
		endRow := row - 1

		f.SetCellValue(sheet1, fmt.Sprintf("A%d", startRow), i+1)
		f.SetCellValue(sheet1, fmt.Sprintf("B%d", startRow), g.Name)

		if startRow < endRow {
			f.MergeCell(sheet1, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", endRow))
			f.MergeCell(sheet1, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", endRow))
		}
		f.SetCellStyle(sheet1, fmt.Sprintf("A%d", startRow), fmt.Sprintf("A%d", endRow), styleCenter)
		f.SetCellStyle(sheet1, fmt.Sprintf("B%d", startRow), fmt.Sprintf("B%d", endRow), styleLeft)
	}

	sheet2 := "Riwayat Substitute"
	f.NewSheet(sheet2)
	headers2 := []string{"Tanggal", "Sesi", "Guru Asli", "Status", "Pengganti"}
	for i, header := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet2, cell, header)
	}
	for i, sh := range history {
		row := i + 2
		f.SetCellValue(sheet2, fmt.Sprintf("A%d", row), sh.Date.Format("2006-01-02"))
		f.SetCellValue(sheet2, fmt.Sprintf("B%d", row), sh.Sesi)
		f.SetCellValue(sheet2, fmt.Sprintf("C%d", row), sh.Original)
		f.SetCellValue(sheet2, fmt.Sprintf("D%d", row), sh.Status)
		f.SetCellValue(sheet2, fmt.Sprintf("E%d", row), sh.Substitute)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("Rekapan_Halaqoh_Guru_%s_%s.xlsx", startDate, endDate)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

func (h *HalaqohStatsHandler) fetchStudentData(startDate, endDate, teacherID, gender string) ([]models.HalaqohAttendance, error) {
	query := h.db.Model(&models.HalaqohAttendance{}).
		Preload("HalaqohAssignment").
		Preload("HalaqohAssignment.Student").
		Preload("HalaqohAssignment.Teacher")

	if startDate != "" && endDate != "" {
		query = query.Where("date BETWEEN ? AND ?", startDate, endDate)
	}
	if teacherID != "" {
		query = query.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.id = halaqoh_attendances.halaqoh_assignment_id").
			Where("halaqoh_assignments.user_id = ?", teacherID)
	}
	if gender != "" {
		if teacherID == "" {
			query = query.Joins("JOIN halaqoh_assignments ON halaqoh_assignments.id = halaqoh_attendances.halaqoh_assignment_id")
		}
		query = query.Joins("JOIN students ON students.id = halaqoh_assignments.student_id").
			Where("students.jenis_kelamin = ?", gender)
	}

	var records []models.HalaqohAttendance
	err := query.Order("date DESC").Find(&records).Error
	return records, err
}

func (h *HalaqohStatsHandler) ExportHalaqohStudentExcel(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	records, err := h.fetchStudentData(startDate, endDate, teacherID, gender)
	if err != nil {
		return c.Status(500).SendString("Database error")
	}

	f := excelize.NewFile()
	sheet := "Record Kehadiran"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No", "Tanggal", "Sesi", "Nama Santri", "Guru Halaqoh", "Status", "Keterangan"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	for i, r := range records {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), r.Date.Format("2006-01-02"))
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), r.Session)

		name := "-"
		if r.HalaqohAssignment.Student.ID != 0 {
			name = r.HalaqohAssignment.Student.NamaLengkap
		}
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), name)

		teacher := "-"
		if r.HalaqohAssignment.Teacher.ID != 0 {
			teacher = r.HalaqohAssignment.Teacher.Name
		}
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), teacher)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), capitalize(r.Status))

		catatan := ""
		if r.Notes != nil {
			catatan = *r.Notes
		}
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), catatan)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("Rekapan_Halaqoh_Santri_%s_%s.xlsx", startDate, endDate)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

func (h *HalaqohStatsHandler) ExportHalaqohStudentPDF(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	teacherID := c.Query("teacher_id")
	gender := c.Query("gender")

	if startDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	records, err := h.fetchStudentData(startDate, endDate, teacherID, gender)
	if err != nil {
		return c.Status(500).SendString("Database error")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Rekapan Kehadiran Halaqoh Santri", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 8, "Periode: "+startDate+" s/d "+endDate, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(10, 8, "No", "1", 0, "C", false, 0, "")
	pdf.CellFormat(25, 8, "Tanggal", "1", 0, "C", false, 0, "")
	pdf.CellFormat(20, 8, "Sesi", "1", 0, "C", false, 0, "")
	pdf.CellFormat(55, 8, "Nama Santri", "1", 0, "L", false, 0, "")
	pdf.CellFormat(45, 8, "Guru Halaqoh", "1", 0, "L", false, 0, "")
	pdf.CellFormat(20, 8, "Status", "1", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	for i, r := range records {
		pdf.CellFormat(10, 8, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		pdf.CellFormat(25, 8, r.Date.Format("02/01/2006"), "1", 0, "C", false, 0, "")
		pdf.CellFormat(20, 8, r.Session, "1", 0, "C", false, 0, "")

		name := "-"
		if r.HalaqohAssignment.Student.ID != 0 {
			name = r.HalaqohAssignment.Student.NamaLengkap
		}
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		pdf.CellFormat(55, 8, name, "1", 0, "L", false, 0, "")

		teacher := "-"
		if r.HalaqohAssignment.Teacher.ID != 0 {
			teacher = r.HalaqohAssignment.Teacher.Name
		}
		if len(teacher) > 23 {
			teacher = teacher[:20] + "..."
		}
		pdf.CellFormat(45, 8, teacher, "1", 0, "L", false, 0, "")

		pdf.CellFormat(20, 8, capitalize(r.Status), "1", 1, "C", false, 0, "")
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekapan_Halaqoh_Santri_%s_%s.pdf", startDate, endDate)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}
