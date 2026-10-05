package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"github.com/go-pdf/fpdf"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/nfnt/resize"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const maxRevitalisasiImageBytes = 2 * 1024 * 1024

// RevitalisasiHandler manages all project modules for the SMA revitalization program.
type RevitalisasiHandler struct {
	db         *gorm.DB
	uploadPath string
}

func NewRevitalisasiHandler(db *gorm.DB, uploadPath string) *RevitalisasiHandler {
	return &RevitalisasiHandler{db: db, uploadPath: uploadPath}
}

func normalizeStatus(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "hadir", "masuk", "datang":
		return "hadir"
	case "izin", "surat izin", "cuti":
		return "izin"
	case "sakit", "sakit hati":
		return "sakit"
	case "alpha", "alpa", "tidak hadir", "belum hadir", "tak hadir":
		return "alpha"
	default:
		return "alpha"
	}
}

func resolveRevitalisasiJenis(path string, queryValue string) string {
	query := strings.ToLower(strings.TrimSpace(queryValue))
	switch query {
	case "smp", "sma":
		return query
	}
	lowerPath := strings.ToLower(strings.TrimSpace(path))
	if strings.Contains(lowerPath, "/revitalisasi-smp") {
		return "smp"
	}
	return "sma"
}

func isAllowedImageExtension(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func safeDateString(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	return time.Parse("2006-01-02", value)
}

type revitalisasiPayrollReportRow struct {
	ID                 uint    `json:"id"`
	Name               string  `json:"name"`
	Divisi             string  `json:"divisi"`
	Area               string  `json:"area"`
	HariHadir          int64   `json:"hari_hadir"`
	GajiHarian         float64 `json:"gaji_harian"`
	Kasbon             float64 `json:"kasbon"`
	CaraPotong         string  `json:"cara_potong"`
	PotonganSaatIni    float64 `json:"potongan_saat_ini"`
	TotalGaji          float64 `json:"total_gaji"`
	TotalSetelahKasbon float64 `json:"total_setelah_kasbon"`
}

func defaultRevitalisasiReportDates() (string, string) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := now
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

func normalizeKasbonJenis(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pelunasan", "bayar", "pembayaran", "payment", "pengurangan":
		return "pelunasan"
	case "penambahan", "tambah", "tambahan", "pinjaman", "kasbon":
		return "penambahan"
	default:
		return "penambahan"
	}
}

func normalizeKasbonMetode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "angsuran", "cicilan", "installment":
		return "angsuran"
	case "langsung", "cash", "tunai":
		return "langsung"
	default:
		return "langsung"
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (h *RevitalisasiHandler) syncTukangKasbonBalance(tukangID uint) error {
	var total float64
	if err := h.db.Model(&models.RevitalisasiKasbon{}).
		Where("tukang_id = ?", tukangID).
		Select("COALESCE(SUM(CASE WHEN jenis = 'pelunasan' THEN -jumlah ELSE jumlah END), 0)").
		Scan(&total).Error; err != nil {
		return err
	}

	return h.db.Model(&models.RevitalisasiTukang{}).Where("id = ?", tukangID).Update("kasbon", total).Error
}

func (h *RevitalisasiHandler) generatePayrollReportRows(jenis, startDate, endDate string) ([]revitalisasiPayrollReportRow, error) {
	if strings.TrimSpace(jenis) == "" {
		jenis = "sma"
	}
	if strings.TrimSpace(startDate) == "" || strings.TrimSpace(endDate) == "" {
		startDate, endDate = defaultRevitalisasiReportDates()
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid date_from: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid date_to: %w", err)
	}
	if end.Before(start) {
		start, end = end, start
	}

	var tukang []models.RevitalisasiTukang
	if err := h.db.Where("jenis = ?", jenis).Order("name asc").Find(&tukang).Error; err != nil {
		return nil, err
	}

	rows := make([]revitalisasiPayrollReportRow, 0, len(tukang))
	for _, item := range tukang {
		var hadirCount int64
		if err := h.db.Model(&models.RevitalisasiAbsenTukang{}).
			Where("jenis = ? AND tukang_id = ? AND tanggal >= ? AND tanggal <= ? AND status = ?", jenis, item.ID, start, end, normalizeStatus("hadir")).
			Count(&hadirCount).Error; err != nil {
			return nil, err
		}

		totalGaji := float64(hadirCount) * item.GajiHarian
		potonganSaatIni := item.Kasbon
		if strings.EqualFold(strings.TrimSpace(item.CaraPotong), "angsuran") {
			if hadirCount > 0 {
				potonganSaatIni = item.Kasbon / float64(hadirCount)
				if potonganSaatIni > item.GajiHarian {
					potonganSaatIni = item.GajiHarian
				}
			} else {
				potonganSaatIni = 0
			}
		}
		if potonganSaatIni < 0 {
			potonganSaatIni = 0
		}
		if item.Kasbon < 0 {
			potonganSaatIni = 0
		}
		totalSetelahKasbon := totalGaji - minFloat(potonganSaatIni, totalGaji)
		if totalSetelahKasbon < 0 {
			totalSetelahKasbon = 0
		}
		rows = append(rows, revitalisasiPayrollReportRow{
			ID:                 item.ID,
			Name:               item.Name,
			Divisi:             item.Divisi,
			Area:               item.Area,
			HariHadir:          hadirCount,
			GajiHarian:         item.GajiHarian,
			Kasbon:             item.Kasbon,
			CaraPotong:         strings.TrimSpace(item.CaraPotong),
			PotonganSaatIni:    potonganSaatIni,
			TotalGaji:          totalGaji,
			TotalSetelahKasbon: totalSetelahKasbon,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Name < rows[j].Name
	})
	return rows, nil
}

func (h *RevitalisasiHandler) GetPayrollReport(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	startDate := strings.TrimSpace(c.Query("date_from"))
	endDate := strings.TrimSpace(c.Query("date_to"))
	rows, err := h.generatePayrollReportRows(jenis, startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: err.Error()})
	}

	totalGaji := 0.0
	totalKasbon := 0.0
	totalSetelahKasbon := 0.0
	for _, row := range rows {
		totalGaji += row.TotalGaji
		totalKasbon += row.Kasbon
		totalSetelahKasbon += row.TotalSetelahKasbon
	}

	return c.JSON(fiber.Map{
		"data": rows,
		"summary": fiber.Map{
			"jenis":                jenis,
			"date_from":            startDate,
			"date_to":              endDate,
			"total_gaji":           totalGaji,
			"total_kasbon":         totalKasbon,
			"total_setelah_kasbon": totalSetelahKasbon,
		},
	})
}

