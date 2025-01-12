package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
}

func NewApiConfig(hitVal int32) *apiConfig {
	// db init
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Panicln("Could not open sql connection, panic")
	}

	cfg := &apiConfig{
		fileserverHits: atomic.Int32{},
		db:             database.New(db),
		platform:       os.Getenv("PLATFORM"),
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
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
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

	retVal := returnVal{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	respondWithJSON(w, 200, retVal)
}

func (cfg *apiConfig) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type returnVal struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
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
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
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
		respondWithError(w, 500, err.Error())
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

	chirps, err := cfg.db.GetAllChirps(r.Context())

	if err != nil {
		respondWithError(w, 500, err.Error())
		return
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
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	type returnVal struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&params)
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
		UserID: params.UserID,
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
