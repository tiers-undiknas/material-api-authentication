# Penjelasan dan Cara Menjalankan

## Simpan Kode

Simpan kode di atas dalam satu file bernama `main.go` di dalam direktori proyek Anda.

## Sesuaikan Konfigurasi

Buka `main.go` dan **WAJIB** sesuaikan konstanta berikut dengan detail Anda:
-   `dbUser`, `dbPassword`, `dbHost`, `dbPort`, `dbName` (untuk koneksi MySQL).
-   `jwtSecretKey`: Ganti dengan string acak yang panjang dan kuat. **Penting:** Di lingkungan produksi, ini harus diambil dari *environment variable* atau sistem manajemen konfigurasi yang aman, bukan di-hardcode.

## Inisialisasi Pengguna Awal (Opsional, untuk Pengujian)

1.  Buka terminal di direktori proyek.
2.  Jalankan perintah berikut untuk membuat tabel `user` (jika belum ada) dan menambahkan pengguna awal (default: `admin@gmail.com` dengan password `password123`):
    ```bash
    go run main.go initadmin
    ```
3.  Anda juga bisa menentukan email dan password kustom:
    ```bash
    go run main.go initadmin penggunaSaya passwordRahasiaSaya
    ```
4.  Perintah ini hanya akan melakukan inisialisasi dan kemudian keluar.

## Menjalankan Server

Untuk menjalankan server aplikasi, gunakan perintah:
```bash
go run main.go
```
Server akan berjalan di `http://localhost:8080`.

## Alur Penggunaan

### a. Registrasi Pengguna Baru
Kirim permintaan `POST` ke `/register` dengan body JSON:
```json
{
    "email": "penggunabaru@gmail.com",
    "password": "passwordkuat123"
}
```
Contoh menggunakan `curl`:
```bash
curl -X POST -H "Content-Type: application/json" -d "{\"email\":\"penggunabaru@gmail.com\",\"password\":\"passwordkuat123\"}" http://localhost:8080/register
```
Jika berhasil, Anda akan mendapatkan respons 201 Created.

### b. Login Pengguna
Kirim permintaan `POST` ke `/login` dengan body JSON:
```json
{
    "email": "penggunabaru@gmail.com",
    "password": "passwordkuat123"
}
```
*(Gunakan `admin@gmail.com` dan `password123` jika Anda menggunakan `initadmin` default).*

Contoh menggunakan `curl`:
```bash
curl -X POST -H "Content-Type: application/json" -d "{\"email\":\"penggunabaru@gmail.com\",\"password\":\"passwordkuat123\"}" http://localhost:8080/login
```
Jika berhasil, responsnya akan berisi JWT:
```json
{
    "message": "Login berhasil!",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6InBlbmdndW5hYmFydSIsImV4cCI6MTYxODQxNjAwMCwiaWF0IjoxNjE4MzI5NjAwLCJpc3MiOiJhcGxpY2FzaS1zYXlhLmNvbSJ9.xxxxxxxxxxxx"
}
```
Simpan token ini.

### c. Mengakses Endpoint Terproteksi
Sertakan JWT yang Anda dapatkan dari login pada header `Authorization` dengan skema `Bearer`.
Ganti `YOUR_JWT_TOKEN_HERE` dengan token yang Anda peroleh.

Contoh menggunakan `curl`:
```bash
curl -H "Authorization: Bearer YOUR_JWT_TOKEN_HERE" http://localhost:8080/api/protected
```
Jika token valid, Anda akan mendapatkan respons sukses. Jika tidak, Anda akan mendapatkan error 401 Unauthorized.

### d. Mengakses Endpoint Publik
Endpoint ini tidak memerlukan autentikasi.
```bash
curl http://localhost:8080/api/public
```

## Detail Kode Go

-   `initDB()`: Menyiapkan koneksi ke MySQL dan membuat tabel `users`.
-   `addUser()`: Melakukan hashing password menggunakan `bcrypt.GenerateFromPassword` sebelum menyimpannya.
-   `findUserByEmail()`: Mengambil data pengguna dari database.
-   `verifyPassword()`: Membandingkan password yang diberikan dengan hash yang tersimpan menggunakan `bcrypt.CompareHashAndPassword`.
-   `generateJWT()`:
    -   Membuat *claims* yang berisi `UserID`, `Email`, dan *claims* standar JWT (`ExpiresAt`, `IssuedAt`, `Issuer`).
    -   Menggunakan `jwt.NewWithClaims` dengan metode signing `HS256`.
    -   Menandatangani token dengan `jwtSecretKey`.
-   `validateJWT()`:
    -   Mem-parsing token string.
    -   Memverifikasi bahwa metode signing adalah `HS256`.
    -   Memvalidasi tanda tangan token menggunakan `jwtSecretKey`.
    -   Memeriksa apakah token masih valid (termasuk belum kedaluwarsa).
-   `authMiddleware()`:
    -   Mengekstrak token dari header `Authorization: Bearer <token>`.
    -   Memanggil `validateJWT()` untuk memverifikasi token.
    -   Jika valid, melanjutkan ke handler berikutnya. Jika tidak, mengirim respons `401 Unauthorized`.
-   **Handler Rute**: Fungsi-fungsi yang menangani logika untuk setiap endpoint (`/register`, `/login`, `/api/protected`, `/api/public`).
-   `main()`: Menginisialisasi database, mengatur router `gorilla/mux`, menerapkan middleware, dan menjalankan server HTTP.

## Pertimbangan untuk Aplikasi Produksi

Ini adalah contoh dasar yang fungsional. Untuk aplikasi produksi, Anda perlu mempertimbangkan hal-hal seperti:

-   **Manajemen Secret Key yang Lebih Aman**: Gunakan *environment variables* atau sistem manajemen konfigurasi (misalnya, HashiCorp Vault, AWS Secrets Manager) untuk `jwtSecretKey`.
-   **Mekanisme Refresh Token**: Implementasikan refresh token untuk sesi yang lebih lama tanpa mengharuskan pengguna mengirim ulang kredensial.
-   **Penanganan Error yang Lebih Detail**: Sediakan logging yang komprehensif dan pesan error yang lebih informatif.
-   **Validasi Input yang Lebih Ketat**: Lakukan validasi menyeluruh pada semua input pengguna.
-   **Penggunaan HTTPS**: Selalu gunakan HTTPS di lingkungan produksi untuk mengenkripsi komunikasi.
-   **Pembatasan Akses (Rate Limiting)**: Lindungi API Anda dari penyalahgunaan.
-   **Pencatatan (Logging)**: Catat aktivitas penting untuk audit dan debugging.