func (h *RevitalisasiHandler) ExportPayrollReportExcel(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	startDate := strings.TrimSpace(c.Query("date_from"))
	endDate := strings.TrimSpace(c.Query("date_to"))
	rows, err := h.generatePayrollReportRows(jenis, startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: err.Error()})
	}

	file := excelize.NewFile()
	sheet := "Payroll"
	file.SetSheetName("Sheet1", sheet)

	styleHeader, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F766E"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    []excelize.Border{{Type: "left", Color: "D1D5DB", Style: 1}, {Type: "right", Color: "D1D5DB", Style: 1}, {Type: "top", Color: "D1D5DB", Style: 1}, {Type: "bottom", Color: "D1D5DB", Style: 1}},
	})

	periodText := fmt.Sprintf("Periode: %s s/d %s", startDate, endDate)
	file.SetCellValue(sheet, "A1", fmt.Sprintf("Laporan Gaji Revitalisasi %s", strings.ToUpper(jenis)))
	file.SetCellValue(sheet, "A2", periodText)
	file.MergeCell(sheet, "A1", "K1")
	file.MergeCell(sheet, "A2", "K2")

	headers := []string{"No", "Nama Tukang", "Divisi", "Area", "Hari Hadir", "Gaji Harian", "Total Gaji", "Kasbon", "Cara Potong", "Total Setelah Kasbon"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(1+i, 4)
		file.SetCellValue(sheet, cell, header)
		file.SetCellStyle(sheet, cell, cell, styleHeader)
	}

	for _, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		file.SetColWidth(sheet, col, col, 18)
	}

	totalGaji := 0.0
	totalKasbon := 0.0
	totalSetelahKasbon := 0.0
	for idx, row := range rows {
		line := idx + 1
		file.SetCellValue(sheet, fmt.Sprintf("A%d", 4+line), line)
		file.SetCellValue(sheet, fmt.Sprintf("B%d", 4+line), row.Name)
		file.SetCellValue(sheet, fmt.Sprintf("C%d", 4+line), row.Divisi)
		file.SetCellValue(sheet, fmt.Sprintf("D%d", 4+line), row.Area)
		file.SetCellValue(sheet, fmt.Sprintf("E%d", 4+line), row.HariHadir)
		file.SetCellValue(sheet, fmt.Sprintf("F%d", 4+line), row.GajiHarian)
		file.SetCellValue(sheet, fmt.Sprintf("G%d", 4+line), row.TotalGaji)
		file.SetCellValue(sheet, fmt.Sprintf("H%d", 4+line), row.Kasbon)
		file.SetCellValue(sheet, fmt.Sprintf("I%d", 4+line), row.CaraPotong)
		file.SetCellValue(sheet, fmt.Sprintf("J%d", 4+line), row.TotalSetelahKasbon)
		totalGaji += row.TotalGaji
		totalKasbon += row.Kasbon
		totalSetelahKasbon += row.TotalSetelahKasbon
	}

	footerRow := 5 + len(rows)
	file.SetCellValue(sheet, fmt.Sprintf("A%d", footerRow), "TOTAL")
	file.SetCellValue(sheet, fmt.Sprintf("G%d", footerRow), totalGaji)
	file.SetCellValue(sheet, fmt.Sprintf("H%d", footerRow), totalKasbon)
	file.SetCellValue(sheet, fmt.Sprintf("J%d", footerRow), totalSetelahKasbon)
	file.MergeCell(sheet, fmt.Sprintf("A%d", footerRow), fmt.Sprintf("F%d", footerRow))

	filename := fmt.Sprintf("Laporan_Gaji_Revitalisasi_%s_%s.xlsx", strings.ToUpper(jenis), time.Now().Format("20060102_150405"))
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := file.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed_to_generate_excel"})
	}
	return nil
}

func (h *RevitalisasiHandler) ExportPayrollReportPDF(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	startDate := strings.TrimSpace(c.Query("date_from"))
	endDate := strings.TrimSpace(c.Query("date_to"))
	rows, err := h.generatePayrollReportRows(jenis, startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: err.Error()})
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, fmt.Sprintf("Laporan Gaji Revitalisasi %s", strings.ToUpper(jenis)), "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s s/d %s", startDate, endDate), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	colWidths := []float64{10, 42, 30, 30, 18, 24, 24, 22, 20, 32}
	headers := []string{"No", "Nama", "Divisi", "Area", "Hadir", "Gaji", "Total", "Kasbon", "Potong", "Setelah Kasbon"}
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(15, 118, 110)
	pdf.SetTextColor(255, 255, 255)
	for i, label := range headers {
		pdf.CellFormat(colWidths[i], 8, label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(245, 245, 245)
	pdf.SetFont("Helvetica", "", 7)

	totalGaji := 0.0
	totalKasbon := 0.0
	totalSetelahKasbon := 0.0
	for idx, row := range rows {
		fill := idx%2 == 1
		vals := []string{
			fmt.Sprintf("%d", idx+1),
			row.Name,
			row.Divisi,
			row.Area,
			fmt.Sprintf("%d", row.HariHadir),
			fmt.Sprintf("Rp %s", formatRupiahForReport(row.GajiHarian)),
			fmt.Sprintf("Rp %s", formatRupiahForReport(row.TotalGaji)),
			fmt.Sprintf("Rp %s", formatRupiahForReport(row.Kasbon)),
			capitalizePayrollLabel(row.CaraPotong),
			fmt.Sprintf("Rp %s", formatRupiahForReport(row.TotalSetelahKasbon)),
		}
		for i, val := range vals {
			pdf.CellFormat(colWidths[i], 7, val, "1", 0, "C", fill, 0, "")
		}
		pdf.Ln(-1)
		totalGaji += row.TotalGaji
		totalKasbon += row.Kasbon
		totalSetelahKasbon += row.TotalSetelahKasbon
	}
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(0, 7, fmt.Sprintf("TOTAL GAJI: Rp %s | TOTAL KASBON: Rp %s | TOTAL SETELAH KASBON: Rp %s", formatRupiahForReport(totalGaji), formatRupiahForReport(totalKasbon), formatRupiahForReport(totalSetelahKasbon)), "", 1, "L", false, 0, "")
	filename := fmt.Sprintf("Laporan_Gaji_Revitalisasi_%s_%s.pdf", strings.ToUpper(jenis), time.Now().Format("20060102_150405"))
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := pdf.Output(c.Response().BodyWriter()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed_to_generate_pdf"})
	}
	return nil
}

func formatRupiahForReport(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', 0, 64)
}

func capitalizePayrollLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "Langsung"
	}
	if len(trimmed) == 1 {
		return strings.ToUpper(trimmed)
	}
	return strings.ToUpper(trimmed[:1]) + strings.ToLower(trimmed[1:])
}

