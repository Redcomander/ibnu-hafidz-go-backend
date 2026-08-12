package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type TagihanHandler struct {
	db *gorm.DB
}

type TagihanDashboardHandler struct {
	db *gorm.DB
}

func NewTagihanHandler(db *gorm.DB) *TagihanHandler {
	return &TagihanHandler{db: db}
}

func NewTagihanDashboardHandler(db *gorm.DB) *TagihanDashboardHandler {
	return &TagihanDashboardHandler{db: db}
}

func (h *TagihanHandler) buildTagihanListQuery(c *fiber.Ctx) *gorm.DB {
	query := h.db.Model(&models.Tagihan{})

	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status_tagihan = ?", status)
	}
	if handlerID := strings.TrimSpace(c.Query("handler_id")); handlerID != "" {
		query = query.Where("handler_id = ?", handlerID)
	}
	if source := strings.TrimSpace(c.Query("sumber_data")); source != "" {
		query = query.Where("sumber_data = ?", source)
	}

	if search := strings.TrimSpace(c.Query("search")); search != "" {
		term := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(nama) LIKE ? OR LOWER(nis) LIKE ? OR LOWER(no_whatsapp) LIKE ? OR LOWER(catatan) LIKE ? OR LOWER(sumber_data) LIKE ?",
			term, term, term, term, term,
		)
	}

	sortBy := sanitizeTagihanSortField(c.Query("sort", "updated_at"))
	order := strings.ToLower(strings.TrimSpace(c.Query("order", "desc")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	return query.Order(sortBy + " " + order)
}

func sanitizeTagihanSortField(field string) string {
	switch strings.TrimSpace(field) {
	case "id", "nama", "nis", "no_whatsapp", "total_tagihan", "status_tagihan", "sumber_data", "last_contact_at", "created_at", "updated_at":
		return strings.TrimSpace(field)
	default:
		return "updated_at"
	}
}

func (h *TagihanHandler) List(c *fiber.Ctx) error {
	var items []models.Tagihan
	var total int64

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := c.QueryInt("per_page", 20)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 1000 {
		perPage = 1000
	}

	query := h.buildTagihanListQuery(c)
	query.Count(&total)

	offset := (page - 1) * perPage
	if err := query.
		Preload("Handler", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).
		Limit(perPage).
		Offset(offset).
		Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to fetch tagihan list"})
	}

	return c.JSON(BuildPaginatedResponse(items, total, page, perPage))
}

func (h *TagihanHandler) SumberOptions(c *fiber.Ctx) error {
	search := strings.TrimSpace(c.Query("search"))
	query := h.db.Model(&models.Tagihan{}).
		Where("sumber_data IS NOT NULL").
		Where("TRIM(sumber_data) <> ''")
	if search != "" {
		query = query.Where("LOWER(sumber_data) LIKE ?", "%"+strings.ToLower(search)+"%")
	}

	var values []string
	if err := query.
		Distinct("sumber_data").
		Order("sumber_data ASC").
		Pluck("sumber_data", &values).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to fetch sumber data options"})
	}
	return c.JSON(fiber.Map{"data": values})
}

func (h *TagihanHandler) DeleteSource(c *fiber.Ctx) error {
	source := strings.TrimSpace(c.Params("sumber"))
	if source == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Sumber wajib diisi"})
	}

	decoded, err := url.QueryUnescape(source)
	if err == nil {
		source = decoded
	}

	result := h.db.Where("sumber_data = ?", source).Delete(&models.Tagihan{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to delete sumber tagihan"})
	}

	return c.JSON(fiber.Map{"message": "Sumber tagihan berhasil dihapus", "deleted_count": result.RowsAffected})
}

func (h *TagihanHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.Tagihan{}, id)
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tagihan not found"})
	}
	return c.JSON(fiber.Map{"message": "Tagihan deleted successfully"})
}

