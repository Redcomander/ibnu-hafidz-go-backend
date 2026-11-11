package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/mobile"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/stretchr/testify/require"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileReleaseLifecycle(t *testing.T) {
	db := securityDB(t)
	db.Config.IgnoreRelationshipsWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&models.MobileAppRelease{}, &models.MobileReleasePolicy{}, &models.MobileReleaseAudit{}))
	require.NoError(t, db.Create(&models.MobileReleasePolicy{ID: 1}).Error)
	cfg := mobile.Config{Directory: t.TempDir(), WebRoot: filepath.Join(t.TempDir(), "public"), PublicURL: "https://ibnuhafidz.ponpes.id", Package: "id.ponpes.ibnuhafidz.absensi", Certificate: strings.Repeat("a", 64), AAPT: "go", APKSigner: "go", MaxBytes: 1024}
	h := NewMobileReleaseHandler(db, cfg)
	h.inspect = func(path string) (mobile.APKInfo, error) {
		b, e := os.ReadFile(path)
		if e != nil {
			return mobile.APKInfo{}, e
		}
		var code int64
		if _, e = fmt.Sscanf(string(b), "valid-apk-%d", &code); e != nil {
			return mobile.APKInfo{}, errors.New("APK tidak valid")
		}
		hash, n, e := mobile.Digest(path)
		return mobile.APKInfo{Package: cfg.Package, Version: "1.2.0", Code: code, Certificate: cfg.Certificate, SHA256: hash, Size: n}, e
	}
	user := &models.User{ID: 1, Roles: []models.Role{{Name: "super_admin"}}}
	app := fiber.New(fiber.Config{StreamRequestBody: true, DisablePreParseMultipartForm: true})
	app.Get("/api/app/version", h.Version)
	app.Get("/api/app/download/:id", h.Download)
	admin := app.Group("/api/admin/app", func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user", user)
		}
		return c.Next()
	}, SuperAdminReleases)
	admin.Get("/releases", h.List)
	admin.Post("/releases", h.Upload)
	admin.Post("/releases/:id/publish", h.Publish)
	admin.Post("/releases/:id/archive", h.Archive)
	admin.Put("/policy", h.Policy)
	request := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, e := app.Test(req)
		require.NoError(t, e)
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}
	version := func(code int) map[string]interface{} {
		res, e := app.Test(httptest.NewRequest("GET", fmt.Sprintf("/api/app/version?version_code=%d", code), nil))
		require.NoError(t, e)
		defer res.Body.Close()
		require.Equal(t, 200, res.StatusCode)
		require.Equal(t, "no-store, max-age=0", res.Header.Get("Cache-Control"))
		var v map[string]interface{}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&v))
		return v
	}
	upload := func(content string) int {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, e := writer.CreateFormFile("apk", "app.apk")
		require.NoError(t, e)
		_, e = part.Write([]byte(content))
		require.NoError(t, e)
		writer.WriteField("changelog", "Perbaikan aplikasi")
		require.NoError(t, writer.Close())
		req := httptest.NewRequest("POST", "/api/admin/app/releases", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		res, e := app.Test(req)
		require.NoError(t, e)
		defer res.Body.Close()
		return res.StatusCode
	}
	require.Equal(t, false, version(11)["update_available"])
	user = nil
	require.Equal(t, 401, upload("valid-apk-12"))
	user = &models.User{ID: 2, Roles: []models.Role{{Name: "admin"}}}
	require.Equal(t, 403, upload("valid-apk-12"))
	user = &models.User{ID: 1, Roles: []models.Role{{Name: "super_admin"}}}
	require.Equal(t, 422, upload("not-apk"))
	require.Equal(t, 201, upload("valid-apk-12"))
	require.Equal(t, 409, upload("valid-apk-12"))
	require.Equal(t, false, version(11)["update_available"], "draft must not be public")
	require.Equal(t, 404, request("GET", "/api/app/download/1", ""))
	require.Equal(t, 400, request("POST", "/api/admin/app/releases/1/publish", `{}`))
	require.Equal(t, 400, request("POST", "/api/admin/app/releases/1/publish", `{"minimum_supported_version_code":13}`))
	var stored models.MobileAppRelease
	require.NoError(t, db.First(&stored, 1).Error)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Directory, stored.APKPath), []byte("valid-apk-99"), 0600))
	require.Equal(t, 422, request("POST", "/api/admin/app/releases/1/publish", `{"minimum_supported_version_code":10}`))
	require.Equal(t, false, version(11)["update_available"])
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Directory, stored.APKPath), []byte("valid-apk-12"), 0600))
	require.Equal(t, 200, request("POST", "/api/admin/app/releases/1/publish", `{"minimum_supported_version_code":10}`))
	require.Equal(t, true, version(9)["force_update"])
	require.Equal(t, false, version(10)["force_update"])
	require.Equal(t, true, version(11)["update_available"])
	require.Equal(t, false, version(12)["update_available"])
	require.Equal(t, false, version(13)["update_available"])
	require.Equal(t, 200, request("GET", "/api/app/download/1", ""))
	require.Equal(t, 200, request("PUT", "/api/admin/app/policy", `{"minimum_supported_version_code":0}`))
	require.Equal(t, false, version(9)["force_update"])
	require.Equal(t, 201, upload("valid-apk-11"))
	require.Equal(t, 409, request("POST", "/api/admin/app/releases/2/publish", `{"minimum_supported_version_code":0}`))
	require.Equal(t, 201, upload("valid-apk-13"))
	require.Equal(t, 200, request("POST", "/api/admin/app/releases/3/publish", `{"minimum_supported_version_code":0}`))
	require.Equal(t, 404, request("GET", "/api/app/download/1", ""))
	require.Equal(t, 200, request("POST", "/api/admin/app/releases/3/archive", `{}`))
	require.Equal(t, false, version(11)["update_available"])
	var count int64
	require.NoError(t, db.Model(&models.MobileAppRelease{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "archive retains release history")
	require.NoError(t, db.Model(&models.MobileReleaseAudit{}).Count(&count).Error)
	require.EqualValues(t, 4, count)
}