func (h *RevitalisasiHandler) ensureUploadDir(subdir string) string {
	destDir := filepath.Join(h.uploadPath, subdir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return ""
	}
	return destDir
}

func (h *RevitalisasiHandler) splitStoredPaths(pathValue *string) []string {
	if pathValue == nil || strings.TrimSpace(*pathValue) == "" {
		return nil
	}
	parts := strings.Split(*pathValue, ";")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths
}

func (h *RevitalisasiHandler) cleanupPhotoPath(pathValue *string) {
	for _, path := range h.splitStoredPaths(pathValue) {
		if path == "" {
			continue
		}
		_ = os.Remove(filepath.Join(h.uploadPath, filepath.FromSlash(strings.TrimPrefix(path, "/"))))
	}
}

func (h *RevitalisasiHandler) saveUploadedFiles(module string, files []*multipart.FileHeader) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path, err := h.processImage(module, file)
		if err != nil {
			return "", err
		}
		paths = append(paths, path)
	}
	return strings.Join(paths, ";"), nil
}

func (h *RevitalisasiHandler) getMultipartFiles(c *fiber.Ctx, fieldName string) []*multipart.FileHeader {
	form, err := c.MultipartForm()
	if err != nil || form == nil || form.File == nil {
		return nil
	}
	files := form.File[fieldName]
	if len(files) == 0 {
		return nil
	}
	return files
}

