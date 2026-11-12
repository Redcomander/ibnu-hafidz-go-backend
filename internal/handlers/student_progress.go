package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"time"
)

type studentAttendanceSource struct{ module, table, date, notes, permission string }

var studentSources = []studentAttendanceSource{
	{"Formal", "absensis", "tanggal", "catatan", "absensi.view"},
	{"Diniyyah", "absensi_diniyyahs", "tanggal", "catatan", "absensi_diniyyah.view"},
	{"Halaqoh", "halaqoh_attendances", "date", "notes", "halaqoh.view"},
}

func studentSourceQuery(db *gorm.DB, source studentAttendanceSource) *gorm.DB {
	q := db.Table(source.table)
	if source.module == "Halaqoh" {
		return q.Joins("JOIN halaqoh_assignments ha ON ha.id = halaqoh_attendances.halaqoh_assignment_id")
	}
	return q.Where(source.table + ".deleted_at IS NULL")
}
func studentIDColumn(source studentAttendanceSource) string {
	if source.module == "Halaqoh" {
		return "ha.student_id"
	}
	return source.table + ".student_id"
}
func studentProgressRange(c *fiber.Ctx) (string, string, error) {
	now := time.Now().In(jakartaLocation())
	end := c.Query("end_date", now.Format("2006-01-02"))
	start := c.Query("start_date", now.AddDate(0, 0, -29).Format("2006-01-02"))
	a, err := time.Parse("2006-01-02", start)
	if err != nil {
		return "", "", fiber.NewError(400, "Tanggal awal tidak valid")
	}
	b, err := time.Parse("2006-01-02", end)
	if err != nil || b.Before(a) || b.Sub(a) > 366*24*time.Hour {
		return "", "", fiber.NewError(400, "Pilih rentang hingga 366 hari")
	}
	return start, b.AddDate(0, 0, 1).Format("2006-01-02"), nil
}
func progressUser(c *fiber.Ctx) (*models.User, error) {
	u, _ := c.Locals("user").(*models.User)
	if u == nil {
		return nil, fiber.NewError(401, "Masuk terlebih dahulu")
	}
	if !u.HasPermission("students.view") {
		return nil, fiber.NewError(403, "Akses profil santri tidak diizinkan")
	}
	return u, nil
}

type progressRecord struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Notes  string `json:"notes"`
	Module string `json:"module"`
}
type progressCount struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
	Module string `json:"module"`
}

