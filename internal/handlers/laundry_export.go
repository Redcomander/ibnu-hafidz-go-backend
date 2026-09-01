package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type LaundryExportHandler struct {
	db *gorm.DB
}

func NewLaundryExportHandler(db *gorm.DB) *LaundryExportHandler {
	return &LaundryExportHandler{db: db}
}

// ExportVendorStatisticsExcel
func (h *LaundryExportHandler) ExportVendorStatisticsExcel(c *fiber.Ctx) error {
	period := c.Query("period", "this_week")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	genderType := c.Query("gender_type", "all")

	var start, end time.Time
	now := time.Now().In(jakartaLocation())

	if startDateStr != "" && endDateStr != "" {
		start, _ = time.ParseInLocation("2006-01-02", startDateStr, jakartaLocation())
		end, _ = time.ParseInLocation("2006-01-02", endDateStr, jakartaLocation())
	} else if period == "last_week" {
		start = getStartOfWeek(now.AddDate(0, 0, -7))
		end = getEndOfWeek(now.AddDate(0, 0, -7))
	} else {
		start = getStartOfWeek(now)
		end = getEndOfWeek(now)
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	endExclusive := end.AddDate(0, 0, 1)

	query := h.db.Model(&models.LaundryVendor{})
	if genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}

	var allVendors []models.LaundryVendor
	query.Find(&allVendors)

	f := excelize.NewFile()
	sheet1 := "Statistik Vendor"
	f.SetSheetName("Sheet1", sheet1)

	headers := []string{"No", "Nama Vendor", "Kategori", "Total Berat (Kg)", "Total Rupiah"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet1, cell, header)
	}

	var grandTotalKg, grandTotalRupiah float64
	for i, v := range allVendors {
		var totalKg, totalRupiah float64
		h.db.Model(&models.LaundryTransaction{}).
			Joins("JOIN laundry_accounts ON laundry_accounts.id = laundry_transactions.laundry_account_id").
			Where("COALESCE(laundry_transactions.vendor_id, laundry_accounts.vendor_id) = ?", v.ID).
			Where("laundry_transactions.tanggal >= ? AND laundry_transactions.tanggal < ?", start, endExclusive).
			Select("COALESCE(SUM(berat_kg), 0) as total_kg, COALESCE(SUM(total_harga), 0) as total_rupiah").
			Row().Scan(&totalKg, &totalRupiah)

		row := i + 2
		f.SetCellValue(sheet1, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet1, fmt.Sprintf("B%d", row), v.Name)
		f.SetCellValue(sheet1, fmt.Sprintf("C%d", row), v.GenderType)
		f.SetCellValue(sheet1, fmt.Sprintf("D%d", row), totalKg)
		f.SetCellValue(sheet1, fmt.Sprintf("E%d", row), totalRupiah)

		grandTotalKg += totalKg
		grandTotalRupiah += totalRupiah
	}

	lastRow := len(allVendors) + 2
	f.SetCellValue(sheet1, fmt.Sprintf("A%d", lastRow), "")
	f.SetCellValue(sheet1, fmt.Sprintf("B%d", lastRow), "Total Keseluruhan")
	f.SetCellValue(sheet1, fmt.Sprintf("C%d", lastRow), "")
	f.SetCellValue(sheet1, fmt.Sprintf("D%d", lastRow), grandTotalKg)
	f.SetCellValue(sheet1, fmt.Sprintf("E%d", lastRow), grandTotalRupiah)

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("Statistik_Vendor_%s.xlsx", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

// ExportVendorStatisticsPDF
func (h *LaundryExportHandler) ExportVendorStatisticsPDF(c *fiber.Ctx) error {
	// For simplicity in this demo, we will output basic PDF similar to old app
	// Actual app had `statistics_pdf` (all vendors) and `slip_gaji_pdf` (single vendor)
	// We'll generate a basic summary list of vendors using FPDF

	period := c.Query("period", "this_week")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	genderType := c.Query("gender_type", "all")
	vendorID := c.Query("vendor_id")

	var start, end time.Time
	now := time.Now().In(jakartaLocation())

	if startDateStr != "" && endDateStr != "" {
		start, _ = time.ParseInLocation("2006-01-02", startDateStr, jakartaLocation())
		end, _ = time.ParseInLocation("2006-01-02", endDateStr, jakartaLocation())
	} else if period == "last_week" {
		start = getStartOfWeek(now.AddDate(0, 0, -7))
		end = getEndOfWeek(now.AddDate(0, 0, -7))
	} else {
		start = getStartOfWeek(now)
		end = getEndOfWeek(now)
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	endExclusive := end.AddDate(0, 0, 1)

	periodStr := fmt.Sprintf("%s - %s", dateIDLong(start), dateIDLong(end))

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetMargins(15, 15, 15)
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Arial", "B", 9)
		pdf.SetTextColor(80, 80, 80)
		pdf.CellFormat(130, 7, "Rekapan Laundry Vendor", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 8)
		pdf.CellFormat(0, 7, "Periode: "+periodStr, "", 1, "R", false, 0, "")
		pdf.SetDrawColor(200, 200, 200)
		pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
		pdf.Ln(2)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetDrawColor(0, 0, 0)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(150, 150, 150)
		pdf.CellFormat(130, 8, fmt.Sprintf("Dicetak: %s", time.Now().Format("02/01/2006 15:04")), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 8, fmt.Sprintf("Halaman %d", pdf.PageNo()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 10, "Rekapan Laundry Vendor", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 7, "Periode: "+periodStr, "", 1, "L", false, 0, "")
	pdf.Ln(4)
	pdf.SetTextColor(0, 0, 0)

	query := h.db.Model(&models.LaundryVendor{})
	if vendorID != "" && vendorID != "all" {
		query = query.Where("id = ?", vendorID)
	}
	if genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}

	var allVendors []models.LaundryVendor
	query.Find(&allVendors)

	var grandTotalKg, grandTotalRupiah float64

	type VendorTotal struct {
		Kg float64
		Rp float64
	}
	vendorTotals := make(map[int64]VendorTotal)

	for _, v := range allVendors {
		var totalKg, totalRupiah float64
		h.db.Model(&models.LaundryTransaction{}).
			Joins("JOIN laundry_accounts ON laundry_accounts.id = laundry_transactions.laundry_account_id").
			Where("COALESCE(laundry_transactions.vendor_id, laundry_accounts.vendor_id) = ?", v.ID).
			Where("laundry_transactions.tanggal >= ? AND laundry_transactions.tanggal < ?", start, endExclusive).
			Select("COALESCE(SUM(berat_kg), 0) as total_kg, COALESCE(SUM(total_harga), 0) as total_rupiah").
			Row().Scan(&totalKg, &totalRupiah)

		vendorTotals[int64(v.ID)] = VendorTotal{Kg: totalKg, Rp: totalRupiah}
		grandTotalKg += totalKg
		grandTotalRupiah += totalRupiah
	}

	// Read merge groups: e.g. merge_groups[]=1,2&merge_groups[]=3,4
	mergeGroupsArgs := c.Request().URI().QueryArgs().PeekMulti("merge_groups[]")

	type MergedGroup struct {
		Names       string
		TotalKg     float64
		TotalRupiah float64
		VendorIDs   []int64
	}

	var mergedGroups []MergedGroup
	var usedVendorIDs = make(map[int64]bool)

	for _, b := range mergeGroupsArgs {
		groupStr := string(b)
		idStrs := strings.Split(groupStr, ",")
		var currentIDs []int64
		for _, s := range idStrs {
			s = strings.TrimSpace(s)
			if s != "" {
				var id int64
				fmt.Sscanf(s, "%d", &id)
				currentIDs = append(currentIDs, id)
			}
		}

		if len(currentIDs) >= 2 {
			var mKg, mRp float64
			var mNames []string
			// find in allVendors
			for _, vid := range currentIDs {
				for _, v := range allVendors {
					if int64(v.ID) == vid {
						mNames = append(mNames, v.Name)
						mKg += vendorTotals[int64(v.ID)].Kg
						mRp += vendorTotals[int64(v.ID)].Rp
						usedVendorIDs[int64(v.ID)] = true
						break
					}
				}
			}
			mergedGroups = append(mergedGroups, MergedGroup{
				Names:       strings.Join(mNames, " + "),
				TotalKg:     mKg,
				TotalRupiah: mRp,
				VendorIDs:   currentIDs,
			})
		}
	}

	// Summary strip: Total Vendor | Total Berat | Total Pendapatan
	pdf.SetDrawColor(220, 220, 220)
	pdf.SetFillColor(245, 247, 251)
	pdf.SetFont("Arial", "B", 7)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(56, 6, "TOTAL VENDOR", "1", 0, "C", true, 0, "")
	pdf.CellFormat(62, 6, "TOTAL BERAT", "1", 0, "C", true, 0, "")
	pdf.CellFormat(62, 6, "TOTAL PENDAPATAN", "1", 1, "C", true, 0, "")
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(14, 165, 233)
	pdf.CellFormat(56, 12, fmt.Sprintf("%d", len(allVendors)), "1", 0, "C", true, 0, "")
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(62, 12, fmt.Sprintf("%s Kg", numberID(grandTotalKg, 2)), "1", 0, "C", true, 0, "")
	pdf.SetTextColor(22, 163, 74)
	pdf.CellFormat(62, 12, "Rp "+rupiahPDF(grandTotalRupiah), "1", 1, "C", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Ln(6)

	// Table header — No, Nama Tukang, Jumlah Kg, Harga/Kg, Jumlah Gajih (matching Laravel)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(8, 8, "No", "1", 0, "C", true, 0, "")
	pdf.CellFormat(78, 8, "Nama Tukang", "1", 0, "L", true, 0, "")
	pdf.CellFormat(28, 8, "Jumlah Kg", "1", 0, "R", true, 0, "")
	pdf.CellFormat(30, 8, "Harga/Kg", "1", 0, "R", true, 0, "")
	pdf.CellFormat(36, 8, "Jumlah Gajih", "1", 1, "R", true, 0, "")

	rowIndex := 1

	for _, mg := range mergedGroups {
		nameToPrint := mg.Names
		if len(nameToPrint) > 44 {
			nameToPrint = nameToPrint[:41] + "..."
		}
		pdf.SetFillColor(254, 243, 199)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(8, 8, fmt.Sprintf("%d", rowIndex), "1", 0, "C", true, 0, "")
		pdf.CellFormat(78, 8, nameToPrint, "1", 0, "L", true, 0, "")
		pdf.CellFormat(28, 8, numberID(mg.TotalKg, 1), "1", 0, "R", true, 0, "")
		pdf.CellFormat(30, 8, "Rp5.000", "1", 0, "R", true, 0, "")
		pdf.CellFormat(36, 8, "Rp"+rupiahPDF(mg.TotalRupiah), "1", 1, "R", true, 0, "")
		rowIndex++
	}

	for _, v := range allVendors {
		if usedVendorIDs[int64(v.ID)] {
			continue
		}
		pdf.SetFillColor(255, 255, 255)
		pdf.SetFont("Arial", "", 9)
		pdf.CellFormat(8, 8, fmt.Sprintf("%d", rowIndex), "1", 0, "C", false, 0, "")
		pdf.CellFormat(78, 8, v.Name, "1", 0, "L", false, 0, "")
		pdf.CellFormat(28, 8, numberID(vendorTotals[int64(v.ID)].Kg, 1), "1", 0, "R", false, 0, "")
		pdf.CellFormat(30, 8, "Rp5.000", "1", 0, "R", false, 0, "")
		pdf.CellFormat(36, 8, "Rp"+rupiahPDF(vendorTotals[int64(v.ID)].Rp), "1", 1, "R", false, 0, "")
		rowIndex++
	}

	pdf.SetFillColor(240, 240, 240)
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(86, 8, "TOTAL KESELURUHAN", "1", 0, "C", true, 0, "")
	pdf.CellFormat(28, 8, numberID(grandTotalKg, 1), "1", 0, "R", true, 0, "")
	pdf.CellFormat(30, 8, "Rp5.000", "1", 0, "R", true, 0, "")
	pdf.CellFormat(36, 8, "Rp"+rupiahPDF(grandTotalRupiah), "1", 1, "R", true, 0, "")

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekapan_Laundry_%s.pdf", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

// ExportExceededAccountsExcel
func (h *LaundryExportHandler) ExportExceededAccountsExcel(c *fiber.Ctx) error {
	search := c.Query("search")
	genderType := c.Query("gender_type")
	ownerStatus := c.Query("owner_status")
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")

	now := time.Now()
	var dateFrom, dateTo time.Time
	if dateFromStr == "" || dateToStr == "" {
		dateFrom = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		dateTo = dateFrom.AddDate(0, 1, -1).Add(time.Hour*23 + time.Minute*59 + time.Second*59)
	} else {
		dateFrom, _ = time.Parse("2006-01-02", dateFromStr)
		dateTo, _ = time.Parse("2006-01-02", dateToStr)
		dateTo = time.Date(dateTo.Year(), dateTo.Month(), dateTo.Day(), 23, 59, 59, 0, dateTo.Location())
	}

	query := h.db.Model(&models.LaundryAccount{}).Preload("Vendor").Preload("Student").Preload("User").Where("active = ?", true)

	if search != "" {
		query = query.Where(
			"nomor_laundry LIKE ? OR student_id IN (SELECT id FROM students WHERE nama_lengkap LIKE ?) OR user_id IN (SELECT id FROM users WHERE name LIKE ?)",
			"%"+search+"%", "%"+search+"%", "%"+search+"%",
		)
	}

	if genderType != "" && genderType != "all" {
		query = query.Where("vendor_id IN (SELECT id FROM laundry_vendors WHERE gender_type = ?)", genderType)
	}

	if ownerStatus == "orphan" || ownerStatus == "unknown" {
		query = query.Where("(student_id IS NULL AND user_id IS NULL) OR (student_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM students WHERE id = laundry_accounts.student_id)) OR (user_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users WHERE id = laundry_accounts.user_id))")
	} else if ownerStatus == "has_owner" {
		query = query.Where("(student_id IS NOT NULL AND EXISTS (SELECT 1 FROM students WHERE id = laundry_accounts.student_id)) OR (user_id IS NOT NULL AND EXISTS (SELECT 1 FROM users WHERE id = laundry_accounts.user_id))")
	}

	var accounts []models.LaundryAccount
	query.Find(&accounts)

	f := excelize.NewFile()
	sheet1 := "Laporan Kelebihan Laundry"
	f.SetSheetName("Sheet1", sheet1)

	headers := []string{"No", "Nomor Laundry", "Nama", "Tipe", "Vendor", "Kategori", "Total Berat (Kg)", "Limit (Kg)", "Kelebihan (Kg)", "Harga/Kg", "Tagihan Hutang"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet1, cell, header)
	}

	rowIndex := 2
	for _, acc := range accounts {
		var periodWeight float64
		h.db.Model(&models.LaundryTransaction{}).
			Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", acc.ID, dateFrom, dateTo).
			Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&periodWeight)

		if periodWeight <= MONTHLY_LIMIT_KG {
			continue
		}

		monthlyExcess := periodWeight - MONTHLY_LIMIT_KG
		debtAmount := monthlyExcess * DEBT_PRICE_PER_KG

		ownerName := "Unknown"
		ownerType := "Unknown"
		if acc.Student != nil {
			ownerName = acc.Student.NamaLengkap
			ownerType = "Siswa"
		} else if acc.User != nil {
			ownerName = acc.User.Name
			ownerType = "User"
		}

		f.SetCellValue(sheet1, fmt.Sprintf("A%d", rowIndex), rowIndex-1)
		f.SetCellValue(sheet1, fmt.Sprintf("B%d", rowIndex), acc.NomorLaundry)
		f.SetCellValue(sheet1, fmt.Sprintf("C%d", rowIndex), ownerName)
		f.SetCellValue(sheet1, fmt.Sprintf("D%d", rowIndex), ownerType)
		f.SetCellValue(sheet1, fmt.Sprintf("E%d", rowIndex), acc.Vendor.Name)
		f.SetCellValue(sheet1, fmt.Sprintf("F%d", rowIndex), acc.Vendor.GenderType)
		f.SetCellValue(sheet1, fmt.Sprintf("G%d", rowIndex), periodWeight)
		f.SetCellValue(sheet1, fmt.Sprintf("H%d", rowIndex), MONTHLY_LIMIT_KG)
		f.SetCellValue(sheet1, fmt.Sprintf("I%d", rowIndex), monthlyExcess)
		f.SetCellValue(sheet1, fmt.Sprintf("J%d", rowIndex), DEBT_PRICE_PER_KG)
		f.SetCellValue(sheet1, fmt.Sprintf("K%d", rowIndex), debtAmount)

		rowIndex++
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("Laporan_Kelebihan_Laundry_%s.xlsx", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

// ExportWeeklyVendorStatisticsPDF generates a weekly PDF recap for a single vendor
func (h *LaundryExportHandler) ExportWeeklyVendorStatisticsPDF(c *fiber.Ctx) error {
	vendorID := c.Query("vendor_id")
	if vendorID == "" {
		return c.Status(400).SendString("Vendor ID is required")
	}

	period := c.Query("period", "this_week")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	var start, end time.Time
	now := time.Now()

	if startDateStr != "" && endDateStr != "" {
		start, _ = time.Parse("2006-01-02", startDateStr)
		end, _ = time.Parse("2006-01-02", endDateStr)
	} else if period == "last_week" {
		start = getStartOfWeek(now.AddDate(0, 0, -7))
		end = getEndOfWeek(now.AddDate(0, 0, -7))
	} else {
		start = getStartOfWeek(now)
		end = getEndOfWeek(now)
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	var vendor models.LaundryVendor
	if err := h.db.Preload("Accounts.Student").Preload("Accounts.User").First(&vendor, vendorID).Error; err != nil {
		return c.Status(404).SendString("Vendor not found")
	}

	// Generate the 7 days
	weekDays := make([]time.Time, 7)
	for i := 0; i < 7; i++ {
		weekDays[i] = start.AddDate(0, 0, i)
	}
	dayNames := []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Ahad"}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 9, "Rekap Laundry Mingguan", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, vendor.Name, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s s/d %s", start.Format("02 Jan 2006"), end.Format("02 Jan 2006")), "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(5)

	pdf.SetFillColor(240, 240, 240)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(40, 8, "Nama Akun", "1", 0, "L", true, 0, "")
	for _, dName := range dayNames {
		pdf.CellFormat(18, 8, dName, "1", 0, "C", true, 0, "")
	}
	pdf.CellFormat(20, 8, "Total", "1", 1, "C", true, 0, "")

	pdf.SetFont("Arial", "", 8)

	grandTotals := make([]float64, 7)
	grandTotalAll := 0.0

	for _, acc := range vendor.Accounts {
		ownerName := "Unknown"
		if acc.Student != nil {
			ownerName = acc.Student.NamaLengkap
		} else if acc.User != nil {
			ownerName = acc.User.Name
		}

		totalWeek := 0.0
		var dailyTotals []float64

		for i, day := range weekDays {
			dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
			dayEnd := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())

			var dailyKg float64
			h.db.Model(&models.LaundryTransaction{}).
				Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", acc.ID, dayStart, dayEnd).
				Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&dailyKg)

			dailyTotals = append(dailyTotals, dailyKg)
			totalWeek += dailyKg
			grandTotals[i] += dailyKg
		}

		if totalWeek > 0 {
			nameToPrint := ownerName
			if len(nameToPrint) > 20 {
				nameToPrint = nameToPrint[:18] + ".."
			}
			pdf.CellFormat(40, 8, nameToPrint, "1", 0, "L", false, 0, "")
			for _, dKg := range dailyTotals {
				valStr := "-"
				if dKg > 0 {
					valStr = fmt.Sprintf("%.2f", dKg)
				}
				pdf.CellFormat(18, 8, valStr, "1", 0, "C", false, 0, "")
			}
			pdf.CellFormat(20, 8, fmt.Sprintf("%.2f", totalWeek), "1", 1, "C", false, 0, "")
			grandTotalAll += totalWeek
		}
	}

	pdf.SetFillColor(255, 232, 60)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(40, 8, "GRAND TOTAL", "1", 0, "C", true, 0, "")
	for _, gt := range grandTotals {
		valStr := "-"
		if gt > 0 {
			valStr = fmt.Sprintf("%.2f", gt)
		}
		pdf.CellFormat(18, 8, valStr, "1", 0, "C", true, 0, "")
	}
	pdf.CellFormat(20, 8, fmt.Sprintf("%.2f", grandTotalAll), "1", 1, "C", true, 0, "")

	pdf.SetFillColor(255, 255, 255)
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(100, 100, 100)
	pdf.Ln(4)
	pdf.CellFormat(0, 6, fmt.Sprintf("Dicetak pada: %s", time.Now().Format("02/01/2006 15:04")), "", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekap_Mingguan_%s_%s.pdf", vendor.Name, time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

// ExportAllWeeklyVendorStatisticsPDF generates a landscape PDF for all vendors weekly stats
func (h *LaundryExportHandler) ExportAllWeeklyVendorStatisticsPDF(c *fiber.Ctx) error {
	period := c.Query("period", "this_week")
	genderType := c.Query("gender_type")

	var start, end time.Time
	now := time.Now()

	if period == "last_week" {
		start = getStartOfWeek(now.AddDate(0, 0, -7))
		end = getEndOfWeek(now.AddDate(0, 0, -7))
	} else {
		start = getStartOfWeek(now)
		end = getEndOfWeek(now)
	}

	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location())

	query := h.db.Model(&models.LaundryVendor{}).Where("active = ?", true)
	if genderType != "" && genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}

	var vendors []models.LaundryVendor
	query.Preload("Accounts.Student").Preload("Accounts.User").Order("name ASC").Find(&vendors)

	weekDays := make([]time.Time, 7)
	for i := 0; i < 7; i++ {
		weekDays[i] = start.AddDate(0, 0, i)
	}
	dayNames := []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Ahad"}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)

	for _, vendor := range vendors {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)
		pdf.CellFormat(0, 9, "Rekap Laundry Mingguan", "", 1, "C", false, 0, "")
		pdf.SetFont("Arial", "B", 13)
		pdf.CellFormat(0, 8, vendor.Name, "", 1, "C", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s s/d %s", start.Format("02 Jan 2006"), end.Format("02 Jan 2006")), "", 1, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.Ln(4)

		pdf.SetFillColor(240, 240, 240)
		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(60, 8, "Nama Akun", "1", 0, "L", true, 0, "")
		for _, dName := range dayNames {
			pdf.CellFormat(25, 8, dName, "1", 0, "C", true, 0, "")
		}
		pdf.CellFormat(25, 8, "Total", "1", 1, "C", true, 0, "")

		pdf.SetFont("Arial", "", 8)

		grandTotals := make([]float64, 7)
		grandTotalAll := 0.0

		for _, acc := range vendor.Accounts {
			ownerName := "Unknown"
			if acc.Student != nil {
				ownerName = acc.Student.NamaLengkap
			} else if acc.User != nil {
				ownerName = acc.User.Name
			}

			totalWeek := 0.0
			var dailyTotals []float64

			for i, day := range weekDays {
				dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
				dayEnd := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())

				var dailyKg float64
				h.db.Model(&models.LaundryTransaction{}).
					Where("laundry_account_id = ? AND tanggal BETWEEN ? AND ?", acc.ID, dayStart, dayEnd).
					Select("COALESCE(SUM(berat_kg), 0)").Row().Scan(&dailyKg)

				dailyTotals = append(dailyTotals, dailyKg)
				totalWeek += dailyKg
				grandTotals[i] += dailyKg
			}

			nameToPrint := ownerName
			if len(nameToPrint) > 35 {
				nameToPrint = nameToPrint[:33] + ".."
			}
			pdf.CellFormat(60, 8, nameToPrint, "1", 0, "L", false, 0, "")
			for _, dKg := range dailyTotals {
				valStr := "-"
				if dKg > 0 {
					valStr = fmt.Sprintf("%.2f", dKg)
				}
				pdf.CellFormat(25, 8, valStr, "1", 0, "C", false, 0, "")
			}
			pdf.CellFormat(25, 8, fmt.Sprintf("%.2f", totalWeek), "1", 1, "C", false, 0, "")
			grandTotalAll += totalWeek
		}

		pdf.SetFillColor(255, 232, 60)
		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(60, 8, "GRAND TOTAL", "1", 0, "C", true, 0, "")
		for _, gt := range grandTotals {
			valStr := "-"
			if gt > 0 {
				valStr = fmt.Sprintf("%.2f", gt)
			}
			pdf.CellFormat(25, 8, valStr, "1", 0, "C", true, 0, "")
		}
		pdf.CellFormat(25, 8, fmt.Sprintf("%.2f", grandTotalAll), "1", 1, "C", true, 0, "")
		pdf.SetFillColor(255, 255, 255)
	}

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Rekap_Mingguan_Semua_Vendor_%s.pdf", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

// ExportAllAccountsPDF generates a PDF list of all active laundry accounts grouped by vendor
func (h *LaundryExportHandler) ExportAllAccountsPDF(c *fiber.Ctx) error {
	genderType := c.Query("gender_type")
	vendorID := c.Query("vendor_id")

	query := h.db.Model(&models.LaundryVendor{}).Where("active = ?", true)
	if genderType != "" && genderType != "all" {
		query = query.Where("gender_type = ?", genderType)
	}
	if vendorID != "" && vendorID != "all" {
		query = query.Where("id = ?", vendorID)
	}

	var vendors []models.LaundryVendor
	// We want active accounts. We can load them via Preload with a condition.
	query.Preload("Accounts", "active = ?", true).
		Preload("Accounts.Student").
		Preload("Accounts.User").
		Order("name ASC").Find(&vendors)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.SetMargins(15, 15, 15)

	titleSuffix := ""
	if vendorID != "" && vendorID != "all" {
		titleSuffix = " - Vendor Tertentu"
	}
	if genderType == "banin" {
		titleSuffix = " - Banin"
	} else if genderType == "banat" {
		titleSuffix = " - Banat"
	}

	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, fmt.Sprintf("Daftar Akun Laundry Semua Vendor%s", titleSuffix), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 7, fmt.Sprintf("Dicetak pada: %s", time.Now().Format("02 Jan 2006 15:04")), "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(5)

	grandTotalAccounts := 0

	for _, vendor := range vendors {
		if len(vendor.Accounts) == 0 {
			continue
		}

		// Black header bar for vendor name — matching Laravel all_accounts_pdf design
		pdf.SetFillColor(0, 0, 0)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(0, 9, "  "+vendor.Name, "", 1, "L", true, 0, "")
		pdf.SetTextColor(0, 0, 0)

		pdf.SetFillColor(240, 240, 240)
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(10, 8, "No", "1", 0, "C", true, 0, "")
		pdf.CellFormat(30, 8, "No. Laundry", "1", 0, "C", true, 0, "")
		pdf.CellFormat(110, 8, "Nama Lengkap", "1", 0, "L", true, 0, "")
		pdf.CellFormat(30, 8, "Tipe", "1", 1, "C", true, 0, "")

		pdf.SetFont("Arial", "", 9)
		for i, acc := range vendor.Accounts {
			ownerName := "Unknown"
			kategori := "Unknown"
			if acc.Student != nil {
				ownerName = acc.Student.NamaLengkap
				kategori = "Santri"
			} else if acc.User != nil {
				ownerName = acc.User.Name
				kategori = "Guru/Staf"
			}

			pdf.CellFormat(10, 8, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
			pdf.CellFormat(30, 8, acc.NomorLaundry, "1", 0, "C", false, 0, "")
			pdf.CellFormat(110, 8, ownerName, "1", 0, "L", false, 0, "")
			pdf.CellFormat(30, 8, kategori, "1", 1, "C", false, 0, "")

			grandTotalAccounts++
		}

		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(0, 7, fmt.Sprintf("Total akun: %d", len(vendor.Accounts)), "", 1, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.Ln(4)
	}

	pdf.Ln(3)
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(0, 9, fmt.Sprintf("Grand Total Akun: %d", grandTotalAccounts), "", 1, "R", false, 0, "")

	c.Set("Content-Type", "application/pdf")
	filename := fmt.Sprintf("Daftar_Akun_Laundry_%s.pdf", time.Now().Format("20060102"))
	c.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	return pdf.Output(c.Response().BodyWriter())
}

// ExportVendorAccountsPDF generates account list PDF for a single vendor.
func (h *LaundryExportHandler) ExportVendorAccountsPDF(c *fiber.Ctx) error {
	vendorID := c.Params("id")
	if vendorID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Message: "Vendor ID wajib diisi"})
	}

	// Reuse the all-accounts exporter with vendor filter to keep format consistent.
	c.Request().URI().QueryArgs().Set("vendor_id", vendorID)
	return h.ExportAllAccountsPDF(c)
}

// rupiahPDF formats a float as Indonesian rupiah without the "Rp" prefix, e.g. 1.234.567
func rupiahPDF(amount float64) string {
	s := fmt.Sprintf("%.0f", amount)
	n := len(s)
	result := ""
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result += "."
		}
		result += string(c)
	}
	return result
}

func numberID(value float64, decimals int) string {
	raw := fmt.Sprintf("%.*f", decimals, value)
	parts := strings.SplitN(raw, ".", 2)
	intPart := parts[0]
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	n := len(intPart)
	formattedInt := ""
	for i, c := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			formattedInt += "."
		}
		formattedInt += string(c)
	}

	if decimals <= 0 {
		return formattedInt
	}

	return formattedInt + "," + fracPart
}

func dateIDLong(t time.Time) string {
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	monthName := months[int(t.Month())-1]
	return fmt.Sprintf("%d %s %d", t.Day(), monthName, t.Year())
}
