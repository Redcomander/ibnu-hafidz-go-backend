package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRevitalisasiTukangIncludesGajiHarian(t *testing.T) {
	payload := RevitalisasiTukang{
		Name:       "Budi",
		GajiHarian: 250000,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if !strings.Contains(string(data), "\"gaji_harian\":250000") {
		t.Fatalf("expected gaji_harian in payload, got %s", string(data))
	}
}
