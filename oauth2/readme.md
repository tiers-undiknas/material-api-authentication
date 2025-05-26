# Penjelasan dan Cara Menjalankan

## Simpan Kode

Simpan kode di atas dalam satu file bernama `main.go`.

## Sesuaikan Konfigurasi

Buka `main.go` dan **WAJIB** sesuaikan konstanta di bagian `// --- Konfigurasi ---` dengan detail koneksi MySQL Anda dan `jwtSecretKey` yang kuat.

## Inisialisasi Awal (Opsional, untuk Pengujian)

1.  Buka terminal di direktori proyek.
2.  **Buat Pengguna Awal:**
    ```bash
    go run main.go inituser user@example.com password123
    ```
3.  **Buat Klien OAuth Awal:**
    ```bash
    go run main.go initclient "Aplikasi Klien Saya" "http://localhost:3000/oauth/callback"
    ```
4.  Perintah ini akan mencetak Client ID dan Client Secret. **Simpan keduanya dengan aman!** Client Secret hanya ditampilkan sekali.

## Menjalankan Server Otorisasi

```bash
go run main.go
```
Server akan berjalan di `http://localhost:8080`.

## Alur Authorization Code Grant (Simulasi)

### a. Klien Mengarahkan Pengguna ke Server Otorisasi
Buka browser dan navigasi ke URL seperti ini (ganti `YOUR_CLIENT_ID` dan `YOUR_REDIRECT_URI` dengan yang Anda dapatkan saat `initclient` atau registrasi klien):

```
http://localhost:8080/oauth/authorize?response_type=code&client_id=YOUR_CLIENT_ID&redirect_uri=YOUR_REDIRECT_URI&scope=read_profile%20write_data&state=xyz123
```
-   `response_type=code`: Menandakan alur Authorization Code.
-   `client_id`: ID klien Anda.
-   `redirect_uri`: Ke mana pengguna akan diarahkan kembali setelah otorisasi. Harus cocok dengan yang terdaftar.
-   `scope`: Izin yang diminta (opsional, contoh: `read_profile write_data`).
-   `state`: String acak untuk proteksi CSRF (opsional tapi direkomendasikan).

### b. Pengguna Login dan Memberikan Persetujuan
Anda akan melihat halaman login sederhana. Masukkan email dan password pengguna yang telah Anda daftarkan (misalnya, `user@example.com` dan `password123`).

Setelah login, server (dalam contoh ini) akan otomatis menganggap persetujuan diberikan dan mengarahkan kembali ke `redirect_uri` klien dengan `code` dan `state` di query string:
```
YOUR_REDIRECT_URI?code=AUTHORIZATION_CODE_FROM_SERVER&state=xyz123
```
Catat `AUTHORIZATION_CODE_FROM_SERVER` ini.

### c. Klien Menukar Kode Otorisasi dengan Access Token
Aplikasi klien Anda (di backend-nya) sekarang akan membuat permintaan `POST` ke endpoint `/oauth/token` di server otorisasi.

Gunakan `curl` atau Postman (ganti placeholder dengan nilai yang sesuai):
```bash
curl -X POST http://localhost:8080/oauth/token \
  -d "grant_type=authorization_code" \
  -d "code=AUTHORIZATION_CODE_FROM_SERVER" \
  -d "redirect_uri=YOUR_REDIRECT_URI" \
  -d "client_id=YOUR_CLIENT_ID" \
  -d "client_secret=YOUR_CLIENT_SECRET"
```
Jika berhasil, responsnya akan berisi `access_token`, `refresh_token`, dll.:
```json
{
    "access_token": "YOUR_ACCESS_TOKEN_JWT",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "YOUR_NEW_REFRESH_TOKEN",
    "scope": "read_profile write_data"
}
```
**Simpan `access_token` dan `refresh_token` ini.**

### d. Klien Mengakses Sumber Daya Terproteksi
Gunakan `access_token` untuk mengakses endpoint API yang dilindungi:
```bash
curl -H "Authorization: Bearer YOUR_ACCESS_TOKEN_JWT" http://localhost:8080/api/protected
```

### e. Menggunakan Refresh Token (Jika Access Token Kedaluwarsa)
Kirim permintaan `POST` ke `/oauth/token`:
```bash
curl -X POST http://localhost:8080/oauth/token \
  -d "grant_type=refresh_token" \
  -d "refresh_token=YOUR_REFRESH_TOKEN" \
  -d "client_id=YOUR_CLIENT_ID" \
  -d "client_secret=YOUR_CLIENT_SECRET"
  # -d "scope=read_profile" # Opsional: scope bisa dipersempit
```
Responsnya akan memberikan `access_token` baru. (Contoh ini tidak merotasi refresh token, jadi refresh token lama masih bisa digunakan sampai kedaluwarsa atau dicabut).

## Detail Kode Go

-   **Database** (`initDB`, `createUser`, `getOAuthClient`, dll.):
    Mengelola penyimpanan dan pengambilan data pengguna, klien, kode otorisasi, dan refresh token. Perhatikan penggunaan `sql.NullTime` untuk kolom yang bisa `NULL` dan parsing timestamp dari MySQL.
-   **Hashing** (`hashPassword`, `checkPasswordHash`, `hashStringSHA256`):
    `bcrypt` digunakan untuk password pengguna dan client secret. `SHA256` digunakan untuk refresh token sebelum disimpan (sebagai lapisan keamanan tambahan, meskipun refresh token itu sendiri sudah acak).
-   **JWT** (`generateAccessToken`, `validateAccessToken`):
    Menggunakan `github.com/golang-jwt/jwt/v5` untuk membuat dan memvalidasi access token.
-   **Middleware** (`authMiddleware`):
    Memeriksa header `Authorization: Bearer <token>`, memvalidasi JWT, dan jika valid, meneruskan permintaan.
-   **Handler** (`authorizeHandler`, `tokenHandler`, dll.):
    Mengimplementasikan logika untuk setiap endpoint OAuth 2.0 dan endpoint API.
    -   `authorizeHandler`: Menangani permintaan awal untuk otorisasi, menampilkan form login (jika `GET`), memproses login, membuat kode otorisasi, dan melakukan redirect.
    -   `tokenHandler`: Menangani penukaran kode otorisasi atau refresh token dengan access token.
-   **Template HTML** (`loginTmpl`):
    Contoh halaman login sangat sederhana yang disajikan oleh `authorizeHandler`.

Ini adalah implementasi yang cukup komprehensif namun tetap merupakan dasar. OAuth 2.0 memiliki banyak detail dan pertimbangan keamanan lainnya yang perlu diperhatikan untuk sistem produksi.