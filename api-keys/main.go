package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // Driver MySQL
	"github.com/gorilla/mux"           // Router
)

// APIKeyRecord struct untuk menyimpan data API Key dari database
type APIKeyRecord struct {
	ID         int64        `json:"id"`
	ClientName string       `json:"client_name"`
	KeyPrefix  string       `json:"key_prefix"` // Beberapa karakter awal dari API Key asli untuk identifikasi
	APIKeyHash string       `json:"-"`          // Hash dari API Key, tidak dikirim ke klien
	IsActive   bool         `json:"is_active"`
	CreatedAt  time.Time    `json:"created_at"`
	LastUsedAt sql.NullTime `json:"last_used_at"` // Menggunakan sql.NullTime untuk kolom yang bisa NULL
}

// Variabel global untuk koneksi database
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

const apiKeyHeader = "X-API-Key" // Nama header untuk API Key
const apiKeyPrefixLength = 8     // Panjang prefix API Key yang disimpan untuk identifikasi

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

	// Buat tabel api_keys jika belum ada
	createTableQuery := `
        CREATE TABLE IF NOT EXISTS api_keys (
            id INT AUTO_INCREMENT PRIMARY KEY,
            client_name VARCHAR(255) NOT NULL,
            key_prefix VARCHAR(10) NOT NULL UNIQUE, -- Untuk identifikasi cepat, bukan untuk auth
            api_key_hash VARCHAR(64) NOT NULL UNIQUE, -- SHA256 hash dalam hex (64 karakter)
            is_active BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            last_used_at TIMESTAMP NULL DEFAULT NULL
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
    `
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Error membuat tabel api_keys: %v", err)
	}
	log.Println("Tabel 'api_keys' siap atau sudah ada.")
}

// generateAPIKey menghasilkan string API Key yang aman secara kriptografis.
// Format: PREFIX_RANDOMBYTES (misalnya, "sk_"... untuk secret key)
// Mengembalikan API Key mentah dan prefix-nya.
func generateAPIKey(prefix string, length int) (string, string, error) {
	if length <= 0 {
		length = 32 // Default length for random bytes
	}
	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", "", fmt.Errorf("gagal menghasilkan byte acak: %w", err)
	}
	// Menggunakan base64 URL encoding untuk karakter yang aman di URL dan header
	apiKey := prefix + "_" + base64.URLEncoding.EncodeToString(randomBytes)

	// Ambil prefix dari API Key yang baru dibuat untuk disimpan
	// Pastikan tidak melebihi panjang API Key itu sendiri
	var keyPrefixForDB string
	if len(apiKey) > apiKeyPrefixLength {
		keyPrefixForDB = apiKey[:apiKeyPrefixLength]
	} else {
		keyPrefixForDB = apiKey
	}

	return apiKey, keyPrefixForDB, nil
}

// hashAPIKey menghasilkan hash SHA256 dari API Key.
func hashAPIKey(apiKey string) string {
	hasher := sha256.New()
	hasher.Write([]byte(apiKey))
	return hex.EncodeToString(hasher.Sum(nil))
}

// storeAPIKey menyimpan informasi klien beserta hash dari API Key ke database.
// Mengembalikan APIKeyRecord tanpa hash.
func storeAPIKey(clientName, apiKey, keyPrefixForDB string) (APIKeyRecord, error) {
	apiKeyHash := hashAPIKey(apiKey)

	result, err := db.Exec(
		"INSERT INTO api_keys (client_name, key_prefix, api_key_hash, is_active) VALUES (?, ?, ?, ?)",
		clientName, keyPrefixForDB, apiKeyHash, true,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			if strings.Contains(err.Error(), "'key_prefix'") {
				return APIKeyRecord{}, fmt.Errorf("prefix kunci '%s' sudah digunakan, coba lagi: %w", keyPrefixForDB, err)
			}
			return APIKeyRecord{}, fmt.Errorf("hash kunci API sudah ada (kemungkinan tabrakan hash atau kunci duplikat): %w", err)
		}
		return APIKeyRecord{}, fmt.Errorf("gagal menyimpan API Key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return APIKeyRecord{}, fmt.Errorf("gagal mendapatkan ID terakhir yang dimasukkan: %w", err)
	}

	return APIKeyRecord{
		ID:         id,
		ClientName: clientName,
		KeyPrefix:  keyPrefixForDB,
		IsActive:   true,
		CreatedAt:  time.Now(), // Waktu saat ini
	}, nil
}

