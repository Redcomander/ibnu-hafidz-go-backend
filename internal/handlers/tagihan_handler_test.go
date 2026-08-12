package handlers

import (
	"testing"

	"github.com/ibnu-hafidz/web-v2/internal/models"
)

func TestMapTagihanExcelHeaders(t *testing.T) {
	headers := mapExcelHeaders([]string{"nis", "nama", "no_whatsapp", "total_tagihan", "sumber_data", "catatan"})
	if _, ok := headers["total_tagihan"]; !ok {
		t.Fatal("total_tagihan header should be mapped")
	}
	if _, ok := headers["sumber_data"]; !ok {
		t.Fatal("sumber_data header should be mapped")
	}
}

func TestNormalizeTagihanTotal(t *testing.T) {
	if got := normalizeCurrencyValue("Rp. 1.250.000"); got != 1250000 {
		t.Fatalf("expected 1250000, got %v", got)
	}
	if got := normalizeCurrencyValue("1,250,000"); got != 1250000 {
		t.Fatalf("expected 1250000, got %v", got)
	}
}

func TestRenderTagihanTemplateMessageSupportsStatusAndAmount(t *testing.T) {
	item := models.Tagihan{
		Nama:          "Ahmad",
		NIS:           strPtr("24001"),
		NoWhatsapp:    "081234567890",
		TotalTagihan:  1250000,
		StatusTagihan: "tertunggak",
	}

	msg := renderTagihanTemplateMessage("Halo {nama}, tagihan {total_tagihan}, status {status_tagihan}, NIS {nis}", item)
	if msg == "" || msg == "Halo {nama}, tagihan {total_tagihan}, status {status_tagihan}, NIS {nis}" {
		t.Fatal("template placeholders were not replaced")
	}
	if got := msg; got != "Halo Ahmad, tagihan Rp 1250000, status tertunggak, NIS 24001" {
		t.Fatalf("unexpected rendered message: %q", got)
	}
}
