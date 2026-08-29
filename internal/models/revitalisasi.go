package models

import (
	"time"

	"gorm.io/gorm"
)

// RevitalisasiTukang stores worker master data for the SMA revitalization project.
type RevitalisasiTukang struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:150;not null" json:"name"`
	Divisi    string         `gorm:"size:150" json:"divisi"`
	Area      string         `gorm:"size:150" json:"area"`
	Phone     string         `gorm:"size:50" json:"phone"`
	Note      string         `gorm:"type:text" json:"note"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Absen []RevitalisasiAbsenTukang `gorm:"foreignKey:TukangID" json:"absen,omitempty"`
}

func (RevitalisasiTukang) TableName() string { return "revitalisasi_tukang" }

// RevitalisasiAbsenTukang stores daily attendance for workers.
type RevitalisasiAbsenTukang struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Tanggal   time.Time `gorm:"type:date;not null;index" json:"tanggal"`
	TukangID  uint      `gorm:"not null;index" json:"tukang_id"`
	Status    string    `gorm:"size:20;not null;default:hadir" json:"status"`
	Note      string    `gorm:"type:text" json:"note"`
	PhotoPath *string   `gorm:"size:255" json:"photo_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Tukang *RevitalisasiTukang `gorm:"foreignKey:TukangID" json:"tukang,omitempty"`
}

func (RevitalisasiAbsenTukang) TableName() string { return "revitalisasi_absen_tukang" }

// RevitalisasiNotaMaterial tracks supplier notes and incoming material receipts.
type RevitalisasiNotaMaterial struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Tanggal    time.Time `gorm:"type:date;not null;index" json:"tanggal"`
	NomorNota  string    `gorm:"size:80;not null;index" json:"nomor_nota"`
	Supplier   string    `gorm:"size:150;not null" json:"supplier"`
	Keterangan string    `gorm:"type:text" json:"keterangan"`
	TotalNilai float64   `gorm:"type:decimal(15,2);default:0" json:"total_nilai"`
	PhotoPath  *string   `gorm:"size:255" json:"photo_path,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (RevitalisasiNotaMaterial) TableName() string { return "revitalisasi_nota_material" }

// RevitalisasiNotaMasuk stores cash-in notes for the revitalization project.
type RevitalisasiNotaMasuk struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Tanggal    time.Time `gorm:"type:date;not null;index" json:"tanggal"`
	NomorNota  string    `gorm:"size:80;not null;index" json:"nomor_nota"`
	Sumber     string    `gorm:"size:150;not null" json:"sumber"`
	Jumlah     float64   `gorm:"type:decimal(15,2);default:0" json:"jumlah"`
	Keterangan string    `gorm:"type:text" json:"keterangan"`
	PhotoPath  *string   `gorm:"size:255" json:"photo_path,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (RevitalisasiNotaMasuk) TableName() string { return "revitalisasi_nota_masuk" }

// RevitalisasiMaterialDatang stores incoming material records by item.
type RevitalisasiMaterialDatang struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	Tanggal              time.Time `gorm:"type:date;not null;index" json:"tanggal"`
	NamaMaterial         string    `gorm:"size:150;not null" json:"nama_material"`
	Supplier             string    `gorm:"size:150" json:"supplier"`
	Jumlah               float64   `gorm:"type:decimal(12,2);default:0" json:"jumlah"`
	Satuan               string    `gorm:"size:30" json:"satuan"`
	Catatan              string    `gorm:"type:text" json:"catatan"`
	NomorNotaPengeluaran string    `gorm:"size:80" json:"nomor_nota_pengeluaran"`
	TotalPengeluaran     float64   `gorm:"type:decimal(15,2);default:0" json:"total_pengeluaran"`
	PhotoPath            *string   `gorm:"size:255" json:"photo_path,omitempty"`
	NotaPengeluaranPath  *string   `gorm:"size:255" json:"nota_pengeluaran_path,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (RevitalisasiMaterialDatang) TableName() string { return "revitalisasi_material_datang" }

// RevitalisasiProgresPembangunan stores area progress and visual documentation.
type RevitalisasiProgresPembangunan struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Tanggal    time.Time `gorm:"type:date;not null;index" json:"tanggal"`
	NamaArea   string    `gorm:"size:150;not null" json:"nama_area"`
	Persentase int       `gorm:"not null;default:0" json:"persentase"`
	Catatan    string    `gorm:"type:text" json:"catatan"`
	PhotoPath  *string   `gorm:"size:255" json:"photo_path,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (RevitalisasiProgresPembangunan) TableName() string { return "revitalisasi_progres_pembangunan" }

// RevitalisasiPrioritas stores the project priority list shown on the dashboard.
type RevitalisasiPrioritas struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Judul     string    `gorm:"size:150;not null" json:"judul"`
	Deskripsi string    `gorm:"type:text" json:"deskripsi"`
	Tingkat   string    `gorm:"size:30;default:medium" json:"tingkat"`
	Urutan    int       `gorm:"default:0" json:"urutan"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (RevitalisasiPrioritas) TableName() string { return "revitalisasi_prioritas" }
