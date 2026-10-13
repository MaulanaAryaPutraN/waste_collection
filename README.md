# Community Waste Collection API

Community Waste Collection API adalah REST API untuk mengelola data rumah tangga, permintaan pengambilan sampah, pembayaran, dan laporan pengelolaan sampah. Proyek ini dibuat menggunakan Go, Chi, PostgreSQL 16, pgx/v5, dan golang-migrate.

## Menjalankan Aplikasi dengan Docker

Ikuti langkah-langkah berikut untuk menjalankan aplikasi:

1. Salin file `.env.example` menjadi `.env`. Kamu bisa menyesuaikan konfigurasi jika diperlukan.
2. Jalankan aplikasi menggunakan perintah:

   ```sh
   docker compose up --build
   ```

3. Setelah aplikasi berjalan, API dapat diakses melalui `http://localhost:8080`. Untuk memeriksa apakah server aktif, buka endpoint `GET /health`.
4. Jika ingin menggunakan data contoh, jalankan:

   ```sh
   make seed
   ```

Aplikasi akan menunggu PostgreSQL siap digunakan sebelum menjalankan migration dan menerima request. Data contoh tidak dimasukkan secara otomatis agar data yang sudah ada tidak berubah tanpa sepengetahuan pengguna.

Data database dan file bukti pembayaran disimpan menggunakan named volume Docker sehingga tetap tersedia ketika container dihentikan atau dijalankan kembali.

## Menjalankan Aplikasi Secara Lokal

Jika ingin menjalankan aplikasi tanpa Docker, pastikan Go 1.27+, PostgreSQL 16, dan database sudah tersedia. CLI `golang-migrate` diperlukan jika ingin menjalankan migration secara manual.

Salin file konfigurasi terlebih dahulu:

```sh
cp .env.example .env
```

Pastikan PostgreSQL berjalan dan konfigurasi database sudah sesuai. Setelah itu, jalankan:

```sh
go mod download
go run ./cmd/api
```

Kamu juga bisa menggunakan perintah Make berikut:

```sh
make run
make test
make build
make vet
```

Migration dijalankan secara otomatis ketika aplikasi dimulai. Jika ingin mengelolanya secara manual, atur `DATABASE_URL` atau variabel konfigurasi database lainnya, lalu jalankan:

```sh
migrate -path migrations -database "$DATABASE_URL" up
migrate -path migrations -database "$DATABASE_URL" down 1
make seed
```

Perintah `make migrate-up` dan `make migrate-down` membutuhkan executable `migrate` yang tersedia di `PATH`. Sementara itu, `make seed` menjalankan `psql` melalui container Docker Compose.

## Struktur dan Arsitektur

Proyek ini menggunakan pemisahan layer agar kode lebih mudah dipelihara dan dikembangkan. Alur dependensi berjalan satu arah, yaitu `handler → service → domain`, sedangkan repository dan storage digunakan melalui interface yang didefinisikan pada domain.

Berikut fungsi masing-masing direktori:

- `cmd/api`: Menyiapkan aplikasi, koneksi database, migration, HTTP server, dan proses shutdown.
- `internal/handler`: Menangani routing, validasi request, DTO, format response, middleware, dan error HTTP.
- `internal/service`: Menjalankan validasi serta aturan bisnis, seperti jenis sampah, tanggal pengambilan, nominal pembayaran, dan filter data.
- `internal/repository`: Menangani query PostgreSQL, transaksi database, serta perubahan status yang harus dilakukan secara konsisten.
- `internal/storage`: Menyimpan file bukti pembayaran secara lokal dengan nama acak dan validasi tipe file.
- `internal/domain`: Berisi model, kontrak repository/service, dan definisi error yang digunakan antar-layer.

Operasi yang berkaitan dengan perubahan status menggunakan transaksi database dan row locking untuk menghindari perubahan data yang tidak konsisten. Ketika pickup diselesaikan, status pickup dan pembuatan payment dilakukan dalam satu transaksi. Dengan demikian, keduanya berhasil bersama-sama atau dibatalkan bersama-sama jika terjadi kegagalan.

