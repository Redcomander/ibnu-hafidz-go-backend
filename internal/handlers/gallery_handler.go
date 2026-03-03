package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gosimple/slug"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type GalleryHandler struct {
	db         *gorm.DB
	uploadPath string
}

func NewGalleryHandler(db *gorm.DB, uploadPath string) *GalleryHandler {
	return &GalleryHandler{db: db, uploadPath: uploadPath}
}

// ────────────────── Albums ──────────────────

func (h *GalleryHandler) ListAlbums(c *fiber.Ctx) error {
	var albums []models.Album
	h.db.Order("created_at desc").Find(&albums)

	type AlbumResponse struct {
		models.Album
		GalleryCount int64 `json:"gallery_count"`
	}
	var response []AlbumResponse
	for _, a := range albums {
		var gc int64
		h.db.Model(&models.Gallery{}).Where("album_id = ?", a.ID).Count(&gc)
		response = append(response, AlbumResponse{Album: a, GalleryCount: gc})
	}

	return c.JSON(fiber.Map{"data": response})
}

func (h *GalleryHandler) GetAlbum(c *fiber.Ctx) error {
	id := c.Params("id")
	var album models.Album
	if err := h.db.First(&album, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Album not found"})
	}

	// Get galleries in this album
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	var total int64
	h.db.Model(&models.Gallery{}).Where("album_id = ?", album.ID).Count(&total)

	var galleries []models.Gallery
	offset := (page - 1) * perPage
	h.db.Where("album_id = ?", album.ID).Order("created_at desc").Limit(perPage).Offset(offset).Find(&galleries)

	return c.JSON(fiber.Map{
		"album":       album,
		"galleries":   galleries,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

func (h *GalleryHandler) CreateAlbum(c *fiber.Ctx) error {
	title := c.FormValue("title")
	description := c.FormValue("description")

	if title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Title is required"})
	}

	album := models.Album{
		Title:       title,
		Slug:        slug.Make(title),
		Description: description,
	}

	// Handle cover image upload
	if file, err := c.FormFile("cover_image"); err == nil && file != nil {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := fmt.Sprintf("cover_%d%s", time.Now().UnixNano(), ext)
		destDir := filepath.Join(h.uploadPath, "albums/covers")
		os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, filename)

		src, err := file.Open()
		if err == nil {
			defer src.Close()
			dst, err := os.Create(destPath)
			if err == nil {
				io.Copy(dst, src)
				dst.Close()
				coverPath := "albums/covers/" + filename
				album.CoverImage = &coverPath
			}
		}
	}

	// Handle duplicate slugs
	origSlug := album.Slug
	count := 1
	for {
		var existing models.Album
		if h.db.Unscoped().Where("slug = ?", album.Slug).First(&existing).RowsAffected == 0 {
			break
		}
		album.Slug = fmt.Sprintf("%s-%d", origSlug, count)
		count++
	}

	if err := h.db.Create(&album).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(album)
}

func (h *GalleryHandler) UpdateAlbum(c *fiber.Ctx) error {
	id := c.Params("id")
	var album models.Album
	if err := h.db.First(&album, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Album not found"})
	}

	title := c.FormValue("title")
	description := c.FormValue("description")

	if title != "" {
		album.Title = title
		album.Slug = slug.Make(title)
	}
	album.Description = description

	// Handle cover image upload
	if file, err := c.FormFile("cover_image"); err == nil && file != nil {
		// Delete old cover
		if album.CoverImage != nil {
			os.Remove(filepath.Join(h.uploadPath, *album.CoverImage))
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := fmt.Sprintf("cover_%d%s", time.Now().UnixNano(), ext)
		destDir := filepath.Join(h.uploadPath, "albums/covers")
		os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, filename)

		src, err := file.Open()
		if err == nil {
			defer src.Close()
			dst, err := os.Create(destPath)
			if err == nil {
				io.Copy(dst, src)
				dst.Close()
				coverPath := "albums/covers/" + filename
				album.CoverImage = &coverPath
			}
		}
	}

	h.db.Save(&album)
	return c.JSON(album)
}

func (h *GalleryHandler) DeleteAlbum(c *fiber.Ctx) error {
	id := c.Params("id")
	var album models.Album
	if err := h.db.First(&album, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Album not found"})
	}

	// Delete cover image
	if album.CoverImage != nil {
		os.Remove(filepath.Join(h.uploadPath, *album.CoverImage))
	}

	// Cascade: remove album_id from galleries in this album
	h.db.Model(&models.Gallery{}).Where("album_id = ?", album.ID).Update("album_id", nil)
	h.db.Delete(&album)

	return c.JSON(fiber.Map{"message": "Album deleted successfully"})
}

func (h *GalleryHandler) AddGalleriesToAlbum(c *fiber.Ctx) error {
	id := c.Params("id")
	var album models.Album
	if err := h.db.First(&album, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Album not found"})
	}

	type Input struct {
		GalleryIDs []uint `json:"gallery_ids"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil || len(input.GalleryIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Please provide gallery_ids"})
	}

	h.db.Model(&models.Gallery{}).Where("id IN ?", input.GalleryIDs).Update("album_id", album.ID)

	return c.JSON(fiber.Map{"message": fmt.Sprintf("%d galleries added to album", len(input.GalleryIDs))})
}

func (h *GalleryHandler) RemoveGalleryFromAlbum(c *fiber.Ctx) error {
	albumID := c.Params("id")
	galleryID := c.Params("gallery_id")

	h.db.Model(&models.Gallery{}).Where("id = ? AND album_id = ?", galleryID, albumID).Update("album_id", nil)

	return c.JSON(fiber.Map{"message": "Gallery removed from album"})
}

// ────────────────── Galleries ──────────────────

func (h *GalleryHandler) ListGalleries(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 15)
	albumID := c.Query("album_id")
	mediaType := c.Query("type")
	search := c.Query("search")

	query := h.db.Model(&models.Gallery{}).Preload("Album")

	if albumID != "" {
		query = query.Where("album_id = ?", albumID)
	}
	if mediaType != "" && mediaType != "all" {
		query = query.Where("type = ?", mediaType)
	}
	if search != "" {
		query = query.Where("title LIKE ?", "%"+search+"%")
	}

	var total int64
	query.Count(&total)

	var galleries []models.Gallery
	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Find(&galleries)

	return c.JSON(fiber.Map{
		"data":        galleries,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

func (h *GalleryHandler) UploadGallery(c *fiber.Ctx) error {
	title := c.FormValue("title")
	albumID := c.FormValue("album_id")
	uploadType := c.FormValue("upload_type", "file")
	videoURL := c.FormValue("video_url")

	if title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Title is required"})
	}

	var albumIDPtr *uint
	if albumID != "" {
		var a models.Album
		if h.db.First(&a, albumID).RowsAffected > 0 {
			albumIDPtr = &a.ID
		}
	}

	// Handle social media embeds
	if uploadType == "youtube" || uploadType == "instagram" || uploadType == "tiktok" {
		if videoURL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Video URL is required"})
		}

		gallery := models.Gallery{
			AlbumID:     albumIDPtr,
			Title:       title,
			Type:        "video",
			Path:        uploadType + ":" + videoURL,
			Source:      uploadType,
			OriginalURL: &videoURL,
		}
		h.db.Create(&gallery)

		return c.Status(fiber.StatusCreated).JSON(gallery)
	}

	// Handle URL video
	if uploadType == "url" && videoURL != "" {
		gallery := models.Gallery{
			AlbumID:     albumIDPtr,
			Title:       title,
			Type:        "video",
			Path:        "url:" + videoURL,
			Source:      "url",
			OriginalURL: &videoURL,
		}
		h.db.Create(&gallery)

		return c.Status(fiber.StatusCreated).JSON(gallery)
	}

	// Handle file uploads
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "No files uploaded"})
	}

	files := form.File["media"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Please select at least one file"})
	}

	var created []models.Gallery
	for i, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
		isVideo := ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".wmv" || ext == ".webm"

		if !isImage && !isVideo {
			continue
		}

		mediaType := "photo"
		if isVideo {
			mediaType = "video"
		}

		// Generate unique filename
		timestamp := time.Now().UnixNano()
		filename := fmt.Sprintf("%d_%d%s", timestamp, i, ext)

		var subDir string
		if isImage {
			subDir = "gallery/original"
		} else {
			subDir = "gallery/videos"
		}

		destDir := filepath.Join(h.uploadPath, subDir)
		os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, filename)

		// Save file
		src, err := file.Open()
		if err != nil {
			continue
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			continue
		}
		io.Copy(dst, src)
		dst.Close()

		storagePath := subDir + "/" + filename

		gallery := models.Gallery{
			AlbumID: albumIDPtr,
			Title:   fmt.Sprintf("%s %d", title, i+1),
			Type:    mediaType,
			Path:    storagePath,
			Source:  "upload",
		}
		h.db.Create(&gallery)
		created = append(created, gallery)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": fmt.Sprintf("%d files uploaded successfully", len(created)),
		"data":    created,
	})
}

func (h *GalleryHandler) UpdateGallery(c *fiber.Ctx) error {
	id := c.Params("id")
	var gallery models.Gallery
	if err := h.db.First(&gallery, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Gallery item not found"})
	}

	type Input struct {
		Title   string `json:"title"`
		AlbumID *uint  `json:"album_id"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid input"})
	}

	if input.Title != "" {
		gallery.Title = input.Title
	}
	gallery.AlbumID = input.AlbumID
	h.db.Save(&gallery)

	return c.JSON(gallery)
}

func (h *GalleryHandler) DeleteGallery(c *fiber.Ctx) error {
	id := c.Params("id")
	var gallery models.Gallery
	if err := h.db.First(&gallery, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Gallery item not found"})
	}

	// Delete file from disk if uploaded
	if gallery.Source == "upload" {
		os.Remove(filepath.Join(h.uploadPath, gallery.Path))
		if gallery.WebpPath != nil {
			os.Remove(filepath.Join(h.uploadPath, *gallery.WebpPath))
		}
	}

	h.db.Delete(&gallery)
	return c.JSON(fiber.Map{"message": "Gallery item deleted"})
}

func (h *GalleryHandler) MassDeleteGalleries(c *fiber.Ctx) error {
	type Input struct {
		IDs []uint `json:"ids"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil || len(input.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Please select items to delete"})
	}

	var galleries []models.Gallery
	h.db.Where("id IN ?", input.IDs).Find(&galleries)

	for _, g := range galleries {
		if g.Source == "upload" {
			os.Remove(filepath.Join(h.uploadPath, g.Path))
			if g.WebpPath != nil {
				os.Remove(filepath.Join(h.uploadPath, *g.WebpPath))
			}
		}
	}

	h.db.Where("id IN ?", input.IDs).Delete(&models.Gallery{})

	return c.JSON(fiber.Map{"message": fmt.Sprintf("%d items deleted", len(galleries))})
}
