package handlers

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
)

type ArrivalHandler struct {
	db *gorm.DB
}

func NewArrivalHandler(db *gorm.DB) *ArrivalHandler {
	return &ArrivalHandler{db: db}
}

type arrivalDashboardRow struct {
	Jenis             string     `json:"jenis"`
	Nama              string     `json:"nama"`
	GenderCode        *string    `json:"gender_code,omitempty"`
	GenderLabel       *string    `json:"gender_label,omitempty"`
	TanggalKedatangan time.Time  `json:"tanggal_kedatangan"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
	Kelas             *string    `json:"kelas,omitempty"`
	IsLate            bool       `json:"is_late"`
}

type arrivalKelasStat struct {
	ID          uint    `json:"id"`
	Nama        string  `json:"nama"`
	Tingkat     *string `json:"tingkat,omitempty"`
	JumlahHadir int64   `json:"jumlah_hadir"`
}

type lateStudentRow struct {
	Nama        string     `json:"nama"`
	GenderCode  *string    `json:"gender_code,omitempty"`
	GenderLabel *string    `json:"gender_label,omitempty"`
	Kelas       *string    `json:"kelas,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	IsLate      bool       `json:"is_late"`
	LateDays    int        `json:"late_days"`
}

func (h *ArrivalHandler) SearchPublic(c *fiber.Ctx) error {
	if !isValidArrivalToken(c.Params("token")) {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Not found"})
	}

	target := strings.TrimSpace(c.Query("target", "students"))
	query := strings.TrimSpace(c.Query("query", ""))
	selectedGender := normalizeGenderCode(c.Query("gender", ""))
	if len([]rune(query)) < 2 {
		return c.JSON(fiber.Map{"data": []any{}})
	}

	if target == "users" {
		var users []models.User
		q := h.db.Select("id, name, gender").Where("name LIKE ?", "%"+query+"%")
		if selectedGender != nil {
			q = q.Where("LOWER(TRIM(gender)) = ?", strings.ToLower(*selectedGender))
		}
		q.Order("name asc").Limit(20).Find(&users)

		data := make([]fiber.Map, 0, len(users))
		for _, u := range users {
			genderCode := normalizeGenderCode(ptrString(u.Gender))
			data = append(data, fiber.Map{
				"id":           u.ID,
				"nama_lengkap": u.Name,
				"gender_code":  genderCode,
				"gender_label": formatGenderLabel(genderCode),
			})
		}
		return c.JSON(fiber.Map{"data": data})
	}

	var students []models.Student
	q := h.db.Select("id, nama_lengkap, jenis_kelamin").Where("nama_lengkap LIKE ?", "%"+query+"%")
	if selectedGender != nil {
		q = applyStudentGenderFilter(q, *selectedGender)
	}
	q.Order("nama_lengkap asc").Limit(20).Find(&students)

	data := make([]fiber.Map, 0, len(students))
	for _, s := range students {
		genderCode := normalizeGenderCode(ptrString(s.JenisKelamin))
		data = append(data, fiber.Map{
			"id":           s.ID,
			"nama_lengkap": s.NamaLengkap,
			"gender_code":  genderCode,
			"gender_label": formatGenderLabel(genderCode),
		})
	}

	return c.JSON(fiber.Map{"data": data})
}

