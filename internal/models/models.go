package models

import (
	"time"

	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────────────────
// User — matches Laravel `users` table exactly
// ──────────────────────────────────────────────────────────────
type User struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Name            string         `gorm:"column:name;size:255;not null" json:"name"`
	Username        string         `gorm:"column:username;size:255;uniqueIndex" json:"username"`
	Email           string         `gorm:"column:email;size:255;uniqueIndex;not null" json:"email"`
	Gender          *string        `gorm:"column:gender;size:10" json:"gender,omitempty"`
	EmailVerifiedAt *time.Time     `gorm:"column:email_verified_at" json:"email_verified_at,omitempty"`
	Password        string         `gorm:"column:password;size:255;not null" json:"-"`
	FotoGuru        *string        `gorm:"column:foto_guru;size:255" json:"foto_guru,omitempty"`
	LastActivity    *time.Time     `gorm:"column:last_activity" json:"last_activity,omitempty"`
	RememberToken   *string        `gorm:"column:remember_token;size:100" json:"-"`
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	// Relationships
	Roles []Role `gorm:"many2many:role_user;" json:"roles,omitempty"`
}

func (User) TableName() string { return "users" }

// HasPermission checks if user has a specific permission through any of their roles
func (u *User) HasPermission(permissionName string) bool {
	for _, role := range u.Roles {
		for _, perm := range role.Permissions {
			if perm.Name == permissionName {
				return true
			}
		}
	}
	return false
}

// HasRole checks if user has a specific role
func (u *User) HasRole(roleName string) bool {
	for _, role := range u.Roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}

// ──────────────────────────────────────────────────────────────
// Role — matches Laravel `roles` table exactly
// ──────────────────────────────────────────────────────────────
type Role struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"column:name;size:255;uniqueIndex;not null" json:"name"`
	DisplayName  *string   `gorm:"column:display_name;size:255" json:"display_name,omitempty"`
	Description  *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	Color        *string   `gorm:"column:color;size:20" json:"color,omitempty"`
	IsSystemRole bool      `gorm:"column:is_system_role;default:false" json:"is_system_role"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
	UsersCount   int64     `gorm:"->;dataType:int" json:"users_count"` // Virtual field for counting users

	// Relationships
	Permissions []Permission `gorm:"many2many:role_permission;" json:"permissions,omitempty"`
	Users       []User       `gorm:"many2many:role_user;" json:"-"`
}

func (Role) TableName() string { return "roles" }

// ──────────────────────────────────────────────────────────────
// Permission — matches Laravel `permissions` table exactly
// ──────────────────────────────────────────────────────────────
type Permission struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"column:name;size:255;uniqueIndex;not null" json:"name"`
	DisplayName *string   `gorm:"column:display_name;size:255" json:"display_name,omitempty"`
	Description *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	Category    *string   `gorm:"column:category;size:255" json:"category,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relationships
	Roles []Role `gorm:"many2many:role_permission;" json:"-"`
}

func (Permission) TableName() string { return "permissions" }

