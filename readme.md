## Pendahuluan

Dalam dunia pengembangan aplikasi modern, Application Programming Interface (API) memegang peranan krusial sebagai jembatan komunikasi antar berbagai layanan perangkat lunak. Seiring dengan meningkatnya ketergantungan pada API, aspek keamanan menjadi sangat penting. Dua pilar utama dalam mengamankan API adalah autentikasi dan autorisasi.

Autentikasi adalah proses verifikasi identitas pengguna atau layanan yang mencoba mengakses API. Ini menjawab pertanyaan, "Siapa Anda?".

Autorisasi adalah proses penentuan hak akses yang dimiliki oleh pengguna atau layanan yang telah terautentikasi. Ini menjawab pertanyaan, "Apa yang boleh Anda lakukan?".

Materi ini akan membahas secara mendalam konsep, metode, dan praktik terbaik dalam mengimplementasikan autentikasi dan autorisasi pada API.

## Autentikasi (Siapa Anda?)

Autentikasi memastikan bahwa hanya entitas yang sah (pengguna, aplikasi lain, atau layanan) yang dapat mengakses API Anda. Berikut adalah beberapa metode autentikasi yang umum digunakan:

### 1. Basic Authentication

**Konsep:** Klien mengirimkan username dan password dalam header Authorization dengan encoding Base64.

**Cara Kerja:**
1. Klien menggabungkan username dan password dengan titik dua (`username:password`).
2. String gabungan tersebut di-encode menggunakan Base64.
3. Hasil encoding dikirim dalam header HTTP: `Authorization: Basic <encoded_string>`.

**Kelebihan:**
- Sederhana untuk diimplementasikan.

**Kekurangan:**
- Kredensial dikirim pada setiap permintaan.
- Base64 bukanlah enkripsi, mudah di-decode. Wajib menggunakan HTTPS untuk melindungi kredensial saat transit.
- Tidak cocok untuk aplikasi pihak ketiga karena mengharuskan pengguna membagikan kredensial utama mereka.

### 2. API Keys

**Konsep:** Klien diberikan sebuah token unik (API Key) yang harus disertakan dalam setiap permintaan ke API.

**Cara Kerja:**
1. Pengembang mendaftarkan aplikasinya dan mendapatkan API Key.
2. API Key dikirim melalui header HTTP kustom (misalnya, `X-API-Key: <api_key_value>`), query parameter (`?api_key=<api_key_value>`), atau dalam body permintaan.

**Kelebihan:**
- Lebih aman daripada Basic Auth jika dikelola dengan baik.
- Memungkinkan pelacakan penggunaan API per klien.
- Mudah dicabut aksesnya jika API Key terkompromi.

**Kekurangan:**
- Jika API Key bocor, penyerang bisa mendapatkan akses.
- Biasanya tidak mengidentifikasi pengguna akhir secara spesifik, lebih ke aplikasi klien.
- Statis, jika tidak ada mekanisme rotasi.

### 3. Bearer Tokens (JSON Web Tokens - JWT)

**Konsep:** Setelah pengguna berhasil login (autentikasi awal), server mengeluarkan sebuah token (JWT) yang ditandatangani secara digital. Klien kemudian menyertakan token ini dalam header `Authorization` sebagai "Bearer token" untuk permintaan selanjutnya.

**Struktur JWT:** Terdiri dari tiga bagian yang dipisahkan oleh titik (`.`):
- **Header:** Berisi tipe token (JWT) dan algoritma penandatanganan (misalnya, HMAC SHA256 atau RSA).
- **Payload (Claims):** Berisi informasi tentang pengguna (misalnya, ID pengguna, peran) dan metadata token (misalnya, waktu kedaluwarsa). Ini adalah data yang ingin dikirimkan.
- **Signature:** Digunakan untuk memverifikasi bahwa token tidak diubah dan, jika menggunakan algoritma asimetris, untuk memverifikasi siapa pengirim token. Dihasilkan dengan menandatangani header yang di-encode, payload yang di-encode, dan sebuah secret (untuk HMAC) atau private key (untuk RSA).

**Cara Kerja:**
1. Pengguna mengirimkan kredensial (misalnya, username dan password) ke server autentikasi.
2. Server memverifikasi kredensial. Jika valid, server membuat JWT dan mengirimkannya kembali ke klien.
3. Klien menyimpan JWT (biasanya di `localStorage`, `sessionStorage`, atau cookies HTTP-only) dan menyertakannya dalam header `Authorization` pada setiap permintaan ke API yang dilindungi: `Authorization: Bearer <jwt_token>`.
4. Server API memverifikasi tanda tangan JWT pada setiap permintaan. Jika valid, permintaan diproses.

