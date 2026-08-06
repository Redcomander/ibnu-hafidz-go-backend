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

type KontakHandler struct {
	db *gorm.DB
}

type TemplatePesanHandler struct {
	db *gorm.DB
}

type ImportExcelHandler struct {
	db *gorm.DB
}

type KontakDashboardHandler struct {
	db *gorm.DB
}

func NewKontakHandler(db *gorm.DB) *KontakHandler {
	return &KontakHandler{db: db}
}

func NewTemplatePesanHandler(db *gorm.DB) *TemplatePesanHandler {
	return &TemplatePesanHandler{db: db}
}

func NewImportExcelHandler(db *gorm.DB) *ImportExcelHandler {
	return &ImportExcelHandler{db: db}
}

func NewKontakDashboardHandler(db *gorm.DB) *KontakDashboardHandler {
	return &KontakDashboardHandler{db: db}
}

func (h *KontakHandler) buildKontakListQuery(c *fiber.Ctx) *gorm.DB {
	query := h.db.Model(&models.Kontak{})

	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		query = query.Where("status_kontak = ?", status)
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
			"LOWER(nama) LIKE ? OR LOWER(nis) LIKE ? OR LOWER(no_whatsapp) LIKE ? OR LOWER(alamat) LIKE ? OR LOWER(alamat_lengkap) LIKE ?",
			term, term, term, term, term,
		)
	}

	sortBy := sanitizeKontakSortField(c.Query("sort", "updated_at"))
	order := strings.ToLower(strings.TrimSpace(c.Query("order", "desc")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	return query.Order(sortBy + " " + order)
}

func sanitizeKontakSortField(field string) string {
	switch strings.TrimSpace(field) {
	case "id", "nama", "nis", "no_whatsapp", "status_kontak", "sumber_data", "last_contact_at", "created_at", "updated_at":
		return strings.TrimSpace(field)
	default:
		return "updated_at"
	}
}

func (h *KontakHandler) List(c *fiber.Ctx) error {
	var items []models.Kontak
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

	query := h.buildKontakListQuery(c)
	query.Count(&total)

	offset := (page - 1) * perPage
	if err := query.
		Preload("Handler", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).
		Limit(perPage).
		Offset(offset).
		Find(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "server_error",
			Message: "Failed to fetch kontak list",
		})
	}

	return c.JSON(BuildPaginatedResponse(items, total, page, perPage))
}

func (h *KontakHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.Kontak{}, id)
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kontak not found"})
	}
	return c.JSON(fiber.Map{"message": "Kontak deleted successfully"})
}

func (h *KontakHandler) BulkDelete(c *fiber.Ctx) error {
	type reqBody struct {
		IDs []uint `json:"ids"`
	}

	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Daftar ID kontak wajib diisi"})
	}

	result := h.db.Where("id IN ?", req.IDs).Delete(&models.Kontak{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to bulk delete kontak"})
	}

	return c.JSON(fiber.Map{
		"message":       "Bulk delete kontak berhasil",
		"deleted_count": result.RowsAffected,
	})
}

func (h *KontakHandler) ExportExcel(c *fiber.Ctx) error {
	query := h.buildKontakListQuery(c)

	var rows []models.Kontak
	if err := query.
		Preload("Handler", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).
		Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to export kontak"})
	}

	f := excelize.NewFile()
	sheet := "Kontak"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No", "NIS", "Nama", "No WhatsApp", "Status Kontak", "Handler", "Sumber Data", "Alamat", "Alamat Lengkap", "Catatan", "Terakhir Dihubungi", "Dibuat"}
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
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowNum), item.StatusKontak)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowNum), userNameOrDash(item.Handler))
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowNum), stringOrDash(item.SumberData))
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowNum), stringOrDash(item.Alamat))
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowNum), stringOrDash(item.AlamatLengkap))
		f.SetCellValue(sheet, fmt.Sprintf("J%d", rowNum), stringOrDash(item.Catatan))
		f.SetCellValue(sheet, fmt.Sprintf("K%d", rowNum), formatDateTimeForExport(item.LastContactAt))
		f.SetCellValue(sheet, fmt.Sprintf("L%d", rowNum), item.CreatedAt.Format("2006-01-02 15:04"))
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	filename := fmt.Sprintf("kontak_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	return f.Write(c.Response().BodyWriter())
}