func (h *RevitalisasiHandler) getMultipartValue(c *fiber.Ctx, fieldName string) string {
	if form, err := c.MultipartForm(); err == nil && form != nil {
		if values, ok := form.Value[fieldName]; ok && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return strings.TrimSpace(c.FormValue(fieldName))
}

func (h *RevitalisasiHandler) getMultipartValues(c *fiber.Ctx, fieldName string) []string {
	if form, err := c.MultipartForm(); err == nil && form != nil {
		if values, ok := form.Value[fieldName]; ok && len(values) > 0 {
			result := make([]string, 0, len(values))
			for _, value := range values {
				trimmed := strings.TrimSpace(value)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}
	value := strings.TrimSpace(c.FormValue(fieldName))
	if value == "" {
		return nil
	}
	return []string{value}
}

func (h *RevitalisasiHandler) getMultipartFloat(c *fiber.Ctx, fieldName string) float64 {
	value := h.getMultipartValue(c, fieldName)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func (h *RevitalisasiHandler) getMultipartInt(c *fiber.Ctx, fieldName string) int {
	value := h.getMultipartValue(c, fieldName)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func parseFieldValues(values ...string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func (h *RevitalisasiHandler) removePhotoPaths(pathValue *string, removeList []string) *string {
	currentPaths := h.splitStoredPaths(pathValue)
	if len(currentPaths) == 0 || len(removeList) == 0 {
		return pathValue
	}
	removeSet := map[string]struct{}{}
	for _, removePath := range removeList {
		removeSet[removePath] = struct{}{}
	}
	remaining := make([]string, 0, len(currentPaths))
	for _, stored := range currentPaths {
		if _, shouldRemove := removeSet[stored]; shouldRemove {
			_ = os.Remove(filepath.Join(h.uploadPath, filepath.FromSlash(strings.TrimPrefix(stored, "/"))))
			continue
		}
		remaining = append(remaining, stored)
	}
	if len(remaining) == 0 {
		return nil
	}
	joined := strings.Join(remaining, ";")
	return &joined
}

// createUploadPath creates a unique stored filename and returns the relative path to be persisted in DB.
func (h *RevitalisasiHandler) createUploadPath(module string, file *multipart.FileHeader) (string, error) {
	if file == nil || file.Size == 0 {
		return "", fmt.Errorf("file is empty")
	}
	if file.Size > maxRevitalisasiImageBytes {
		return "", fmt.Errorf("file exceeds maximum size of 2MB")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !isAllowedImageExtension(ext) {
		return "", fmt.Errorf("unsupported file type")
	}

	dir := h.ensureUploadDir(module)
	if dir == "" {
		return "", fmt.Errorf("upload directory could not be created")
	}

	namePrefix := filepath.Base(module)
	if namePrefix == "." || namePrefix == "" || namePrefix == "/" {
		namePrefix = "upload"
	}
	filename := fmt.Sprintf("%s_%d%s", namePrefix, time.Now().UnixNano(), ext)
	fullPath := filepath.Join(dir, filename)

	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", err
	}

	return filepath.ToSlash(filepath.Join(module, filename)), nil
}

func (h *RevitalisasiHandler) processImage(module string, file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", fmt.Errorf("image file is empty")
	}
	if file.Size < 1 || file.Size > maxRevitalisasiImageBytes {
		return "", fmt.Errorf("image must be between 1 byte and 2MB")
	}
	if !isAllowedImageExtension(filepath.Ext(file.Filename)) {
		return "", fmt.Errorf("unsupported image type")
	}

	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	imgBytes, err := io.ReadAll(src)
	if err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	var img image.Image
	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(bytes.NewReader(imgBytes))
	case ".png":
		img, err = png.Decode(bytes.NewReader(imgBytes))
	case ".webp":
		return h.createUploadPath(module, file)
	default:
		return "", fmt.Errorf("unsupported image type")
	}
	if err != nil {
		return "", err
	}
	if img.Bounds().Dx() > 1400 || img.Bounds().Dy() > 1400 {
		img = resize.Resize(1400, 0, img, resize.Lanczos3)
	}

	destDir := h.ensureUploadDir(module)
	if destDir == "" {
		return "", fmt.Errorf("upload directory not available")
	}

	namePrefix := filepath.Base(module)
	if namePrefix == "." || namePrefix == "" || namePrefix == "/" {
		namePrefix = "upload"
	}
	timestamp := time.Now().UnixNano()
	jpgName := fmt.Sprintf("%s_%d.jpg", namePrefix, timestamp)
	jpgPath := filepath.Join(destDir, jpgName)
	jf, err := os.Create(jpgPath)
	if err != nil {
		return "", err
	}
	if err := jpeg.Encode(jf, img, &jpeg.Options{Quality: 80}); err != nil {
		jf.Close()
		return "", err
	}
	jf.Close()

	webpName := fmt.Sprintf("%s_%d.webp", namePrefix, timestamp)
	webpPath := filepath.Join(destDir, webpName)
	wf, err := os.Create(webpPath)
	if err != nil {
		return filepath.ToSlash(filepath.Join(module, jpgName)), nil
	}
	if err := nativewebp.Encode(wf, img, nil); err != nil {
		wf.Close()
		os.Remove(webpPath)
		return filepath.ToSlash(filepath.Join(module, jpgName)), nil
	}
	wf.Close()

	return filepath.ToSlash(filepath.Join(module, jpgName)), nil
}

// ============ Tukang ============

func (h *RevitalisasiHandler) ListKasbon(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	tukangIDQuery := strings.TrimSpace(c.Query("tukang_id"))

	query := h.db.Where("jenis = ?", jenis).Order("tanggal asc, id asc").Preload("Tukang")
	if tukangIDQuery != "" {
		parsed, err := strconv.ParseUint(tukangIDQuery, 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "tukang_id harus berupa angka valid"})
		}
		query = query.Where("tukang_id = ?", uint(parsed))
	}

	var entries []models.RevitalisasiKasbon
	if err := query.Find(&entries).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}

	runningBalance := 0.0
	for i := range entries {
		if entries[i].Jenis == "pelunasan" {
			runningBalance -= entries[i].Jumlah
		} else {
			runningBalance += entries[i].Jumlah
		}
		entries[i].Saldo = runningBalance
	}

	return c.JSON(fiber.Map{
		"data": entries,
		"summary": fiber.Map{
			"jenis":        jenis,
			"total_kasbon": runningBalance,
		},
	})
}

func (h *RevitalisasiHandler) CreateKasbon(c *fiber.Ctx) error {
	payload := struct {
		Jenis      string  `json:"jenis"`
		Tanggal    string  `json:"tanggal"`
		TukangID   uint    `json:"tukang_id"`
		Jumlah     float64 `json:"jumlah"`
		Metode     string  `json:"metode"`
		Keterangan string  `json:"keterangan"`
	}{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "payload tidak valid"})
	}
	if payload.TukangID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "tukang_id wajib diisi"})
	}
	if payload.Jumlah <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "jumlah kasbon harus lebih dari 0"})
	}
	parsedDate, err := safeDateString(payload.Tanggal)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: err.Error()})
	}

	entry := models.RevitalisasiKasbon{
		Jenis:      normalizeKasbonJenis(payload.Jenis),
		Tanggal:    parsedDate,
		TukangID:   payload.TukangID,
		Jumlah:     payload.Jumlah,
		Metode:     normalizeKasbonMetode(payload.Metode),
		Keterangan: strings.TrimSpace(payload.Keterangan),
	}
	if err := h.db.Create(&entry).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	if err := h.syncTukangKasbonBalance(payload.TukangID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	if err := h.db.First(&entry, entry.ID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(entry)
}

func (h *RevitalisasiHandler) UpdateKasbon(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "id kasbon tidak valid"})
	}

	var existing models.RevitalisasiKasbon
	if err := h.db.First(&existing, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "kasbon tidak ditemukan"})
	}

	payload := struct {
		Jenis      string  `json:"jenis"`
		Tanggal    string  `json:"tanggal"`
		TukangID   uint    `json:"tukang_id"`
		Jumlah     float64 `json:"jumlah"`
		Metode     string  `json:"metode"`
		Keterangan string  `json:"keterangan"`
	}{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "payload tidak valid"})
	}
	if payload.TukangID != 0 {
		existing.TukangID = payload.TukangID
	}
	if payload.Jumlah > 0 {
		existing.Jumlah = payload.Jumlah
	}
	if strings.TrimSpace(payload.Jenis) != "" {
		existing.Jenis = normalizeKasbonJenis(payload.Jenis)
	}
	if strings.TrimSpace(payload.Metode) != "" {
		existing.Metode = normalizeKasbonMetode(payload.Metode)
	}
	if strings.TrimSpace(payload.Keterangan) != "" || payload.Keterangan == "" {
		existing.Keterangan = strings.TrimSpace(payload.Keterangan)
	}
	if strings.TrimSpace(payload.Tanggal) != "" {
		parsedDate, err := safeDateString(payload.Tanggal)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: err.Error()})
		}
		existing.Tanggal = parsedDate
	}
	if existing.Jumlah <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "jumlah kasbon harus lebih dari 0"})
	}
	if err := h.db.Save(&existing).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	if err := h.syncTukangKasbonBalance(existing.TukangID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	return c.JSON(existing)
}

func (h *RevitalisasiHandler) DeleteKasbon(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "id kasbon tidak valid"})
	}

	var entry models.RevitalisasiKasbon
	if err := h.db.First(&entry, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "kasbon tidak ditemukan"})
	}
	if err := h.db.Delete(&entry).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	if err := h.syncTukangKasbonBalance(entry.TukangID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "database_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Kasbon berhasil dihapus"})
}

func (h *RevitalisasiHandler) ListTukang(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	search := strings.TrimSpace(c.Query("search"))
	statusFilter := strings.TrimSpace(c.Query("status"))
	query := h.db.Model(&models.RevitalisasiTukang{}).Where("jenis = ?", jenis).Order("updated_at desc")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("name LIKE ? OR divisi LIKE ? OR area LIKE ? OR phone LIKE ?", like, like, like, like)
	}
	if statusFilter != "" && statusFilter != "all" {
		active, err := strconv.ParseBool(statusFilter)
		if err == nil {
			query = query.Where("is_active = ?", active)
		}
	}

	var items []models.RevitalisasiTukang
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items, "total": len(items)})
}

func (h *RevitalisasiHandler) CreateTukang(c *fiber.Ctx) error {
	var payload models.RevitalisasiTukang
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid payload"})
	}
	payload.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Divisi = strings.TrimSpace(payload.Divisi)
	payload.Area = strings.TrimSpace(payload.Area)
	payload.Phone = strings.TrimSpace(payload.Phone)
	if payload.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Nama tukang wajib diisi"})
	}
	if err := h.db.Create(&payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(payload)
}

