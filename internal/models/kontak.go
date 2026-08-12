package models

import (
	"time"

	"gorm.io/gorm"
)

// ImportBatch stores one Excel import execution summary.
type ImportBatch struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Filename     string         `gorm:"size:255;not null" json:"filename"`
	TotalRows    int            `gorm:"default:0" json:"total_rows"`
	InsertedRows int            `gorm:"default:0" json:"inserted_rows"`
	UpdatedRows  int            `gorm:"default:0" json:"updated_rows"`
	SkippedRows  int            `gorm:"default:0" json:"skipped_rows"`
	ImportedByID *uint          `gorm:"index" json:"imported_by_id,omitempty"`
	ImportedBy   *User          `gorm:"foreignKey:ImportedByID" json:"imported_by,omitempty"`
	ImportNotes  *string        `gorm:"type:text" json:"import_notes,omitempty"`
}

func (ImportBatch) TableName() string { return "import_batches" }

// Kontak stores calon santri contact data and follow-up status.
type Kontak struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	NIS               *string        `gorm:"column:nis;size:64;index" json:"nis,omitempty"`
	Nama              string         `gorm:"column:nama;size:255;not null" json:"nama"`
	TTL               *string        `gorm:"column:ttl;size:255" json:"ttl,omitempty"`
	Alamat            *string        `gorm:"column:alamat;type:text" json:"alamat,omitempty"`
	AlamatLengkap     *string        `gorm:"column:alamat_lengkap;type:text" json:"alamat_lengkap,omitempty"`
	NamaAyah          *string        `gorm:"column:nama_ayah;size:255" json:"nama_ayah,omitempty"`
	NamaIbu           *string        `gorm:"column:nama_ibu;size:255" json:"nama_ibu,omitempty"`
	NoWhatsapp        string         `gorm:"column:no_whatsapp;size:32;not null;index" json:"no_whatsapp"`
	AsalSekolah       *string        `gorm:"column:asal_sekolah;size:255" json:"asal_sekolah,omitempty"`
	JenisKelamin      *string        `gorm:"column:jenis_kelamin;size:50" json:"jenis_kelamin,omitempty"`
	JenjangPendidikan *string        `gorm:"column:jenjang_pendidikan;size:100" json:"jenjang_pendidikan,omitempty"`
	Tunggakan         *string        `gorm:"column:tunggakan;size:100" json:"tunggakan,omitempty"`
	StatusKontak      string         `gorm:"column:status_kontak;size:50;not null;default:'baru';index" json:"status_kontak"`
	HandlerID         *uint          `gorm:"column:handler_id;index" json:"handler_id,omitempty"`
	Handler           *User          `gorm:"foreignKey:HandlerID" json:"handler,omitempty"`
	SumberData        *string        `gorm:"column:sumber_data;size:100" json:"sumber_data,omitempty"`
	Catatan           *string        `gorm:"column:catatan;type:text" json:"catatan,omitempty"`
	LastContactAt     *time.Time     `gorm:"column:last_contact_at" json:"last_contact_at,omitempty"`
	ImportBatchID     *uint          `gorm:"column:import_batch_id;index" json:"import_batch_id,omitempty"`
	ImportBatch       *ImportBatch   `gorm:"foreignKey:ImportBatchID" json:"import_batch,omitempty"`
}

func (Kontak) TableName() string { return "kontak" }

// TemplatePesan stores reusable WhatsApp message templates.
type TemplatePesan struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Nama      string         `gorm:"column:nama;size:120;not null" json:"nama"`
	Konten    string         `gorm:"column:konten;type:text;not null" json:"konten"`
	Aktif     bool           `gorm:"column:aktif;default:true" json:"aktif"`
}

func (TemplatePesan) TableName() string { return "template_pesan" }

// RiwayatKontak stores interaction history for each contact.
type RiwayatKontak struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	KontakID        uint           `gorm:"column:kontak_id;not null;index" json:"kontak_id"`
	Kontak          *Kontak        `gorm:"foreignKey:KontakID" json:"kontak,omitempty"`
	TemplatePesanID *uint          `gorm:"column:template_pesan_id;index" json:"template_pesan_id,omitempty"`
	TemplatePesan   *TemplatePesan `gorm:"foreignKey:TemplatePesanID" json:"template_pesan,omitempty"`
	UserID          *uint          `gorm:"column:user_id;index" json:"user_id,omitempty"`
	User            *User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
	StatusAwal      *string        `gorm:"column:status_awal;size:50" json:"status_awal,omitempty"`
	StatusAkhir     *string        `gorm:"column:status_akhir;size:50" json:"status_akhir,omitempty"`
	PesanFinal      *string        `gorm:"column:pesan_final;type:text" json:"pesan_final,omitempty"`
	Catatan         *string        `gorm:"column:catatan;type:text" json:"catatan,omitempty"`
	DikirimVia      *string        `gorm:"column:dikirim_via;size:20" json:"dikirim_via,omitempty"`
}

func (RiwayatKontak) TableName() string { return "riwayat_kontak" }
