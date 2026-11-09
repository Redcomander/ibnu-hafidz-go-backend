// Package maintenance stores maintenance configuration without schema changes.
package maintenance

import (
	"encoding/json"
	"errors"

	"github.com/ibnu-hafidz/web-v2/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const Key = "maintenance_mode"
const DefaultMessage = "Kami sedang melakukan pemeliharaan sistem. Silakan kembali beberapa saat lagi."

type Settings struct {
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

func IsAdmin(user *models.User) bool {
	return user != nil && (user.HasRole("super_admin") || user.HasRole("admin"))
}

func Read(db *gorm.DB) (Settings, error) {
	settings := Settings{Scope: "all", Message: DefaultMessage}
	var row models.CmsSetting
	err := db.Where("`key` = ?", Key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal([]byte(row.Value), &settings); err != nil {
		return settings, err
	}
	if settings.Scope != "all" && settings.Scope != "public" {
		return settings, errors.New("invalid maintenance scope")
	}
	return settings, nil
}

func Save(db *gorm.DB, settings Settings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	row := models.CmsSetting{Key: Key, Value: string(data)}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&row).Error
}
