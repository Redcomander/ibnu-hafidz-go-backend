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
	"strings"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/nfnt/resize"
	"gorm.io/gorm"
)

type PrestasiHandler struct {
	db         *gorm.DB
	uploadPath string
}

func NewPrestasiHandler(db *gorm.DB, uploadPath string) *PrestasiHandler {
	return &PrestasiHandler{db: db, uploadPath: uploadPath}
}

// ────────────────── List ──────────────────

func (h *PrestasiHandler) ListPrestasi(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 12)
	search := c.Query("search")
	kategori := c.Query("kategori")
	tingkat := c.Query("tingkat")

	query := h.db.Model(&models.Prestasi{}).Preload("Student").Preload("User")

	if search != "" {
		query = query.Where("nama_prestasi LIKE ? OR deskripsi LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if kategori != "" {
		query = query.Where("kategori = ?", kategori)
	}
	if tingkat != "" {
		query = query.Where("tingkat = ?", tingkat)
	}

	var total int64
	query.Count(&total)

	var prestasis []models.Prestasi
	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Find(&prestasis)

	var categories []string
	h.db.Model(&models.Prestasi{}).Distinct("kategori").Where("kategori != ''").Pluck("kategori", &categories)
	var levels []string
	h.db.Model(&models.Prestasi{}).Where("tingkat IS NOT NULL AND tingkat != ''").Distinct("tingkat").Pluck("tingkat", &levels)

	return c.JSON(fiber.Map{
		"data":        prestasis,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
		"categories":  categories,
		"levels":      levels,
	})
}

// ────────────────── Create ──────────────────

func (h *PrestasiHandler) CreatePrestasi(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	nama := c.FormValue("nama_prestasi")
	kategori := c.FormValue("kategori")

	if nama == "" || kategori == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "validation_error", Message: "Nama prestasi dan kategori wajib diisi",
		})
	}

	prestasi := models.Prestasi{
		NamaPrestasi: nama,
		Kategori:     kategori,
		UserID:       userID,
	}

	if v := c.FormValue("tingkat"); v != "" {
		prestasi.Tingkat = &v
	}
	if v := c.FormValue("tanggal"); v != "" {
		prestasi.Tanggal = &v
	}
	if v := c.FormValue("deskripsi"); v != "" {
		prestasi.Deskripsi = &v
	}
	if sid := c.FormValue("student_id"); sid != "" {
		var student models.Student
		if h.db.First(&student, sid).RowsAffected > 0 {
			prestasi.StudentID = &student.ID
		}
	}

	// Handle photo upload → compress + WebP
	file, err := c.FormFile("photo")
	if err == nil && file != nil {
		photoPath, webpPath, pErr := h.processImage(file)
		if pErr == nil {
			prestasi.PhotoPath = &photoPath
			if webpPath != "" {
				prestasi.WebpPath = &webpPath
			}
		}
	}

	if err := h.db.Create(&prestasi).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}

	h.db.Preload("Student").Preload("User").First(&prestasi, prestasi.ID)
	return c.Status(fiber.StatusCreated).JSON(prestasi)
}

// ────────────────── Update ──────────────────

func (h *PrestasiHandler) UpdatePrestasi(c *fiber.Ctx) error {
	id := c.Params("id")
	var prestasi models.Prestasi
	if err := h.db.First(&prestasi, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Prestasi not found"})
	}

	if v := c.FormValue("nama_prestasi"); v != "" {
		prestasi.NamaPrestasi = v
	}
	if v := c.FormValue("kategori"); v != "" {
		prestasi.Kategori = v
	}

	tingkat := c.FormValue("tingkat")
	if tingkat != "" {
		prestasi.Tingkat = &tingkat
	} else {
		prestasi.Tingkat = nil
	}
	tanggal := c.FormValue("tanggal")
	if tanggal != "" {
		prestasi.Tanggal = &tanggal
	} else {
		prestasi.Tanggal = nil
	}
	deskripsi := c.FormValue("deskripsi")
	if deskripsi != "" {
		prestasi.Deskripsi = &deskripsi
	} else {
		prestasi.Deskripsi = nil
	}

	sid := c.FormValue("student_id")
	if sid != "" {
		var student models.Student
		if h.db.First(&student, sid).RowsAffected > 0 {
			prestasi.StudentID = &student.ID
		}
	} else {
		prestasi.StudentID = nil
	}

	// Handle photo upload
	file, err := c.FormFile("photo")
	if err == nil && file != nil {
		h.deletePhotoFiles(&prestasi)
		photoPath, webpPath, pErr := h.processImage(file)
		if pErr == nil {
			prestasi.PhotoPath = &photoPath
			if webpPath != "" {
				prestasi.WebpPath = &webpPath
			} else {
				prestasi.WebpPath = nil
			}
		}
	}

	h.db.Save(&prestasi)
	h.db.Preload("Student").Preload("User").First(&prestasi, prestasi.ID)
	return c.JSON(prestasi)
}

