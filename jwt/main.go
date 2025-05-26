package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // Driver MySQL
	"github.com/golang-jwt/jwt/v5"     // Untuk JWT
	"github.com/gorilla/mux"           // Router
	"golang.org/x/crypto/bcrypt"       // Untuk hashing password
)

// --- Konfigurasi ---
// !!! SESUAIKAN DETAIL INI DENGAN KONFIGURASI ANDA !!!
const (
	dbUser     = "root"
	dbPassword = "root"      // Ganti dengan password MySQL Anda
	dbHost     = "localhost" // atau alamat IP server MySQL Anda
	dbPort     = "3306"
	dbName     = "auth-example" // Nama database yang telah Anda buat

	// PENTING: Ganti jwtSecretKey dengan kunci rahasia yang kuat dan acak di lingkungan produksi!
	// Sebaiknya disimpan sebagai environment variable.
	jwtSecretKey = "KunciRahasiaSuperKuatDanPanjangUntukJWT"
	tokenIssuer  = "aplikasi-saya.com"
)

// --- Model ---

// User struct untuk menyimpan data pengguna dari database
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // Jangan kirim hash password ke klien
	CreatedAt time.Time `json:"createdAt"`
}

// Claims struct untuk data yang akan disimpan dalam JWT
type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Variabel global untuk koneksi database
var db *sql.DB

// --- Fungsi-fungsi Database ---

// initDB menginisialisasi koneksi ke database MySQL dan membuat tabel jika belum ada.
func initDB() {
	var err error
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", dbUser, dbPassword, dbHost, dbPort, dbName)
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error membuka koneksi database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Error ping ke database: %v", err)
	}
	log.Println("Berhasil terhubung ke database MySQL.")

	// Buat tabel user jika belum ada
	createTableQuery := `
        CREATE TABLE IF NOT EXISTS user (
            id INT AUTO_INCREMENT PRIMARY KEY,
            email VARCHAR(255) UNIQUE NOT NULL,
            password VARCHAR(255) NOT NULL,
            createdAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
    `
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Error membuat tabel user: %v", err)
	}
	log.Println("Tabel 'user' siap atau sudah ada.")
}

// addUser menambahkan pengguna baru ke database dengan password yang di-hash.
func addUser(email, password string) (User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("error hashing password: %w", err)
	}

	result, err := db.Exec("INSERT INTO user (email, password) VALUES (?, ?)", email, hashedPassword)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return User{}, fmt.Errorf("email '%s' sudah digunakan: %w", email, err)
		}
		return User{}, fmt.Errorf("error menambahkan pengguna: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("error mendapatkan last insert ID: %w", err)
	}

	return User{ID: id, Email: email, CreatedAt: time.Now()}, nil
}

// findUserByEmail mencari pengguna berdasarkan email.
func findUserByEmail(email string) (User, error) {
	var user User
	var createdAtRaw []byte // Untuk menangani format timestamp dari MySQL

	row := db.QueryRow("SELECT id, email, password, createdAt FROM user WHERE email = ?", email)
	err := row.Scan(&user.ID, &user.Email, &user.Password, &createdAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, fmt.Errorf("pengguna '%s' tidak ditemukan", email)
		}
		return User{}, fmt.Errorf("error mencari pengguna: %w", err)
	}

	// Konversi createdAtRaw (yang merupakan []uint8 atau []byte) ke time.Time
	// Format timestamp dari MySQL biasanya 'YYYY-MM-DD HH:MM:SS'
	layout := "2006-01-02 15:04:05"
	createdAtStr := string(createdAtRaw)
	user.CreatedAt, err = time.Parse(layout, createdAtStr)
	if err != nil {
		log.Printf("Peringatan: Gagal mem-parsing createdAt untuk pengguna %s: %v. Menggunakan time.Now() sebagai fallback.", user.Email, err)
		user.CreatedAt = time.Now() // Fallback jika parsing gagal
	}

	return user, nil
}

// verifyPassword membandingkan password plain text dengan hash yang tersimpan.
func verifyPassword(plainPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	return err == nil
}

// --- Fungsi-fungsi JWT ---

// generateJWT membuat dan menandatangani JWT baru untuk pengguna.
func generateJWT(user User) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour) // Token berlaku selama 24 jam
	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    tokenIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecretKey))
	if err != nil {
		return "", fmt.Errorf("gagal menandatangani token: %w", err)
	}
	return tokenString, nil
}

// validateJWT memvalidasi token JWT yang diberikan.
// Mengembalikan claims jika token valid, atau error jika tidak.
func validateJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Pastikan metode signing adalah yang diharapkan (HS256)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing tidak terduga: %v", token.Header["alg"])
		}
		return []byte(jwtSecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("gagal mem-parsing token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token tidak valid")
	}

	return claims, nil
}

// --- Middleware Autentikasi JWT ---

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Akses Ditolak: Header Authorization tidak ditemukan.", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && strings.ToLower(parts[0]) == "bearer") {
			http.Error(w, "Akses Ditolak: Format header Authorization tidak valid (harus 'Bearer token').", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := validateJWT(tokenString)
		if err != nil {
			log.Printf("Validasi JWT gagal: %v", err)
			http.Error(w, "Akses Ditolak: Token tidak valid atau kedaluwarsa.", http.StatusUnauthorized)
			return
		}

		// Token valid. Anda bisa menyimpan claims di context jika diperlukan oleh handler selanjutnya.
		log.Printf("Akses diberikan untuk pengguna: %s (ID: %d)", claims.Email, claims.UserID)
		// Contoh menyimpan email di context (opsional):
		// ctx := context.WithValue(r.Context(), "email", claims.Email)
		// ctx = context.WithValue(ctx, "userID", claims.UserID)
		// next.ServeHTTP(w, r.WithContext(ctx))
		next.ServeHTTP(w, r)
	}
}

