# Deploy Beta Production di cPanel (Clone dari GitHub)

Runbook ini melakukan deploy:

- Go API dari `ibnu-go-backend`
- Frontend Vue dari `ibnu-hafidz-vue-frontend`

menggunakan cPanel Git Version Control.

## 1) Siapkan branch GitHub

1. Push perubahan backend dan frontend ke branch beta Anda.
2. Pastikan setiap repo memiliki:
   - `.cpanel.yml`
   - `deploy/cpanel/deploy.sh`

## 2) Clone repository di cPanel

Di cPanel, buka **Git Version Control** lalu clone kedua repo dari GitHub:

- path clone repo backend: direktori aplikasi yang Anda pilih
- path clone repo frontend: direktori aplikasi yang Anda pilih

Set keduanya ke branch beta.

## 3) Setup sekali untuk backend (Go API)

Jalankan di cPanel Terminal pada root repo backend:

```bash
chmod +x deploy/cpanel/deploy.sh
mkdir -p "$HOME/.local/ibnu-go-backend"
cp deploy/cpanel/.env.production.example "$HOME/.local/ibnu-go-backend/.env.production"
```

Edit file env dan isi nilai production beta:

- `APP_ENV=production`
- kredensial DB
- secret JWT
- `CORS_ORIGINS=https://domain-anda.com`
- `PORT=18080` (atau port internal pilihan Anda)

Deploy dari cPanel Git UI menggunakan **Deploy HEAD Commit**.

## 4) Setup sekali untuk frontend (Vue app)

Jalankan di cPanel Terminal pada root repo frontend:

```bash
chmod +x deploy/cpanel/deploy.sh
```

Deploy dari cPanel Git UI menggunakan **Deploy HEAD Commit**.

Secara default script deploy frontend mem-publish ke:

- `$HOME/public_html/beta`

Override opsional sebelum deploy:

```bash
export CPANEL_FRONTEND_PUBLISH_DIR="$HOME/public_html/beta"
```

## 5) Proxy API dan routing SPA

Deploy frontend akan menyalin `deploy/cpanel/.htaccess` ke direktori publish.

Isinya mencakup:

- proxy `/api/*` dan `/uploads/*` ke `http://127.0.0.1:18080`
- fallback SPA ke `/index.html`

Jika hosting Anda memblokir aturan proxy di `.htaccess`, gunakan host terpisah `api-beta` lalu sesuaikan strategi routing frontend.

## 6) Smoke test setelah deploy

1. Log backend:

```bash
tail -n 100 "$HOME/logs/ibnu-go-backend/app.log"
```

2. Health probe lokal backend:

```bash
curl http://127.0.0.1:18080/api/public/home
```

3. Pengecekan di browser:

- `https://domain-anda.com/beta`
- Login
- Dashboard terbuka normal
- Halaman publik terbuka normal
- Jalur upload file (`/uploads/...`)

## 7) Jalur rollback cepat

1. Di cPanel Git UI, pindahkan ke commit stabil sebelumnya.
2. Deploy commit tersebut untuk backend dan frontend.

## Catatan

- Pada beberapa paket shared hosting, proses user jangka panjang bisa dihentikan; Go API mungkin membutuhkan hosting level VPS agar stabil.
- Simpan secret hanya di file env server (`$HOME/.local/ibnu-go-backend/.env.production`), jangan pernah di Git.
