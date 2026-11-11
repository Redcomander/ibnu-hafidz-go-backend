# Pembaruan APK privat: Go, Vue dan Flutter

## Arsitektur dan status

- Backend yang ada: Fiber v2, GORM/MySQL, JWT/refresh session dan RBAC.
- Vue menggunakan layout/admin, Axios, toast, serta guard `super_admin` yang ada.
- Flutter menggunakan Provider dan kanal Android tambahan, tanpa WebView.
- Rute web: `/dashboard/mobile-app/releases`, alias `/admin/mobile-app/releases`.
- Tidak ada APK, keystore atau password penandatangan yang disimpan di Git.
- Tidak ada perubahan database produksi atau deployment yang dilakukan oleh agen.

## Sebelum deployment cPanel

1. Simpan dan uji pemulihan backup database serta penyimpanan lama. Rehearsal
   migrasi harus memakai MySQL terpisah hasil restore, bukan database produksi.
2. Siapkan Android Build Tools yang kompatibel di hosting: `aapt`, `apksigner`
   (atau `apksigner.jar`) dan Java 17+. Pastikan binary Linux sesuai arsitektur
   hosting dan dependensi sistem tersedia. Tidak perlu Android emulator.
3. Pilih direktori APK privat di luar semua document root dan di luar repository
   backend. Contoh `/home/ACCOUNT/private/mobile-releases`, mode 0700. Jangan
   gunakan `/uploads`, folder Vite `dist`, `public_html` atau object storage publik.
4. Isi `MOBILE_*` dari [.env.example](.env.example). `MOBILE_PUBLIC_URL` adalah
   origin HTTPS tanpa path, query, user/password atau fragment. Gunakan domain
   yang dipercaya build Flutter (`ibnuhafidz.ponpes.id` secara default).
5. `MOBILE_SIGNER_SHA256` harus fingerprint sertifikat APK yang sudah terpasang.
   Ambil dengan `apksigner verify --print-certs existing-app.apk`. Fingerprint
   publik boleh disimpan di konfigurasi; keystore/password tetap di mesin build.
6. Periksa batas unggah Apache/reverse proxy/Imunify360 serta timeout agar cukup
   untuk `MOBILE_APK_MAX_BYTES` ditambah overhead multipart. Izinkan endpoint
   APK untuk klien Android tanpa tantangan JavaScript. Tetap wajib HTTPS.
7. Pastikan disk cukup untuk APK sementara, hasil validasi dan riwayat APK.
   Hanya satu unggahan per proses diperiksa bersamaan. Tidak ada penghapusan
   otomatis terhadap APK yang tersimpan dalam riwayat.

Jika alat validasi, sertifikat atau storage belum siap, dashboard menampilkan
alasannya dan menonaktifkan unggahan. Backend menolak unggah/publikasi; tidak
ada jalur yang mengklaim APK sudah diverifikasi tanpa `aapt` dan `apksigner`.

## Migrasi 002 yang eksplisit dan aditif

Perintah status tetap hanya membaca. Baseline 001 tidak diubah. Migrasi 002
hanya menambahkan `mobile_app_releases`, `mobile_release_policies`, dan
`mobile_release_audits`, plus entri pada tabel riwayat migrasi.

Jalankan dahulu pada rehearsal dan bandingkan seluruh data bisnis lama:

```sh
export PATH="$HOME/.local/go/bin:$PATH"
go run ./cmd/migrate -command=status
go run ./cmd/migrate -command=mobile-releases \
  -backup-verified -confirm-database=EXACT_DB_DATABASE
go run ./cmd/migrate -command=status
go test ./...
go build -o /private/path/server-candidate ./cmd/server
```

Ganti database dengan nama yang tepat dari lingkungan target. Setelah rehearsal
berhasil dan backup produksi telah diverifikasi, operator dapat menjalankan
migrasi yang sama pada produksi, lalu mengganti binary dan restart dengan proses
hosting yang sudah ada. Ini bukan instruksi untuk menghapus atau mengimpor ulang
schema produksi. Tidak ada AutoMigrate saat startup.

MySQL DDL tidak atomik. Runner memakai advisory lock dan menulis marker dirty
sebelum DDL. Kegagalan parsial memblokir pemeriksaan sampai diinvestigasi; jangan
menghapus marker/tabel atau mengulang SQL secara membabi buta. Tidak ada perintah
"down" destruktif. Binary lama yang hanya mengenal 001 tidak cocok dengan
riwayat 002; rollback binary harus tetap mengenal migrasi 002.

## Kontrak API dan alur rilis

Publik, tersedia sebelum login dan saat maintenance:

- `GET /api/app/version?version_code=12`: hanya rilis aktif yang dipublikasikan,
  `Cache-Control: no-store`, pembandingan numerik Android versionCode.
