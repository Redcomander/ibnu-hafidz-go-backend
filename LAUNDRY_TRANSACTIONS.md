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

## Jika tambah transaksi mendapat HTTP 503

Pada versi awal, middleware menyamarkan 503 ini menjadi `server_error` / `Internal Server Error`. Jalur pembuatan transaksi mengembalikan 503 ketika tabel pencatatan penginput belum tersedia. Versi perbaikan mengirim kode aman `laundry_not_ready` beserta ID permintaan, tanpa membuka SQL atau detail internal.

Periksa database yang dipakai proses backend (gunakan environment runtime yang sama), bukan hanya `.env` checkout. Jika migrasi 003 belum diterapkan, ikuti persiapan deployment di atas: backup terbaru, uji restore/rehearsal, lalu `laundry-inputs` dengan nama database yang benar dan `status`, sebelum deploy ulang. Migrasi bersifat tambahan; jangan menghapus tabel/history atau melewati pencatatan penginput untuk menghilangkan 503. Jika status sudah versi 3 tetapi tetap 503, bandingkan target database runtime dan CLI, izin database, serta ID permintaan pada log produksi sebelum mengulang migrasi.

### Pilihan operator: lanjut tanpa uji restore

Khusus `laundry-inputs`, operator dapat memilih backup gzip yang belum diuji restore dengan `-backup-without-restore=/absolute/backup.sql.gz` sebagai pengganti `-backup-verified`. CLI membaca seluruh gzip untuk memeriksa integritas dan menolak file kosong/rusak. Ini bukan pembuktian bahwa dump dapat di-restore atau berasal dari target yang benar. Nama database tetap wajib dikonfirmasi. Opsi ini tidak berlaku untuk baseline/mobile-releases dan tidak dapat digabung dengan `-backup-verified`. Pemilihan ini ditulis pada output CLI; tidak mengubah aturan history, checksum, advisory lock, atau sifat tambahan migrasi 003.

```bash
go run ./cmd/migrate -command=laundry-inputs -confirm-database=ibnuhafi_database -backup-without-restore="$HOME/private/backups/before-laundry-003-20261116-151807.sql.gz"
go run ./cmd/migrate -command=status
```

Gunakan environment runtime backend dan pastikan dump tersebut adalah backup target yang dimaksud sebelum menjalankan perintah. Tidak ada restore yang dijalankan oleh opsi ini.