func (h *TagihanHandler) BulkDelete(c *fiber.Ctx) error {
	type reqBody struct {
		IDs []uint `json:"ids"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Daftar ID tagihan wajib diisi"})
	}

	result := h.db.Where("id IN ?", req.IDs).Delete(&models.Tagihan{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to bulk delete tagihan"})
	}
	return c.JSON(fiber.Map{"message": "Bulk delete tagihan berhasil", "deleted_count": result.RowsAffected})
}

func (h *TagihanHandler) ExportExcel(c *fiber.Ctx) error {
	query := h.buildTagihanListQuery(c)
	var rows []models.Tagihan
	if err := query.Preload("Handler", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to export tagihan"})
	}

	f := excelize.NewFile()
	sheet := "Tagihan"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No", "NIS", "Nama", "No WhatsApp", "Total Tagihan", "Status Tagihan", "Handler", "Sumber Data", "Catatan", "Terakhir Dihubungi", "Dibuat"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	for i, item := range rows {
		rowNum := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowNum), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowNum), stringOrDash(item.NIS))
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowNum), item.Nama)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowNum), item.NoWhatsapp)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowNum), item.TotalTagihan)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowNum), item.StatusTagihan)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowNum), userNameOrDash(item.Handler))
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowNum), stringOrDash(item.SumberData))
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowNum), stringOrDash(item.Catatan))
		f.SetCellValue(sheet, fmt.Sprintf("J%d", rowNum), formatDateTimeForExport(item.LastContactAt))
		f.SetCellValue(sheet, fmt.Sprintf("K%d", rowNum), item.CreatedAt.Format("2006-01-02 15:04"))
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("tagihan_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	return f.Write(c.Response().BodyWriter())
}

func (h *TagihanHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Tagihan
	if err := h.db.Preload("Handler", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tagihan not found"})
	}
	return c.JSON(item)
}

func (h *TagihanHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Tagihan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tagihan not found"})
	}

	type request struct {
		NIS           *string `json:"nis"`
		Nama          string  `json:"nama"`
		NoWhatsapp    string  `json:"no_whatsapp"`
		TotalTagihan  any     `json:"total_tagihan"`
		StatusTagihan string  `json:"status_tagihan"`
		HandlerID     *uint   `json:"handler_id"`
		SumberData    *string `json:"sumber_data"`
		Catatan       *string `json:"catatan"`
	}
	var req request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	if strings.TrimSpace(req.Nama) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Nama wajib diisi"})
	}
	normalized := utils.NormalizeWhatsAppNumber(req.NoWhatsapp)
	if strings.TrimSpace(normalized) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "No WhatsApp wajib diisi"})
	}
	if req.HandlerID != nil {
		var user models.User
		if err := h.db.Select("id").First(&user, *req.HandlerID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Handler user tidak ditemukan"})
		}
	}

	if req.NIS != nil {
		trimmed := strings.TrimSpace(*req.NIS)
		if trimmed == "" {
			req.NIS = nil
		} else {
			req.NIS = &trimmed
		}
	}
	statusBaru := strings.TrimSpace(req.StatusTagihan)
	if statusBaru == "" {
		statusBaru = item.StatusTagihan
	}
	oldStatus := item.StatusTagihan
	item.NIS = req.NIS
	item.Nama = strings.TrimSpace(req.Nama)
	item.NoWhatsapp = normalized
	item.TotalTagihan = coerceTotalTagihan(req.TotalTagihan, item.TotalTagihan)
	item.StatusTagihan = statusBaru
	item.HandlerID = req.HandlerID
	item.SumberData = req.SumberData
	item.Catatan = req.Catatan
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update tagihan"})
	}

	if oldStatus != item.StatusTagihan {
		userID, _ := c.Locals("userID").(uint)
		statusBefore := oldStatus
		statusAfter := item.StatusTagihan
		_ = h.db.Create(&models.RiwayatKontak{KontakID: 0, StatusAwal: &statusBefore, StatusAkhir: &statusAfter}).Error
		_ = userID
	}

	return c.JSON(item)
}

func coerceTotalTagihan(value any, fallback int64) int64 {
	switch v := value.(type) {
	case nil:
		return fallback
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		return normalizeCurrencyValue(v)
	case []byte:
		return normalizeCurrencyValue(string(v))
	default:
		if s, ok := value.(fmt.Stringer); ok {
			return normalizeCurrencyValue(s.String())
		}
		return fallback
	}
}

func (h *TagihanHandler) UpdateStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Tagihan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tagihan not found"})
	}

	type reqBody struct {
		StatusTagihan string  `json:"status_tagihan"`
		Catatan       *string `json:"catatan"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	newStatus := strings.TrimSpace(req.StatusTagihan)
	if newStatus == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Status tagihan wajib diisi"})
	}
	item.StatusTagihan = newStatus
	now := time.Now()
	item.LastContactAt = &now
	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update status tagihan"})
	}
	_ = req.Catatan
	return c.JSON(fiber.Map{"message": "Status tagihan berhasil diperbarui", "data": item})
}

