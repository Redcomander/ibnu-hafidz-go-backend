package models

import "time"

type MobileAppRelease struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	VersionName  string     `json:"version_name" gorm:"size:80;not null"`
	VersionCode  int64      `json:"version_code" gorm:"uniqueIndex;not null"`
	PackageName  string     `json:"package_name" gorm:"size:200;not null"`
	APKPath      string     `json:"-" gorm:"column:apk_path;size:100;not null"`
	SHA256       string     `json:"sha256" gorm:"column:sha256;size:64;not null"`
	SignerSHA256 string     `json:"signer_sha256" gorm:"size:64;not null"`
	FileSize     int64      `json:"file_size"`
	Changelog    string     `json:"changelog" gorm:"type:text"`
	ForceUpdate  bool       `json:"force_update"`
	Status       string     `json:"status" gorm:"size:20;index"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedBy    uint       `json:"created_by"`
	Uploader     *User      `json:"uploader,omitempty" gorm:"foreignKey:CreatedBy"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (MobileAppRelease) TableName() string { return "mobile_app_releases" }

type MobileReleasePolicy struct {
	ID                          uint  `json:"id" gorm:"primaryKey"`
	CurrentReleaseID            *uint `json:"current_release_id"`
	MinimumSupportedVersionCode int64 `json:"minimum_supported_version_code"`
	HighestVersionCode          int64 `json:"highest_version_code"`
}

func (MobileReleasePolicy) TableName() string { return "mobile_release_policies" }

type MobileReleaseAudit struct {
	ID        uint `gorm:"primaryKey"`
	ReleaseID uint
	ActorID   uint
	Action    string `gorm:"size:80"`
	Details   string `gorm:"type:text"`
	CreatedAt time.Time
}

func (MobileReleaseAudit) TableName() string { return "mobile_release_audits" }