// validateAPIKey memeriksa apakah API Key yang diberikan valid dan aktif.
// Mengembalikan record APIKey jika valid, atau error jika tidak.
func validateAPIKey(apiKeyFromHeader string) (*APIKeyRecord, error) {
	if apiKeyFromHeader == "" {
		return nil, fmt.Errorf("API Key tidak boleh kosong")
	}

	apiKeyHash := hashAPIKey(apiKeyFromHeader)

	var apiKeyRec APIKeyRecord
	var lastUsed sql.NullTime // Variabel untuk menampung last_used_at

	row := db.QueryRow(
		"SELECT id, client_name, key_prefix, api_key_hash, is_active, created_at, last_used_at FROM api_keys WHERE api_key_hash = ? AND is_active = TRUE",
		apiKeyHash,
	)
	err := row.Scan(
		&apiKeyRec.ID,
		&apiKeyRec.ClientName,
		&apiKeyRec.KeyPrefix,
		&apiKeyRec.APIKeyHash, // Meskipun kita tidak membutuhkannya lagi, kita scan untuk konsistensi
		&apiKeyRec.IsActive,
		&apiKeyRec.CreatedAt,
		&lastUsed, // Scan ke sql.NullTime
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("API Key tidak valid atau tidak aktif")
		}
		return nil, fmt.Errorf("error saat memvalidasi API Key: %w", err)
	}
	apiKeyRec.LastUsedAt = lastUsed // Tetapkan nilai yang discan

	return &apiKeyRec, nil
}

// recordAPIKeyUsage memperbarui kolom last_used_at untuk API Key yang diberikan.
func recordAPIKeyUsage(apiKeyID int64) error {
	_, err := db.Exec("UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", apiKeyID)
	if err != nil {
		return fmt.Errorf("gagal mencatat penggunaan API Key: %w", err)
	}
	return nil
}

// --- Middleware Autentikasi API Key ---

func apiKeyAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get(apiKeyHeader)
		if apiKey == "" {
			log.Println("Upaya akses tanpa API Key.")
			http.Error(w, "Akses Ditolak: API Key diperlukan.", http.StatusUnauthorized)
			return
		}

		apiKeyRecord, err := validateAPIKey(apiKey)
		if err != nil {
			log.Printf("Upaya akses dengan API Key tidak valid ('%s'): %v", apiKey[:min(len(apiKey), 12)]+"...", err) // Log prefix kunci
			http.Error(w, "Akses Ditolak: API Key tidak valid atau tidak aktif.", http.StatusForbidden)
			return
		}

		// API Key valid
		log.Printf("Akses diberikan untuk klien: %s (ID Kunci: %d, Prefix: %s)", apiKeyRecord.ClientName, apiKeyRecord.ID, apiKeyRecord.KeyPrefix)

		// Catat penggunaan API Key (bisa dijalankan sebagai goroutine jika tidak ingin memblokir)
		go func(id int64) {
			if err := recordAPIKeyUsage(id); err != nil {
				log.Printf("Peringatan: Gagal mencatat penggunaan API Key untuk ID %d: %v", id, err)
			}
		}(apiKeyRecord.ID)

		// Anda bisa menambahkan informasi klien ke context request jika diperlukan oleh handler selanjutnya
		// ctx := context.WithValue(r.Context(), "clientName", apiKeyRecord.ClientName)
		// next.ServeHTTP(w, r.WithContext(ctx))
		next.ServeHTTP(w, r)
	}
}

// Fungsi utilitas min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Handler Rute ---