func (h *TagihanHandler) WhatsAppLink(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Tagihan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tagihan not found"})
	}

	templateIDParam := strings.TrimSpace(c.Query("template_id"))
	customMessage := strings.TrimSpace(c.Query("message"))
	message := customMessage
	if templateIDParam != "" {
		if tid64, err := strconv.ParseUint(templateIDParam, 10, 64); err == nil {
			if tmpl, err := fetchTemplateByID(h.db, tid64); err == nil {
				message = renderTagihanTemplateMessage(tmpl.Konten, item)
			}
		}
	}
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("Assalamu'alaikum, tagihan %s sebesar Rp %d sudah kami catat.", item.Nama, item.TotalTagihan)
	}
	normalized := utils.NormalizeWhatsAppNumber(item.NoWhatsapp)
	waURL := "https://wa.me/" + normalized + "?text=" + url.QueryEscape(message)
	return c.JSON(fiber.Map{"url": waURL, "nomor_normalize": normalized, "message": message})
}

func (h *TagihanDashboardHandler) Summary(c *fiber.Ctx) error {
	var totalTagihan int64
	h.db.Model(&models.Tagihan{}).Count(&totalTagihan)

	var assigned int64
	h.db.Model(&models.Tagihan{}).Where("handler_id IS NOT NULL").Count(&assigned)
	var unassigned int64
	h.db.Model(&models.Tagihan{}).Where("handler_id IS NULL").Count(&unassigned)
	var importTotal int64
	h.db.Model(&models.ImportBatch{}).Count(&importTotal)
	var totalBelumLunas int64
	h.db.Model(&models.Tagihan{}).Where("status_tagihan IN ?", []string{"belum_lunas", "tertunggak"}).Count(&totalBelumLunas)
	var totalLunas int64
	h.db.Model(&models.Tagihan{}).Where("status_tagihan = ?", "lunas").Count(&totalLunas)

	return c.JSON(fiber.Map{"data": fiber.Map{
		"total_tagihan": totalTagihan,
		"assigned":      assigned,
		"unassigned":    unassigned,
		"total_import":  importTotal,
		"belum_lunas":   totalBelumLunas,
		"lunas":         totalLunas,
	}})
}

