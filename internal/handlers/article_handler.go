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

type ArticleHandler struct {
	db         *gorm.DB
	uploadPath string
}

func NewArticleHandler(db *gorm.DB, uploadPath string) *ArticleHandler {
	return &ArticleHandler{db: db, uploadPath: uploadPath}
}

// ────────────────── Categories ──────────────────

func (h *ArticleHandler) ListCategories(c *fiber.Ctx) error {
	type CategoryWithCount struct {
		models.Category
		ArticlesCount int64 `json:"articles_count"`
	}

	var categories []models.Category
	h.db.Order("name").Find(&categories)

	var result []CategoryWithCount
	for _, cat := range categories {
		var count int64
		h.db.Model(&models.Article{}).Where("category_id = ?", cat.ID).Count(&count)
		result = append(result, CategoryWithCount{Category: cat, ArticlesCount: count})
	}

	return c.JSON(fiber.Map{"data": result})
}

func (h *ArticleHandler) CreateCategory(c *fiber.Ctx) error {
	type Input struct {
		Name string `json:"name"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil || input.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Name is required"})
	}

	cat := models.Category{
		Name: input.Name,
		Slug: slug.Make(input.Name),
	}
	if err := h.db.Create(&cat).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(cat)
}

func (h *ArticleHandler) DeleteCategory(c *fiber.Ctx) error {
	id := c.Params("id")
	var cat models.Category
	if err := h.db.First(&cat, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Category not found"})
	}
	// Nullify articles in this category
	h.db.Model(&models.Article{}).Where("category_id = ?", cat.ID).Update("category_id", nil)
	h.db.Delete(&cat)
	return c.JSON(fiber.Map{"message": "Category deleted"})
}

func (h *ArticleHandler) UpdateCategory(c *fiber.Ctx) error {
	id := c.Params("id")
	var cat models.Category
	if err := h.db.First(&cat, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Category not found"})
	}

	type Input struct {
		Name string `json:"name"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil || input.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Name is required"})
	}

	cat.Name = input.Name
	cat.Slug = slug.Make(input.Name)
	h.db.Save(&cat)
	return c.JSON(cat)
}

// ────────────────── Tags ──────────────────

func (h *ArticleHandler) ListTags(c *fiber.Ctx) error {
	var tags []models.Tag
	h.db.Order("name").Find(&tags)
	return c.JSON(fiber.Map{"data": tags})
}

func (h *ArticleHandler) CreateTag(c *fiber.Ctx) error {
	type Input struct {
		Name string `json:"name"`
	}
	var input Input
	if err := c.BodyParser(&input); err != nil || input.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "Name is required"})
	}

	tag := models.Tag{
		Name: input.Name,
		Slug: slug.Make(input.Name),
	}
	if err := h.db.Create(&tag).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(tag)
}

func (h *ArticleHandler) DeleteTag(c *fiber.Ctx) error {
	id := c.Params("id")
	var tag models.Tag
	if err := h.db.First(&tag, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Tag not found"})
	}
	// Clear article associations
	h.db.Model(&tag).Association("Articles").Clear()
	h.db.Delete(&tag)
	return c.JSON(fiber.Map{"message": "Tag deleted"})
}

// ────────────────── Articles ──────────────────