// registerClientHandler menangani pembuatan API Key baru untuk klien.
// PENTING: Endpoint ini idealnya harus diamankan (misalnya, hanya untuk admin).
// Untuk contoh ini, kita biarkan terbuka.
func registerClientHandler(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		ClientName string `json:"client_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Request body tidak valid.", http.StatusBadRequest)
		return
	}

	if requestBody.ClientName == "" {
		http.Error(w, "Nama klien ('client_name') diperlukan.", http.StatusBadRequest)
		return
	}

	// Generate API Key baru
	// "myapp" adalah contoh prefix, Anda bisa membuatnya lebih dinamis atau tetap
	rawAPIKey, keyPrefixForDB, err := generateAPIKey("myapp", 32) // 32 byte random menghasilkan ~43 char base64
	if err != nil {
		log.Printf("Error menghasilkan API Key: %v", err)
		http.Error(w, "Gagal menghasilkan API Key.", http.StatusInternalServerError)
		return
	}

	// Simpan API Key (hash-nya) ke database
	// Loop kecil untuk menangani kemungkinan tabrakan prefix yang sangat jarang terjadi
	var storedRecord APIKeyRecord
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		storedRecord, err = storeAPIKey(requestBody.ClientName, rawAPIKey, keyPrefixForDB)
		if err == nil {
			break // Berhasil disimpan
		}
		if strings.Contains(err.Error(), "prefix kunci") && i < maxRetries-1 {
			log.Printf("Tabrakan prefix kunci, mencoba generate ulang (%d/%d)...", i+1, maxRetries)
			rawAPIKey, keyPrefixForDB, err = generateAPIKey("myapp", 32) // Generate ulang
			if err != nil {
				log.Printf("Error menghasilkan API Key ulang: %v", err)
				http.Error(w, "Gagal menghasilkan API Key ulang.", http.StatusInternalServerError)
				return
			}
			continue
		}
		log.Printf("Error menyimpan API Key untuk klien '%s': %v", requestBody.ClientName, err)
		http.Error(w, "Gagal menyimpan API Key.", http.StatusInternalServerError)
		return
	}
	if err != nil { // Jika masih error setelah retry
		log.Printf("Gagal menyimpan API Key setelah %d percobaan untuk klien '%s': %v", maxRetries, requestBody.ClientName, err)
		http.Error(w, "Gagal menyimpan API Key setelah beberapa percobaan.", http.StatusInternalServerError)
		return
	}

	log.Printf("API Key baru dibuat untuk klien: %s, Prefix: %s", storedRecord.ClientName, storedRecord.KeyPrefix)

	// Kirim API Key MENTAH ke klien. Ini adalah SATU-SATUNYA saat klien melihat kunci ini.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message":     "API Key berhasil dibuat. Simpan kunci ini dengan aman!",
		"client_name": storedRecord.ClientName,
		"api_key":     rawAPIKey, // Kunci mentah
		"key_prefix":  storedRecord.KeyPrefix,
	})
}

// protectedResourceHandler adalah contoh endpoint yang dilindungi.
func protectedResourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Selamat! Anda berhasil mengakses sumber daya terproteksi dengan API Key.",
		"data":    "Ini adalah data rahasia yang hanya bisa diakses dengan API Key yang valid.",
	})
}

// publicResourceHandler adalah contoh endpoint publik.
func publicResourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Ini adalah sumber daya publik, tidak memerlukan API Key.",
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

	// Inisialisasi klien contoh jika argumen "initclient" diberikan
	// Ini hanya untuk memudahkan pengujian, di produksi Anda akan memiliki cara lain untuk mengelola ini.
	if len(os.Args) > 1 && os.Args[1] == "initclient" {
		clientName := "Klien Pengujian Awal"
		if len(os.Args) > 2 {
			clientName = os.Args[2] // Ambil nama klien dari argumen kedua jika ada
		}

		rawAPIKey, keyPrefixForDB, err := generateAPIKey("init", 32)
		if err != nil {
			log.Fatalf("Gagal generate API Key untuk initclient: %v", err)
		}

		_, err = storeAPIKey(clientName, rawAPIKey, keyPrefixForDB)
		if err != nil {
			if strings.Contains(err.Error(), "sudah digunakan") || strings.Contains(err.Error(), "sudah ada") {
				log.Printf("Klien '%s' atau API Key-nya mungkin sudah ada.", clientName)
			} else {
				log.Fatalf("Gagal menyimpan API Key untuk initclient '%s': %v", clientName, err)
			}
		} else {
			log.Printf("API Key MENTAH untuk klien '%s' (prefix: %s) adalah: %s. SIMPAN INI!", clientName, keyPrefixForDB, rawAPIKey)
			log.Println("Ini hanya ditampilkan sekali saat inisialisasi.")
		}
		return // Keluar setelah inisialisasi
	}

	// Router
	r := mux.NewRouter()

	// Middleware untuk logging setiap request (sederhana)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			log.Printf("[%s] %s %s %s", req.Method, req.RequestURI, req.RemoteAddr, time.Since(start))
		})
	})

	// Rute
	// Endpoint untuk registrasi klien dan mendapatkan API Key
	r.HandleFunc("/register-client", registerClientHandler).Methods("POST")

	// Endpoint publik
	r.HandleFunc("/api/public-resource", publicResourceHandler).Methods("GET")

	// Endpoint yang dilindungi API Key
	// Cara 1: Menerapkan middleware langsung ke handler
	r.HandleFunc("/api/protected-resource", apiKeyAuthMiddleware(protectedResourceHandler)).Methods("GET")

	// Cara 2: Membuat subrouter dan menerapkan middleware ke subrouter (jika punya banyak endpoint terproteksi)
	// apiProtected := r.PathPrefix("/api/v2").Subrouter()
	// apiProtected.Use(apiKeyAuthMiddleware) // Middleware diterapkan ke semua rute di bawah /api/v2
	// apiProtected.HandleFunc("/another-protected", anotherProtectedHandler).Methods("GET")

	// Handler untuk rute tidak ditemukan
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Maaf, endpoint tidak ditemukan.", http.StatusNotFound)
	})

	port := "8080" // Port server Go
	log.Printf("Server Go berjalan di http://localhost:%s", port)
	log.Println("Gunakan 'go run main.go initclient [NamaKlienOpsional]' untuk membuat API Key awal jika diperlukan.")

	// Mulai server HTTP
	srv := &http.Server{
		Handler:      r,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