func renderTagihanTemplateMessage(template string, item models.Tagihan) string {
	tunggakanText := "-"
	if item.Tunggakan != nil && strings.TrimSpace(*item.Tunggakan) != "" {
		tunggakanText = strings.TrimSpace(*item.Tunggakan)
	}

	totalTagihanText := fmt.Sprintf("Rp %d", item.TotalTagihan)
	if item.TotalTagihan == 0 && tunggakanText != "-" {
		totalTagihanText = tunggakanText
	} else if item.Tunggakan != nil && strings.TrimSpace(*item.Tunggakan) != "" {
		totalTagihanText = tunggakanText
	}

	sumberText := "-"
	if item.SumberData != nil && strings.TrimSpace(*item.SumberData) != "" {
		sumberText = strings.TrimSpace(*item.SumberData)
	}

	replacer := strings.NewReplacer(
		"{nama}", item.Nama,
		"{no_whatsapp}", item.NoWhatsapp,
		"{sumber_data}", sumberText,
		"{total_tagihan}", totalTagihanText,
		"{total_tunggakan}", totalTagihanText,
		"{tunggakan}", tunggakanText,
		"{status_tagihan}", item.StatusTagihan,
		"{status_kontak}", item.StatusTagihan,
		"{ttl}", valueOrDash(item.TTL),
		"{alamat}", valueOrDash(item.Alamat),
		"{nama_ayah}", valueOrDash(item.NamaAyah),
		"{nama_ibu}", valueOrDash(item.NamaIbu),
		"{asal_sekolah}", valueOrDash(item.AsalSekolah),
		"{jenis_kelamin}", valueOrDash(item.JenisKelamin),
		"{jenjang_pendidikan}", valueOrDash(item.JenjangPendidikan),
	)
	msg := replacer.Replace(template)
	if item.NIS != nil {
		msg = strings.ReplaceAll(msg, "{nis}", *item.NIS)
	} else {
		msg = strings.ReplaceAll(msg, "{nis}", "-")
	}
	return msg
}

func fetchTemplateByID(db *gorm.DB, templateID uint64) (models.TemplatePesan, error) {
	var tmpl models.TemplatePesan
	if err := db.First(&tmpl, templateID).Error; err != nil {
		return models.TemplatePesan{}, err
	}
	return tmpl, nil
}

func (h *ImportExcelHandler) DownloadTagihanTemplate(c *fiber.Ctx) error {
	f := excelize.NewFile()
	sheet := "Template Import Tagihan"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"nama", "ttl", "alamat", "nama_ayah", "nama_ibu", "no_whatsapp", "asal_sekolah", "jenis_kelamin", "jenjang_pendidikan", "tunggakan", "nis", "status_tagihan", "catatan", "sumber_data"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}
	sample := []string{"Ahmad Fulan", "Banyumas, 15-06-2010", "Jl. Mawar No. 1", "H. Fulan", "Siti Aminah", "081234567890", "SMPN 1 Banyumas", "Laki-laki", "SMP", "1250000", "240001", "belum_lunas", "Tagihan umum bulan ini", "excel"}
	for i, value := range sample {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet, cell, value)
	}
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=\"template_import_tagihan.xlsx\"")
	return f.Write(c.Response().BodyWriter())
}

