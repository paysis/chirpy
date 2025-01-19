package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/paysis/chirpy/internal/auth"
	"github.com/paysis/chirpy/internal/database"
)

func main() {
	godotenv.Load()

	const port = "8080"
	smux := http.NewServeMux()

	apiCfg := NewApiConfig(0)

	smux.Handle("/app/", apiCfg.middlewareMetricsInc(
		http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
	),
	)

	smux.HandleFunc("GET /admin/metrics", apiCfg.HandleMetrics)
	smux.HandleFunc("POST /admin/reset", apiCfg.HandleReset)

	smux.HandleFunc("POST /api/users", apiCfg.HandleCreateUser)
	smux.HandleFunc("POST /api/login", apiCfg.HandleLogin)
	smux.HandleFunc("POST /api/chirps", apiCfg.HandleCreateChirp)
	smux.HandleFunc("GET /api/chirps", apiCfg.HandleGetAllChirps)
	smux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.HandleGetChirp)
	smux.HandleFunc("POST /api/refresh", apiCfg.HandleRefreshToken)
	smux.HandleFunc("POST /api/revoke", apiCfg.HandleRevoke)
	smux.HandleFunc("PUT /api/users", apiCfg.HandleUpdateUser)
	smux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.HandleDeleteChirp)

	smux.HandleFunc("POST /api/polka/webhooks", apiCfg.HandlePolkaWebhook)

	smux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Handler: smux,
		Addr:    ":" + port,
	}

	log.Printf("Running on port: %s\n", port)
	log.Fatal(server.ListenAndServe())
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	jwtSecret      string
	polkaSecret    string
}

func NewApiConfig(hitVal int32) *apiConfig {
	// db init
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Panicln("Could not open sql connection, panic")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	polkaSecret := os.Getenv("POLKA_KEY")

	cfg := &apiConfig{
		fileserverHits: atomic.Int32{},
		db:             database.New(db),
		platform:       os.Getenv("PLATFORM"),
		jwtSecret:      jwtSecret,
		polkaSecret:    polkaSecret,
	}
	cfg.fileserverHits.Store(hitVal)
	return cfg
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("hits before middleware: %v\n", cfg.fileserverHits.Load())
		_ = cfg.fileserverHits.Add(1)
		log.Printf("hits after middleware: %v\n", cfg.fileserverHits.Load())
		next.ServeHTTP(w, req)
	})
}

func (cfg *apiConfig) HandleMetrics(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)

	hitCount := int(cfg.fileserverHits.Load())

	buf := bufio.NewWriter(w)
	defer buf.Flush()
	buf.WriteString("<html>")
	buf.WriteString("<body>")
	buf.WriteString("<h1>Welcome, Chirpy Admin</h1>")
	buf.WriteString(fmt.Sprintf("<p>Chirpy has been visited %d times!</p>", hitCount))
	buf.WriteString("</body>")
	buf.WriteString("</html>")
}

func (cfg *apiConfig) HandleReset(w http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
		return
	}

	cfg.fileserverHits.Store(0)
	log.Printf("reset the hits to %v", cfg.fileserverHits.Load())
	err := cfg.db.DeleteAllUsers(req.Context())
	if err != nil {
		log.Printf("Could not delete all users: %v\n", err)
	}

	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
}

func (cfg *apiConfig) HandleLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type returnVal struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		IsChirpyRed  bool      `json:"is_chirpy_red"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&params)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	dbUser, err := cfg.db.GetUserByEmail(r.Context(), params.Email)

	if err != nil {
		respondWithError(w, 500, "Something went wrong with db")
		return
	}

	err = auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)

	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	token, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	refreshToken, err := auth.MakeRefreshToken()

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	dbToken := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    dbUser.ID,
		ExpiresAt: time.Now().UTC().Add(60 * 24 * time.Hour),
	}

	err = cfg.db.CreateRefreshToken(r.Context(), dbToken)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	retVal := returnVal{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		IsChirpyRed:  dbUser.IsChirpyRed,
		Token:        token,
		RefreshToken: refreshToken,
	}

	respondWithJSON(w, 200, retVal)
}

func (cfg *apiConfig) HandleRefreshToken(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	row, err := cfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	if row.ExpiresAt.Before(time.Now().UTC()) {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	if row.RevokedAt.Valid {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	jwtToken, err := auth.MakeJWT(row.UserID, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	type retval struct {
		Token string `json:"token"`
	}

	retVal := retval{
		Token: jwtToken,
	}

	respondWithJSON(w, 200, retVal)
}

func (cfg *apiConfig) HandleDeleteChirp(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	if chirpID == "" {
		respondWithError(w, 403, "Forbidden")
		return
	}

	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, 403, "Forbidden")
		return
	}

	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userId, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	chirp, err := cfg.db.GetChirp(r.Context(), chirpUUID)

	if err != nil {
		respondWithError(w, 404, "Not found")
		return
	}

	if chirp.UserID != userId {
		respondWithError(w, 403, "Forbidden")
		return
	}

	err = cfg.db.DeleteChirp(r.Context(), chirp.ID)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) HandlePolkaWebhook(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID uuid.UUID `json:"user_id"`
		} `json:"data"`
	}

	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	if apiKey != cfg.polkaSecret {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, "Bad request")
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(204) // we do not care about other events
		return
	}

	dbParams := database.UpdateUserRedParams{
		IsChirpyRed: true,
		ID:          params.Data.UserID,
	}
	err = cfg.db.UpdateUserRed(r.Context(), dbParams)
	if err != nil {
		respondWithError(w, 404, "Not found")
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type returnVal struct {
		ID          uuid.UUID `json:"id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Email       string    `json:"email"`
		IsChirpyRed bool      `json:"is_chirpy_red"`
	}

	jwtToken, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userId, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	user, err := cfg.db.GetUserById(r.Context(), userId)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	dbParams := database.UpdateUserParams{
		ID:             user.ID,
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}
	dbUser, err := cfg.db.UpdateUser(r.Context(), dbParams)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	retval := returnVal{
		ID:          dbUser.ID,
		CreatedAt:   dbUser.CreatedAt,
		UpdatedAt:   dbUser.UpdatedAt,
		Email:       dbUser.Email,
		IsChirpyRed: dbUser.IsChirpyRed,
	}

	respondWithJSON(w, 200, retval)
}