- `GET /api/app/download/:id`: stream APK aktif, tanpa path internal. Draft dan
  arsip mengembalikan 404. Redirect unduhan tidak diperlukan.

Terautentikasi dan khusus Super Admin:

- `GET/POST /api/admin/app/releases`
- `GET /api/admin/app/releases/:id`
- `GET /api/admin/app/releases/:id/download` (boleh untuk draft/arsip)
- `POST /api/admin/app/releases/:id/publish`
- `POST /api/admin/app/releases/:id/archive`
- `PUT /api/admin/app/policy`

Unggah multipart field `apk`, `changelog`, `force_update`; `version_name` dan
`version_code` opsional untuk mencocokkan hasil ekstraksi. Konten ZIP/manifest,
package, build, non-debuggable, sertifikat dan tanda tangan diverifikasi; SHA-256
serta ukuran dihitung server. Berkas memakai nama acak dan mode privat.

Publikasi membutuhkan JSON `minimum_supported_version_code` yang eksplisit.
Nilai 0 berarti semua build lama boleh menunda. Nilai 12 mewajibkan build <12.
Pilihan wajib pada form unggah hanya usulan draft, bukan kebijakan aktif.

Transaksi mengunci row kebijakan, memvalidasi ulang berkas, mengarsipkan rilis
lama dan memperbarui rilis aktif serta audit. Versi produksi harus lebih tinggi
melampaui high-water mark; versi lebih rendah tidak boleh diterbitkan ulang.
Arsip menyimpan berkas dan riwayat. Menurunkan kebijakan minimum adalah rollback
metadata yang aman: tidak mencoba downgrade Android. Mengarsipkan rilis aktif
menghentikan tawaran update sampai versi lebih tinggi diterbitkan.

## Build Flutter dan penandatanganan

Gunakan Flutter/Dart dari proyek dan dependency lock. `package_info_plus` 8.3.1
sesuai AGP proyek yang ada; tidak memaksa upgrade seluruh Android build toolchain.
Konfigurasi key melalui environment di mesin build:

- `IBNU_KEYSTORE_PATH`: path key yang SAMA dengan aplikasi terpasang.
- `IBNU_KEYSTORE_PASSWORD`, `IBNU_KEY_ALIAS`, `IBNU_KEY_PASSWORD`.

```sh
flutter pub get
flutter build apk --release --build-name=1.1.0 --build-number=13 \
  --dart-define=APP_BASE_URL=https://ibnuhafidz.ponpes.id \
  --dart-define=APP_UPDATE_TRUSTED_HOSTS=ibnuhafidz.ponpes.id
```

Build release tanpa key yang dipilih eksplisit akan ditolak. Jangan mengubah
package name atau mengganti sertifikat secara diam-diam. Instalasi historis di
workspace memakai key pengembangan lokal. APK kompatibilitas yang dibuat agen
mempertahankan sertifikat itu; ini bukan pembuatan key produksi baru. Amankan
key yang sudah dipakai dan rencanakan rotasi resmi secara terpisah bila diperlukan.
Tidak ada password atau keystore yang dimasukkan ke repository/server rilis.

APKs debuggable (`debug`/`profile` lama) ditolak oleh backend. Gunakan hasil
`build/app/outputs/apk/release/app-release.apk`, bukan APK debug.

## Flutter: perilaku dan kepercayaan

- Pemeriksaan startup/foreground diberi throttle lima menit; tombol manual dapat
  mencoba lagi tanpa menunggu. Permintaan bersamaan digabung.
- Informasi build dibaca dari package_info_plus dan dikonfirmasi PackageManager
  saat foreground setelah pengaturan/installer.
- Server yang gagal tidak memblokir startup normal. Kebijakan mandatory yang
  sebelumnya diterima melalui HTTPS tetap diberlakukan sampai diperiksa ulang.
- Dialog Indonesia menampilkan versi, catatan, ukuran, progres, batal, coba lagi
  dan pilihan tunda untuk pembaruan opsional.
- Unduhan memakai allowlist domain eksplisit, HTTPS port 443, path download yang
  diharapkan, tanpa redirect/credential/fragment. Berkas di cache privat aplikasi.
- Ukuran dan SHA-256 diperiksa sebelum installer. Android memeriksa package,
  build lebih tinggi dan kecocokan signer dengan aplikasi terpasang. FileProvider
  hanya mengekspos subfolder cache `updates/`, tanpa izin penyimpanan umum.
- Android 8+ dapat meminta izin "install unknown apps". Setelah memberi izin,
  pengguna menekan pasang lagi. Installer OS tetap meminta persetujuan pengguna.
- Membuka installer bukan bukti pemasangan selesai. Aplikasi membaca ulang
  build saat kembali; tidak menghapus database, antrean atau data login.