func (h *KontakHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Kontak

	if err := h.db.Preload("Handler", func(tx *gorm.DB) *gorm.DB {
		return tx.Select("id", "name", "email")
	}).First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "not_found",
			Message: "Kontak not found",
		})
	}

	return c.JSON(item)
}

func (h *KontakHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Kontak

	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kontak not found"})
	}

	type request struct {
		NIS           *string `json:"nis"`
		Nama          string  `json:"nama"`
		NoWhatsapp    string  `json:"no_whatsapp"`
		Alamat        *string `json:"alamat"`
		AlamatLengkap *string `json:"alamat_lengkap"`
		StatusKontak  string  `json:"status_kontak"`
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
		nisTrimmed := strings.TrimSpace(*req.NIS)
		if nisTrimmed == "" {
			req.NIS = nil
		} else {
			req.NIS = &nisTrimmed
		}
	}

	statusBaru := strings.TrimSpace(req.StatusKontak)
	if statusBaru == "" {
		statusBaru = item.StatusKontak
	}

	oldStatus := item.StatusKontak
	item.NIS = req.NIS
	item.Nama = strings.TrimSpace(req.Nama)
	item.NoWhatsapp = normalized
	item.Alamat = req.Alamat
	item.AlamatLengkap = req.AlamatLengkap
	item.StatusKontak = statusBaru
	item.HandlerID = req.HandlerID
	item.SumberData = req.SumberData
	item.Catatan = req.Catatan

	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update kontak"})
	}

	if oldStatus != item.StatusKontak {
		userID, _ := c.Locals("userID").(uint)
		statusBefore := oldStatus
		statusAfter := item.StatusKontak
		entry := models.RiwayatKontak{
			KontakID:    item.ID,
			StatusAwal:  &statusBefore,
			StatusAkhir: &statusAfter,
			Catatan:     req.Catatan,
		}
		if userID != 0 {
			entry.UserID = &userID
		}
		_ = h.db.Create(&entry).Error
	}

	h.db.Preload("Handler", func(tx *gorm.DB) *gorm.DB {
		return tx.Select("id", "name", "email")
	}).First(&item, item.ID)

	return c.JSON(item)
}

func (h *KontakHandler) UpdateStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Kontak

	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kontak not found"})
	}

	type reqBody struct {
		StatusKontak string  `json:"status_kontak"`
		Catatan      *string `json:"catatan"`
	}

	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}

	newStatus := strings.TrimSpace(req.StatusKontak)
	if newStatus == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Status kontak wajib diisi"})
	}

	oldStatus := item.StatusKontak
	item.StatusKontak = newStatus
	now := time.Now()
	item.LastContactAt = &now

	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update status kontak"})
	}

	userID, _ := c.Locals("userID").(uint)
	entry := models.RiwayatKontak{KontakID: item.ID, StatusAwal: &oldStatus, StatusAkhir: &newStatus, Catatan: req.Catatan}
	if userID != 0 {
		entry.UserID = &userID
	}
	_ = h.db.Create(&entry).Error

	return c.JSON(fiber.Map{"message": "Status kontak berhasil diperbarui", "data": item})
}

func (h *KontakHandler) WhatsAppLink(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Kontak

	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Kontak not found"})
	}

	templateIDParam := strings.TrimSpace(c.Query("template_id"))
	customMessage := strings.TrimSpace(c.Query("message"))
	message := customMessage
	var templateID *uint

	if templateIDParam != "" {
		tid64, err := strconv.ParseUint(templateIDParam, 10, 64)
		if err == nil {
			tid := uint(tid64)
			templateID = &tid

			var tmpl models.TemplatePesan
			if tErr := h.db.First(&tmpl, tid).Error; tErr == nil {
				message = renderTemplateMessage(tmpl.Konten, item)
			}
		}
	}

	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("Assalamu'alaikum, kami dari tim PPDB ingin menindaklanjuti data %s.", item.Nama)
	}

	normalized := utils.NormalizeWhatsAppNumber(item.NoWhatsapp)
	waURL := "https://wa.me/" + normalized + "?text=" + url.QueryEscape(message)

	if c.Query("log") == "1" {
		now := time.Now()
		item.LastContactAt = &now
		_ = h.db.Save(&item).Error

		userID, _ := c.Locals("userID").(uint)
		entry := models.RiwayatKontak{
			KontakID:        item.ID,
			TemplatePesanID: templateID,
			PesanFinal:      &message,
			DikirimVia:      strPtr("whatsapp"),
		}
		if userID != 0 {
			entry.UserID = &userID
		}
		_ = h.db.Create(&entry).Error
	}

	return c.JSON(fiber.Map{
		"url":             waURL,
		"nomor_normalize": normalized,
		"message":         message,
	})
}

