package handlers

import (
	"context"
	"strings"

	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

type logCountryLookup func(context.Context, []string) map[string]string

// Enrich only the rows being viewed, and preserve countries supplied by the
// original request. Persist successful matches so old and new logs both benefit.
func enrichLogCountries(ctx context.Context, db *gorm.DB, logs []models.UserActivityLog, lookup logCountryLookup) {
	ips := []string{}
	for _, row := range logs {
		if (row.CountryCode == nil || strings.TrimSpace(*row.CountryCode) == "") && row.IPAddress != nil {
			ips = append(ips, *row.IPAddress)
		}
	}
	if len(ips) == 0 {
		return
	}
	countries := lookup(ctx, ips)
	idsByCountry := make(map[string][]uint)
	for i := range logs {
		row := &logs[i]
		if row.CountryCode != nil && strings.TrimSpace(*row.CountryCode) != "" || row.IPAddress == nil {
			continue
		}
		code := countries[utils.PublicIP(*row.IPAddress)]
		if code == "" {
			continue
		}
		row.CountryCode = &code
		idsByCountry[code] = append(idsByCountry[code], row.ID)
	}
	for code, ids := range idsByCountry {
		db.Model(&models.UserActivityLog{}).Where("id IN ? AND (country_code IS NULL OR country_code = '')", ids).Update("country_code", code)
	}
}