func (h *ArrivalHandler) SubmitPublic(c *fiber.Ctx) error {
	if !isValidArrivalToken(c.Params("token")) {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{Error: "not_found", Message: "Not found"})
	}

	type reqBody struct {
		TanggalKedatangan string `json:"tanggal_kedatangan"`
		ArrivalType       string `json:"arrival_type"`
		StudentID         *uint  `json:"student_id"`
		UserID            *uint  `json:"user_id"`
	}

	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}

	req.ArrivalType = strings.TrimSpace(strings.ToLower(req.ArrivalType))
	if req.ArrivalType != "student" && req.ArrivalType != "user" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "arrival_type must be student or user"})
	}

	tanggal, err := time.Parse("2006-01-02", strings.TrimSpace(req.TanggalKedatangan))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "tanggal_kedatangan tidak valid"})
	}

	ip := c.IP()
	if req.ArrivalType == "student" {
		if req.StudentID == nil || *req.StudentID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "student_id wajib diisi"})
		}

		arrival := models.StudentArrival{StudentID: *req.StudentID, TanggalKedatangan: tanggal}
		if err := h.db.Where("student_id = ? AND tanggal_kedatangan = ?", arrival.StudentID, arrival.TanggalKedatangan).
			FirstOrCreate(&arrival, models.StudentArrival{DicatatOlehIP: &ip}).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal menyimpan kedatangan"})
		}

		created := arrival.CreatedAt.Equal(arrival.UpdatedAt)
		if created {
			return c.JSON(fiber.Map{"message": "Absen kedatangan santri berhasil disimpan.", "created": true})
		}
		return c.JSON(fiber.Map{"message": "Data kedatangan untuk santri dan tanggal tersebut sudah tercatat.", "created": false})
	}

	if req.UserID == nil || *req.UserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "user_id wajib diisi"})
	}

	arrival := models.UserArrival{UserID: *req.UserID, TanggalKedatangan: tanggal}
	if err := h.db.Where("user_id = ? AND tanggal_kedatangan = ?", arrival.UserID, arrival.TanggalKedatangan).
		FirstOrCreate(&arrival, models.UserArrival{DicatatOlehIP: &ip}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal menyimpan kedatangan"})
	}

	created := arrival.CreatedAt.Equal(arrival.UpdatedAt)
	if created {
		return c.JSON(fiber.Map{"message": "Absen kedatangan pengajar berhasil disimpan.", "created": true})
	}
	return c.JSON(fiber.Map{"message": "Data kedatangan untuk pengajar dan tanggal tersebut sudah tercatat.", "created": false})
}

