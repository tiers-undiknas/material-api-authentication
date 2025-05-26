package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // Driver MySQL
	"github.com/gorilla/mux"           // Router
	"golang.org/x/crypto/bcrypt"       // Untuk hashing password
)

// User struct untuk menyimpan data pengguna dari database
type User struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Password string `json:"-"` // Jangan kirim hash password ke klien
}

// Variabel global untuk koneksi database (dalam aplikasi nyata, pertimbangkan dependency injection)
var db *sql.DB

// --- Konfigurasi Database ---
// !!! SESUAIKAN DETAIL INI DENGAN KONFIGURASI MYSQL ANDA !!!
const (
	dbUser     = "root"
	dbPassword = "root"      // Ganti dengan password MySQL Anda
	dbHost     = "localhost" // atau alamat IP server MySQL Anda
	dbPort     = "3306"
	dbName     = "auth-example" // Nama database yang telah Anda buat
)

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

	// Buat tabel users jika belum ada
	createTableQuery := `
        CREATE TABLE IF NOT EXISTS user (
            id INT AUTO_INCREMENT PRIMARY KEY,
            email VARCHAR(255) UNIQUE NOT NULL,
            password VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
    `
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Error membuat tabel users: %v", err)
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
		// Cek apakah error karena duplikat email
		if strings.Contains(err.Error(), "Duplicate entry") {
			return User{}, fmt.Errorf("email '%s' sudah digunakan: %w", email, err)
		}
		return User{}, fmt.Errorf("error menambahkan pengguna: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("error mendapatkan last insert ID: %w", err)
	}

	return User{ID: id, Email: email}, nil
}

// findUserByemail mencari pengguna berdasarkan email.
func findUserByemail(email string) (User, error) {
	var user User
	row := db.QueryRow("SELECT id, email, password FROM user WHERE email = ?", email)
	err := row.Scan(&user.ID, &user.Email, &user.Password)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, fmt.Errorf("pengguna '%s' tidak ditemukan", email)
		}
		return User{}, fmt.Errorf("error mencari pengguna: %w", err)
	}
	return user, nil
}

// verifyPassword membandingkan password plain text dengan hash yang tersimpan.
func verifyPassword(plainPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	return err == nil
}

// --- Middleware Autentikasi ---

// basicAuthMiddleware adalah middleware untuk Basic Authentication.
func basicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		//jika header Authorization tidak berisi data
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Area Terproteksi"`)
			http.Error(w, "Header otorisasi tidak ditemukan.", http.StatusUnauthorized)
			return
		}

		//jika header Authorization tidak mengandung basic dan base64 email:password
		parts := strings.SplitN(authHeader, " ", 2) // split "basic base64(email:password) menjadi array
		if len(parts) != 2 || strings.ToLower(parts[0]) != "basic" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Area Terproteksi"`)
			http.Error(w, "Tipe otorisasi tidak valid. Harap gunakan Basic Auth.", http.StatusUnauthorized)
			return
		}

		//jika email:password tidak di decode menjadi base64
		payload, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "Format kredensial tidak valid (base64 decode error).", http.StatusBadRequest)
			return
		}

		pair := strings.SplitN(string(payload), ":", 2)       // split email:password menjadi array
		if len(pair) != 2 || pair[0] == "" || pair[1] == "" { //secara berurutan jika panjang array tidak 2 || email kosong || password kosong
			http.Error(w, "email atau password tidak ada dalam kredensial.", http.StatusBadRequest)
			return
		}

		email := pair[0]
		password := pair[1]

		user, err := findUserByemail(email) // mencari email dari tabel user

		//jika pengguna tidak ditemukan
		if err != nil {
			log.Printf("Upaya login gagal: Pengguna '%s' tidak ditemukan: %v", email, err)
			w.Header().Set("WWW-Authenticate", `Basic realm="Area Terproteksi"`)
			http.Error(w, "Kredensial tidak valid (pengguna tidak ditemukan).", http.StatusUnauthorized)
			return
		}

		//jika password tidak sesuai dengan email
		if !verifyPassword(password, user.Password) {
			log.Printf("Upaya login gagal: Password salah untuk pengguna '%s'.", email)
			w.Header().Set("WWW-Authenticate", `Basic realm="Area Terproteksi"`)
			http.Error(w, "Kredensial tidak valid (password salah).", http.StatusUnauthorized)
			return
		}

		// Autentikasi berhasil. Anda bisa menyimpan info user di context jika perlu.
		// Untuk contoh ini, kita langsung panggil handler berikutnya.
		log.Printf("Pengguna '%s' berhasil login.", email)
		// Simpan email di context untuk digunakan oleh handler (opsional)
		// ctx := context.WithValue(r.Context(), "email", user.email)
		// next.ServeHTTP(w, r.WithContext(ctx))
		next.ServeHTTP(w, r)
	}
}

