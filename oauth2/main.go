package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

// --- Konfigurasi ---
const (
	dbUser     = "root"
	dbPassword = "" // GANTI DENGAN PASSWORD MYSQL ANDA
	dbHost     = "localhost"
	dbPort     = "3306"
	dbName     = "auth-example" // GANTI JIKA NAMA DB BERBEDA

	jwtSecretKey         = "KunciRahasiaSuperAmanUntukJWTIniHarusPanjangDanAcak" // GANTI DENGAN KUNCI PRODUKSI
	tokenIssuer          = "aplikasi-oauth-saya.com"
	accessTokenDuration  = 1 * time.Hour      // Durasi access token
	authCodeDuration     = 10 * time.Minute   // Durasi authorization code
	refreshTokenDuration = 7 * 24 * time.Hour // Durasi refresh token
)

// --- Model ---
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

type OAuthClient struct {
	ID               int64     `json:"id"`
	ClientID         string    `json:"client_id"`
	ClientSecretHash string    `json:"-"` // Simpan hash dari client secret
	ClientName       string    `json:"client_name"`
	RedirectURIs     []string  `json:"redirect_uris"` // Disimpan sebagai JSON string di DB
	CreatedAt        time.Time `json:"createdAt"`
}

type AuthCode struct {
	ID          int64
	Code        string
	ClientID    string
	UserID      int64
	RedirectURI string
	Scopes      string // Comma-separated string
	ExpiresAt   time.Time
	Used        bool
}

type RefreshToken struct {
	ID        int64
	Token     string // Hash dari refresh token disimpan di DB
	UserID    int64
	ClientID  string
	Scopes    string
	ExpiresAt time.Time
	Revoked   bool
}

type JWTClaims struct {
	UserID   int64    `json:"user_id"`
	Email    string   `json:"email"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
	jwt.RegisteredClaims
}

var db *sql.DB
var loginTmpl *template.Template // Untuk halaman login sederhana

// --- Inisialisasi ---
func initDB() {
	var err error
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", dbUser, dbPassword, dbHost, dbPort, dbName)
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error membuka koneksi database: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("Error ping ke database: %v", err)
	}
	log.Println("Berhasil terhubung ke database MySQL.")

	// Membuat tabel-tabel
	queries := []string{
		`CREATE TABLE IF NOT EXISTS user (
            id INT AUTO_INCREMENT PRIMARY KEY,
            email VARCHAR(255) UNIQUE NOT NULL,
            password VARCHAR(255) NOT NULL,
            createdAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
            id INT AUTO_INCREMENT PRIMARY KEY,
            client_id VARCHAR(255) UNIQUE NOT NULL,
            client_secret_hash VARCHAR(255) NOT NULL,
            client_name VARCHAR(255) NOT NULL,
            redirect_uris TEXT NOT NULL, -- JSON array string
            createdAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS oauth_auth_codes (
            id INT AUTO_INCREMENT PRIMARY KEY,
            code VARCHAR(255) UNIQUE NOT NULL,
            client_id VARCHAR(255) NOT NULL,
            user_id INT NOT NULL,
            redirect_uri VARCHAR(1024) NOT NULL,
            scopes TEXT,
            expires_at TIMESTAMP NOT NULL,
            used BOOLEAN DEFAULT FALSE,
            createdAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (user_id) REFERENCES user(id) ON DELETE CASCADE,
            FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
		`CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
            id INT AUTO_INCREMENT PRIMARY KEY,
            token_hash VARCHAR(255) UNIQUE NOT NULL, -- Simpan hash dari refresh token
            user_id INT NOT NULL,
            client_id VARCHAR(255) NOT NULL,
            scopes TEXT,
            expires_at TIMESTAMP NOT NULL,
            revoked BOOLEAN DEFAULT FALSE,
            createdAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (user_id) REFERENCES user(id) ON DELETE CASCADE,
            FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	}

	for _, query := range queries {
		if _, err = db.Exec(query); err != nil {
			log.Fatalf("Error membuat tabel: %v\nQuery: %s", err, query)
		}
	}
	log.Println("Semua tabel OAuth 2.0 siap atau sudah ada.")
}

func loadTemplates() {
	// Halaman login sederhana
	loginTmpl = template.Must(template.New("login.html").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Login untuk Otorisasi</title>
    <style>
        body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; background-color: #f4f4f4; margin: 0; }
        .container { background-color: #fff; padding: 30px; border-radius: 8px; box-shadow: 0 0 15px rgba(0,0,0,0.1); width: 300px; }
        h2 { text-align: center; color: #333; }
        label { display: block; margin-bottom: 8px; color: #555; }
        input[type="email"], input[type="password"] { width: calc(100% - 20px); padding: 10px; margin-bottom: 15px; border: 1px solid #ddd; border-radius: 4px; }
        input[type="submit"] { background-color: #007bff; color: white; padding: 10px 15px; border: none; border-radius: 4px; cursor: pointer; width: 100%; }
        input[type="submit"]:hover { background-color: #0056b3; }
        .error { color: red; text-align: center; margin-bottom: 10px; }
        .hidden-fields input { display: none; }
    </style>
</head>
<body>
    <div class="container">
        <h2>Login untuk Otorisasi Klien: {{.ClientName}}</h2>
        {{if .Error}}
            <p class="error">{{.Error}}</p>
        {{end}}
        <form method="POST" action="/oauth/authorize">
            <div class="hidden-fields">
                <input type="hidden" name="response_type" value="{{.ResponseType}}">
                <input type="hidden" name="client_id" value="{{.ClientID}}">
                <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
                <input type="hidden" name="scope" value="{{.Scope}}">
                <input type="hidden" name="state" value="{{.State}}">
            </div>
            <div>
                <label for="email">Email:</label>
                <input type="email" id="email" name="email" required>
            </div>
            <div>
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required>
            </div>
            <input type="submit" value="Login & Otorisasi">
        </form>
    </div>
</body>
</html>
    `))
}

