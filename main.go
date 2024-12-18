package main

import (
	"log"
	"net/http"
)

func main() {
	const port = "8080"
	smux := http.NewServeMux()

	smux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))

	smux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
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
