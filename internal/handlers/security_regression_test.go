package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/config"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func securityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Role{}, &models.Permission{}, &models.RefreshSession{}))
	return db
}

func TestSecurityPrimaryKeysAndSort(t *testing.T) {
	db := securityDB(t)
	user := models.User{Name: "Safe", Username: "safe", Email: "safe@example.test", Password: "hash"}
	require.NoError(t, db.Create(&user).Error)
	for _, input := range []string{"1=1", "1 OR 1=1", "1; DELETE FROM users", "-1", "0", "18446744073709551616"} {
		var found models.User
		require.ErrorIs(t, db.First(&found, primaryKey(input)).Error, gorm.ErrRecordNotFound, input)
		result := db.Delete(&models.User{}, primaryKey(input))
		require.NoError(t, result.Error)
		require.Zero(t, result.RowsAffected, input)
	}
	var found models.User
	require.NoError(t, db.First(&found, primaryKey("1")).Error)
	for _, field := range []string{"(SELECT 1)", "name desc; DELETE FROM users", "name`", "unknown"} {
		stmt := safeOrder(db.Session(&gorm.Session{DryRun: true}).Model(&models.User{}), field, "desc; --").Find(&[]models.User{}).Statement
		require.NotContains(t, stmt.SQL.String(), field)
		require.Contains(t, stmt.SQL.String(), "`users`.`created_at` DESC")
	}
	stmt := safeOrder(db.Session(&gorm.Session{DryRun: true}).Model(&models.User{}), "name", "asc").Find(&[]models.User{}).Statement
	require.Contains(t, stmt.SQL.String(), "`users`.`name`")
	require.NotContains(t, stmt.SQL.String(), " DESC")

	h := NewUserHandler(db)
	app := fiber.New()
	app.Get("/users/:id", h.Get)
	response, err := app.Test(httptest.NewRequest("GET", "/users/1%3D1", nil))
	require.NoError(t, err)
	require.Equal(t, 404, response.StatusCode)
}

func authSecurityApp(t *testing.T) (*gorm.DB, *fiber.App, models.User) {
	t.Helper()
	db := securityDB(t)
	password, err := utils.HashPassword("old-secret123")
	require.NoError(t, err)
	user := models.User{Name: "Tester", Username: "tester", Email: "tester@example.test", Password: password}
	require.NoError(t, db.Create(&user).Error)
	h := NewAuthHandler(db, &config.Config{JWTSecret: "test-access", Environment: "production"})
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/api/auth/login", h.Login)
	app.Post("/api/auth/refresh", h.Refresh)
	app.Post("/api/auth/logout", h.Logout)
	app.Put("/profile", func(c *fiber.Ctx) error { c.Locals("userID", user.ID); return h.UpdateProfile(c) })
	app.Put("/users/:id", NewUserHandler(db).Update)
	return db, app, user
}

func securityRequest(t *testing.T, app *fiber.App, method, target, body string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := app.Test(req)
	require.NoError(t, err)
	return res
}

func securityLogin(t *testing.T, app *fiber.App) *http.Cookie {
	t.Helper()
	res := securityRequest(t, app, "POST", "/api/auth/login", `{"username":"tester","password":"old-secret123"}`, nil)
	require.Equal(t, 200, res.StatusCode)
	require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	require.Len(t, res.Cookies(), 1)
	cookie := res.Cookies()[0]
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, "/api/auth", cookie.Path)
	require.Len(t, cookie.Value, 43)
	return cookie
}

func TestSecurityRefreshRotationAndReplay(t *testing.T) {
	db, app, _ := authSecurityApp(t)
	old := securityLogin(t, app)
	var stored models.RefreshSession
	require.NoError(t, db.First(&stored).Error)
	require.NotEqual(t, old.Value, stored.TokenHash)
	otherSession := securityLogin(t, app)
	res := securityRequest(t, app, "POST", "/api/auth/refresh", "", old)
	require.Equal(t, 200, res.StatusCode)
	next := res.Cookies()[0]
	require.NotEqual(t, old.Value, next.Value)
	require.WithinDuration(t, old.Expires, next.Expires, time.Second)
	// Reuse of a consumed token revokes its replacement as well.
	require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", old).StatusCode)
	require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", next).StatusCode)
	require.Equal(t, 200, securityRequest(t, app, "POST", "/api/auth/refresh", "", otherSession).StatusCode)
}