func (h *KontakHandler) Riwayat(c *fiber.Ctx) error {
	kontakID := c.Params("kontak_id")
	var rows []models.RiwayatKontak

	if err := h.db.
		Preload("User", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "name", "email") }).
		Preload("TemplatePesan").
		Where("kontak_id = ?", kontakID).
		Order("created_at DESC").
		Limit(300).
		Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to load riwayat"})
	}

	return c.JSON(fiber.Map{"data": rows})
}

func (h *TemplatePesanHandler) List(c *fiber.Ctx) error {
	var rows []models.TemplatePesan
	query := h.db.Model(&models.TemplatePesan{})

	if active := c.Query("aktif"); active != "" {
		if active == "1" || strings.EqualFold(active, "true") {
			query = query.Where("aktif = ?", true)
		} else if active == "0" || strings.EqualFold(active, "false") {
			query = query.Where("aktif = ?", false)
		}
	}

	if err := query.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to load template"})
	}

	return c.JSON(fiber.Map{"data": rows})
}

func (h *TemplatePesanHandler) Create(c *fiber.Ctx) error {
	type reqBody struct {
		Nama   string `json:"nama"`
		Konten string `json:"konten"`
		Aktif  *bool  `json:"aktif"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	if strings.TrimSpace(req.Nama) == "" || strings.TrimSpace(req.Konten) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Nama dan konten template wajib diisi"})
	}

	item := models.TemplatePesan{Nama: strings.TrimSpace(req.Nama), Konten: strings.TrimSpace(req.Konten), Aktif: true}
	if req.Aktif != nil {
		item.Aktif = *req.Aktif
	}

	if err := h.db.Create(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to create template"})
	}

	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *TemplatePesanHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.TemplatePesan
	if err := h.db.First(&item, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Template not found"})
	}

	type reqBody struct {
		Nama   string `json:"nama"`
		Konten string `json:"konten"`
		Aktif  *bool  `json:"aktif"`
	}
	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}

	if strings.TrimSpace(req.Nama) != "" {
		item.Nama = strings.TrimSpace(req.Nama)
	}
	if strings.TrimSpace(req.Konten) != "" {
		item.Konten = strings.TrimSpace(req.Konten)
	}
	if req.Aktif != nil {
		item.Aktif = *req.Aktif
	}

	if err := h.db.Save(&item).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Failed to update template"})
	}
	return c.JSON(item)
}

func (h *TemplatePesanHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	result := h.db.Delete(&models.TemplatePesan{}, id)
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Template not found"})
	}
	return c.JSON(fiber.Map{"message": "Template deleted successfully"})
}

func (h *ImportExcelHandler) DownloadTemplate(c *fiber.Ctx) error {
	f := excelize.NewFile()
	sheet := "Template Import Kontak"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"nis", "nama", "no_whatsapp", "status_kontak", "sumber_data", "alamat", "alamat_lengkap", "catatan"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, header)
	}

	sample := []string{"240001", "Ahmad Fulan", "081234567890", "baru", "excel", "Jl. Mawar No. 1", "Jl. Mawar No. 1, RT 01 RW 02, Kota", "Calon santri gelombang 1"}
	for i, value := range sample {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(sheet, cell, value)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=\"template_import_kontak.xlsx\"")

	return f.Write(c.Response().BodyWriter())
}

func (h *ImportExcelHandler) ImportKontak(c *fiber.Ctx) error {
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
	inserted := 0
	updated := 0
	skipped := 0

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

		nisVal := strings.TrimSpace(readExcelCell(vals, headers, "nis"))
		var nis *string
		if nisVal != "" {
			nis = &nisVal
		}
		alamat := nullableString(readExcelCell(vals, headers, "alamat"))
		alamatLengkap := nullableString(readExcelCell(vals, headers, "alamat_lengkap"))
		statusKontak := strings.TrimSpace(readExcelCell(vals, headers, "status_kontak"))
		if statusKontak == "" {
			statusKontak = "baru"
		}
		sumberData := nullableString(readExcelCell(vals, headers, "sumber_data"))
		catatan := nullableString(readExcelCell(vals, headers, "catatan"))

		if nis != nil {
			var existing models.Kontak
			err := h.db.Where("nis = ?", *nis).First(&existing).Error
			if err == nil {
				existing.Nama = nama
				existing.NoWhatsapp = wa
				existing.Alamat = alamat
				existing.AlamatLengkap = alamatLengkap
				existing.StatusKontak = statusKontak
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

		item := models.Kontak{
			NIS:           nis,
			Nama:          nama,
			NoWhatsapp:    wa,
			Alamat:        alamat,
			AlamatLengkap: alamatLengkap,
			StatusKontak:  statusKontak,
			SumberData:    sumberData,
			Catatan:       catatan,
			ImportBatchID: &batch.ID,
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

	return c.JSON(fiber.Map{
		"message": "Import kontak selesai",
		"data": fiber.Map{
			"batch_id":      batch.ID,
			"filename":      batch.Filename,
			"total_rows":    batch.TotalRows,
			"inserted_rows": inserted,
			"updated_rows":  updated,
			"skipped_rows":  skipped,
		},
	})
}

func (h *KontakDashboardHandler) Summary(c *fiber.Ctx) error {
	var totalKontak int64
	h.db.Model(&models.Kontak{}).Count(&totalKontak)

	type statusRow struct {
		Status string `json:"status"`
		Total  int64  `json:"total"`
	}
	var rows []statusRow
	h.db.Model(&models.Kontak{}).
		Select("status_kontak as status, count(*) as total").
		Group("status_kontak").
		Order("total DESC").
		Scan(&rows)

	var assigned int64
	h.db.Model(&models.Kontak{}).Where("handler_id IS NOT NULL").Count(&assigned)

	var unassigned int64
	h.db.Model(&models.Kontak{}).Where("handler_id IS NULL").Count(&unassigned)

	var importTotal int64
	h.db.Model(&models.ImportBatch{}).Count(&importTotal)

	var riwayatTotal int64
	h.db.Model(&models.RiwayatKontak{}).Count(&riwayatTotal)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"total_kontak":     totalKontak,
			"assigned":         assigned,
			"unassigned":       unassigned,
			"total_import":     importTotal,
			"total_riwayat":    riwayatTotal,
			"status_breakdown": rows,
		},
	})
}

func renderTemplateMessage(template string, kontak models.Kontak) string {
	replacer := strings.NewReplacer(
		"{nama}", kontak.Nama,
		"{no_whatsapp}", kontak.NoWhatsapp,
	)
	msg := replacer.Replace(template)
	if kontak.NIS != nil {
		msg = strings.ReplaceAll(msg, "{nis}", *kontak.NIS)
	} else {
		msg = strings.ReplaceAll(msg, "{nis}", "-")
	}
	return msg
}

func strPtr(v string) *string {
	vv := v
	return &vv
}

func nullableString(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func readExcelCell(row []string, headerMap map[string]int, key string) string {
	idx, ok := headerMap[key]
	if !ok || idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func mapExcelHeaders(headerRow []string) map[string]int {
	result := map[string]int{}
	for i, h := range headerRow {
		n := normalizeHeader(h)
		switch n {
		case "nis":
			result["nis"] = i
		case "nama", "nama_santri", "nama_lengkap":
			result["nama"] = i
		case "whatsapp", "no_whatsapp", "no_wa", "nomor_whatsapp", "nomor_wa", "telepon", "phone":
			result["no_whatsapp"] = i
		case "alamat":
			result["alamat"] = i
		case "alamat_lengkap", "alamatlengkap":
			result["alamat_lengkap"] = i
		case "status", "status_kontak":
			result["status_kontak"] = i
		case "sumber", "sumber_data":
			result["sumber_data"] = i
		case "catatan", "keterangan", "notes":
			result["catatan"] = i
		}
	}
	return result
}

func normalizeHeader(v string) string {
	out := strings.ToLower(strings.TrimSpace(v))
	out = strings.ReplaceAll(out, " ", "_")
	out = strings.ReplaceAll(out, "-", "_")
	return out
}

func isExcelRowEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func stringOrDash(v *string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return "-"
	}
	return strings.TrimSpace(*v)
}

func userNameOrDash(user *models.User) string {
	if user == nil || strings.TrimSpace(user.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(user.Name)
}

func formatDateTimeForExport(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}