func (h *RevitalisasiHandler) UpdateTukang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiTukang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tukang tidak ditemukan"})
	}

	var payload models.RevitalisasiTukang
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid payload"})
	}

	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	item.Name = strings.TrimSpace(payload.Name)
	item.Divisi = strings.TrimSpace(payload.Divisi)
	item.Area = strings.TrimSpace(payload.Area)
	item.Phone = strings.TrimSpace(payload.Phone)
	item.Note = strings.TrimSpace(payload.Note)
	item.IsActive = payload.IsActive
	if item.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Nama tukang wajib diisi"})
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteTukang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiTukang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tukang tidak ditemukan"})
	}
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Tukang berhasil dihapus"})
}

func (h *RevitalisasiHandler) ForceDeleteTukang(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "id tukang tidak valid"})
	}

	var item models.RevitalisasiTukang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tukang tidak ditemukan"})
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("tukang_id = ?", item.ID).Delete(&models.RevitalisasiKasbon{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("tukang_id = ?", item.ID).Delete(&models.RevitalisasiAbsenTukang{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&item).Error
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Tukang berhasil dihapus permanen"})
}

// ============ Absen Tukang ============

func (h *RevitalisasiHandler) ListAbsenTukang(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))

	query := h.db.Model(&models.RevitalisasiAbsenTukang{}).Where("revitalisasi_absen_tukang.jenis = ?", jenis).Preload("Tukang")
	if dateFrom != "" {
		if t, err := safeDateString(dateFrom); err == nil {
			query = query.Where("tanggal >= ?", t)
		}
	}
	if dateTo != "" {
		if t, err := safeDateString(dateTo); err == nil {
			query = query.Where("tanggal <= ?", t)
		}
	}
	if status != "" && status != "all" {
		query = query.Where("status = ?", normalizeStatus(status))
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Joins("LEFT JOIN revitalisasi_tukang ON revitalisasi_tukang.id = revitalisasi_absen_tukang.tukang_id").Where("revitalisasi_tukang.name LIKE ? OR revitalisasi_absen_tukang.note LIKE ?", like, like)
	}

	var items []models.RevitalisasiAbsenTukang
	if err := query.Order("tanggal desc, id desc").Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreateAbsenTukang(c *fiber.Ctx) error {
	var payload struct {
		Tanggal  string `json:"tanggal"`
		TukangID uint   `json:"tukang_id"`
		Status   string `json:"status"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&payload); err != nil {
		payload.Tanggal = h.getMultipartValue(c, "tanggal")
		payload.Status = h.getMultipartValue(c, "status")
		payload.Note = h.getMultipartValue(c, "note")
		if tukangIDRaw := h.getMultipartValue(c, "tukang_id"); tukangIDRaw != "" {
			if parsed, err := strconv.ParseUint(tukangIDRaw, 10, 32); err == nil {
				payload.TukangID = uint(parsed)
			}
		}
		if payload.Tanggal == "" && payload.TukangID == 0 && payload.Status == "" && payload.Note == "" {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid payload"})
		}
	} else {
		payload.Tanggal = strings.TrimSpace(payload.Tanggal)
		payload.Status = strings.TrimSpace(payload.Status)
		payload.Note = strings.TrimSpace(payload.Note)
		if payload.Tanggal == "" || payload.TukangID == 0 {
			payload.Tanggal = h.getMultipartValue(c, "tanggal")
			payload.Status = h.getMultipartValue(c, "status")
			payload.Note = h.getMultipartValue(c, "note")
			if tukangIDRaw := h.getMultipartValue(c, "tukang_id"); tukangIDRaw != "" {
				if parsed, err := strconv.ParseUint(tukangIDRaw, 10, 32); err == nil {
					payload.TukangID = uint(parsed)
				}
			}
		}
	}
	if payload.Tanggal == "" || payload.TukangID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Tanggal dan tukang wajib diisi"})
	}

	var tukang models.RevitalisasiTukang
	if err := h.db.First(&tukang, payload.TukangID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tukang tidak ditemukan"})
	}

	dateValue, err := safeDateString(payload.Tanggal)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Format tanggal tidak valid"})
	}

	item := models.RevitalisasiAbsenTukang{
		Jenis:    resolveRevitalisasiJenis(c.Path(), ""),
		Tanggal:  dateValue,
		TukangID: payload.TukangID,
		Status:   normalizeStatus(payload.Status),
		Note:     strings.TrimSpace(payload.Note),
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/absen", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if err := h.db.Create(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	if err := h.db.Preload("Tukang").First(&item, item.ID).Error; err != nil {
		return c.JSON(item)
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *RevitalisasiHandler) UpdateAbsenTukang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiAbsenTukang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Data absensi tidak ditemukan"})
	}

	var payload struct {
		Tanggal  string `json:"tanggal"`
		TukangID uint   `json:"tukang_id"`
		Status   string `json:"status"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&payload); err != nil {
		payload.Tanggal = h.getMultipartValue(c, "tanggal")
		payload.Status = h.getMultipartValue(c, "status")
		payload.Note = h.getMultipartValue(c, "note")
		if tukangIDRaw := h.getMultipartValue(c, "tukang_id"); tukangIDRaw != "" {
			if parsed, err := strconv.ParseUint(tukangIDRaw, 10, 32); err == nil {
				payload.TukangID = uint(parsed)
			}
		}
	} else {
		payload.Tanggal = strings.TrimSpace(payload.Tanggal)
		payload.Status = strings.TrimSpace(payload.Status)
		payload.Note = strings.TrimSpace(payload.Note)
		if payload.Tanggal == "" || payload.TukangID == 0 {
			payload.Tanggal = h.getMultipartValue(c, "tanggal")
			payload.Status = h.getMultipartValue(c, "status")
			payload.Note = h.getMultipartValue(c, "note")
			if tukangIDRaw := h.getMultipartValue(c, "tukang_id"); tukangIDRaw != "" {
				if parsed, err := strconv.ParseUint(tukangIDRaw, 10, 32); err == nil {
					payload.TukangID = uint(parsed)
				}
			}
		}
	}
	if payload.Tanggal != "" {
		dateValue, err := safeDateString(payload.Tanggal)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Format tanggal tidak valid"})
		}
		item.Tanggal = dateValue
	}
	if payload.TukangID > 0 {
		var tukang models.RevitalisasiTukang
		if err := h.db.First(&tukang, payload.TukangID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tukang tidak ditemukan"})
		}
		item.TukangID = payload.TukangID
	}
	if payload.Status != "" {
		item.Status = normalizeStatus(payload.Status)
	}
	item.Note = strings.TrimSpace(payload.Note)

	removeList := parseFieldValues(h.getMultipartValues(c, "remove_photo")...)
	if len(removeList) > 0 {
		item.PhotoPath = h.removePhotoPaths(item.PhotoPath, removeList)
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/absen", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		mergedPaths := append(h.splitStoredPaths(item.PhotoPath), parseFieldValues(photoPaths)...)
		if len(mergedPaths) == 0 {
			item.PhotoPath = nil
		} else {
			joined := strings.Join(mergedPaths, ";")
			item.PhotoPath = &joined
		}
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	if err := h.db.Preload("Tukang").First(&item, item.ID).Error; err != nil {
		return c.JSON(item)
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteAbsenTukang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiAbsenTukang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Data absensi tidak ditemukan"})
	}
	h.cleanupPhotoPath(item.PhotoPath)
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Data absensi berhasil dihapus"})
}

// ============ Nota Material ============

func (h *RevitalisasiHandler) ListNotaMaterial(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.RevitalisasiNotaMaterial{}).Where("jenis = ?", jenis).Order("tanggal desc, id desc")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("nomor_nota LIKE ? OR supplier LIKE ? OR keterangan LIKE ?", like, like, like)
	}
	var items []models.RevitalisasiNotaMaterial
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreateNotaMaterial(c *fiber.Ctx) error {
	var payload models.RevitalisasiNotaMaterial
	if err := c.BodyParser(&payload); err != nil {
		payload.Tanggal, _ = safeDateString(h.getMultipartValue(c, "tanggal"))
		payload.NomorNota = h.getMultipartValue(c, "nomor_nota")
		payload.Supplier = h.getMultipartValue(c, "supplier")
		payload.Keterangan = h.getMultipartValue(c, "keterangan")
		payload.TotalNilai = h.getMultipartFloat(c, "total_nilai")
	}
	payload.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	if payload.Tanggal.IsZero() || payload.NomorNota == "" || payload.Supplier == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Tanggal, nomor nota, dan supplier wajib diisi"})
	}
	payload.NomorNota = strings.TrimSpace(payload.NomorNota)
	payload.Supplier = strings.TrimSpace(payload.Supplier)
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/nota", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		payload.PhotoPath = &photoPaths
	}
	if err := h.db.Create(&payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(payload)
}