Pembuatan pickup, payment manual, dan penyelesaian pickup juga menggunakan penguncian household untuk menjaga konsistensi pemeriksaan payment yang masih pending.

## Konfigurasi Environment

Pengaturan aplikasi dapat disesuaikan melalui file `.env`.

| Variabel                   | Default                      | Keterangan                                                  |
| -------------------------- | ---------------------------- | ----------------------------------------------------------- |
| `APP_PORT`                 | `8080`                       | Port server API                                             |
| `DB_HOST`                  | `localhost`                  | Host PostgreSQL                                             |
| `DB_PORT`                  | `5432`                       | Port PostgreSQL                                             |
| `DB_USER`                  | `postgres`                   | Username database                                           |
| `DB_PASSWORD`              | `postgres`                   | Password database                                           |
| `DB_NAME`                  | `waste_collection`           | Nama database                                               |
| `DB_SSLMODE`               | `disable`                    | Pengaturan SSL database                                     |
| `DATABASE_URL`             | Dibentuk dari konfigurasi DB | Connection string alternatif yang diprioritaskan jika diisi |
| `UPLOAD_DIR`               | `uploads`                    | Direktori penyimpanan bukti pembayaran                      |
| `MAX_UPLOAD_MB`            | `5`                          | Batas ukuran file upload dalam MB                           |
| `SHUTDOWN_TIMEOUT_SECONDS` | `10`                         | Batas waktu graceful shutdown                               |

## API Endpoints

Semua endpoint mengembalikan response dengan format yang konsisten. Contohnya:

```json
{
  "success": true,
  "message": "household created",
  "data": {
    "id": "..."
  }
}
```

Jika terjadi kesalahan, response menggunakan `success: false`, pesan error, dan array `errors`.

Untuk endpoint yang mengembalikan daftar data, response dilengkapi metadata pagination berupa `total`, `page`, `limit`, dan `total_pages`. Secara default, halaman dimulai dari 1 dengan 10 data per halaman. Jumlah data yang ditampilkan dapat diatur hingga maksimal 100 data per halaman.

### 1. Household

Endpoint household digunakan untuk mengelola data rumah tangga yang terdaftar dalam sistem.

| Method | Endpoint                          | Fungsi                                     |
| ------ | --------------------------------- | ------------------------------------------ |
| POST   | `/api/households`                 | Menambahkan household baru                 |
| GET    | `/api/households?page=1&limit=10` | Melihat daftar household dengan pagination |
| GET    | `/api/households/{id}`            | Melihat detail household                   |
| DELETE | `/api/households/{id}`            | Menghapus household                        |

Untuk membuat household, kirim request berikut:

```sh
curl -X POST localhost:8080/api/households \
  -H 'Content-Type: application/json' \
  -d '{"owner_name":"Ayu Lestari","address":"Jl. Melati No. 1"}'
```

Contoh response:

```json
{
  "success": true,
  "message": "household created",
  "data": {
    "id": "...",
    "owner_name": "Ayu Lestari",
    "address": "Jl. Melati No. 1",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

Household tidak dapat dihapus jika masih digunakan oleh data lain yang memiliki relasi dengannya.

### 2. Waste Pickup

Endpoint pickup digunakan untuk membuat permintaan pengambilan sampah, mengatur jadwal, menyelesaikan pengambilan, dan membatalkan permintaan.

| Method | Endpoint                                                          | Fungsi                              |
| ------ | ----------------------------------------------------------------- | ----------------------------------- |
| POST   | `/api/pickups`                                                    | Membuat permintaan pickup baru      |
| GET    | `/api/pickups?status=pending&household_id={uuid}&page=1&limit=10` | Melihat dan memfilter daftar pickup |
| PUT    | `/api/pickups/{id}/schedule`                                      | Menentukan jadwal pickup            |
| PUT    | `/api/pickups/{id}/complete`                                      | Menandai pickup selesai             |
| PUT    | `/api/pickups/{id}/cancel`                                        | Membatalkan pickup                  |

Setiap permintaan pickup harus memiliki `household_id` dan `type`. Jenis sampah yang didukung adalah `organic`, `plastic`, `paper`, dan `electronic`. Untuk sampah elektronik, field `safety_check` wajib disertakan.

Contoh membuat pickup:

```sh
curl -X POST localhost:8080/api/pickups \
  -H 'Content-Type: application/json' \
  -d '{"household_id":"10000000-0000-4000-8000-000000000001","type":"organic"}'
