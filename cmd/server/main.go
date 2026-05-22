package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"whois-parser/internal/api"
)

//go:embed web
var webFiles embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	webFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/lookup", api.LookupHandler)
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	addr := ":" + port
	log.Printf("Listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
