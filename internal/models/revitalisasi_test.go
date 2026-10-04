package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRevitalisasiTukangIncludesGajiHarianAndKasbon(t *testing.T) {
	payload := RevitalisasiTukang{
		Name:       "Budi",
		GajiHarian: 250000,
		Kasbon:     50000,
		CaraPotong: "angsuran",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if !strings.Contains(string(data), "\"gaji_harian\":250000") {
		t.Fatalf("expected gaji_harian in payload, got %s", string(data))
	}
	if !strings.Contains(string(data), "\"kasbon\":50000") {
		t.Fatalf("expected kasbon in payload, got %s", string(data))
	}
	if !strings.Contains(string(data), "\"cara_potong\":\"angsuran\"") {
		t.Fatalf("expected cara_potong in payload, got %s", string(data))
	}
}