func (h *ArrivalHandler) Dashboard(c *fiber.Ctx) error {
	selectedDate := strings.TrimSpace(c.Query("date", time.Now().Format("2006-01-02")))
	selectedGender := normalizeGenderCode(c.Query("gender", ""))
	tanggal, err := time.Parse("2006-01-02", selectedDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "date tidak valid"})
	}

	var setting models.StudentArrivalSetting
	if err := h.db.First(&setting).Error; err != nil && err != gorm.ErrRecordNotFound {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal mengambil pengaturan kedatangan"})
	}
	studentDeadlineAt := setting.StudentDeadlineAt

	var studentArrivals []models.StudentArrival
	studentQ := h.db.Preload("Student").Preload("Student.Kelas").Where("DATE(tanggal_kedatangan) = ?", tanggal.Format("2006-01-02"))
	if selectedGender != nil {
		studentQ = studentQ.Joins("JOIN students ON students.id = student_arrivals.student_id").Scopes(func(db *gorm.DB) *gorm.DB {
			return applyStudentGenderFilter(db, *selectedGender)
		})
	}
	studentQ.Order("student_arrivals.id desc").Find(&studentArrivals)

	var userArrivals []models.UserArrival
	userQ := h.db.Preload("User").Where("DATE(tanggal_kedatangan) = ?", tanggal.Format("2006-01-02"))
	if selectedGender != nil {
		userQ = userQ.Joins("JOIN users ON users.id = user_arrivals.user_id").Where("LOWER(TRIM(users.gender)) = ?", strings.ToLower(*selectedGender))
	}
	userQ.Order("user_arrivals.id desc").Find(&userArrivals)

	rows := make([]arrivalDashboardRow, 0, len(studentArrivals)+len(userArrivals))
	studentLateCount := int64(0)
	studentBaninCount := int64(0)
	studentBanatCount := int64(0)
	userBaninCount := int64(0)
	userBanatCount := int64(0)
	lateStudents := make([]lateStudentRow, 0)

	for _, a := range studentArrivals {
		var kelasLabel *string
		if a.Student != nil && a.Student.Kelas != nil {
			name := strings.TrimSpace(strings.TrimSpace(a.Student.Kelas.Nama) + " " + strings.TrimSpace(a.Student.Kelas.Tingkat))
			if name != "" {
				kelasLabel = &name
			}
		}
		studentName := ""
		genderCode := (*string)(nil)
		if a.Student != nil {
			studentName = strings.TrimSpace(a.Student.NamaLengkap)
			genderCode = normalizeGenderCode(ptrString(a.Student.JenisKelamin))
		}
		if genderCode != nil {
			if *genderCode == "L" {
				studentBaninCount++
			}
			if *genderCode == "P" {
				studentBanatCount++
			}
		}
		isLate := isLateStudentArrival(a.CreatedAt, studentDeadlineAt)
		if isLate {
			studentLateCount++
			createdAt := a.CreatedAt
			lateStudents = append(lateStudents, lateStudentRow{
				Nama:        studentName,
				GenderCode:  genderCode,
				GenderLabel: formatGenderLabel(genderCode),
				Kelas:       kelasLabel,
				CreatedAt:   &createdAt,
				IsLate:      true,
				LateDays:    lateDaysForStudentArrival(a.CreatedAt, studentDeadlineAt),
			})
		}
		rows = append(rows, arrivalDashboardRow{
			Jenis:             "Santri",
			Nama:              studentName,
			GenderCode:        genderCode,
			GenderLabel:       formatGenderLabel(genderCode),
			TanggalKedatangan: a.TanggalKedatangan,
			CreatedAt:         &a.CreatedAt,
			Kelas:             kelasLabel,
			IsLate:            isLate,
		})
	}
	for _, a := range userArrivals {
		var createdAt = a.CreatedAt
		userName := ""
		genderCode := (*string)(nil)
		if a.User != nil {
			userName = strings.TrimSpace(a.User.Name)
			genderCode = normalizeGenderCode(ptrString(a.User.Gender))
		}
		if genderCode != nil {
			if *genderCode == "L" {
				userBaninCount++
			}
			if *genderCode == "P" {
				userBanatCount++
			}
		}
		rows = append(rows, arrivalDashboardRow{
			Jenis:             "Pengajar",
			Nama:              userName,
			GenderCode:        genderCode,
			GenderLabel:       formatGenderLabel(genderCode),
			TanggalKedatangan: a.TanggalKedatangan,
			CreatedAt:         &createdAt,
			IsLate:            false,
		})
	}

	// Sort merged rows by newest creation time.
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			ti := int64(0)
			tj := int64(0)
			if rows[i].CreatedAt != nil {
				ti = rows[i].CreatedAt.Unix()
			}
			if rows[j].CreatedAt != nil {
				tj = rows[j].CreatedAt.Unix()
			}
			if tj > ti {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}

	var kelasStats []arrivalKelasStat
	h.db.Raw(`
		SELECT k.id, k.nama, k.tingkat,
			COUNT(DISTINCT CASE WHEN sa.id IS NOT NULL THEN s.id END) AS jumlah_hadir
		FROM kelas k
		LEFT JOIN students s ON s.kelas_id = k.id AND s.deleted_at IS NULL
		LEFT JOIN student_arrivals sa
			ON sa.student_id = s.id
			AND DATE(sa.tanggal_kedatangan) = ?
			AND sa.deleted_at IS NULL
		WHERE k.deleted_at IS NULL
		GROUP BY k.id, k.nama, k.tingkat
		ORDER BY k.tingkat, k.nama
	`, tanggal.Format("2006-01-02")).Scan(&kelasStats)

	totalStudentArrivals := int64(len(studentArrivals))
	totalUserArrivals := int64(len(userArrivals))
	totalArrivals := totalStudentArrivals + totalUserArrivals
	totalBaninCount := studentBaninCount + userBaninCount
	totalBanatCount := studentBanatCount + userBanatCount
	totalKelasDenganKedatangan := int64(0)
	for _, k := range kelasStats {
		if k.JumlahHadir > 0 {
			totalKelasDenganKedatangan++
		}
	}

	for i := 0; i < len(lateStudents); i++ {
		for j := i + 1; j < len(lateStudents); j++ {
			if lateStudents[j].LateDays > lateStudents[i].LateDays {
				lateStudents[i], lateStudents[j] = lateStudents[j], lateStudents[i]
			}
		}
	}

	publicFormURL := ""
	if token := os.Getenv("STUDENT_ARRIVAL_ACCESS_TOKEN"); strings.TrimSpace(token) != "" {
		publicFormURL = "/absen-kedatangan/" + strings.TrimSpace(token)
	}

	return c.JSON(fiber.Map{
		"selected_date":         selectedDate,
		"selected_gender":       selectedGender,
		"arrivals":              rows,
		"kelas_stats":           kelasStats,
		"student_deadline_at":   studentDeadlineAt,
		"late_student_arrivals": lateStudents,
		"totals": fiber.Map{
			"all":                   totalArrivals,
			"students":              totalStudentArrivals,
			"users":                 totalUserArrivals,
			"student_late":          studentLateCount,
			"student_banin":         studentBaninCount,
			"student_banat":         studentBanatCount,
			"user_banin":            userBaninCount,
			"user_banat":            userBanatCount,
			"total_banin":           totalBaninCount,
			"total_banat":           totalBanatCount,
			"classes_with_arrivals": totalKelasDenganKedatangan,
		},
		"public_form_url": publicFormURL,
	})
}

