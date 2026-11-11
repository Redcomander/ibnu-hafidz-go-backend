package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/mobile"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MobileReleaseHandler struct {
	db      *gorm.DB
	cfg     mobile.Config
	inspect func(string) (mobile.APKInfo, error)
	upload  sync.Mutex
}

func NewMobileReleaseHandler(db *gorm.DB, cfg mobile.Config) *MobileReleaseHandler {
	return &MobileReleaseHandler{db: db, cfg: cfg, inspect: cfg.Inspect}
}
func SuperAdminReleases(c *fiber.Ctx) error {
	u, _ := c.Locals("user").(*models.User)
	if u == nil {
		return fiber.NewError(401, "Silakan masuk")
	}
	if !u.HasRole("super_admin") {
		return fiber.NewError(403, "Hanya Super Admin dapat mengelola rilis")
	}
	return c.Next()
}
func (h *MobileReleaseHandler) policy(tx *gorm.DB, lock bool) (models.MobileReleasePolicy, error) {
	q := tx
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var p models.MobileReleasePolicy
	e := q.First(&p, 1).Error
	return p, e
}
func (h *MobileReleaseHandler) Version(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store, max-age=0")
	c.Set("Pragma", "no-cache")
	installed, e := strconv.ParseInt(c.Query("version_code", "0"), 10, 64)
	if e != nil || installed < 0 || installed > 2100000000 {
		return fiber.NewError(400, "version_code tidak valid")
	}
	p, e := h.policy(h.db, false)
	if e != nil {
		return fiber.NewError(503, "Layanan pembaruan belum siap")
	}
	result := fiber.Map{"update_available": false, "latest_version": "", "latest_version_code": 0, "minimum_supported_version_code": 0, "force_update": false, "download_url": "", "sha256": "", "file_size": 0, "changelog": "", "published_at": nil, "package_name": h.cfg.Package}
	if p.CurrentReleaseID == nil {
		return c.JSON(result)
	}
	var r models.MobileAppRelease
	if e = h.db.Where("id = ? AND status = ?", *p.CurrentReleaseID, "published").First(&r).Error; e != nil {
		return fiber.NewError(503, "Rilis aktif belum tersedia")
	}
	if h.cfg.PublicURL == "" || !strings.HasPrefix(h.cfg.PublicURL, "https://") {
		return fiber.NewError(503, "Alamat unduhan belum dikonfigurasi")
	}
	newer := r.VersionCode > installed
	return c.JSON(fiber.Map{"update_available": newer, "latest_version": r.VersionName, "latest_version_code": r.VersionCode, "minimum_supported_version_code": p.MinimumSupportedVersionCode, "force_update": newer && installed < p.MinimumSupportedVersionCode, "download_url": h.cfg.PublicURL + "/api/app/download/" + strconv.FormatUint(uint64(r.ID), 10), "sha256": r.SHA256, "file_size": r.FileSize, "changelog": r.Changelog, "published_at": r.PublishedAt, "package_name": r.PackageName})
}
func (h *MobileReleaseHandler) List(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	page := c.QueryInt("page", 1)
	if page < 1 || page > 100000 {
		return fiber.NewError(400, "Halaman tidak valid")
	}
	var rows []models.MobileAppRelease
	var total int64
	if e := h.db.Model(&models.MobileAppRelease{}).Count(&total).Error; e != nil {
		return fiber.NewError(503, "Migrasi rilis belum tersedia")
	}
	if e := h.db.Preload("Uploader", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name") }).Order("version_code DESC").Offset((page - 1) * 20).Limit(20).Find(&rows).Error; e != nil {
		return fiber.NewError(500, "Gagal memuat rilis")
	}
	p, e := h.policy(h.db, false)
	if e != nil {
		return fiber.NewError(503, "Kebijakan rilis belum tersedia")
	}
	ready := h.cfg.Ready()
	message := ""
	if ready != nil {
		message = ready.Error()
	}
	return c.JSON(fiber.Map{"data": rows, "total": total, "page": page, "per_page": 20, "policy": p, "upload_ready": ready == nil, "readiness_message": message, "max_file_size": h.cfg.MaxBytes})
}
func (h *MobileReleaseHandler) Get(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var r models.MobileAppRelease
	if e := h.db.Preload("Uploader", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name") }).First(&r, primaryKey(c.Params("id"))).Error; e != nil {
		return fiber.NewError(404, "Rilis tidak ditemukan")
	}
	return c.JSON(r)
}
func (h *MobileReleaseHandler) Upload(c *fiber.Ctx) error {
	if e := h.cfg.Ready(); e != nil {
		return fiber.NewError(503, e.Error())
	}
	if !h.upload.TryLock() {
		return fiber.NewError(429, "Unggahan lain sedang diperiksa. Coba lagi.")
	}
	defer h.upload.Unlock()
	if c.Request().Header.ContentLength() > int(h.cfg.MaxBytes+1024*1024) {
		return fiber.NewError(413, "APK melebihi batas ukuran")
	}
	media, params, e := mime.ParseMediaType(c.Get("Content-Type"))
	if e != nil || media != "multipart/form-data" || params["boundary"] == "" {
		return fiber.NewError(400, "Unggah menggunakan multipart APK")
	}
	if e = os.MkdirAll(h.cfg.Directory, 0700); e != nil {
		return fiber.NewError(500, "Penyimpanan privat belum tersedia")
	}
	if e = h.cfg.Ready(); e != nil {
		return fiber.NewError(503, e.Error())
	}
	temp, e := os.CreateTemp(h.cfg.Directory, "upload-*.part")
	if e != nil {
		return fiber.NewError(500, "Gagal menyiapkan penyimpanan")
	}
	tempPath := temp.Name()
	temp.Close()
	defer os.Remove(tempPath)
	body := c.Request().BodyStream()
	if body == nil {
		body = bytes.NewReader(c.Body())
	}
	reader := multipart.NewReader(io.LimitReader(body, h.cfg.MaxBytes+1024*1024), params["boundary"])
	fields := map[string]string{}
	fileSeen := false
	for count := 0; ; count++ {
		if count > 10 {
			return fiber.NewError(400, "Terlalu banyak bagian unggahan")
		}
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fiber.NewError(400, "Unggahan tidak lengkap")
		}
		if part.FileName() != "" {
			if fileSeen || part.FormName() != "apk" || strings.ToLower(filepath.Ext(part.FileName())) != ".apk" {
				part.Close()
				return fiber.NewError(400, "Hanya satu berkas APK diperbolehkan")
			}
			fileSeen = true
			f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				part.Close()
				return fiber.NewError(500, "Penyimpanan tidak dapat ditulis")
			}
			n, copyErr := io.Copy(f, io.LimitReader(part, h.cfg.MaxBytes+1))
			syncErr := f.Sync()
			closeErr := f.Close()
			part.Close()
			if n > h.cfg.MaxBytes {
				return fiber.NewError(413, "APK melebihi batas ukuran")
			}
			if copyErr != nil || syncErr != nil || closeErr != nil {
				return fiber.NewError(500, "APK gagal disimpan; periksa ruang penyimpanan")
			}
		} else {
			value, err := io.ReadAll(io.LimitReader(part, 10001))
			part.Close()
			if err != nil || len(value) > 10000 {
				return fiber.NewError(400, "Isian terlalu panjang")
			}
			if _, exists := fields[part.FormName()]; exists {
				return fiber.NewError(400, "Isian unggahan duplikat")
			}
			fields[part.FormName()] = string(value)
		}
	}
	if !fileSeen {
		return fiber.NewError(400, "Berkas APK wajib diisi")
	}
	info, e := h.inspect(tempPath)
	if e != nil {
		return fiber.NewError(422, e.Error())
	}
	if v := fields["version_name"]; v != "" && v != info.Version {
		return fiber.NewError(422, "Nama versi tidak cocok dengan APK")
	}
	if v := fields["version_code"]; v != "" && v != strconv.FormatInt(info.Code, 10) {
		return fiber.NewError(422, "Build number tidak cocok dengan APK")
	}
	var existing int64
	if e = h.db.Model(&models.MobileAppRelease{}).Where("version_code = ?", info.Code).Count(&existing).Error; e != nil {
		return fiber.NewError(503, "Migrasi rilis belum tersedia")
	}
	if existing > 0 {
		return fiber.NewError(409, "Build number sudah terdaftar")
	}
	random := make([]byte, 24)
	if _, e = rand.Read(random); e != nil {
		return fiber.NewError(500, "Gagal membuat nama berkas")
	}
	name := hex.EncodeToString(random) + ".apk"
	final := filepath.Join(h.cfg.Directory, name)
	if e = os.Rename(tempPath, final); e != nil {
		return fiber.NewError(500, "APK gagal disimpan")
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(final)
		}
	}()
	u := c.Locals("user").(*models.User)
	r := models.MobileAppRelease{VersionName: info.Version, VersionCode: info.Code, PackageName: info.Package, APKPath: name, SHA256: info.SHA256, SignerSHA256: info.Certificate, FileSize: info.Size, Changelog: strings.TrimSpace(fields["changelog"]), ForceUpdate: fields["force_update"] == "true", Status: "draft", CreatedBy: u.ID}
	if e = h.db.Create(&r).Error; e != nil {
		return fiber.NewError(409, "Rilis belum tersimpan; build mungkin sudah terdaftar")
	}
	keep = true
	return c.Status(201).JSON(r)
}
func (h *MobileReleaseHandler) Publish(c *fiber.Ctx) error {
	var req struct {
		Minimum *int64 `json:"minimum_supported_version_code"`
	}
	if c.BodyParser(&req) != nil || req.Minimum == nil || *req.Minimum < 0 {
		return fiber.NewError(400, "Tentukan versi minimum yang didukung (0 untuk semua)")
	}
	u := c.Locals("user").(*models.User)
	e := h.db.Transaction(func(tx *gorm.DB) error {
		p, e := h.policy(tx, true)
		if e != nil {
			return fiber.NewError(503, "Kebijakan rilis belum tersedia")
		}
		var r models.MobileAppRelease
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, primaryKey(c.Params("id"))).Error; e != nil {
			return fiber.NewError(404, "Rilis tidak ditemukan")
		}
		if r.Status != "draft" || r.VersionCode <= p.HighestVersionCode {
			return fiber.NewError(409, "Hanya draft build yang lebih tinggi dapat dipublikasikan")
		}
		if *req.Minimum > r.VersionCode {
			return fiber.NewError(400, "Versi minimum melebihi rilis terbaru")
		}
		info, e := h.inspect(filepath.Join(h.cfg.Directory, r.APKPath))
		if e != nil {
			return fiber.NewError(422, "Validasi ulang APK gagal: "+e.Error())
		}
		if info.SHA256 != r.SHA256 || info.Code != r.VersionCode || info.Version != r.VersionName || info.Size != r.FileSize || info.Certificate != r.SignerSHA256 {
			return fiber.NewError(422, "APK berubah setelah diunggah")
		}
		if e = tx.Model(&models.MobileAppRelease{}).Where("status = ?", "published").Update("status", "archived").Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		r.Status = "published"
		r.PublishedAt = &now
		r.ForceUpdate = *req.Minimum > 0
		if e = tx.Save(&r).Error; e != nil {
			return e
		}
		p.CurrentReleaseID = &r.ID
		p.MinimumSupportedVersionCode = *req.Minimum
		p.HighestVersionCode = r.VersionCode
		if e = tx.Save(&p).Error; e != nil {
			return e
		}
		return tx.Create(&models.MobileReleaseAudit{ReleaseID: r.ID, ActorID: u.ID, Action: "publish", Details: "minimum_supported_version_code=" + strconv.FormatInt(*req.Minimum, 10)}).Error
	})
	if e != nil {
		return releaseError(e)
	}
	return c.JSON(fiber.Map{"message": "Rilis dipublikasikan"})
}
func releaseError(e error) error {
	var fe *fiber.Error
	if errors.As(e, &fe) {
		return fe
	}
	return fiber.NewError(500, "Operasi rilis gagal. Tidak ada perubahan yang dikonfirmasi.")
}
func (h *MobileReleaseHandler) Archive(c *fiber.Ctx) error {
	u := c.Locals("user").(*models.User)
	e := h.db.Transaction(func(tx *gorm.DB) error {
		p, e := h.policy(tx, true)
		if e != nil {
			return e
		}
		var r models.MobileAppRelease
		if tx.First(&r, primaryKey(c.Params("id"))).Error != nil {
			return fiber.NewError(404, "Rilis tidak ditemukan")
		}
		if r.Status == "archived" {
			return nil
		}
		r.Status = "archived"
		if e = tx.Save(&r).Error; e != nil {
			return e
		}
		if p.CurrentReleaseID != nil && *p.CurrentReleaseID == r.ID {
			p.CurrentReleaseID = nil
			p.MinimumSupportedVersionCode = 0
			if e = tx.Save(&p).Error; e != nil {
				return e
			}
		}
		return tx.Create(&models.MobileReleaseAudit{ReleaseID: r.ID, ActorID: u.ID, Action: "archive", Details: "APK dipertahankan; Android tidak diturunkan versinya"}).Error
	})
	if e != nil {
		return releaseError(e)
	}
	return c.JSON(fiber.Map{"message": "Rilis diarsipkan; berkas dan riwayat tetap disimpan"})
}
func (h *MobileReleaseHandler) Policy(c *fiber.Ctx) error {
	var req struct {
		Minimum *int64 `json:"minimum_supported_version_code"`
	}
	if c.BodyParser(&req) != nil || req.Minimum == nil || *req.Minimum < 0 {
		return fiber.NewError(400, "Versi minimum tidak valid")
	}
	u := c.Locals("user").(*models.User)
	e := h.db.Transaction(func(tx *gorm.DB) error {
		p, e := h.policy(tx, true)
		if e != nil {
			return e
		}
		if p.CurrentReleaseID == nil {
			return fiber.NewError(409, "Publikasikan rilis dahulu")
		}
		var r models.MobileAppRelease
		if e = tx.First(&r, *p.CurrentReleaseID).Error; e != nil {
			return e
		}
		if *req.Minimum > r.VersionCode {
			return fiber.NewError(400, "Versi minimum melebihi versi aktif")
		}
		p.MinimumSupportedVersionCode = *req.Minimum
		if e = tx.Save(&p).Error; e != nil {
			return e
		}
		return tx.Create(&models.MobileReleaseAudit{ReleaseID: r.ID, ActorID: u.ID, Action: "minimum_policy", Details: strconv.FormatInt(*req.Minimum, 10)}).Error
	})
	if e != nil {
		return releaseError(e)
	}
	return c.JSON(fiber.Map{"message": "Kebijakan minimum diperbarui"})
}
func (h *MobileReleaseHandler) Download(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store, no-transform")
	var r models.MobileAppRelease
	if h.db.First(&r, primaryKey(c.Params("id"))).Error != nil {
		return fiber.NewError(404, "APK tidak ditemukan")
	}
	admin, _ := c.Locals("user").(*models.User)
	if admin == nil || !admin.HasRole("super_admin") {
		p, e := h.policy(h.db, false)
		if e != nil || r.Status != "published" || p.CurrentReleaseID == nil || *p.CurrentReleaseID != r.ID {
			return fiber.NewError(404, "APK tidak tersedia")
		}
	}
	if filepath.Base(r.APKPath) != r.APKPath || !strings.HasSuffix(r.APKPath, ".apk") {
		return fiber.NewError(404, "APK tidak tersedia")
	}
	file := filepath.Join(h.cfg.Directory, r.APKPath)
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Size() != r.FileSize {
		return fiber.NewError(404, "APK tidak tersedia")
	}
	c.Set("Content-Type", "application/vnd.android.package-archive")
	c.Set("Content-Disposition", `attachment; filename="ibnu-hafidz-`+strconv.FormatInt(r.VersionCode, 10)+`.apk"`)
	stream, err := os.Open(file)
	if err != nil {
		return fiber.NewError(404, "APK tidak tersedia")
	}
	return c.SendStream(stream, int(info.Size()))
}
