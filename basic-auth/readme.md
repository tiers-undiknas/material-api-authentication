# Penjelasan dan Cara Menjalankan

## Simpan Kode

Simpan kode di atas dalam satu file bernama `main.go` di dalam direktori proyek Anda (misalnya, `basic-auth-go-mysql/main.go`).

## Sesuaikan Konfigurasi Database

Buka `main.go` dan **WAJIB** sesuaikan konstanta `dbUser`, `dbPassword`, `dbHost`, `dbPort`, dan `dbName` dengan konfigurasi server MySQL Anda.

## Inisialisasi Pengguna Admin (Opsional, tapi direkomendasikan untuk pertama kali)

1.  Buka terminal di direktori proyek.
2.  Jalankan perintah berikut untuk membuat tabel `users` (jika belum ada) dan menambahkan pengguna `admin` dengan password `password123`:
    ```bash
    go run main.go initadmin
    ```
3.  Perintah ini hanya akan melakukan inisialisasi dan kemudian keluar.

## Menjalankan Server

Untuk menjalankan server aplikasi, gunakan perintah:
```bash
go run main.go
```
Server akan berjalan di `http://localhost:8080` (port default Go adalah 8080, bisa diubah jika mau).

## Menambahkan Pengguna Baru (Opsional, melalui API)

Endpoint `POST /register` tersedia.

Contoh menggunakan `curl`:
```bash
curl -X POST -H "Content-Type: application/json" -d "{\"email\":\"testuser@gmail.com\",\"password\":\"testpass\"}" http://localhost:8080/register
```

## Menguji Endpoint yang Diproteksi

Endpoint `/api/protected-data` dilindungi oleh Basic Auth.

Menggunakan `curl`:

Untuk pengguna admin dengan password password123:
```bash
curl -u "admin@gmail.com:password123" http://localhost:8080/api/protected-data
```

Untuk pengguna testuser dengan password testpass (jika sudah ditambahkan):
```bash
curl -u "testuser@gmail.com:testpass" http://localhost:8080/api/protected-data
```

Tanpa kredensial (akan gagal dengan status 401):
```bash
curl http://localhost:8080/api/protected-data
```

Dengan kredensial salah:
```bash
curl -u "admin:salahpassword" http://localhost:8080/api/protected-data
```

## Menguji Endpoint Publik

Endpoint `/api/public-data` tidak memerlukan autentikasi.

Menggunakan `curl`:
```bash
curl http://localhost:8080/api/public-data
```

## Detail Kode Go

-   `initDB()`: Menyiapkan koneksi ke MySQL dan membuat tabel `user`.
-   `addUser()`, `findUserByEmail()`, `verifyPassword()`: Fungsi-fungsi untuk operasi pengguna dan verifikasi password.
-   `basicAuthMiddleware()`:
    -   Mengambil header `Authorization`.
    -   Mem-parsing dan mendekode kredensial Basic Auth.
    -   Memanggil `findUserByEmail` dan `verifyPassword`.
    -   Mengirim respons `401 Unauthorized` dengan header `WWW-Authenticate` jika autentikasi gagal.
    -   Memanggil handler berikutnya jika berhasil.
-   `registerUserHandler()`, `protectedDataHandler()`, `publicDataHandler()`: Handler untuk masing-masing rute.
-   `main()`:
    -   Memanggil `initDB()` untuk menyiapkan database.
    -   Menyediakan opsi `initadmin` untuk setup pengguna awal.
    -   Menggunakan `gorilla/mux` untuk routing.
    -   Menjalankan server HTTP menggunakan `http.ListenAndServe`.

Contoh ini memberikan implementasi Basic Authentication yang fungsional di Go dengan backend MySQL. Ingatlah untuk selalu memprioritaskan keamanan, terutama penggunaan HTTPS di lingkungan produksi.