func (h *RevitalisasiHandler) UpdateNotaMaterial(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiNotaMaterial
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Nota tidak ditemukan"})
	}
	var payload models.RevitalisasiNotaMaterial
	if err := c.BodyParser(&payload); err != nil {
		if dateValue, err := safeDateString(h.getMultipartValue(c, "tanggal")); err == nil {
			payload.Tanggal = dateValue
		}
		payload.NomorNota = h.getMultipartValue(c, "nomor_nota")
		payload.Supplier = h.getMultipartValue(c, "supplier")
		payload.Keterangan = h.getMultipartValue(c, "keterangan")
		payload.TotalNilai = h.getMultipartFloat(c, "total_nilai")
	}
	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	if payload.NomorNota != "" {
		item.NomorNota = strings.TrimSpace(payload.NomorNota)
	}
	if payload.Supplier != "" {
		item.Supplier = strings.TrimSpace(payload.Supplier)
	}
	if !payload.Tanggal.IsZero() {
		item.Tanggal = payload.Tanggal
	}
	item.Keterangan = strings.TrimSpace(payload.Keterangan)
	if payload.TotalNilai != 0 || h.getMultipartValue(c, "total_nilai") != "" {
		item.TotalNilai = payload.TotalNilai
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		h.cleanupPhotoPath(item.PhotoPath)
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/nota", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteNotaMaterial(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiNotaMaterial
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Nota tidak ditemukan"})
	}
	h.cleanupPhotoPath(item.PhotoPath)
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Nota berhasil dihapus"})
}

// ============ Nota Masuk ============

func (h *RevitalisasiHandler) ListNotaMasuk(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.RevitalisasiNotaMasuk{}).Where("jenis = ?", jenis).Order("tanggal desc, id desc")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("nomor_nota LIKE ? OR sumber LIKE ? OR keterangan LIKE ?", like, like, like)
	}
	var items []models.RevitalisasiNotaMasuk
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreateNotaMasuk(c *fiber.Ctx) error {
	var payload models.RevitalisasiNotaMasuk
	if err := c.BodyParser(&payload); err != nil {
		if dateValue, err := safeDateString(h.getMultipartValue(c, "tanggal")); err == nil {
			payload.Tanggal = dateValue
		}
		payload.NomorNota = h.getMultipartValue(c, "nomor_nota")
		payload.Sumber = h.getMultipartValue(c, "sumber")
		payload.Jumlah = h.getMultipartFloat(c, "jumlah")
		payload.Keterangan = h.getMultipartValue(c, "keterangan")
	}
	payload.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	if payload.Tanggal.IsZero() || payload.NomorNota == "" || payload.Sumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Tanggal, nomor nota, dan sumber wajib diisi"})
	}
	payload.NomorNota = strings.TrimSpace(payload.NomorNota)
	payload.Sumber = strings.TrimSpace(payload.Sumber)
	payload.Keterangan = strings.TrimSpace(payload.Keterangan)
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/masuk", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		payload.PhotoPath = &photoPaths
	}
	if err := h.db.Create(&payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(payload)
}

