package handlers

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	_ "golang.org/x/image/webp"
)

const maxArticleThumbnail = 5 * 1024 * 1024

var ownedThumbnailName = regexp.MustCompile(`^thumb_[A-Za-z0-9_-]+\.(jpg|jpeg|png|webp)$`)

func localArticleImage(value string) (string, bool) {
	if strings.ContainsAny(value, "\\\x00") || strings.Contains(value, ":") {
		return "", false
	}
	value = strings.TrimPrefix(strings.TrimPrefix(value, "/uploads/"), "uploads/")
	if strings.HasPrefix(value, "/") || value == "" || path.Clean(value) != value {
		return "", false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "." {
			return "", false
		}
	}
	switch strings.ToLower(path.Ext(value)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return value, true
	}
	return "", false
}

func (h *ArticleHandler) articleThumbnail(c *fiber.Ctx, existing *string) (*string, error) {
	form, err := c.MultipartForm()
	if err != nil || form == nil {
		return nil, fiber.NewError(400, "Invalid multipart form")
	}
	files := form.File["thumbnail"]
	if len(files) > 0 {
		file := files[0]
		if file.Size <= 0 || file.Size > maxArticleThumbnail {
			return nil, fiber.NewError(400, "Thumbnail must be at most 5 MB")
		}
		src, err := file.Open()
		if err != nil {
			return nil, fiber.NewError(400, "Cannot read thumbnail")
		}
		defer src.Close()
		data, err := io.ReadAll(io.LimitReader(src, maxArticleThumbnail+1))
		if err != nil || len(data) > maxArticleThumbnail {
			return nil, fiber.NewError(400, "Invalid thumbnail")
		}
		ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(data)]
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if ext == "" || err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
			return nil, fiber.NewError(400, "Thumbnail must be a JPEG, PNG, or WebP image up to 20 megapixels")
		}
		if err := os.MkdirAll(h.uploadPath, 0755); err != nil {
			return nil, fiber.NewError(500, "Cannot prepare uploads")
		}
		root, err := os.OpenRoot(h.uploadPath)
		if err != nil {
			return nil, fiber.NewError(500, "Cannot open uploads")
		}
		defer root.Close()
		if err := root.Mkdir("thumbnails", 0755); err != nil && !os.IsExist(err) {
			return nil, fiber.NewError(500, "Cannot prepare thumbnails")
		}
		random, err := newRefreshToken()
		if err != nil {
			return nil, fiber.NewError(500, "Cannot generate thumbnail name")
		}
		relative := "thumbnails/thumb_" + random + ext
		dst, err := root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return nil, fiber.NewError(500, "Cannot save thumbnail")
		}
		_, writeErr := dst.Write(data)
		closeErr := dst.Close()
		if writeErr != nil || closeErr != nil {
			_ = root.Remove(relative)
			return nil, fiber.NewError(500, "Cannot save thumbnail")
		}
		return &relative, nil
	}
	galleryID := c.FormValue("thumbnail_gallery_id")
	legacyPath := c.FormValue("thumbnail_from_gallery")
	if galleryID != "" || legacyPath != "" {
		var gallery models.Gallery
		query := h.db.Where("type = ? AND source = ?", "photo", "upload")
		if galleryID != "" {
			query = query.Where(primaryKey(galleryID))
		} else {
			if _, ok := localArticleImage(legacyPath); !ok {
				return nil, fiber.NewError(400, "Invalid gallery thumbnail")
			}
			query = query.Where("path = ? OR webp_path = ?", legacyPath, legacyPath)
		}
		if err := query.First(&gallery).Error; err != nil {
			return nil, fiber.NewError(400, "Gallery thumbnail not found")
		}
		value, ok := localArticleImage(gallery.Path)
		if !ok {
			return nil, fiber.NewError(400, "Invalid gallery thumbnail")
		}
		return &value, nil
	}
	return existing, nil
}

// Gallery assets are shared: only generated, article-owned thumbnails may be
// removed. os.Root also blocks symlink escapes from the configured upload root.
func (h *ArticleHandler) removeArticleThumbnail(value *string) {
	if value == nil || path.Dir(*value) != "thumbnails" || !ownedThumbnailName.MatchString(path.Base(*value)) {
		return
	}
	root, err := os.OpenRoot(filepath.Clean(h.uploadPath))
	if err != nil {
		return
	}
	defer root.Close()
	_ = root.Remove(*value)
}