func (h *StudentHandler) Progress(c *fiber.Ctx) error {
	user, err := progressUser(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || id == 0 {
		return fiber.NewError(400, "ID santri tidak valid")
	}
	start, end, err := studentProgressRange(c)
	if err != nil {
		return err
	}
	var student models.Student
	if err := h.db.Preload("Kelas").First(&student, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.NewError(404, "Santri tidak ditemukan")
		}
		return fiber.NewError(500, "Gagal memuat profil")
	}
	counts := []progressCount{}
	history := []progressRecord{}
	modules := []string{}
	for _, source := range studentSources {
		if !user.HasPermission(source.permission) {
			continue
		}
		modules = append(modules, source.module)
		base := func() *gorm.DB {
			return studentSourceQuery(h.db, source).Where(studentIDColumn(source)+" = ?", id).Where(source.table+"."+source.date+" >= ? AND "+source.table+"."+source.date+" < ?", start, end)
		}
		var rows []progressCount
		if err := base().Select(source.table + "." + source.date + " AS date, LOWER(" + source.table + ".status) AS status, COUNT(*) AS count").Group(source.table + "." + source.date + ", " + source.table + ".status").Scan(&rows).Error; err != nil {
			return fiber.NewError(500, "Gagal memuat ringkasan kehadiran")
		}
		for _, r := range rows {
			r.Module = source.module
			if len(r.Date) > 10 {
				r.Date = r.Date[:10]
			}
			counts = append(counts, r)
		}
		var recent []progressRecord
		if err := base().Select(source.table + "." + source.date + " AS date, " + source.table + ".status, COALESCE(" + source.table + "." + source.notes + ", '') AS notes").Order(source.table + "." + source.date + " DESC").Limit(30).Scan(&recent).Error; err != nil {
			return fiber.NewError(500, "Gagal memuat catatan kehadiran")
		}
		for _, r := range recent {
			r.Module = source.module
			if len(r.Date) > 10 {
				r.Date = r.Date[:10]
			}
			history = append(history, r)
		}
	}
	sort.SliceStable(history, func(i, j int) bool { return history[i].Date > history[j].Date })
	if len(history) > 30 {
		history = history[:30]
	}
	return c.JSON(fiber.Map{"student": fiber.Map{"id": student.ID, "nama_lengkap": student.NamaLengkap, "nisn": student.NISN, "kelas": student.Kelas}, "counts": counts, "history": history, "modules": modules, "start_date": start, "end_exclusive": end})
}
func (h *StudentHandler) AttendanceSignals(c *fiber.Ctx) error {
	user, err := progressUser(c)
	if err != nil {
		return err
	}
	start, end, err := studentProgressRange(c)
	if err != nil {
		return err
	}
	sourceName := c.Query("module", "Formal")
	var source *studentAttendanceSource
	for i := range studentSources {
		if strings.EqualFold(sourceName, studentSources[i].module) {
			source = &studentSources[i]
			break
		}
	}
	if source == nil {
		return fiber.NewError(400, "Modul tidak valid")
	}
	if !user.HasPermission(source.permission) {
		return fiber.NewError(403, "Akses rekapan modul tidak diizinkan")
	}
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		return fiber.NewError(400, "Halaman tidak valid")
	}
	type signal struct {
		StudentID uint   `json:"student_id"`
		Name      string `json:"name"`
		Count     int64  `json:"count"`
	}
	rows := []signal{}
	status := strings.ToLower(strings.TrimSpace(c.Query("status", "alpha")))
	statuses := []string{status}
	switch status {
	case "alpha", "alpa", "tidak_hadir":
		status = "alpha"
		statuses = []string{"alpha", "alpa", "tidak_hadir"}
	case "izin", "sakit":
	default:
		return fiber.NewError(400, "Status tidak valid")
	}
	threshold := c.QueryInt("threshold", 3)
	if threshold < 1 || threshold > 10000 {
		return fiber.NewError(400, "Batas jumlah tidak valid")
	}
	q := studentSourceQuery(h.db, *source).Joins("JOIN students ON students.id = "+studentIDColumn(*source)).Where("students.deleted_at IS NULL").Where(source.table+"."+source.date+" >= ? AND "+source.table+"."+source.date+" < ?", start, end).Where("LOWER("+source.table+".status) IN ?", statuses)
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		q = q.Where("students.nama_lengkap LIKE ?", "%"+search+"%")
	}
	grouped := q.Select("students.id AS student_id, students.nama_lengkap AS name, COUNT(*) AS count").Group("students.id, students.nama_lengkap").Having("COUNT(*) >= ?", threshold)
	var totals struct {
		Students int64
		Records  int64
	}
	if err := h.db.Table("(?) AS ranked_students", grouped.Session(&gorm.Session{})).Select("COUNT(*) AS students, COALESCE(SUM(count), 0) AS records").Scan(&totals).Error; err != nil {
		return fiber.NewError(500, "Gagal memuat total indikator")
	}
	err = grouped.Order("count DESC, students.id").Offset((page - 1) * 20).Limit(21).Scan(&rows).Error
	if err != nil {
		return fiber.NewError(500, "Gagal memuat indikator")
	}
	more := len(rows) > 20
	if more {
		rows = rows[:20]
	}
	return c.JSON(fiber.Map{"data": rows, "has_more": more, "page": page, "threshold": threshold, "status": status, "total_students": totals.Students, "total_records": totals.Records, "module": source.module, "start_date": start, "end_exclusive": end})
}