func (h *RevitalisasiHandler) UpdateNotaMasuk(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiNotaMasuk
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Nota masuk tidak ditemukan"})
	}
	var payload models.RevitalisasiNotaMasuk
	if err := c.BodyParser(&payload); err != nil {
		if dateValue, err := safeDateString(h.getMultipartValue(c, "tanggal")); err == nil {
			payload.Tanggal = dateValue
		}
		payload.NomorNota = h.getMultipartValue(c, "nomor_nota")
		payload.Sumber = h.getMultipartValue(c, "sumber")
		payload.Jumlah = h.getMultipartFloat(c, "jumlah")
		payload.Keterangan = h.getMultipartValue(c, "keterangan")
	}
	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	if payload.NomorNota != "" {
		item.NomorNota = strings.TrimSpace(payload.NomorNota)
	}
	if payload.Sumber != "" {
		item.Sumber = strings.TrimSpace(payload.Sumber)
	}
	if !payload.Tanggal.IsZero() {
		item.Tanggal = payload.Tanggal
	}
	if payload.Jumlah != 0 || h.getMultipartValue(c, "jumlah") != "" {
		item.Jumlah = payload.Jumlah
	}
	item.Keterangan = strings.TrimSpace(payload.Keterangan)
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		h.cleanupPhotoPath(item.PhotoPath)
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/masuk", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteNotaMasuk(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiNotaMasuk
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Nota masuk tidak ditemukan"})
	}
	h.cleanupPhotoPath(item.PhotoPath)
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Nota masuk berhasil dihapus"})
}

// ============ Material Datang ============

func (h *RevitalisasiHandler) ListMaterialDatang(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.RevitalisasiMaterialDatang{}).Where("jenis = ?", jenis).Order("tanggal desc, id desc")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("nama_material LIKE ? OR supplier LIKE ? OR catatan LIKE ?", like, like, like)
	}
	var items []models.RevitalisasiMaterialDatang
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreateMaterialDatang(c *fiber.Ctx) error {
	var payload struct {
		Tanggal              string  `form:"tanggal" json:"tanggal"`
		NamaMaterial         string  `form:"nama_material" json:"nama_material"`
		Supplier             string  `form:"supplier" json:"supplier"`
		Jumlah               float64 `form:"jumlah" json:"jumlah"`
		Satuan               string  `form:"satuan" json:"satuan"`
		Catatan              string  `form:"catatan" json:"catatan"`
		NomorNotaPengeluaran string  `form:"nomor_nota_pengeluaran" json:"nomor_nota_pengeluaran"`
		TotalPengeluaran     float64 `form:"total_pengeluaran" json:"total_pengeluaran"`
	}
	if err := c.BodyParser(&payload); err != nil {
		payload.Tanggal = h.getMultipartValue(c, "tanggal")
		payload.NamaMaterial = h.getMultipartValue(c, "nama_material")
		payload.Supplier = h.getMultipartValue(c, "supplier")
		payload.Jumlah = h.getMultipartFloat(c, "jumlah")
		payload.Satuan = h.getMultipartValue(c, "satuan")
		payload.Catatan = h.getMultipartValue(c, "catatan")
		payload.NomorNotaPengeluaran = h.getMultipartValue(c, "nomor_nota_pengeluaran")
		payload.TotalPengeluaran = h.getMultipartFloat(c, "total_pengeluaran")
	}
	dateValue, err := safeDateString(payload.Tanggal)
	if err != nil || strings.TrimSpace(payload.NamaMaterial) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Tanggal dan nama material wajib diisi"})
	}
	item := models.RevitalisasiMaterialDatang{
		Jenis:                resolveRevitalisasiJenis(c.Path(), ""),
		Tanggal:              dateValue,
		NamaMaterial:         strings.TrimSpace(payload.NamaMaterial),
		Supplier:             strings.TrimSpace(payload.Supplier),
		Jumlah:               payload.Jumlah,
		Satuan:               strings.TrimSpace(payload.Satuan),
		Catatan:              strings.TrimSpace(payload.Catatan),
		NomorNotaPengeluaran: strings.TrimSpace(payload.NomorNotaPengeluaran),
		TotalPengeluaran:     payload.TotalPengeluaran,
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/material", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if files := h.getMultipartFiles(c, "nota_pengeluaran"); len(files) > 0 {
		notaPaths, pErr := h.saveUploadedFiles("revitalisasi/material-nota", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.NotaPengeluaranPath = &notaPaths
	}
	if err := h.db.Create(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *RevitalisasiHandler) UpdateMaterialDatang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiMaterialDatang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Material tidak ditemukan"})
	}
	var payload struct {
		Jenis                string  `form:"jenis" json:"jenis"`
		Tanggal              string  `form:"tanggal" json:"tanggal"`
		NamaMaterial         string  `form:"nama_material" json:"nama_material"`
		Supplier             string  `form:"supplier" json:"supplier"`
		Jumlah               float64 `form:"jumlah" json:"jumlah"`
		Satuan               string  `form:"satuan" json:"satuan"`
		Catatan              string  `form:"catatan" json:"catatan"`
		NomorNotaPengeluaran string  `form:"nomor_nota_pengeluaran" json:"nomor_nota_pengeluaran"`
		TotalPengeluaran     float64 `form:"total_pengeluaran" json:"total_pengeluaran"`
	}
	if err := c.BodyParser(&payload); err != nil {
		payload.Tanggal = h.getMultipartValue(c, "tanggal")
		payload.NamaMaterial = h.getMultipartValue(c, "nama_material")
		payload.Supplier = h.getMultipartValue(c, "supplier")
		payload.Jumlah = h.getMultipartFloat(c, "jumlah")
		payload.Satuan = h.getMultipartValue(c, "satuan")
		payload.Catatan = h.getMultipartValue(c, "catatan")
		payload.NomorNotaPengeluaran = h.getMultipartValue(c, "nomor_nota_pengeluaran")
		payload.TotalPengeluaran = h.getMultipartFloat(c, "total_pengeluaran")
	}
	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	if payload.NamaMaterial != "" {
		item.NamaMaterial = strings.TrimSpace(payload.NamaMaterial)
	}
	if payload.Tanggal != "" {
		dateValue, err := safeDateString(payload.Tanggal)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Format tanggal tidak valid"})
		}
		item.Tanggal = dateValue
	}
	if payload.Supplier != "" || payload.Tanggal != "" || payload.NamaMaterial != "" {
		item.Supplier = strings.TrimSpace(payload.Supplier)
	}
	if payload.Jumlah != 0 || h.getMultipartValue(c, "jumlah") != "" {
		item.Jumlah = payload.Jumlah
	}
	if payload.Satuan != "" || h.getMultipartValue(c, "satuan") != "" {
		item.Satuan = strings.TrimSpace(payload.Satuan)
	}
	if payload.Catatan != "" || h.getMultipartValue(c, "catatan") != "" {
		item.Catatan = strings.TrimSpace(payload.Catatan)
	}
	if payload.NomorNotaPengeluaran != "" || h.getMultipartValue(c, "nomor_nota_pengeluaran") != "" {
		item.NomorNotaPengeluaran = strings.TrimSpace(payload.NomorNotaPengeluaran)
	}
	if payload.TotalPengeluaran != 0 || h.getMultipartValue(c, "total_pengeluaran") != "" {
		item.TotalPengeluaran = payload.TotalPengeluaran
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		h.cleanupPhotoPath(item.PhotoPath)
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/material", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if files := h.getMultipartFiles(c, "nota_pengeluaran"); len(files) > 0 {
		h.cleanupPhotoPath(item.NotaPengeluaranPath)
		notaPaths, pErr := h.saveUploadedFiles("revitalisasi/material-nota", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.NotaPengeluaranPath = &notaPaths
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteMaterialDatang(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiMaterialDatang
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Material tidak ditemukan"})
	}
	h.cleanupPhotoPath(item.PhotoPath)
	h.cleanupPhotoPath(item.NotaPengeluaranPath)
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Material berhasil dihapus"})
}

// ============ Progres Pembangunan ============

func (h *RevitalisasiHandler) ListProgresPembangunan(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.RevitalisasiProgresPembangunan{}).Where("jenis = ?", jenis).Order("tanggal desc, id desc")
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("nama_area LIKE ? OR catatan LIKE ?", like, like)
	}
	var items []models.RevitalisasiProgresPembangunan
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreateProgresPembangunan(c *fiber.Ctx) error {
	var payload models.RevitalisasiProgresPembangunan
	if err := c.BodyParser(&payload); err != nil {
		if dateValue, err := safeDateString(h.getMultipartValue(c, "tanggal")); err == nil {
			payload.Tanggal = dateValue
		}
		payload.NamaArea = h.getMultipartValue(c, "nama_area")
		payload.Persentase = h.getMultipartInt(c, "persentase")
		payload.Catatan = h.getMultipartValue(c, "catatan")
	}
	payload.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	if payload.Tanggal.IsZero() || strings.TrimSpace(payload.NamaArea) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Tanggal dan area wajib diisi"})
	}
	payload.NamaArea = strings.TrimSpace(payload.NamaArea)
	payload.Catatan = strings.TrimSpace(payload.Catatan)
	if payload.Persentase < 0 || payload.Persentase > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Persentase harus di antara 0 dan 100"})
	}
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/progres", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		payload.PhotoPath = &photoPaths
	}
	if err := h.db.Create(&payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(payload)
}

