package main

import (
	"fmt"
	"net/http"
)

func main() {
	smux := http.NewServeMux()

	server := &http.Server{}
	server.Handler = smux
	server.Addr = ":8080"

	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Server exits with error: %v", err)
	}
}