func (h *ArticleHandler) ListArticles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 10)
	search := c.Query("search")
	status := c.Query("status")
	categoryID := c.Query("category_id")

	query := h.db.Model(&models.Article{}).
		Preload("Category").
		Preload("Author").
		Preload("Tags")

	if search != "" {
		query = query.Where("title LIKE ? OR body LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	var total int64
	query.Count(&total)

	type ArticleListItem struct {
		models.Article
		ViewsCount int64 `json:"views_count"`
	}

	var articles []models.Article
	offset := (page - 1) * perPage
	query.Order("created_at desc").Limit(perPage).Offset(offset).Find(&articles)

	var result []ArticleListItem
	for _, a := range articles {
		var vc int64
		h.db.Model(&models.ArticleView{}).Where("article_id = ?", a.ID).Count(&vc)
		result = append(result, ArticleListItem{Article: a, ViewsCount: vc})
	}

	return c.JSON(fiber.Map{
		"data":        result,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": (total + int64(perPage) - 1) / int64(perPage),
	})
}

func (h *ArticleHandler) GetArticle(c *fiber.Ctx) error {
	slugParam := c.Params("slug")
	var article models.Article

	if err := h.db.
		Preload("Category").
		Preload("Author").
		Preload("Tags").
		Preload("Comments", "parent_id IS NULL AND is_approved = ?", true).
		Preload("Comments.Replies").
		Where("slug = ?", slugParam).
		First(&article).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Article not found"})
	}

	// Record view
	userID := c.Locals("userID")
	var userIDPtr *uint
	if uid, ok := userID.(uint); ok {
		userIDPtr = &uid
	}
	ip := c.IP()
	ua := c.Get("User-Agent")
	h.db.Create(&models.ArticleView{
		ArticleID: article.ID,
		UserID:    userIDPtr,
		IPAddress: ip,
		UserAgent: ua,
	})

	// View counts
	var totalViews int64
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).Count(&totalViews)
	var uniqueViews int64
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).
		Distinct("ip_address").Count(&uniqueViews)

	// Reading time
	wordCount := len(strings.Fields(article.Body))
	readingTime := (wordCount / 200) + 1

	return c.JSON(fiber.Map{
		"article":      article,
		"total_views":  totalViews,
		"unique_views": uniqueViews,
		"reading_time": fmt.Sprintf("%d min read", readingTime),
	})
}

func (h *ArticleHandler) CreateArticle(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)

	title := c.FormValue("title")
	body := c.FormValue("body")
	excerpt := c.FormValue("excerpt")
	categoryID := c.FormValue("category_id")
	status := c.FormValue("status", "draft")
	tagIDs := c.FormValue("tag_ids") // comma-separated

	if title == "" || body == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "validation_error", Message: "Title and body are required",
		})
	}

	articleSlug := slug.Make(title)
	// Handle duplicate slugs
	origSlug := articleSlug
	count := 1
	for {
		var existing models.Article
		if h.db.Where("slug = ?", articleSlug).First(&existing).RowsAffected == 0 {
			break
		}
		articleSlug = fmt.Sprintf("%s-%d", origSlug, count)
		count++
	}

	article := models.Article{
		Title:    title,
		Slug:     articleSlug,
		Body:     body,
		AuthorID: userID,
		Status:   status,
	}

	if excerpt != "" {
		article.Excerpt = &excerpt
	}

	if categoryID != "" {
		var cat models.Category
		if h.db.First(&cat, categoryID).RowsAffected > 0 {
			article.CategoryID = &cat.ID
		}
	}

	if status == "published" {
		now := time.Now()
		article.PublishedAt = &now
	}

	// Handle thumbnail upload
	file, err := c.FormFile("thumbnail")
	if err == nil && file != nil {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := fmt.Sprintf("thumb_%d%s", time.Now().UnixNano(), ext)
		destDir := filepath.Join(h.uploadPath, "thumbnails")
		os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, filename)

		src, _ := file.Open()
		defer src.Close()
		dst, _ := os.Create(destPath)
		io.Copy(dst, src)
		dst.Close()

		thumbPath := "thumbnails/" + filename
		article.Thumbnail = &thumbPath
	} else if galleryPath := c.FormValue("thumbnail_from_gallery"); galleryPath != "" {
		article.Thumbnail = &galleryPath
	}

	if err := h.db.Create(&article).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: err.Error()})
	}

	// Attach tags
	if tagIDs != "" {
		ids := strings.Split(tagIDs, ",")
		var tags []models.Tag
		h.db.Where("id IN ?", ids).Find(&tags)
		h.db.Model(&article).Association("Tags").Replace(tags)
	}

	h.db.Preload("Category").Preload("Author").Preload("Tags").First(&article, article.ID)

	return c.Status(fiber.StatusCreated).JSON(article)
}

