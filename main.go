package main

import (
	"bufio"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
)

func main() {
	const port = "8080"
	smux := http.NewServeMux()

	apiCfg := NewApiConfig(0)

	smux.Handle("/app/", apiCfg.middlewareMetricsInc(
		http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
	),
	)

	smux.HandleFunc("GET /metrics", apiCfg.HandleMetrics)
	smux.HandleFunc("POST /reset", apiCfg.HandleReset)

	smux.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
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
}

func NewApiConfig(hitVal int32) *apiConfig {
	cfg := &apiConfig{
		fileserverHits: atomic.Int32{},
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
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	buf := bufio.NewWriter(w)
	defer buf.Flush()
	buf.WriteString("Hits: ")
	log.Printf("metrics hits: %v\n", cfg.fileserverHits.Load())
	buf.WriteString(strconv.Itoa(int(cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) HandleReset(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	log.Printf("reset the hits to %v", cfg.fileserverHits.Load())
}