// ──────────────────────────────────────────────────────────────
// Student — matches Laravel `students` table exactly
// All column names use Indonesian naming from Laravel schema.
// ──────────────────────────────────────────────────────────────
type Student struct {
	ID                       uint    `gorm:"primaryKey" json:"id"`
	JenisKelamin             *string `gorm:"column:jenis_kelamin;size:20" json:"jenis_kelamin,omitempty"`
	StatusPeriode            *string `gorm:"column:status_periode;size:50" json:"status_periode,omitempty"`
	JenjangPendidikanPeriode *string `gorm:"column:jenjang_pendidikan_periode;size:100" json:"jenjang_pendidikan_periode,omitempty"`
	NamaLengkap              string  `gorm:"column:nama_lengkap;size:255;not null" json:"nama_lengkap"`
	TempatLahir              *string `gorm:"column:tempat_lahir;size:255" json:"tempat_lahir,omitempty"`
	TanggalLahir             *string `gorm:"column:tanggal_lahir;size:20" json:"tanggal_lahir,omitempty"`
	NISN                     *string `gorm:"column:nisn;size:50" json:"nisn,omitempty"`
	NIK                      *string `gorm:"column:nik;size:50" json:"nik,omitempty"`
	NoKK                     *string `gorm:"column:no_kk;size:50" json:"no_kk,omitempty"`
	NoAkte                   *string `gorm:"column:no_akte;size:50" json:"no_akte,omitempty"`
	Alamat                   *string `gorm:"column:alamat;type:text" json:"alamat,omitempty"`
	RT                       *string `gorm:"column:rt;size:10" json:"rt,omitempty"`
	RW                       *string `gorm:"column:rw;size:10" json:"rw,omitempty"`
	DesaKelurahan            *string `gorm:"column:desa_kelurahan;size:255" json:"desa_kelurahan,omitempty"`
	Kecamatan                *string `gorm:"column:kecamatan;size:255" json:"kecamatan,omitempty"`
	KabupatenKota            *string `gorm:"column:kabupaten_kota;size:255" json:"kabupaten_kota,omitempty"`
	Provinsi                 *string `gorm:"column:provinsi;size:255" json:"provinsi,omitempty"`
	KodePos                  *string `gorm:"column:kode_pos;size:10" json:"kode_pos,omitempty"`
	NoHP                     *string `gorm:"column:no_hp;size:50" json:"no_hp,omitempty"`
	AnakKe                   *int    `gorm:"column:anak_ke" json:"anak_ke,omitempty"`
	DariSaudara              *int    `gorm:"column:dari_saudara" json:"dari_saudara,omitempty"`
	SekolahAsal              *string `gorm:"column:sekolah_asal;size:255" json:"sekolah_asal,omitempty"`
	NamaAyah                 *string `gorm:"column:nama_ayah;size:255" json:"nama_ayah,omitempty"`
	NIKAyah                  *string `gorm:"column:nik_ayah;size:50" json:"nik_ayah,omitempty"`
	TahunLahirAyah           *string `gorm:"column:tahun_lahir_ayah;size:10" json:"tahun_lahir_ayah,omitempty"`
	PekerjaanAyah            *string `gorm:"column:pekerjaan_ayah;size:255" json:"pekerjaan_ayah,omitempty"`
	PendidikanAyah           *string `gorm:"column:pendidikan_ayah;size:255" json:"pendidikan_ayah,omitempty"`
	NamaIbu                  *string `gorm:"column:nama_ibu;size:255" json:"nama_ibu,omitempty"`
	NIKIbu                   *string `gorm:"column:nik_ibu;size:50" json:"nik_ibu,omitempty"`
	TahunLahirIbu            *string `gorm:"column:tahun_lahir_ibu;size:10" json:"tahun_lahir_ibu,omitempty"`
	PekerjaanIbu             *string `gorm:"column:pekerjaan_ibu;size:255" json:"pekerjaan_ibu,omitempty"`
	PendidikanIbu            *string `gorm:"column:pendidikan_ibu;size:255" json:"pendidikan_ibu,omitempty"`
	FileKTPOrtu              *string `gorm:"column:file_ktp_ortu;size:255" json:"file_ktp_ortu,omitempty"`
	FileKartuKeluarga        *string `gorm:"column:file_kartu_keluarga;size:255" json:"file_kartu_keluarga,omitempty"`
	FileIjazah               *string `gorm:"column:file_ijazah;size:255" json:"file_ijazah,omitempty"`
	FileSuratKeteranganLulus *string `gorm:"column:file_surat_keterangan_lulus;size:255" json:"file_surat_keterangan_lulus,omitempty"`
	FileAktaKelahiran        *string `gorm:"column:file_akta_kelahiran;size:255" json:"file_akta_kelahiran,omitempty"`
	FileSuratPindah          *string `gorm:"column:file_surat_pindah;size:255" json:"file_surat_pindah,omitempty"`
	FotoSantri               *string `gorm:"column:foto_santri;size:255" json:"foto_santri,omitempty"`
	// Legacy/Duplicate column in some environments
	Name *string `gorm:"column:name;size:255" json:"-"`

	// Relationships
	Kamars  []Kamar `gorm:"many2many:kamar_siswa;" json:"kamars,omitempty"`
	KelasID *uint   `gorm:"column:kelas_id" json:"kelas_id"`
	Kelas   *Kelas  `gorm:"foreignKey:KelasID" json:"kelas,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Student) TableName() string { return "students" }

// BeforeSave hook ensures Name is populated from NamaLengkap if missing
func (s *Student) BeforeSave(tx *gorm.DB) (err error) {
	if s.Name == nil && s.NamaLengkap != "" {
		s.Name = &s.NamaLengkap
	}
	return
}

// ──────────────────────────────────────────────────────────────
// Request / Response DTOs
// ──────────────────────────────────────────────────────────────

// LoginRequest for auth handler
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required,min=6"`
}