func (h *ArticleHandler) UpdateArticle(c *fiber.Ctx) error {
	id := c.Params("id")
	var article models.Article
	if err := h.db.First(&article, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Article not found"})
	}

	title := c.FormValue("title")
	body := c.FormValue("body")
	excerpt := c.FormValue("excerpt")
	categoryID := c.FormValue("category_id")
	status := c.FormValue("status")
	tagIDs := c.FormValue("tag_ids")

	if title != "" {
		article.Title = title
		article.Slug = slug.Make(title)
	}
	if body != "" {
		article.Body = body
	}
	if excerpt != "" {
		article.Excerpt = &excerpt
	}
	if categoryID != "" {
		var cat models.Category
		if h.db.First(&cat, categoryID).RowsAffected > 0 {
			article.CategoryID = &cat.ID
		}
	}
	if status != "" {
		article.Status = status
		if status == "published" && article.PublishedAt == nil {
			now := time.Now()
			article.PublishedAt = &now
		}
	}

	// Handle thumbnail upload
	file, err := c.FormFile("thumbnail")
	if err == nil && file != nil {
		// Delete old thumbnail
		if article.Thumbnail != nil {
			os.Remove(filepath.Join(h.uploadPath, *article.Thumbnail))
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := fmt.Sprintf("thumb_%d%s", time.Now().UnixNano(), ext)
		destDir := filepath.Join(h.uploadPath, "thumbnails")
		os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, filename)

		src, _ := file.Open()
		defer src.Close()
		dst, _ := os.Create(destPath)
		io.Copy(dst, src)
		dst.Close()

		thumbPath := "thumbnails/" + filename
		article.Thumbnail = &thumbPath
	} else if galleryPath := c.FormValue("thumbnail_from_gallery"); galleryPath != "" {
		article.Thumbnail = &galleryPath
	}

	h.db.Save(&article)

	// Update tags
	if tagIDs != "" {
		ids := strings.Split(tagIDs, ",")
		var tags []models.Tag
		h.db.Where("id IN ?", ids).Find(&tags)
		h.db.Model(&article).Association("Tags").Replace(tags)
	}

	h.db.Preload("Category").Preload("Author").Preload("Tags").First(&article, article.ID)

	return c.JSON(article)
}

func (h *ArticleHandler) DeleteArticle(c *fiber.Ctx) error {
	id := c.Params("id")
	var article models.Article
	if err := h.db.First(&article, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Article not found"})
	}

	// Delete thumbnail
	if article.Thumbnail != nil {
		os.Remove(filepath.Join(h.uploadPath, *article.Thumbnail))
	}

	// Clear tags association
	h.db.Model(&article).Association("Tags").Clear()

	// Delete views
	h.db.Where("article_id = ?", article.ID).Delete(&models.ArticleView{})

	// Delete comments
	h.db.Where("article_id = ?", article.ID).Delete(&models.Comment{})

	h.db.Delete(&article)

	return c.JSON(fiber.Map{"message": "Article deleted"})
}

func (h *ArticleHandler) GetAnalytics(c *fiber.Ctx) error {
	id := c.Params("id")
	var article models.Article
	if err := h.db.First(&article, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Article not found"})
	}

	var totalViews int64
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).Count(&totalViews)

	var uniqueViews int64
	h.db.Model(&models.ArticleView{}).Where("article_id = ?", article.ID).
		Distinct("ip_address").Count(&uniqueViews)

	// Views by day (last 30 days)
	type ViewByDay struct {
		Date  string `json:"date"`
		Views int64  `json:"views"`
	}
	var viewsByDay []ViewByDay
	h.db.Model(&models.ArticleView{}).
		Select("DATE(created_at) as date, count(*) as views").
		Where("article_id = ? AND created_at >= ?", article.ID, time.Now().AddDate(0, 0, -30)).
		Group("date").
		Order("date").
		Find(&viewsByDay)

	return c.JSON(fiber.Map{
		"article":      article,
		"total_views":  totalViews,
		"unique_views": uniqueViews,
		"views_by_day": viewsByDay,
	})
}
