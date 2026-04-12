# cPanel Git Deploy (Beta)

Folder ini menyediakan alur deploy Go API yang kompatibel dengan cPanel.

## Fungsi setup ini

- Build binary dari repository Git yang di-clone.
- Restart proses API setelah setiap deploy.
- Memuat secret runtime dari file env di server yang tidak di-commit ke Git.

## Prasyarat

- Akun cPanel dengan Git Version Control aktif.
- Akses terminal di cPanel.
- Toolchain Go tersedia di shell (`go version`).
- Database/user MySQL untuk beta sudah dibuat.

## Setup server sekali saja

1. Clone repository di cPanel Git Version Control.
2. Pastikan branch mengarah ke branch beta Anda.
3. Di terminal, siapkan file env runtime:

```bash
mkdir -p "$HOME/.local/ibnu-go-backend"
cp deploy/cpanel/.env.production.example "$HOME/.local/ibnu-go-backend/.env.production"
```

4. Edit `$HOME/.local/ibnu-go-backend/.env.production` dengan nilai sebenarnya.
5. Jadikan script deploy executable:

```bash
chmod +x deploy/cpanel/deploy.sh
```

6. Klik **Deploy HEAD Commit** di cPanel Git UI (atau push ke branch lalu deploy lagi).

## Override environment opsional

Anda bisa mendefinisikan ini di profile shell cPanel jika diperlukan:

- `CPANEL_BACKEND_ENV_FILE` (default: `$HOME/.local/ibnu-go-backend/.env.production`)
- `CPANEL_BACKEND_LOG_DIR` (default: `$HOME/logs/ibnu-go-backend`)
- `CPANEL_RUNTIME_DIR` (default: `$HOME/.local/ibnu-go-backend`)

## Health check

Setelah deploy, cek:

```bash
cat "$HOME/logs/ibnu-go-backend/app.log" | tail -n 50
```

Jika API berjalan di `PORT=18080`, uji:

```bash
curl http://127.0.0.1:18080/api/public/home
```

## Catatan penting cPanel

Proses ini menjalankan API sebagai proses user (`nohup`). Jika provider hosting menghentikan proses jangka panjang di shared hosting, pindahkan API ke VPS atau gunakan paket hosting yang mendukung proses persisten.