```

Untuk menjadwalkan pickup, gunakan endpoint berikut:

```sh
curl -X PUT localhost:8080/api/pickups/{id}/schedule \
  -H 'Content-Type: application/json' \
  -d '{"pickup_date":"2030-01-01T09:00:00Z"}'
```

Tanggal jadwal menggunakan format RFC3339. Pickup hanya dapat dijadwalkan ketika statusnya masih `pending`. Khusus sampah elektronik, penjadwalan hanya diperbolehkan jika pemeriksaan keamanan sudah dinyatakan lolos (`safety_check: true`).

Pickup hanya dapat diselesaikan ketika statusnya `scheduled`. Saat pickup selesai, sistem akan membuat payment secara otomatis dalam transaksi yang sama. Biaya pengambilan adalah Rp50.000 untuk sampah organik, plastik, dan kertas, serta Rp100.000 untuk sampah elektronik.

Pickup yang masih berstatus `pending` atau `scheduled` dapat dibatalkan. Perubahan status yang tidak sesuai aturan akan ditolak oleh API.

### 3. Payment

Endpoint payment digunakan untuk mencatat pembayaran, melihat riwayat pembayaran, dan mengonfirmasi pembayaran melalui bukti transfer atau pembayaran.

| Method | Endpoint                                                                                                | Fungsi                                        |
| ------ | ------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| POST   | `/api/payments`                                                                                         | Membuat payment manual                        |
| GET    | `/api/payments?status=paid&household_id={uuid}&date_from=2026-10-01&date_to=2026-10-31&page=1&limit=10` | Melihat dan memfilter daftar payment          |
| PUT    | `/api/payments/{id}/confirm`                                                                            | Mengonfirmasi payment dengan bukti pembayaran |

Pembuatan payment manual membutuhkan `household_id`, `waste_id`, dan `amount`. Household dan pickup harus sudah terdaftar, serta pickup harus berasal dari household yang sama. Satu pickup hanya dapat memiliki satu payment.

Untuk mengonfirmasi pembayaran, gunakan `multipart/form-data` dengan field file bernama `proof`. Format file yang diterima adalah JPG, PNG, atau PDF, dengan ukuran maksimal 5 MB secara default.

Contoh menggunakan cURL:

```sh
curl -X PUT localhost:8080/api/payments/{id}/confirm \
  -F 'proof=@proof.pdf;type=application/pdf'