// --- Fungsi Helper ---
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func generateSecureRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func hashStringSHA256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// --- Fungsi Database (CRUD) ---
func createUser(email, password string) (User, error) {
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	result, err := db.Exec("INSERT INTO user (email, password) VALUES (?, ?)", email, hashedPassword)
	if err != nil {
		return User{}, err
	}
	id, _ := result.LastInsertId()
	return User{ID: id, Email: email, CreatedAt: time.Now()}, nil
}

func getUserByEmail(email string) (User, error) {
	var user User
	var createdAtRaw []byte
	err := db.QueryRow("SELECT id, email, password, createdAt FROM user WHERE email = ?", email).Scan(&user.ID, &user.Email, &user.Password, &createdAtRaw)
	if err != nil {
		return User{}, err
	}
	layout := "2006-01-02 15:04:05" // Format timestamp dari MySQL
	user.CreatedAt, _ = time.Parse(layout, string(createdAtRaw))
	return user, nil
}

func createOAuthClient(name string, redirectURIs []string) (OAuthClient, string, error) {
	clientID := uuid.New().String()
	rawClientSecret, err := generateSecureRandomString(32)
	if err != nil {
		return OAuthClient{}, "", err
	}
	clientSecretHash, err := hashPassword(rawClientSecret) // Hash client secret sebelum disimpan
	if err != nil {
		return OAuthClient{}, "", err
	}

	redirectURIsJSON, err := json.Marshal(redirectURIs)
	if err != nil {
		return OAuthClient{}, "", err
	}

	_, err = db.Exec("INSERT INTO oauth_clients (client_id, client_secret_hash, client_name, redirect_uris) VALUES (?, ?, ?, ?)",
		clientID, clientSecretHash, name, string(redirectURIsJSON))
	if err != nil {
		return OAuthClient{}, "", err
	}
	// Ambil ID yang baru dibuat untuk konsistensi, meskipun tidak selalu dibutuhkan
	var id int64
	err = db.QueryRow("SELECT id FROM oauth_clients WHERE client_id = ?", clientID).Scan(&id)
	if err != nil {
		// Ini seharusnya tidak terjadi jika INSERT berhasil
		log.Printf("Peringatan: Gagal mengambil ID klien yang baru dibuat: %v", err)
	}

	return OAuthClient{ID: id, ClientID: clientID, ClientName: name, RedirectURIs: redirectURIs, CreatedAt: time.Now()}, rawClientSecret, nil
}