func (h *ArrivalHandler) UpdateStudentDeadline(c *fiber.Ctx) error {
	type reqBody struct {
		StudentDeadlineAt *string `json:"student_deadline_at"`
	}

	var req reqBody
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}

	var setting models.StudentArrivalSetting
	if err := h.db.FirstOrCreate(&setting, models.StudentArrivalSetting{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal menyimpan pengaturan"})
	}

	if req.StudentDeadlineAt == nil || strings.TrimSpace(*req.StudentDeadlineAt) == "" {
		setting.StudentDeadlineAt = nil
		if err := h.db.Save(&setting).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal mengosongkan batas waktu"})
		}
		return c.JSON(fiber.Map{"message": "Batas waktu terakhir datang santri berhasil dikosongkan.", "student_deadline_at": nil})
	}

	parsed, err := parseFlexibleDateTime(*req.StudentDeadlineAt)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{Error: "validation_error", Message: "student_deadline_at tidak valid"})
	}
	parsed = parsed.Truncate(time.Minute)
	setting.StudentDeadlineAt = &parsed
	if err := h.db.Save(&setting).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal menyimpan batas waktu"})
	}

	return c.JSON(fiber.Map{"message": "Batas waktu terakhir datang santri berhasil disimpan.", "student_deadline_at": setting.StudentDeadlineAt})
}

func (h *ArrivalHandler) ResetStudentArrivals(c *fiber.Ctx) error {
	deleted := int64(0)
	if err := h.db.Model(&models.StudentArrival{}).Where("1 = 1").Count(&deleted).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal membaca data kedatangan"})
	}

	if err := h.db.Where("1 = 1").Delete(&models.StudentArrival{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal reset data kedatangan santri"})
	}

	if err := h.db.Model(&models.StudentArrivalSetting{}).Where("1 = 1").Update("student_deadline_at", nil).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "server_error", Message: "Gagal mengosongkan batas waktu"})
	}

	return c.JSON(fiber.Map{
		"message":      "Reset data kedatangan santri selesai.",
		"deleted_rows": deleted,
	})
}

func isValidArrivalToken(token string) bool {
	valid := strings.TrimSpace(os.Getenv("STUDENT_ARRIVAL_ACCESS_TOKEN"))
	return valid != "" && valid == strings.TrimSpace(token)
}

func parseFlexibleDateTime(value string) (time.Time, error) {
	v := strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fiber.NewError(fiber.StatusBadRequest, "invalid datetime")
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func normalizeGenderCode(value string) *string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, "_", "")
	v = strings.ReplaceAll(v, " ", "")

	switch v {
	case "l", "lakilaki", "male", "banin", "putra":
		g := "L"
		return &g
	case "p", "perempuan", "female", "banat", "putri":
		g := "P"
		return &g
	default:
		return nil
	}
}

func formatGenderLabel(genderCode *string) *string {
	if genderCode == nil {
		return nil
	}
	if *genderCode == "L" {
		v := "Banin (L)"
		return &v
	}
	if *genderCode == "P" {
		v := "Banat (P)"
		return &v
	}
	return nil
}

func applyStudentGenderFilter(db *gorm.DB, genderCode string) *gorm.DB {
	if strings.TrimSpace(genderCode) == "L" {
		return db.Where("LOWER(REPLACE(REPLACE(REPLACE(TRIM(students.jenis_kelamin), '-', ''), '_', ''), ' ', '')) IN ?", []string{"l", "lakilaki", "banin", "putra", "male"})
	}
	if strings.TrimSpace(genderCode) == "P" {
		return db.Where("LOWER(REPLACE(REPLACE(REPLACE(TRIM(students.jenis_kelamin), '-', ''), '_', ''), ' ', '')) IN ?", []string{"p", "perempuan", "banat", "putri", "female"})
	}
	return db
}

func isLateStudentArrival(createdAt time.Time, studentDeadlineAt *time.Time) bool {
	if studentDeadlineAt == nil {
		return false
	}
	return createdAt.After(*studentDeadlineAt)
}

func lateDaysForStudentArrival(createdAt time.Time, studentDeadlineAt *time.Time) int {
	if studentDeadlineAt == nil {
		return 0
	}
	if !createdAt.After(*studentDeadlineAt) {
		return 0
	}
	lateSeconds := createdAt.Sub(*studentDeadlineAt).Seconds()
	return int(math.Max(1, math.Ceil(lateSeconds/86400)))
}