```

Jika konfirmasi berhasil, status payment berubah menjadi `paid`, `payment_date` diisi dengan waktu konfirmasi, dan `proof_file_url` menyimpan lokasi file bukti pembayaran.

File dapat diakses melalui URL yang dikembalikan API, misalnya:

```text
http://localhost:8080/uploads/{uuid}.pdf
```

Batas ukuran upload dapat disesuaikan melalui `MAX_UPLOAD_MB`. Sistem juga memeriksa tipe konten file dan menggunakan nama file acak untuk menghindari penggunaan nama file asli secara langsung.

Filter payment mendukung status, household, dan rentang tanggal. Parameter tanggal menerima format RFC3339 atau `YYYY-MM-DD`. Jika menggunakan `date_to` berupa tanggal kalender, data pada tanggal tersebut tetap dihitung hingga akhir hari.

### 4. Reporting

Endpoint reporting menyediakan ringkasan data yang dapat digunakan untuk memantau aktivitas pengambilan sampah dan pembayaran.

| Method | Endpoint                       | Fungsi                                                 |
| ------ | ------------------------------ | ------------------------------------------------------ |
| GET    | `/api/reports/waste-summary`   | Melihat jumlah pickup berdasarkan jenis dan status     |
| GET    | `/api/reports/payment-summary` | Melihat jumlah payment per status dan total pendapatan |
| GET    | `/health`                      | Memeriksa apakah server API aktif                      |

Ringkasan pickup menampilkan jumlah data berdasarkan jenis sampah dan statusnya. Sementara itu, ringkasan payment menampilkan jumlah serta total nominal pembayaran per status.

Total pendapatan (`total_revenue`) dihitung dari payment yang berstatus `paid`, sehingga pembayaran yang masih pending atau gagal tidak ikut dihitung sebagai pendapatan.

## Aturan Bisnis

Beberapa aturan diterapkan untuk menjaga konsistensi data dan proses pengambilan sampah:

- Household yang masih memiliki payment pending tidak dapat membuat pickup baru.
- Penjadwalan hanya diperbolehkan untuk pickup berstatus `pending`.
- Pickup elektronik hanya dapat dijadwalkan jika `safety_check` bernilai `true`.
- Pickup hanya dapat diselesaikan jika sudah berstatus `scheduled`.
- Penyelesaian pickup dan pembuatan payment otomatis dilakukan dalam satu transaksi.
- Pembatalan hanya diperbolehkan untuk pickup berstatus `pending` atau `scheduled`.
- Payment hanya dapat dikonfirmasi jika masih berstatus `pending` dan bukti pembayaran disertakan.
- Payment manual harus mengacu pada household dan pickup yang valid serta saling berhubungan.
- Household tidak dapat dihapus jika masih direferensikan oleh data lain.
- File bukti pembayaran divalidasi berdasarkan ukuran dan tipe konten. Jika konfirmasi gagal setelah file baru disimpan, sistem akan menghapus file tersebut.

Transaksi database dan penguncian baris digunakan pada proses yang melibatkan perubahan status agar operasi yang berjalan bersamaan tidak menghasilkan data yang tidak konsisten.

## Migration, Seed, dan Pengujian

File migration tersedia di direktori `migrations/`, masing-masing untuk perubahan skema database dan pembatalannya. Data contoh disediakan melalui `seeds/seed.sql`.

Seed berisi lima household beserta data pickup dan payment dengan berbagai status. UUID pada data contoh dibuat tetap agar dapat digunakan kembali ketika melakukan pengujian melalui Postman.

Untuk memasukkan data contoh, jalankan:

```sh
make seed
```

Pastikan database dan aplikasi sudah siap sebelum menjalankan seed. Perlu diperhatikan bahwa data contoh dapat memengaruhi database yang digunakan, jadi gunakan lingkungan pengujian yang sesuai.

Untuk menjalankan pengujian dan memeriksa kode, gunakan:

```sh
go test ./...
go vet ./...
go build ./...
```

## Pengujian dengan Postman

Proyek menyediakan collection Postman dan environment lokal agar setiap endpoint dapat diuji tanpa perlu memasukkan URL dan konfigurasi secara berulang.

Import kedua file berikut ke Postman:

- `postman/Sampah.postman_collection.json`
- `postman/local.postman_environment.json`

Setelah import, pilih environment **Community Waste Collection - Local**. Variabel `base_url` secara default menggunakan `http://localhost:8080`.

Collection mencakup endpoint household, pickup, payment, reporting, health check, serta pengujian upload bukti pembayaran menggunakan multipart form-data.

Dengan collection tersebut, pengujian API dapat dilakukan secara berurutan mulai dari pembuatan household, permintaan pickup, penjadwalan, penyelesaian atau pembatalan pickup, konfirmasi payment, hingga pemeriksaan laporan.