func (h *ImportExcelHandler) ImportTagihan(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader == nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "File Excel wajib diunggah"})
	}
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Gagal membuka file upload"})
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Gagal membaca file upload"})
	}
	xlsx, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Format file tidak valid. Gunakan .xlsx"})
	}
	defer xlsx.Close()

	sheetName := xlsx.GetSheetName(0)
	if sheetName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Sheet Excel tidak ditemukan"})
	}
	rows, err := xlsx.GetRows(sheetName)
	if err != nil || len(rows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Data Excel kosong"})
	}

	userID, _ := c.Locals("userID").(uint)
	batch := models.ImportBatch{Filename: fileHeader.Filename}
	if userID != 0 {
		batch.ImportedByID = &userID
	}
	if err := h.db.Create(&batch).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal membuat import batch"})
	}

	headers := mapExcelHeaders(rows[0])
	inserted, updated, skipped := 0, 0, 0
	for i := 1; i < len(rows); i++ {
		vals := rows[i]
		if isExcelRowEmpty(vals) {
			continue
		}
		nama := readExcelCell(vals, headers, "nama")
		if strings.TrimSpace(nama) == "" {
			skipped++
			continue
		}
		wa := utils.NormalizeWhatsAppNumber(readExcelCell(vals, headers, "no_whatsapp"))
		if strings.TrimSpace(wa) == "" {
			skipped++
			continue
		}
		statusTagihan := strings.TrimSpace(readExcelCell(vals, headers, "status_tagihan"))
		if statusTagihan == "" {
			statusTagihan = "belum_lunas"
		}
		nisVal := strings.TrimSpace(readExcelCell(vals, headers, "nis"))
		var nis *string
		if nisVal != "" {
			nis = &nisVal
		}
		ttl := nullableString(readExcelCell(vals, headers, "ttl"))
		alamat := nullableString(readExcelCell(vals, headers, "alamat"))
		namaAyah := nullableString(readExcelCell(vals, headers, "nama_ayah"))
		namaIbu := nullableString(readExcelCell(vals, headers, "nama_ibu"))
		asalSekolah := nullableString(readExcelCell(vals, headers, "asal_sekolah"))
		jenisKelamin := nullableString(readExcelCell(vals, headers, "jenis_kelamin"))
		jenjangPendidikan := nullableString(readExcelCell(vals, headers, "jenjang_pendidikan"))
		tunggakanValue := nullableString(readExcelCell(vals, headers, "tunggakan"))
		if tunggakanValue == nil {
			tunggakanValue = nullableString(readExcelCell(vals, headers, "total_tagihan"))
		}
		totalTagihan := normalizeCurrencyValue(readExcelCell(vals, headers, "total_tagihan"))
		if totalTagihan == 0 && tunggakanValue != nil {
			totalTagihan = normalizeCurrencyValue(*tunggakanValue)
		}
		sumberData := nullableString(readExcelCell(vals, headers, "sumber_data"))
		catatan := nullableString(readExcelCell(vals, headers, "catatan"))

		if nis != nil {
			var existing models.Tagihan
			if err := h.db.Where("nis = ?", *nis).First(&existing).Error; err == nil {
				existing.Nama = nama
				existing.TTL = ttl
				existing.Alamat = alamat
				existing.NamaAyah = namaAyah
				existing.NamaIbu = namaIbu
				existing.NoWhatsapp = wa
				existing.AsalSekolah = asalSekolah
				existing.JenisKelamin = jenisKelamin
				existing.JenjangPendidikan = jenjangPendidikan
				existing.Tunggakan = tunggakanValue
				existing.TotalTagihan = totalTagihan
				existing.StatusTagihan = statusTagihan
				existing.SumberData = sumberData
				existing.Catatan = catatan
				existing.ImportBatchID = &batch.ID
				if saveErr := h.db.Save(&existing).Error; saveErr == nil {
					updated++
				} else {
					skipped++
				}
				continue
			}
		}

		item := models.Tagihan{
			NIS:               nis,
			Nama:              nama,
			TTL:               ttl,
			Alamat:            alamat,
			NamaAyah:          namaAyah,
			NamaIbu:           namaIbu,
			NoWhatsapp:        wa,
			AsalSekolah:       asalSekolah,
			JenisKelamin:      jenisKelamin,
			JenjangPendidikan: jenjangPendidikan,
			Tunggakan:         tunggakanValue,
			TotalTagihan:      totalTagihan,
			StatusTagihan:     statusTagihan,
			SumberData:        sumberData,
			Catatan:           catatan,
			ImportBatchID:     &batch.ID,
		}
		if err := h.db.Create(&item).Error; err != nil {
			skipped++
			continue
		}
		inserted++
	}

	batch.TotalRows = maxInt(len(rows)-1, 0)
	batch.InsertedRows = inserted
	batch.UpdatedRows = updated
	batch.SkippedRows = skipped
	_ = h.db.Save(&batch).Error

	return c.JSON(fiber.Map{"message": "Import tagihan selesai", "data": fiber.Map{
		"batch_id":      batch.ID,
		"filename":      batch.Filename,
		"total_rows":    batch.TotalRows,
		"inserted_rows": inserted,
		"updated_rows":  updated,
		"skipped_rows":  skipped,
	}})
}