**Kelebihan:**
- **Stateless:** Server tidak perlu menyimpan informasi sesi token. Informasi pengguna ada di dalam token itu sendiri.
- **Skalabilitas:** Cocok untuk arsitektur microservices.
- **Portabilitas:** Dapat digunakan di berbagai domain.
- **Keamanan:** Tanda tangan digital memastikan integritas token.

**Kekurangan:**
- Ukuran token bisa menjadi besar jika terlalu banyak claims.
- Setelah dikeluarkan, JWT tidak dapat dicabut secara langsung sebelum waktu kedaluwarsanya (perlu mekanisme tambahan seperti blacklist token).
- Jika secret key atau private key bocor, keamanan seluruh sistem terancam.

### 4. OAuth 2.0 (Open Authorization)

**Konsep:** Sebuah kerangka kerja otorisasi yang memungkinkan aplikasi pihak ketiga mendapatkan akses terbatas ke sumber daya pengguna di server HTTP, tanpa mengekspos kredensial pengguna. OAuth 2.0 lebih fokus pada delegasi otorisasi daripada autentikasi pengguna secara langsung, meskipun sering digunakan bersamaan dengan OpenID Connect (OIDC) untuk autentikasi.

**Peran Utama dalam OAuth 2.0:**
- **Resource Owner:** Pengguna yang memiliki data (atau dalam kasus M2M, bisa juga Klien itu sendiri yang memiliki sumber daya).
- **Client:** Aplikasi (klien) yang ingin mengakses data atau layanan.
- **Authorization Server:** Server yang mengeluarkan access token setelah memverifikasi identitas Klien dan mendapatkan persetujuannya (atau berdasarkan konfigurasi Klien untuk M2M).
- **Resource Server:** Server API yang menyimpan data/layanan dan menerima access token untuk memberikan akses.

**Alur Umum (misalnya, Authorization Code Grant - untuk aplikasi dengan pengguna):**
1. Klien mengarahkan Resource Owner ke Authorization Server.
2. Resource Owner login ke Authorization Server dan memberikan izin kepada Klien.
3. Authorization Server mengembalikan authorization code ke Klien.
4. Klien menukar authorization code tersebut dengan access token (dan opsional refresh token) dari Authorization Server.
5. Klien menggunakan access token untuk mengakses Resource Server atas nama Resource Owner.

**Alur Client Credentials Grant (untuk Machine-to-Machine - M2M):**

**Konsep:** Digunakan ketika Klien (aplikasi/layanan) mengakses sumber daya miliknya sendiri atau sumber daya lain yang tidak terkait dengan pengguna akhir tertentu. Tidak ada interaksi Resource Owner (pengguna) yang terlibat dalam proses pemberian izin. Klien mengautentikasi dirinya sendiri secara langsung ke Authorization Server untuk mendapatkan access token.

**Kapan Digunakan:**
- Komunikasi antar microservices di backend.
- Aplikasi daemon atau proses batch yang berjalan di server.
- Ketika Klien bertindak atas namanya sendiri, bukan atas nama pengguna.

**Cara Kerja:**
1. Klien (yang sudah terdaftar dan memiliki Client ID serta Client Secret) membuat permintaan langsung ke endpoint token di Authorization Server.
2. Permintaan ini menyertakan `grant_type=client_credentials` dan kredensial Klien (biasanya Client ID dan Client Secret dalam header `Authorization` sebagai Basic Auth, atau dalam body permintaan).
3. Authorization Server mengautentikasi Klien berdasarkan Client ID dan Client Secret.
4. Jika autentikasi berhasil, Authorization Server mengeluarkan access token yang terikat dengan Klien tersebut (bukan pengguna). Scope yang diberikan biasanya telah dikonfigurasi sebelumnya untuk Klien tersebut.
5. Klien menggunakan access token ini untuk mengakses Resource Server.

**Kelebihan OAuth 2.0:**
- Aman untuk aplikasi pihak ketiga (terutama dengan alur yang melibatkan pengguna): Pengguna tidak perlu membagikan kredensial utama mereka.
- Akses terbatas (scoped access): Klien hanya mendapatkan izin untuk tindakan tertentu.
- Pengguna dapat mencabut akses kapan saja (untuk alur yang melibatkan pengguna).
- Menyediakan alur standar untuk berbagai skenario, termasuk M2M (Client Credentials Grant) yang lebih sederhana untuk kasus tersebut.

**Kekurangan OAuth 2.0:**
- Lebih kompleks untuk diimplementasikan dibandingkan metode lain, terutama jika semua alur harus didukung.
- Banyak grant types yang berbeda, perlu dipilih dan dipahami dengan benar sesuai skenario.
- Implementasi yang salah dapat menimbulkan kerentanan.