- SHA-256 dari server yang diretas bukan bukti penerbit independen. Sertifikat
  Android, keamanan key, HTTPS dan pembatasan Super Admin tetap penting.

## Checklist manual dua APK

Untuk contoh persis 1.0.0+1 → 1.1.0+2, gunakan perangkat/emulator uji bersih dan
build updater yang sama dengan build-name/build-number berbeda. Jangan memasang
build 1/2 di atas perangkat pengguna yang sudah menjalankan build 11/12.
Pada perangkat pengguna saat ini gunakan build 12 → 13 atau lebih tinggi.

1. Pasang APK awal yang sudah mengandung updater. Simpan akun, jadwal, filter,
   dan satu absensi offline; catat nilainya.
2. Build APK kedua non-debuggable dengan package dan key yang sama.
3. Super Admin unggah: harus menjadi draft. API versi belum menawarkan update;
   URL publik draft 404. Akun teacher/admin biasa tidak boleh mengunggah.
4. Publikasikan sebagai opsional (minimum 0). APK awal menampilkan versi/catatan/
   ukuran; "Nanti Saja" bekerja dan pemeriksaan manual dapat membukanya lagi.
5. Uji unduh, progres, pembatalan, putus jaringan, disk penuh dan coba ulang.
   SHA-256 salah harus ditolak sebelum installer dibuka.
6. Izinkan instalasi dari aplikasi, buka installer, batalkan dahulu. Aplikasi
   tidak boleh mengaku instalasi sukses. Ulangi dan konfirmasi pemasangan.
7. Buka APK kedua; build diperbarui dan akun/jadwal/filter/absensi lokal tetap ada.
   Sambungkan internet dan konfirmasi antrean terkirim.
8. Publikasikan build lebih tinggi dengan minimum di atas build perangkat lama.
   Periksa UI wajib, tombol back, coba ulang, offline setelah kebijakan diterima,
   foreground dari halaman detail, dan kunci biometrik jika diaktifkan.
9. Uji signer salah, package salah, duplikat build, versi lebih rendah, upload
   palsu, file berubah setelah draft, archive aktif dan rollback minimum.
10. Uji Apache/proxy asli: unggah APK besar, download HTTPS dan content length.

Aplikasi build 11 ke bawah belum memiliki updater: distribusikan build 12 sekali
secara manual tanpa uninstall. Setelah itu rilis dapat dikelola lewat dashboard.
Pengujian installer nyata dan migrasi pada hosting MySQL harus dilakukan operator;
unit test/mocked browser bukan pengganti langkah tersebut.

## Hasil verifikasi implementasi

- `go test ./...` lulus; tes tambahan migrasi dan perubahan draft APK lulus.
- Validator produksi juga dijalankan terhadap APK release build 12 yang nyata:
  aapt + apksigner + SHA-256 + kecocokan sertifikat lulus.
- 90 tes Flutter lulus; 11 tes updater mencakup versi, UI opsional/wajib,
  unduhan/progres/batal, file rusak, redirect, kegagalan installer dan lifecycle.
- Analisis Flutter bersih. APK release/debug berhasil dibangun; pemeriksaan
  Android release tidak dinonaktifkan.
- Build Vite dan unit session lulus; enam tes browser desktop/mobile rilis lulus.
- Belum dijalankan: migrasi pada MySQL hosting, deployment produksi, serta
  pembaruan melalui installer di perangkat nyata menggunakan dua versi APK.

## Ringkasan berkas

| Repo | Berkas / direktori | Tujuan |
| --- | --- | --- |
| Go | `internal/mobile/` | Pemeriksaan storage dan APK melalui alat resmi |
| Go | `internal/models/mobile_release.go` | Rilis, kebijakan dan audit |
| Go | `internal/handlers/mobile_releases.go` | API versi, unggah, lifecycle dan stream unduhan |
| Go | `internal/database/migrations/002_mobile_releases.sql` | Tiga tabel tambahan |
| Go | `internal/database/mobile_migration.go`, `cmd/migrate/main.go` | Runner eksplisit dengan dirty marker dan advisory lock |
| Go | `cmd/server/main.go`, `internal/middleware/maintenance.go` | Routing, upload streaming, akses updater sebelum login |
| Go | `.env.example`, `MOBILE_APP_RELEASES.md` | Konfigurasi dan peluncuran |
| Flutter | `lib/core/updates/` | Model/service/controller/UI updater |
| Flutter | `AppUpdateInstaller.kt`, manifest, FileProvider XML | Pemeriksaan native dan installer Android |
| Flutter | `lib/main.dart`, Settings, Gradle, pubspec | Startup, manual check, key release dan dependency |
| Vue | `src/views/mobile-app/MobileReleases.vue` | Dashboard Super Admin |
| Vue | Router, AdminLayout, tes e2e, `docs/MOBILE_RELEASES.md` | Menu, role guard dan pengujian |