func TestSecurityRefreshFailureDoesNotConsumeCookie(t *testing.T) {
	db, app, _ := authSecurityApp(t)
	cookie := securityLogin(t, app)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test_fail_rotation", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "RefreshSession" {
			tx.AddError(errors.New("simulated persistence failure"))
		}
	}))
	response := securityRequest(t, app, "POST", "/api/auth/refresh", "", cookie)
	require.Equal(t, 503, response.StatusCode)
	require.Empty(t, response.Cookies())
	var session models.RefreshSession
	require.NoError(t, db.First(&session, "token_hash = ?", tokenDigest(cookie.Value)).Error)
	require.Nil(t, session.ConsumedAt)
	require.NoError(t, db.Callback().Create().Remove("test_fail_rotation"))
	require.Equal(t, 200, securityRequest(t, app, "POST", "/api/auth/refresh", "", cookie).StatusCode)
}

func TestSecurityLogoutRevokesFamilyWithoutAccessToken(t *testing.T) {
	_, app, _ := authSecurityApp(t)
	old := securityLogin(t, app)
	rotated := securityRequest(t, app, "POST", "/api/auth/refresh", "", old)
	require.Equal(t, 200, rotated.StatusCode)
	next := rotated.Cookies()[0]
	// Even an older cookie can identify and revoke its entire family.
	require.Equal(t, 200, securityRequest(t, app, "POST", "/api/auth/logout", "", old).StatusCode)
	require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", next).StatusCode)
}

func TestSecurityPasswordChangesRevokeAllRefreshSessions(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "self", true: "admin"}[admin], func(t *testing.T) {
			db, app, _ := authSecurityApp(t)
			first, second := securityLogin(t, app), securityLogin(t, app)
			target := "/profile"
			body := `{"name":"Tester","username":"tester","email":"tester@example.test","current_password":"old-secret123","new_password":"new-secret123"}`
			if admin {
				target = "/users/1"
				body = `{"password":"new-secret123"}`
			}
			require.Equal(t, 200, securityRequest(t, app, "PUT", target, body, nil).StatusCode)
			for _, cookie := range []*http.Cookie{first, second} {
				require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", cookie).StatusCode)
			}
			var active int64
			require.NoError(t, db.Model(&models.RefreshSession{}).Where("revoked_at IS NULL").Count(&active).Error)
			require.Zero(t, active)
		})
	}
}

func TestSecurityExpiredAndLegacyRefreshTokensRejected(t *testing.T) {
	db, app, user := authSecurityApp(t)
	cookie := securityLogin(t, app)
	require.NoError(t, db.Model(&models.RefreshSession{}).Where("token_hash = ?", tokenDigest(cookie.Value)).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", cookie).StatusCode)
	oldJWT, err := utils.GenerateRefreshToken(user.ID, user.Email, "old-secret")
	require.NoError(t, err)
	require.Equal(t, 401, securityRequest(t, app, "POST", "/api/auth/refresh", "", &http.Cookie{Name: "refresh_token", Value: oldJWT}).StatusCode)
}

func TestSecurityLegacyArticleHTMLAndAuthorPrivacy(t *testing.T) {
	db := securityDB(t)
	// The production schema uses MySQL ENUM; SQLite fixtures use TEXT.
	require.NoError(t, db.Exec(`CREATE TABLE articles (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT, slug TEXT, body TEXT, excerpt TEXT, thumbnail TEXT, category_id INTEGER, author_id INTEGER, status TEXT, published_at DATETIME, created_at DATETIME, updated_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE article_views (id INTEGER PRIMARY KEY AUTOINCREMENT, article_id INTEGER, user_id INTEGER, ip_address TEXT, user_agent TEXT, created_at DATETIME)`).Error)
	private := "PRIVATE-MARKER"
	author := models.User{Name: "Public Author", Username: private, Email: private, Password: private, NIK: &private, TempatLahir: &private, TanggalLahir: &private}
	require.NoError(t, db.Create(&author).Error)
	now := time.Now()
	article := models.Article{Title: "Test", Slug: "test", Body: `<h2 style="text-align: center">Allowed</h2><script>alert(1)</script><img src="/uploads/test.png" onerror="alert(1)"><a href="javascript:alert(1)">bad</a>`, AuthorID: author.ID, Status: "published", PublishedAt: &now}
	require.NoError(t, db.Create(&article).Error)
	require.NotContains(t, article.Body, "onerror")
	require.Contains(t, article.Body, "text-align: center")
	// Bypass hooks to emulate a malicious article already stored before the fix.
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", article.ID).UpdateColumn("body", `<p>Legacy</p><img src="x" onerror="alert(1)"><script>alert(1)</script>`).Error)
	h := NewPublicHandler(db)
	app := fiber.New()
	app.Get("/articles/:slug", h.GetArticleDetail)
	app.Get("/articles", h.GetArticles)
	for _, target := range []string{"/articles/test", "/articles"} {
		res, err := app.Test(httptest.NewRequest("GET", target, nil))
		require.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)
		var payload map[string]interface{}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NotContains(t, string(data), private)
		require.NotContains(t, string(data), "onerror")
		require.NotContains(t, string(data), "script")
		require.Contains(t, string(data), "Public Author")
		require.Contains(t, string(data), "views") // Preserve embedding/metrics.
	}
}