// --- Handler Rute ---

// registerUserHandler menangani registrasi pengguna baru.
// Di produksi, endpoint ini harus diproteksi atau dihapus.
func registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Request body tidak valid.", http.StatusBadRequest)
		return
	}

	if creds.Email == "" || creds.Password == "" {
		http.Error(w, "email dan password diperlukan.", http.StatusBadRequest)
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

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Pengguna '%s' berhasil dibuat.", user.Email),
		"userId":  user.ID,
	})
}

// protectedDataHandler menangani permintaan ke endpoint yang dilindungi.
func protectedDataHandler(w http.ResponseWriter, r *http.Request) {
	// email := r.Context().Value("email").(string) // Ambil email dari context jika disimpan
	// Untuk kesederhanaan, kita tidak mengambil email dari context di contoh ini,
	// karena middleware sudah memvalidasi.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message": "Anda berhasil mengakses data terproteksi dari API!",
		"data": []map[string]interface{}{
			{"id": 1, "item": "Data Rahasia API 1"},
			{"id": 2, "item": "Data Rahasia API 2"},
		},
	})
}

// publicDataHandler menangani permintaan ke endpoint publik.
func publicDataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Ini adalah data publik dari API yang bisa diakses siapa saja.",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func logHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s %s\n", req.RemoteAddr, req.Method, req.URL)
		next.ServeHTTP(w, req)
	})
}

// --- Fungsi Main ---

func main() {
	// Inisialisasi database
	initDB()
	defer db.Close() // Pastikan koneksi database ditutup saat aplikasi berhenti

	// Inisialisasi pengguna admin jika argumen "initadmin" diberikan
	if len(os.Args) > 1 && os.Args[1] == "initadmin" {
		_, err := addUser("admin", "password123")
		if err != nil {
			if strings.Contains(err.Error(), "sudah digunakan") {
				log.Println("Pengguna 'admin' sudah ada.")
			} else {
				log.Printf("Error menambahkan pengguna admin: %v", err)
			}
		} else {
			log.Println("Pengguna 'admin' berhasil ditambahkan dengan password 'password123'.")
		}
		// Keluar setelah inisialisasi admin agar tidak menjalankan server
		return
	}

	// Router
	r := mux.NewRouter()

	// Middleware untuk logging setiap request (sederhana)
	r.Use(logHandler)

	// Rute
	r.HandleFunc("/register", registerUserHandler).Methods("POST")
	r.HandleFunc("/api/public-data", publicDataHandler).Methods("GET")
	r.HandleFunc("/api/protected-data", basicAuthMiddleware(protectedDataHandler)).Methods("GET")

	// Handler untuk rute tidak ditemukan
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Maaf, halaman tidak ditemukan.", http.StatusNotFound)
	})

	port := "8080" // Port server Go
	log.Printf("Server Go berjalan di http://localhost:%s", port)
	log.Println("Gunakan 'go run main.go initadmin' untuk membuat pengguna 'admin' jika belum ada.")

	// Mulai server HTTP
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		log.Fatalf("Error memulai server: %v", err)
	}
}
