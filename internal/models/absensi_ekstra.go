package models

import (
	"time"

	"gorm.io/gorm"
)

type AbsensiEkstraGroup struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	NamaGrup     string    `gorm:"type:varchar(255);not null" json:"nama_grup"`
	Deskripsi    string    `gorm:"type:text" json:"deskripsi"`
	JenisKelamin string    `gorm:"type:enum('putra','putri','campur');default:'campur'" json:"jenis_kelamin"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedByID  uint      `gorm:"column:created_by" json:"created_by_id"`
	CreatedBy    *User     `gorm:"foreignKey:CreatedByID;references:ID" json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Students    []*Student              `gorm:"many2many:absensi_ekstra_group_students;" json:"students,omitempty"`
	Supervisors []*User                 `gorm:"many2many:absensi_ekstra_group_supervisors;" json:"supervisors,omitempty"`
	Sessions    []*AbsensiEkstraSession `gorm:"foreignKey:AbsensiEkstraGroupID" json:"sessions,omitempty"`
}

type AbsensiEkstraGroupStudent struct {
	AbsensiEkstraGroupID uint       `gorm:"primaryKey" json:"absensi_ekstra_group_id"`
	StudentID            uint       `gorm:"primaryKey" json:"student_id"`
	TanggalMasuk         time.Time  `gorm:"type:date" json:"tanggal_masuk"`
	TanggalKeluar        *time.Time `gorm:"type:date" json:"tanggal_keluar"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// Ensure table name matches exactly what was expected, even though gorm infers.
func (AbsensiEkstraGroupStudent) TableName() string {
	return "absensi_ekstra_group_students"
}

type AbsensiEkstraGroupSupervisor struct {
	AbsensiEkstraGroupID uint      `gorm:"primaryKey" json:"absensi_ekstra_group_id"`
	UserID               uint      `gorm:"primaryKey" json:"user_id"`
	Role                 string    `gorm:"type:enum('admin','pengawas');not null" json:"role"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (AbsensiEkstraGroupSupervisor) TableName() string {
	return "absensi_ekstra_group_supervisors"
}

type AbsensiEkstraSession struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	AbsensiEkstraGroupID uint       `gorm:"not null" json:"absensi_ekstra_group_id"`
	NamaSesi             string     `gorm:"type:varchar(255);not null" json:"nama_sesi"`
	Tanggal              time.Time  `gorm:"type:date;not null" json:"tanggal"`
	WaktuMulai           *string    `gorm:"type:time" json:"waktu_mulai"`
	WaktuSelesai         *string    `gorm:"type:time" json:"waktu_selesai"`
	BatasWaktu           *time.Time `gorm:"type:datetime" json:"batas_waktu"`
	Status               string     `gorm:"type:enum('open','closed');default:'open'" json:"status"`
	CreatedByID          uint       `gorm:"column:created_by;not null" json:"created_by_id"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

	// Relationships
	Group     *AbsensiEkstraGroup           `gorm:"foreignKey:AbsensiEkstraGroupID" json:"group,omitempty"`
	CreatedBy *User                         `gorm:"foreignKey:CreatedByID" json:"created_by,omitempty"`
	Records   []*AbsensiEkstraSessionRecord `gorm:"foreignKey:AbsensiEkstraSessionID;constraint:OnDelete:CASCADE" json:"records,omitempty"`
}

type AbsensiEkstraSessionRecord struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	AbsensiEkstraSessionID uint      `gorm:"not null" json:"absensi_ekstra_session_id"`
	StudentID              uint      `gorm:"not null" json:"student_id"`
	Kehadiran              string    `gorm:"type:enum('hadir','izin','sakit','alpha','terlambat');default:'alpha'" json:"kehadiran"`
	Keterangan             string    `gorm:"type:text" json:"keterangan"`
	SubmittedByID          uint      `gorm:"column:submitted_by" json:"submitted_by_id"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`

	// Relationships
	Session     *AbsensiEkstraSession `gorm:"foreignKey:AbsensiEkstraSessionID" json:"session,omitempty"`
	Student     *Student              `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	SubmittedBy *User                 `gorm:"foreignKey:SubmittedByID" json:"submitted_by,omitempty"`
}

// Function to calculate counts on demand since they are accessed often.
func (s *AbsensiEkstraSession) GetStatistics(db *gorm.DB) map[string]interface{} {
	var hadir, izin, sakit, alpha, terlambat int64
	var total int64

	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ?", s.ID).Count(&total)

	if total == 0 {
		return map[string]interface{}{
			"total":      0,
			"hadir":      0,
			"izin":       0,
			"sakit":      0,
			"alpha":      0,
			"terlambat":  0,
			"percentage": 0,
		}
	}

	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ? AND kehadiran = ?", s.ID, "hadir").Count(&hadir)
	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ? AND kehadiran = ?", s.ID, "izin").Count(&izin)
	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ? AND kehadiran = ?", s.ID, "sakit").Count(&sakit)
	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ? AND kehadiran = ?", s.ID, "alpha").Count(&alpha)
	db.Model(&AbsensiEkstraSessionRecord{}).Where("absensi_ekstra_session_id = ? AND kehadiran = ?", s.ID, "terlambat").Count(&terlambat)

	return map[string]interface{}{
		"total":      total,
		"hadir":      hadir,
		"izin":       izin,
		"sakit":      sakit,
		"alpha":      alpha,
		"terlambat":  terlambat,
		"percentage": float64(hadir) / float64(total) * 100,
	}
}