func articleFormRequest(t *testing.T, app *fiber.App, target string, fields map[string]string, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(t, writer.WriteField(k, v))
	}
	if filename != "" {
		file, err := writer.CreateFormFile("thumbnail", filename)
		require.NoError(t, err)
		_, err = file.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest("POST", target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := app.Test(req)
	require.NoError(t, err)
	return res
}

func TestSecurityArticleThumbnailContainmentAndTypes(t *testing.T) {
	db := securityDB(t)
	require.NoError(t, db.Exec(`CREATE TABLE galleries (id INTEGER PRIMARY KEY AUTOINCREMENT, album_id INTEGER, title TEXT, type TEXT, path TEXT, webp_path TEXT, source TEXT, original_url TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`).Error)
	dir := t.TempDir()
	uploads := filepath.Join(dir, "uploads")
	require.NoError(t, os.MkdirAll(uploads, 0755))
	outside := filepath.Join(dir, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0600))
	h := NewArticleHandler(db, uploads)
	unsafe := "../outside.txt"
	h.removeArticleThumbnail(&unsafe)
	require.FileExists(t, outside)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/thumbnail", func(c *fiber.Ctx) error {
		value, err := h.articleThumbnail(c, nil)
		if err != nil {
			return err
		}
		return c.JSON(value)
	})
	require.Equal(t, 400, articleFormRequest(t, app, "/thumbnail", map[string]string{"thumbnail_from_gallery": "../outside.txt"}, "", nil).StatusCode)
	require.Equal(t, 400, articleFormRequest(t, app, "/thumbnail", nil, "x.jpg", []byte(`<html><script>alert(1)</script></html>`)).StatusCode)
	var imageData bytes.Buffer
	require.NoError(t, png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	res := articleFormRequest(t, app, "/thumbnail", nil, "misleading.html", imageData.Bytes())
	require.Equal(t, 200, res.StatusCode)
	var saved string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&saved))
	require.True(t, strings.HasSuffix(saved, ".png"))
	require.FileExists(t, filepath.Join(uploads, filepath.FromSlash(saved)))
	h.removeArticleThumbnail(&saved)
	require.NoFileExists(t, filepath.Join(uploads, filepath.FromSlash(saved)))
	require.FileExists(t, outside)
	// Registered gallery assets remain shared, never article-owned.
	require.NoError(t, os.MkdirAll(filepath.Join(uploads, "gallery"), 0755))
	galleryPath := "gallery/shared.png"
	require.NoError(t, os.WriteFile(filepath.Join(uploads, "gallery", "shared.png"), imageData.Bytes(), 0600))
	require.NoError(t, db.Create(&models.Gallery{Title: "Shared", Type: "photo", Source: "upload", Path: galleryPath}).Error)
	galleryResponse := articleFormRequest(t, app, "/thumbnail", map[string]string{"thumbnail_gallery_id": "1"}, "", nil)
	require.Equal(t, 200, galleryResponse.StatusCode)
	h.removeArticleThumbnail(&galleryPath)
	require.FileExists(t, filepath.Join(uploads, "gallery", "shared.png"))
}