// AuthResponse returned after successful login
type AuthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	User        User   `json:"user"`
}

// PaginatedResponse for list endpoints with search/filter/sort
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

// ErrorResponse for API errors
type ErrorResponse struct {
	Error   string      `json:"error"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// ──────────────────────────────────────────────────────────────
// CmsSetting — key/value store for editable public content.
// Keys: "home_content", "profile_content" (JSON blobs).
// When a key is absent the backend falls back to hardcoded defaults.
// ──────────────────────────────────────────────────────────────
type CmsSetting struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"column:key;size:100;uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"column:value;type:longtext;not null" json:"value"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (CmsSetting) TableName() string { return "cms_settings" }

// ──────────────────────────────────────────────────────────────
// Visitor — for dashboard analytics
// ──────────────────────────────────────────────────────────────
type Visitor struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	IPAddress *string   `gorm:"size:255" json:"ip_address"`
	UserAgent *string   `gorm:"type:text" json:"user_agent"`
	CreatedAt time.Time `gorm:"index" json:"created_at"` // Index for fast dashboard stats
	UpdatedAt time.Time `json:"updated_at"`
}

func (Visitor) TableName() string { return "visitors" }

// ──────────────────────────────────────────────────────────────
// Kamar — matches Laravel `kamars` table
// ──────────────────────────────────────────────────────────────
type Kamar struct {
	ID          uint    `gorm:"primaryKey" json:"id"`
	NamaKamar   string  `gorm:"column:nama_kamar;size:255;not null" json:"nama_kamar"`
	Kapasitas   int     `gorm:"column:kapasitas;not null;default:0" json:"kapasitas"`
	Keterangan  *string `gorm:"column:keterangan;type:text" json:"keterangan,omitempty"`
	WaliKamarID uint    `gorm:"column:wali_kamar_id" json:"wali_kamar_id"`
	WaliKamar   *User   `gorm:"foreignKey:WaliKamarID" json:"wali_kamar,omitempty"`

	// Relationships matching Laravel pivot tables
	Students        []*Student `gorm:"many2many:kamar_siswa;" json:"students,omitempty"`
	SecondaryWalies []*User    `gorm:"many2many:kamar_user;" json:"secondary_walies,omitempty"`

	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Kamar) TableName() string { return "kamars" }

// ──────────────────────────────────────────────────────────────
// Kelas — matches Laravel `kelas` table
// ──────────────────────────────────────────────────────────────
type Kelas struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Nama                string         `gorm:"column:nama;size:255;not null" json:"nama"`
	Tingkat             string         `gorm:"column:tingkat;size:50;not null" json:"tingkat"`
	Gender              *string        `gorm:"column:gender;type:enum('banin', 'banat')" json:"gender,omitempty"`
	WaliKelasID         *uint          `gorm:"column:wali_kelas_id" json:"wali_kelas_id"`
	WaliKelas           *User          `gorm:"foreignKey:WaliKelasID" json:"wali_kelas,omitempty"`
	WaliKelasDiniyyahID *uint          `gorm:"column:wali_kelas_diniyyah_id" json:"wali_kelas_diniyyah_id"`
	WaliKelasDiniyyah   *User          `gorm:"foreignKey:WaliKelasDiniyyahID" json:"wali_kelas_diniyyah,omitempty"`
	Students            []Student      `gorm:"foreignKey:KelasID" json:"students,omitempty"`
	CreatedAt           time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Kelas) TableName() string { return "kelas" }
