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

func TestMapFlexibleImportHeaders(t *testing.T) {
	headers := mapExcelHeaders([]string{"nama", "ttl", "alamat", "nama_ayah", "nama_ibu", "no_whatsapp", "asal_sekolah", "jenis_kelamin", "jenjang_pendidikan", "tunggakan", "nis", "catatan"})
	for _, key := range []string{"nama", "ttl", "alamat", "nama_ayah", "nama_ibu", "no_whatsapp", "asal_sekolah", "jenis_kelamin", "jenjang_pendidikan", "tunggakan", "nis", "catatan"} {
		if _, ok := headers[key]; !ok {
			t.Fatalf("header %q should be mapped", key)
		}
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
		Tunggakan:     strPtr("Rp 1.250.000"),
	}

	msg := renderTagihanTemplateMessage("Halo {nama}, tagihan {total_tagihan}, tunggakan {tunggakan}, status {status_tagihan}, NIS {nis}", item)
	if msg == "" || msg == "Halo {nama}, tagihan {total_tagihan}, tunggakan {tunggakan}, status {status_tagihan}, NIS {nis}" {
		t.Fatal("template placeholders were not replaced")
	}
	if got := msg; got != "Halo Ahmad, tagihan Rp 1.250.000, tunggakan Rp 1.250.000, status tertunggak, NIS 24001" {
		t.Fatalf("unexpected rendered message: %q", got)
	}
}

func TestRenderTagihanTemplateMessageFormatsNumericTunggakanAsRupiah(t *testing.T) {
	item := models.Tagihan{
		Nama:          "Budi",
		NIS:           strPtr("24002"),
		NoWhatsapp:    "081234567891",
		TotalTagihan:  1250000,
		StatusTagihan: "tertunggak",
		Tunggakan:     strPtr("1250000"),
	}

	msg := renderTagihanTemplateMessage("Halo {nama}, tunggakan {tunggakan}, total {total_tagihan}, status {status_tagihan}", item)
	if got := msg; got != "Halo Budi, tunggakan Rp 1.250.000, total Rp 1.250.000, status tertunggak" {
		t.Fatalf("expected formatted rupiah, got %q", got)
	}
}

func TestRenderTemplateMessageFormatsStudentTunggakanAsRupiah(t *testing.T) {
	kontak := models.Kontak{
		Nama:              "Citra",
		NIS:               strPtr("24003"),
		NoWhatsapp:        "081234567892",
		TTL:               strPtr("Banyumas, 10-05-2010"),
		Alamat:            strPtr("Jl. Mawar 15"),
		NamaAyah:          strPtr("Pak Budi"),
		NamaIbu:           strPtr("Ibu Sari"),
		AsalSekolah:       strPtr("SMPN 1 Banyumas"),
		JenisKelamin:      strPtr("Perempuan"),
		JenjangPendidikan: strPtr("SMP"),
		StatusKontak:      "aktif",
		SumberData:        strPtr("excel"),
		Tunggakan:         strPtr("2500000"),
	}

	msg := renderTemplateMessage("Nama: {nama}\nTTL: {ttl}\nAlamat: {alamat}\nAyah: {nama_ayah}\nIbu: {nama_ibu}\nWA: {no_whatsapp}\nSekolah: {asal_sekolah}\nJK: {jenis_kelamin}\nJenjang: {jenjang_pendidikan}\nTunggakan: {tunggakan}", kontak)
	if got := msg; got != "Nama: Citra\nTTL: Banyumas, 10-05-2010\nAlamat: Jl. Mawar 15\nAyah: Pak Budi\nIbu: Ibu Sari\nWA: 081234567892\nSekolah: SMPN 1 Banyumas\nJK: Perempuan\nJenjang: SMP\nTunggakan: Rp 2.500.000" {
		t.Fatalf("unexpected formatted student message: %q", got)
	}
}