func getOAuthClient(clientID string) (OAuthClient, error) {
	var client OAuthClient
	var redirectURIsJSON string
	var createdAtRaw []byte
	err := db.QueryRow("SELECT id, client_id, client_secret_hash, client_name, redirect_uris, createdAt FROM oauth_clients WHERE client_id = ?", clientID).Scan(
		&client.ID, &client.ClientID, &client.ClientSecretHash, &client.ClientName, &redirectURIsJSON, &createdAtRaw)
	if err != nil {
		return OAuthClient{}, err
	}
	if err := json.Unmarshal([]byte(redirectURIsJSON), &client.RedirectURIs); err != nil {
		return OAuthClient{}, err
	}
	layout := "2006-01-02 15:04:05"
	client.CreatedAt, _ = time.Parse(layout, string(createdAtRaw))
	return client, nil
}

func storeAuthCode(code, clientID string, userID int64, redirectURI, scopes string) error {
	expiresAt := time.Now().Add(authCodeDuration)
	_, err := db.Exec("INSERT INTO oauth_auth_codes (code, client_id, user_id, redirect_uri, scopes, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		code, clientID, userID, redirectURI, scopes, expiresAt)
	return err
}

func getAuthCode(code string) (AuthCode, error) {
	var authCode AuthCode
	err := db.QueryRow("SELECT id, code, client_id, user_id, redirect_uri, scopes, expires_at, used FROM oauth_auth_codes WHERE code = ?", code).Scan(
		&authCode.ID, &authCode.Code, &authCode.ClientID, &authCode.UserID, &authCode.RedirectURI, &authCode.Scopes, &authCode.ExpiresAt, &authCode.Used)
	return authCode, err
}

func markAuthCodeAsUsed(code string) error {
	_, err := db.Exec("UPDATE oauth_auth_codes SET used = TRUE WHERE code = ?", code)
	return err
}

func storeRefreshToken(rawToken string, userID int64, clientID, scopes string) error {
	tokenHash := hashStringSHA256(rawToken) // Simpan hash dari refresh token
	expiresAt := time.Now().Add(refreshTokenDuration)
	_, err := db.Exec("INSERT INTO oauth_refresh_tokens (token_hash, user_id, client_id, scopes, expires_at) VALUES (?, ?, ?, ?, ?)",
		tokenHash, userID, clientID, scopes, expiresAt)
	return err
}

func getRefreshToken(rawToken string) (RefreshToken, error) {
	tokenHash := hashStringSHA256(rawToken)
	var rt RefreshToken
	err := db.QueryRow("SELECT id, token_hash, user_id, client_id, scopes, expires_at, revoked FROM oauth_refresh_tokens WHERE token_hash = ?", tokenHash).Scan(
		&rt.ID, &rt.Token, &rt.UserID, &rt.ClientID, &rt.Scopes, &rt.ExpiresAt, &rt.Revoked)
	return rt, err
}

func revokeRefreshToken(rawToken string) error {
	tokenHash := hashStringSHA256(rawToken)
	_, err := db.Exec("UPDATE oauth_refresh_tokens SET revoked = TRUE WHERE token_hash = ?", tokenHash)
	return err
}

// --- Fungsi JWT ---
func generateAccessToken(userID int64, email, clientID string, scopes []string) (string, error) {
	expirationTime := time.Now().Add(accessTokenDuration)
	claims := &JWTClaims{
		UserID:   userID,
		Email:    email,
		ClientID: clientID,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    tokenIssuer,
			Subject:   fmt.Sprintf("%d", userID),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecretKey))
}

func validateAccessToken(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing tidak terduga: %v", token.Header["alg"])
		}
		return []byte(jwtSecretKey), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("token tidak valid")
	}
	return claims, nil
}

// --- Middleware ---
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Header Authorization tidak ditemukan", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && strings.ToLower(parts[0]) == "bearer") {
			http.Error(w, "Format header Authorization tidak valid", http.StatusUnauthorized)
			return
		}
		claims, err := validateAccessToken(parts[1])
		if err != nil {
			http.Error(w, "Token tidak valid: "+err.Error(), http.StatusUnauthorized)
			return
		}
		// Simpan claims di context untuk digunakan oleh handler
		ctx := context.WithValue(r.Context(), "userClaims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// --- Handler HTTP ---
func registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Request body tidak valid", http.StatusBadRequest)
		return
	}
	if creds.Email == "" || creds.Password == "" {
		http.Error(w, "Email dan password diperlukan", http.StatusBadRequest)
		return
	}
	user, err := createUser(creds.Email, creds.Password)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			http.Error(w, "Email sudah terdaftar", http.StatusConflict)
			return
		}
		http.Error(w, "Gagal mendaftarkan pengguna: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": user.ID, "email": user.Email})
}

