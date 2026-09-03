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
	"strconv"
	"strings"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/nfnt/resize"
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
