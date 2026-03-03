package models

import (
	"time"

	"gorm.io/gorm"
)

type LaundryVendor struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name              string         `gorm:"size:255;not null" json:"name"`
	Phone             string         `gorm:"size:20" json:"phone"`
	Address           string         `gorm:"type:text" json:"address"`
	GenderType        string         `gorm:"type:enum('banin','banat');not null" json:"gender_type"`
	Active            bool           `gorm:"default:true" json:"active"`
	WeeklyTotalKg     float64        `gorm:"type:decimal(10,2);default:0" json:"weekly_total_kg"`
	WeeklyTotalRupiah float64        `gorm:"type:decimal(15,2);default:0" json:"weekly_total_rupiah"`

	Accounts []LaundryAccount `gorm:"foreignKey:VendorID" json:"accounts,omitempty"`
}

type LaundryAccount struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	UserID            *uint          `json:"user_id,omitempty"`    // Nullable, refers to system users (teachers)
	StudentID         *uint          `json:"student_id,omitempty"` // Nullable, refers to students
	VendorID          uint           `gorm:"not null" json:"vendor_id"`
	NomorLaundry      string         `gorm:"size:50;not null;uniqueIndex" json:"nomor_laundry"`
	TotalKg           float64        `gorm:"type:decimal(10,2);default:0" json:"total_kg"`
	TotalTransactions int            `gorm:"default:0" json:"total_transactions"`
	Active            bool           `gorm:"default:true" json:"active"`
	Blocked           bool           `gorm:"default:false" json:"blocked"`

	User         *User                `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Student      *Student             `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	Vendor       *LaundryVendor       `gorm:"foreignKey:VendorID" json:"vendor,omitempty"`
	Transactions []LaundryTransaction `gorm:"foreignKey:LaundryAccountID" json:"transactions,omitempty"`
}

type LaundryTransaction struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	LaundryAccountID uint           `gorm:"not null" json:"laundry_account_id"`
	Tanggal          time.Time      `gorm:"type:date;not null" json:"tanggal"`
	BeratKg          float64        `gorm:"type:decimal(8,2);not null" json:"berat_kg"`
	HargaPerKg       float64        `gorm:"type:decimal(10,2);not null" json:"harga_per_kg"`
	TotalHarga       float64        `gorm:"type:decimal(12,2);not null" json:"total_harga"`
	Catatan          *string        `gorm:"type:text" json:"catatan,omitempty"`
	Status           string         `gorm:"type:enum('pending','picked_up');default:'pending';not null" json:"status"`
	PickedUpAt       *time.Time     `json:"picked_up_at,omitempty"`
	PickedUpByID     *uint          `json:"picked_up_by_id,omitempty"`

	Account    *LaundryAccount `gorm:"foreignKey:LaundryAccountID" json:"account,omitempty"`
	PickedUpBy *User           `gorm:"foreignKey:PickedUpByID" json:"picked_up_by,omitempty"`
}

type LaundryLog struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      *uint     `json:"user_id"`
	Action      string    `gorm:"size:100;not null" json:"action"`
	Description string    `gorm:"type:text;not null" json:"description"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