func registerClientHandler(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Request body tidak valid", http.StatusBadRequest)
		return
	}
	if reqBody.ClientName == "" || len(reqBody.RedirectURIs) == 0 {
		http.Error(w, "Nama klien dan minimal satu redirect URI diperlukan", http.StatusBadRequest)
		return
	}
	// Validasi sederhana redirect URI
	for _, uri := range reqBody.RedirectURIs {
		if _, err := url.ParseRequestURI(uri); err != nil {
			http.Error(w, fmt.Sprintf("Redirect URI tidak valid: %s", uri), http.StatusBadRequest)
			return
		}
	}

	client, rawSecret, err := createOAuthClient(reqBody.ClientName, reqBody.RedirectURIs)
	if err != nil {
		http.Error(w, "Gagal mendaftarkan klien: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"client_id":     client.ClientID,
		"client_secret": rawSecret, // Kirim secret mentah HANYA SEKALI saat registrasi
		"client_name":   client.ClientName,
	})
}

func authorizeHandler(w http.ResponseWriter, r *http.Request) {

	// Jika metode GET, tampilkan halaman login/persetujuan
	if r.Method == http.MethodGet {

		// Ambil parameter dari query string
		clientID := r.URL.Query().Get("client_id")
		responseType := r.URL.Query().Get("response_type")
		redirectURI := r.URL.Query().Get("redirect_uri")
		scope := r.URL.Query().Get("scope") // Bisa comma-separated
		state := r.URL.Query().Get("state") // Opsional tapi direkomendasikan

		if clientID == "" || responseType == "" || redirectURI == "" {
			http.Error(w, "Parameter client_id, response_type, dan redirect_uri diperlukan", http.StatusBadRequest)
			return
		}
		if responseType != "code" {
			http.Error(w, "response_type harus 'code'", http.StatusBadRequest)
			return
		}

		client, err := getOAuthClient(clientID)
		if err != nil {
			http.Error(w, "client_id tidak valid", http.StatusBadRequest)
			return
		}

		validRedirectURI := false
		for _, uri := range client.RedirectURIs {
			if uri == redirectURI {
				validRedirectURI = true
				break
			}
		}
		if !validRedirectURI {
			http.Error(w, "redirect_uri tidak valid untuk klien ini", http.StatusBadRequest)
			return
		}

		// Di aplikasi nyata, Anda akan memeriksa sesi pengguna. Jika sudah login, langsung ke persetujuan.
		// Untuk contoh ini, kita selalu tampilkan form login yang akan POST ke endpoint ini lagi.
		data := map[string]string{
			"ClientID":     clientID,
			"ClientName":   client.ClientName,
			"ResponseType": responseType,
			"RedirectURI":  redirectURI,
			"Scope":        scope,
			"State":        state,
			"Error":        r.URL.Query().Get("error"), // Jika ada error dari POST sebelumnya
		}
		loginTmpl.Execute(w, data)
		return
	}

	// Jika metode POST, proses login dan persetujuan
	if r.Method == http.MethodPost {
		r.ParseForm()
		email := r.FormValue("email")
		password := r.FormValue("password")
		// Ambil kembali parameter OAuth dari hidden fields
		postClientID := r.FormValue("client_id")
		postResponseType := r.FormValue("response_type")
		postRedirectURI := r.FormValue("redirect_uri")
		postScope := r.FormValue("scope")
		postState := r.FormValue("state")

		user, err := getUserByEmail(email)
		if err != nil || !checkPassword(password, user.Password) {
			// Redirect kembali ke form login dengan pesan error
			errorMsg := url.QueryEscape("Email atau password salah.")
			http.Redirect(w, r, fmt.Sprintf("/oauth/authorize?response_type=%s&client_id=%s&redirect_uri=%s&scope=%s&state=%s&error=%s",
				postResponseType, postClientID, url.QueryEscape(postRedirectURI), url.QueryEscape(postScope), url.QueryEscape(postState), errorMsg), http.StatusFound)
			return
		}

		// Pengguna berhasil login.
		// Di aplikasi nyata, di sini ada langkah persetujuan cakupan (scopes).
		// Untuk contoh ini, kita anggap pengguna selalu setuju.
		authCodeVal, err := generateSecureRandomString(32)
		if err != nil {
			http.Error(w, "Gagal membuat authorization code", http.StatusInternalServerError)
			return
		}
		if err := storeAuthCode(authCodeVal, postClientID, user.ID, postRedirectURI, postScope); err != nil {
			http.Error(w, "Gagal menyimpan authorization code", http.StatusInternalServerError)
			return
		}

		// Redirect ke client redirect_uri dengan code dan state
		redirectURL, _ := url.Parse(postRedirectURI)
		q := redirectURL.Query()
		q.Set("code", authCodeVal)
		if postState != "" {
			q.Set("state", postState)
		}
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	}
}

func tokenHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal mem-parsing form data", http.StatusBadRequest)
		return
	}

	grantType := r.PostFormValue("grant_type")
	clientID := r.PostFormValue("client_id")
	clientSecret := r.PostFormValue("client_secret") // Klien mengirim secret mentah

	client, err := getOAuthClient(clientID)
	if err != nil {
		http.Error(w, "client_id tidak valid", http.StatusUnauthorized)
		return
	}
	// Verifikasi client secret
	if !checkPassword(clientSecret, client.ClientSecretHash) {
		http.Error(w, "client_secret tidak valid", http.StatusUnauthorized)
		return
	}

	var accessToken, newRefreshTokenValue string
	var expiresIn int64
	var user User
	var scopes []string

	if grantType == "authorization_code" {
		code := r.PostFormValue("code")
		redirectURI := r.PostFormValue("redirect_uri") // Harus sama dengan yang digunakan saat meminta code

		authCode, err := getAuthCode(code)
		log.Printf("AuthCode: ", authCode, "\n")
		if err != nil || authCode.Used || time.Now().After(authCode.ExpiresAt) || authCode.ClientID != clientID || authCode.RedirectURI != redirectURI {
			http.Error(w, "Authorization code tidak valid, kedaluwarsa, atau sudah digunakan", http.StatusBadRequest)
			return
		}

		if err := markAuthCodeAsUsed(code); err != nil {
			http.Error(w, "Gagal menandai authorization code sebagai terpakai", http.StatusInternalServerError)
			return
		}

		// Ambil data pengguna
		err = db.QueryRow("SELECT id, email FROM user WHERE id = ?", authCode.UserID).Scan(&user.ID, &user.Email)
		if err != nil {
			http.Error(w, "Gagal mengambil data pengguna", http.StatusInternalServerError)
			return
		}
		scopes = strings.Split(authCode.Scopes, ",") // Asumsi comma-separated

	} else if grantType == "refresh_token" {
		refreshTokenValue := r.PostFormValue("refresh_token")
		rt, err := getRefreshToken(refreshTokenValue)
		if err != nil || rt.Revoked || time.Now().After(rt.ExpiresAt) || rt.ClientID != clientID {
			http.Error(w, "Refresh token tidak valid, kedaluwarsa, atau dicabut", http.StatusBadRequest)
			return
		}
		// Opsional: cabut refresh token lama setelah digunakan (untuk keamanan lebih)
		// if err := revokeRefreshToken(refreshTokenValue); err != nil { /* log error */ }

		err = db.QueryRow("SELECT id, email FROM user WHERE id = ?", rt.UserID).Scan(&user.ID, &user.Email)
		if err != nil {
			http.Error(w, "Gagal mengambil data pengguna dari refresh token", http.StatusInternalServerError)
			return
		}
		scopes = strings.Split(rt.Scopes, ",")

	} else {
		http.Error(w, "grant_type tidak didukung", http.StatusBadRequest)
		return
	}

	// Buat access token
	accessToken, err = generateAccessToken(user.ID, user.Email, clientID, scopes)
	if err != nil {
		http.Error(w, "Gagal membuat access token", http.StatusInternalServerError)
		return
	}
	expiresIn = int64(accessTokenDuration.Seconds())

	// Buat refresh token baru jika grant_type adalah authorization_code atau jika Anda ingin merotasi refresh token
	if grantType == "authorization_code" { // Selalu buat refresh token baru untuk auth_code
		newRefreshTokenValue, err = generateSecureRandomString(32)
		if err != nil {
			http.Error(w, "Gagal membuat refresh token", http.StatusInternalServerError)
			return
		}
		if err := storeRefreshToken(newRefreshTokenValue, user.ID, clientID, strings.Join(scopes, ",")); err != nil {
			http.Error(w, "Gagal menyimpan refresh token", http.StatusInternalServerError)
			return
		}
	} else if grantType == "refresh_token" {
		// Jika Anda ingin merotasi refresh token (praktik yang baik):
		// 1. Cabut refresh token lama (sudah dilakukan atau bisa dilakukan di sini)
		// 2. Buat refresh token baru
		// Untuk contoh ini, kita tidak merotasi refresh token yang sudah ada, tapi Anda bisa menambahkannya.
		// Jika tidak merotasi, refresh token yang ada tetap valid sampai kedaluwarsa atau dicabut manual.
		// newRefreshTokenValue = r.PostFormValue("refresh_token") // Gunakan kembali yang lama jika tidak dirotasi
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": newRefreshTokenValue, // Hanya kirim jika baru dibuat atau dirotasi
		"scope":         strings.Join(scopes, " "),
	})
}

func protectedResourceHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("userClaims").(*JWTClaims)
	if !ok {
		http.Error(w, "Gagal mendapatkan claims pengguna dari context", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Halo %s (User ID: %d, Client ID: %s), Anda berhasil mengakses sumber daya terproteksi!", claims.Email, claims.UserID, claims.ClientID),
		"scopes":  claims.Scopes,
		"data":    "Ini adalah data super rahasia.",
	})
}

// --- Fungsi Main ---
func main() {
	initDB()
	loadTemplates()
	defer db.Close()

	// Inisialisasi pengguna/klien awal jika ada argumen
	if len(os.Args) > 1 {
		if os.Args[1] == "inituser" {
			email := "user@example.com"
			password := "password123"
			if len(os.Args) > 2 {
				email = os.Args[2]
			}
			if len(os.Args) > 3 {
				password = os.Args[3]
			}
			if _, err := createUser(email, password); err != nil {
				if strings.Contains(err.Error(), "Duplicate entry") {
					log.Printf("Pengguna '%s' sudah ada.", email)
				} else {
					log.Fatalf("Gagal membuat pengguna awal: %v", err)
				}
			} else {
				log.Printf("Pengguna awal '%s' berhasil dibuat.", email)
			}
			return
		}
		if os.Args[1] == "initclient" {
			clientName := "Aplikasi Klien Contoh"
			redirectURI := "http://localhost:3000/callback" // Ganti dengan URI klien Anda
			if len(os.Args) > 2 {
				clientName = os.Args[2]
			}
			if len(os.Args) > 3 {
				redirectURI = os.Args[3]
			}

			client, secret, err := createOAuthClient(clientName, []string{redirectURI})
			if err != nil {
				if strings.Contains(err.Error(), "Duplicate entry") {
					log.Printf("Klien '%s' atau ID-nya sudah ada.", clientName)
				} else {
					log.Fatalf("Gagal membuat klien awal: %v", err)
				}
			} else {
				log.Printf("Klien awal '%s' berhasil dibuat.", clientName)
				log.Printf("Client ID: %s", client.ClientID)
				log.Printf("Client Secret: %s (SIMPAN INI DENGAN AMAN!)", secret)
			}
			return
		}
	}

	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log.Printf("[%s] %s %s", req.Method, req.RequestURI, req.RemoteAddr)
			next.ServeHTTP(w, req)
		})
	})

	r.HandleFunc("/register-user", registerUserHandler).Methods("POST")
	r.HandleFunc("/register-client", registerClientHandler).Methods("POST") // Sebaiknya diamankan

	r.HandleFunc("/oauth/authorize", authorizeHandler).Methods("GET", "POST")
	r.HandleFunc("/oauth/token", tokenHandler).Methods("POST")

	r.HandleFunc("/api/protected", authMiddleware(protectedResourceHandler)).Methods("GET")

	port := "8080"
	log.Printf("Server OAuth 2.0 berjalan di http://localhost:%s", port)
	log.Println("Gunakan 'go run main.go inituser [email] [password]' atau 'go run main.go initclient [nama_klien] [redirect_uri]' untuk setup awal.")
	log.Fatal(http.ListenAndServe(":"+port, r))
}
