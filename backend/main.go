package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"cloud-comfort/backend/handlers"

	"github.com/rs/cors"
)

func main() {
	workDir := filepath.Join("workdir")

	mux := http.NewServeMux()

	// Diagram generation
	mux.HandleFunc("POST /api/diagram", handlers.HandleDiagram(workDir))

	// Health check
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	})

	log.Println("Backend listening on :8080")
	if err := http.ListenAndServe(":8080", c.Handler(mux)); err != nil {
		log.Fatal(err)
	}
}
