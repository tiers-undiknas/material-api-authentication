# Penjelasan dan Cara Menjalankan

## Simpan Kode

Simpan kode di atas dalam satu file bernama `main.go` di dalam direktori proyek Anda.

## Sesuaikan Konfigurasi Database

Buka `main.go` dan **WAJIB** sesuaikan konstanta `dbUser`, `dbPassword`, `dbHost`, `dbPort`, dan `dbName` dengan konfigurasi server MySQL Anda.

## Inisialisasi Klien Contoh (Opsional, untuk pengujian)

1.  Buka terminal di direktori proyek.
2.  Jalankan perintah berikut untuk membuat tabel `api_keys` (jika belum ada) dan menambahkan API Key untuk klien pengujian:

    ```bash
    go run main.go initclient "Klien Pengujian Saya"
    ```

    Atau cukup:
    
    ```bash
    go run main.go initclient
    ```
    (Ini akan menggunakan nama klien default "Klien Pengujian Awal").

3.  Perintah ini akan mencetak API Key mentah ke konsol. **Simpan API Key ini!** Anda hanya akan melihatnya sekali ini.

## Menjalankan Server

Untuk menjalankan server aplikasi, gunakan perintah:
```bash
go run main.go
```
Server akan berjalan di http://localhost:8080.

## Mendaftarkan Klien Baru dan Mendapatkan API Key (melalui API)

Gunakan `curl` atau Postman untuk mengirim permintaan `POST` ke endpoint `/register-client`.

Contoh menggunakan curl:
```bash
curl -X POST -H "Content-Type: application/json" -d "{\"client_name\":\"Aplikasi Keren Saya\"}" http://localhost:8080/register-client
```
Responsnya akan berisi API Key mentah yang baru dibuat. Simpan kunci ini dengan aman.

## Menguji Endpoint yang Diproteksi

Endpoint `/api/protected-resource` dilindungi oleh API Key.
Ganti `YOUR_API_KEY_HERE` dengan API Key yang Anda dapatkan.

Menggunakan `curl`:
```bash
curl -H "X-API-Key: YOUR_API_KEY_HERE" http://localhost:8080/api/protected-resource
```

Tanpa API Key (akan gagal dengan status 401):
```bash
curl http://localhost:8080/api/protected-resource
```

Dengan API Key salah (akan gagal dengan status 403):
```bash
curl -H "X-API-Key: KUNCI_SALAH_ATAU_TIDAK_AKTIF" http://localhost:8080/api/protected-resource
```

## Menguji Endpoint Publik

Endpoint `/api/public-resource` tidak memerlukan autentikasi.

Menggunakan `curl`:
```bash
curl http://localhost:8080/api/public-resource
```

## Detail Kode Go

-   `initDB()`: Menyiapkan koneksi ke MySQL dan membuat tabel `api_keys`.
-   `generateAPIKey()`: Menghasilkan string acak yang aman secara kriptografis menggunakan `crypto/rand` dan meng-encode-nya ke Base64. Ini juga menghasilkan `key_prefix` untuk identifikasi.
-   `hashAPIKey()`: Menggunakan SHA256 untuk membuat hash dari API Key. Ini adalah praktik yang baik untuk tidak menyimpan API Key mentah di database.
-   `storeAPIKey()`: Menyimpan nama klien, prefix kunci, dan hash API Key ke database.
-   `validateAPIKey()`: Menerima API Key dari header, membuat hash-nya, lalu mencari hash tersebut di database untuk memvalidasi dan memeriksa apakah kunci aktif.
-   `recordAPIKeyUsage()`: Memperbarui kolom `last_used_at` di database setiap kali API Key digunakan. Ini dijalankan sebagai goroutine agar tidak memblokir respons utama.
-   `apiKeyAuthMiddleware()`: Middleware yang mengekstrak API Key dari header `X-API-Key`, memvalidasinya menggunakan `validateAPIKey`, dan mencatat penggunaannya.
-   `registerClientHandler()`: Handler untuk endpoint `/register-client`. Menghasilkan API Key baru, menyimpannya (hash-nya), dan mengembalikan API Key mentah ke klien.
-   `protectedResourceHandler()` dan `publicResourceHandler()`: Contoh handler untuk endpoint yang dilindungi dan publik.
-   `main()`: Menginisialisasi database, mengatur router menggunakan `gorilla/mux`, dan menjalankan server HTTP. Menyediakan opsi `initclient` untuk setup API Key awal.

Contoh ini memberikan dasar yang solid untuk implementasi autentikasi API Key di Go. Anda dapat mengembangkannya lebih lanjut dengan fitur seperti pencabutan kunci, rotasi kunci, pembatasan tarif (rate limiting), dan kuota berdasarkan API Key.