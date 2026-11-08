package models

import "time"

// Only a SHA-256 digest is stored; the random bearer token exists in the cookie
// only. Consumed rows stay until expiry to detect replay and revoke the family.
type RefreshSession struct {
	TokenHash      string     `gorm:"size:64;primaryKey" json:"-"`
	FamilyID       string     `gorm:"size:64;not null;index" json:"-"`
	UserID         uint       `gorm:"not null;index" json:"-"`
	CredentialHash string     `gorm:"size:64;not null" json:"-"`
	ExpiresAt      time.Time  `gorm:"not null;index" json:"-"`
	ConsumedAt     *time.Time `json:"-"`
	RevokedAt      *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"-"`
}