// --- Handler Rute ---

// registerHandler menangani registrasi pengguna baru.
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Request body tidak valid.", http.StatusBadRequest)
		return
	}

	if creds.Email == "" || creds.Password == "" {
		http.Error(w, "Email dan password diperlukan.", http.StatusBadRequest)
		return
	}
	if len(creds.Password) < 6 { // Contoh validasi panjang password
		http.Error(w, "Password minimal harus 6 karakter.", http.StatusBadRequest)
		return
	}

	user, err := addUser(creds.Email, creds.Password)
	if err != nil {
		if strings.Contains(err.Error(), "sudah digunakan") {
			http.Error(w, err.Error(), http.StatusConflict) // 409 Conflict
			return
		}
		log.Printf("Error saat registrasi pengguna '%s': %v", creds.Email, err)
		http.Error(w, "Error internal server saat registrasi.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Pengguna '%s' berhasil dibuat.", user.Email),
		"user_id": user.ID,
		"email":   user.Email,
	})
}

// loginHandler menangani login pengguna dan pembuatan JWT.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Request body tidak valid.", http.StatusBadRequest)
		return
	}

	if creds.Email == "" || creds.Password == "" {
		http.Error(w, "Email dan password diperlukan.", http.StatusBadRequest)
		return
	}

	user, err := findUserByEmail(creds.Email)
	if err != nil {
		log.Printf("Upaya login gagal (pengguna tidak ditemukan): %s", creds.Email)
		http.Error(w, "Email atau password salah.", http.StatusUnauthorized)
		return
	}

	if !verifyPassword(creds.Password, user.Password) {
		log.Printf("Upaya login gagal (password salah): %s", creds.Email)
		http.Error(w, "Email atau password salah.", http.StatusUnauthorized)
		return
	}

	tokenString, err := generateJWT(user)
	if err != nil {
		log.Printf("Error membuat JWT untuk pengguna '%s': %v", user.Email, err)
		http.Error(w, "Gagal membuat token autentikasi.", http.StatusInternalServerError)
		return
	}

	log.Printf("Pengguna '%s' berhasil login. Token JWT dibuat.", user.Email)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Login berhasil!",
		"token":   tokenString,
	})
}

// protectedHandler adalah contoh endpoint yang dilindungi.
func protectedHandler(w http.ResponseWriter, r *http.Request) {
	// Jika Anda menyimpan data pengguna dari JWT di context, Anda bisa mengambilnya di sini.
	// Misalnya: email := r.Context().Value("email").(string)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Selamat! Anda berhasil mengakses sumber daya terproteksi dengan JWT.",
		"data":    "Ini adalah data rahasia yang hanya bisa diakses dengan token yang valid.",
	})
}

// publicHandler adalah contoh endpoint publik.
func publicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Ini adalah sumber daya publik, tidak memerlukan autentikasi JWT.",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// --- Fungsi Main ---

func main() {
	// Inisialisasi database
	initDB()
	defer func() {
		if db != nil {
			db.Close()
			log.Println("Koneksi database MySQL ditutup.")
		}
	}()

	// Inisialisasi pengguna admin jika argumen "initadmin" diberikan
	if len(os.Args) > 1 && os.Args[1] == "initadmin" {
		adminEmail := "admin@gmail.com"
		adminPassword := "password123" // Ganti dengan password yang lebih aman jika perlu
		if len(os.Args) > 2 {
			adminEmail = os.Args[2]
		}
		if len(os.Args) > 3 {
			adminPassword = os.Args[3]
		}

		_, err := addUser(adminEmail, adminPassword)
		if err != nil {
			if strings.Contains(err.Error(), "sudah digunakan") {
				log.Printf("Pengguna '%s' sudah ada.", adminEmail)
			} else {
				log.Printf("Error menambahkan pengguna '%s': %v", adminEmail, err)
			}
		} else {
			log.Printf("Pengguna '%s' berhasil ditambahkan dengan password '%s'.", adminEmail, adminPassword)
		}
		return // Keluar setelah inisialisasi
	}

	// Router
	r := mux.NewRouter()

	// Middleware untuk logging setiap request (sederhana)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			// Panggil handler berikutnya
			next.ServeHTTP(w, req)
			// Catat log setelah handler selesai
			log.Printf("[%s] %s %s %s", req.Method, req.RequestURI, req.RemoteAddr, time.Since(start))
		})
	})

	// Rute Autentikasi
	r.HandleFunc("/register", registerHandler).Methods("POST")
	r.HandleFunc("/login", loginHandler).Methods("POST")

	// Rute Publik
	r.HandleFunc("/api/public", publicHandler).Methods("GET")

	// Rute Terproteksi (memerlukan JWT)
	r.HandleFunc("/api/protected", authMiddleware(protectedHandler)).Methods("GET")

	// Handler untuk rute tidak ditemukan
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Maaf, endpoint tidak ditemukan.", http.StatusNotFound)
	})

	port := "8080" // Port server Go
	log.Printf("Server Go berjalan di http://localhost:%s", port)
	log.Println("Gunakan 'go run main.go initadmin [email_opsional] [password_opsional]' untuk membuat pengguna awal jika diperlukan.")

	// Mulai server HTTP
	srv := &http.Server{
		Handler:      r,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