// ────────────────── Delete ──────────────────

func (h *PrestasiHandler) DeletePrestasi(c *fiber.Ctx) error {
	id := c.Params("id")
	var prestasi models.Prestasi
	if err := h.db.First(&prestasi, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Prestasi not found"})
	}

	h.deletePhotoFiles(&prestasi)
	h.db.Delete(&prestasi)
	return c.JSON(fiber.Map{"message": "Prestasi deleted"})
}

// ────────────────── Image Processing ──────────────────

func (h *PrestasiHandler) processImage(fh *multipart.FileHeader) (string, string, error) {
	src, err := fh.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	imgBytes, err := io.ReadAll(src)
	if err != nil {
		return "", "", err
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	var img image.Image

	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(bytes.NewReader(imgBytes))
	case ".png":
		img, err = png.Decode(bytes.NewReader(imgBytes))
	default:
		return h.saveRawFile(imgBytes, ext)
	}
	if err != nil {
		return h.saveRawFile(imgBytes, ext)
	}

	// Resize if wider than 1200px
	if img.Bounds().Dx() > 1200 {
		img = resize.Resize(1200, 0, img, resize.Lanczos3)
	}

	timestamp := time.Now().UnixNano()
	destDir := filepath.Join(h.uploadPath, "prestasi")
	os.MkdirAll(destDir, 0755)

	// Save compressed JPEG
	jpegName := fmt.Sprintf("prestasi_%d.jpg", timestamp)
	jpegFullPath := filepath.Join(destDir, jpegName)
	jf, err := os.Create(jpegFullPath)
	if err != nil {
		return "", "", err
	}
	if err := jpeg.Encode(jf, img, &jpeg.Options{Quality: 80}); err != nil {
		jf.Close()
		return "", "", err
	}
	jf.Close()

	// Save WebP using nativewebp (pure Go, no CGo)
	webpName := fmt.Sprintf("prestasi_%d.webp", timestamp)
	webpFullPath := filepath.Join(destDir, webpName)
	wf, err := os.Create(webpFullPath)
	if err != nil {
		// Still return jpeg if webp creation fails
		return "prestasi/" + jpegName, "", nil
	}
	if err := nativewebp.Encode(wf, img, nil); err != nil {
		wf.Close()
		os.Remove(webpFullPath)
		return "prestasi/" + jpegName, "", nil
	}
	wf.Close()

	return "prestasi/" + jpegName, "prestasi/" + webpName, nil
}

func (h *PrestasiHandler) saveRawFile(data []byte, ext string) (string, string, error) {
	timestamp := time.Now().UnixNano()
	destDir := filepath.Join(h.uploadPath, "prestasi")
	os.MkdirAll(destDir, 0755)
	name := fmt.Sprintf("prestasi_%d%s", timestamp, ext)
	if err := os.WriteFile(filepath.Join(destDir, name), data, 0644); err != nil {
		return "", "", err
	}
	return "prestasi/" + name, "", nil
}

func (h *PrestasiHandler) deletePhotoFiles(p *models.Prestasi) {
	if p.PhotoPath != nil {
		os.Remove(filepath.Join(h.uploadPath, *p.PhotoPath))
	}
	if p.WebpPath != nil {
		os.Remove(filepath.Join(h.uploadPath, *p.WebpPath))
	}
}
