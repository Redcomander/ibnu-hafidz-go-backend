# Filter, transaksi mirip, dan penginput laundry

## Perubahan

- GET `/api/laundry/transactions?vendor_id=...` memakai vendor historis transaksi. Untuk transaksi lama tanpa vendor snapshot, digunakan vendor akun saat ini. Filter tetap dapat digabung dengan pencarian, status, tanggal, serta pagination.
- `same_day_count` menghitung transaksi yang belum dihapus untuk tanggal dan akun/pemilik yang sama. Identitas memakai ID santri atau pengguna, lalu ID akun jika tidak memiliki pemilik. Nama yang sama bukan bukti orang yang sama. Hitungan mencakup hasil di vendor/status/halaman lain; sorotan adalah informasi pemeriksaan, bukan penetapan duplikat.
- `input_by` menampilkan identitas penginput yang diambil dari sesi server, bukan dari payload klien. Snapshot nama disimpan dalam tabel tambahan `laundry_transaction_inputs` dalam transaksi database yang sama dengan setoran dan perubahan total akun. Pengeditan/pengambilan tidak mengganti penginput awal.
- Transaksi lama tetap utuh dan ditampilkan sebagai **Belum tercatat**. Tidak ada penebakan identitas dari log teks ataupun backfill.

## Persiapan deployment

Migrasi `003` hanya membuat tabel tambahan dan metadata migrasi; tidak mengubah kolom atau baris bisnis lama. Tidak dijalankan saat startup. Membutuhkan baseline dan migrasi `mobile-releases` sebelumnya.

1. Siapkan backup terbaru dan uji restore pada database salinan.
2. Gunakan kode backend terbaru untuk menjalankan rehearsal pada database salinan, lalu bandingkan tabel/data lama.
3. Setelah rehearsal berhasil, jalankan perintah berikut pada target yang benar, menggunakan environment runtime backend dan nama database yang telah diverifikasi:

```bash
go run ./cmd/migrate -command=laundry-inputs -backup-verified -confirm-database=NAMA_DATABASE_TARGET
go run ./cmd/migrate -command=status
```

4. Deploy backend kemudian frontend dengan prosedur hosting yang sudah digunakan. Pertahankan konfigurasi proxy API pada `.htaccess` produksi.
5. Uji transaksi baru, penginput, edit tanpa perubahan penginput, filter vendor, serta dua transaksi akun/pemilik yang sama pada tanggal sama.

Jika migrasi belum diterapkan, daftar transaksi masih bisa dibaca; pembuatan transaksi baru memberi 503 agar tidak ada setoran baru tanpa pencatatan penginput. Sesudah migrasi versi 3, binary lama yang hanya mengenal versi 1/2 akan menolak startup. Gunakan perbaikan kode yang kompatibel untuk rollback fitur; jangan menghapus metadata/tabel migrasi secara manual.

## Verifikasi lokal

Tes backend memeriksa filter vendor historis/fallback, transaksi mirip lintas filter dan pagination, orang berbeda bernama sama, hari berbeda, soft delete, sumber penginput dari sesi, dan rollback atomik saat pencatatan gagal. Tes migrasi SQLite memeriksa data/definisi lama dan history dirty/checksum. Jalur migrasi MySQL/MariaDB diuji oleh integration test opsional dalam database terisolasi ketika environment pengujian MySQL tersedia; rehearsal hosting tetap diperlukan. Database produksi tidak disentuh dari sesi pengembangan ini.
