package main

import (
	"log"
	"net/http"
)

func main() {
	const port = "8080"
	smux := http.NewServeMux()

	smux.Handle("/", http.FileServer(http.Dir(".")))

	server := &http.Server{
		Handler: smux,
		Addr:    ":" + port,
	}

	log.Printf("Running on port: %s\n", port)
	log.Fatal(server.ListenAndServe())
}
