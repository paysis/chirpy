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

	smux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	smux.HandleFunc("POST /api/validate_chirp", HandleValidateChirpy)

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

func (cfg *apiConfig) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
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

	user, err := cfg.db.CreateUser(r.Context(), params.Email)
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

func HandleValidateChirpy(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVal struct {
		CleanedBody string `json:"cleaned_body"`
	}

	params := parameters{}
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Something went wrong")
		return
	}

	// length validation
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

	cleanBody := filterProfaneWords(params.Body, profaneList)

	retVal := returnVal{
		CleanedBody: cleanBody,
	}

	respondWithJSON(w, 200, retVal)
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