func (cfg *apiConfig) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	err = cfg.db.RevokeRefreshToken(r.Context(), refreshToken)

	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type returnVal struct {
		ID          uuid.UUID `json:"id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Email       string    `json:"email"`
		IsChirpyRed bool      `json:"is_chirpy_red"`
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)

	if err != nil {
		respondWithError(w, 400, "Could not hash the password")
		return
	}

	dbUser := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	user, err := cfg.db.CreateUser(r.Context(), dbUser)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	retVal := returnVal{
		ID:          user.ID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	}

	respondWithJSON(w, 201, retVal)
}

func (cfg *apiConfig) HandleGetChirp(w http.ResponseWriter, r *http.Request) {
	type returnVal struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))

	if err != nil {
		log.Printf("uuid Parse returned err: %v\n", err)
		respondWithError(w, 400, "Please make sure the chirp ID is of type UUID")
		return
	}

	chirp, err := cfg.db.GetChirp(r.Context(), chirpID)

	if err != nil {
		respondWithError(w, 404, "Not found")
		return
	}

	retVal := returnVal{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	respondWithJSON(w, 200, retVal)
}

func (cfg *apiConfig) HandleGetAllChirps(w http.ResponseWriter, r *http.Request) {
	type returnVal struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	authorId := r.URL.Query().Get("author_id")
	authorUUID, err := uuid.Parse(authorId)

	sortArg := r.URL.Query().Get("sort")
	sortByAsc := true

	if sortArg == "desc" {
		sortByAsc = false
	}

	if err != nil {
		authorId = ""
	}

	var chirps []database.Chirp

	if authorId == "" {
		chirps, err = cfg.db.GetAllChirps(r.Context())
	} else {
		chirps, err = cfg.db.GetChirpsByUserId(r.Context(), authorUUID)
	}

	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	if !sortByAsc {
		slices.SortFunc(chirps, func(a database.Chirp, b database.Chirp) int {
			if a.CreatedAt.After(b.CreatedAt) {
				return -1
			} else if a.CreatedAt.Before(b.CreatedAt) {
				return 1
			} else {
				return 0
			}
		})
	}

	retVals := make([]returnVal, 0, len(chirps))
	for _, chirp := range chirps {
		retVals = append(retVals, returnVal{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})
	}

	respondWithJSON(w, 200, retVals)
}

func (cfg *apiConfig) HandleCreateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVal struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	token, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)

	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	// profane
	profaneList := []string{
		"kerfuffle",
		"sharbert",
		"fornax",
	}

	params.Body = filterProfaneWords(params.Body, profaneList)

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   params.Body,
		UserID: userId,
	})

	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	respondWithJSON(w, 201, returnVal{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})
}

func filterProfaneWords(src string, profaneList []string) string {
	splits := strings.Split(src, " ")

	for i, word := range splits {
		for _, profane := range profaneList {
			if strings.ToLower(word) == profane {
				splits[i] = "****"
			}
		}
	}

	finalText := strings.Join(splits, " ")
	return finalText
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorVal struct {
		Error string `json:"error"`
	}

	data, err := json.Marshal(errorVal{
		Error: msg,
	})

	if err != nil {
		log.Printf("Can't marshal")
		return
	}

	w.WriteHeader(code)
	w.Write(data)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Panicf("Can't marshal, panic: %v\n", payload)
	}

	w.WriteHeader(code)
	w.Write(data)
}