func (h *RevitalisasiHandler) UpdateProgresPembangunan(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiProgresPembangunan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Progress tidak ditemukan"})
	}
	var payload models.RevitalisasiProgresPembangunan
	if err := c.BodyParser(&payload); err != nil {
		if dateValue, err := safeDateString(h.getMultipartValue(c, "tanggal")); err == nil {
			payload.Tanggal = dateValue
		}
		payload.NamaArea = h.getMultipartValue(c, "nama_area")
		payload.Persentase = h.getMultipartInt(c, "persentase")
		payload.Catatan = h.getMultipartValue(c, "catatan")
	}
	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	if payload.NamaArea != "" {
		item.NamaArea = strings.TrimSpace(payload.NamaArea)
	}
	if !payload.Tanggal.IsZero() {
		item.Tanggal = payload.Tanggal
	}
	if payload.Persentase >= 0 || h.getMultipartValue(c, "persentase") != "" {
		item.Persentase = payload.Persentase
	}
	item.Catatan = strings.TrimSpace(payload.Catatan)
	if files := h.getMultipartFiles(c, "photo"); len(files) > 0 {
		h.cleanupPhotoPath(item.PhotoPath)
		photoPaths, pErr := h.saveUploadedFiles("revitalisasi/progres", files)
		if pErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: pErr.Error()})
		}
		item.PhotoPath = &photoPaths
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeleteProgresPembangunan(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiProgresPembangunan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Progress tidak ditemukan"})
	}
	h.cleanupPhotoPath(item.PhotoPath)
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Progress berhasil dihapus"})
}

// ============ Prioritas Dashboard ============

func (h *RevitalisasiHandler) ListPrioritas(c *fiber.Ctx) error {
	jenis := resolveRevitalisasiJenis(c.Path(), c.Query("jenis"))
	query := h.db.Model(&models.RevitalisasiPrioritas{}).Where("jenis = ?", jenis).Order("urutan asc, id asc")
	var items []models.RevitalisasiPrioritas
	if err := query.Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"data": items})
}

func (h *RevitalisasiHandler) CreatePrioritas(c *fiber.Ctx) error {
	var payload models.RevitalisasiPrioritas
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid payload"})
	}
	payload.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	payload.Judul = strings.TrimSpace(payload.Judul)
	payload.Deskripsi = strings.TrimSpace(payload.Deskripsi)
	payload.Tingkat = strings.TrimSpace(payload.Tingkat)
	if payload.Judul == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Judul prioritas wajib diisi"})
	}
	if payload.Tingkat == "" {
		payload.Tingkat = "medium"
	}
	if err := h.db.Create(&payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(payload)
}

func (h *RevitalisasiHandler) UpdatePrioritas(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiPrioritas
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Prioritas tidak ditemukan"})
	}
	var payload models.RevitalisasiPrioritas
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid payload"})
	}
	if payload.Jenis != "" {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), payload.Jenis)
	} else {
		item.Jenis = resolveRevitalisasiJenis(c.Path(), item.Jenis)
	}
	if payload.Judul != "" {
		item.Judul = strings.TrimSpace(payload.Judul)
	}
	if payload.Deskripsi != "" || payload.Tingkat != "" || payload.Urutan != 0 || payload.IsActive != item.IsActive {
		item.Deskripsi = strings.TrimSpace(payload.Deskripsi)
		item.Tingkat = strings.TrimSpace(payload.Tingkat)
		if payload.Urutan != 0 {
			item.Urutan = payload.Urutan
		}
		item.IsActive = payload.IsActive
	}
	if item.Judul == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Judul prioritas wajib diisi"})
	}
	if item.Tingkat == "" {
		item.Tingkat = "medium"
	}
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(item)
}

func (h *RevitalisasiHandler) DeletePrioritas(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.RevitalisasiPrioritas
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Prioritas tidak ditemukan"})
	}
	if err := h.db.Delete(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Prioritas berhasil dihapus"})
}
