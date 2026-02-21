package main

import (
	"fmt"
	"log"
	"net/http"

	"cloud-comfort/backend/handlers"

	"github.com/rs/cors"
)

func main() {
	mux := http.NewServeMux()

	// Terraform endpoints
	mux.HandleFunc("POST /api/terraform/init", handlers.HandleInit)
	mux.HandleFunc("POST /api/terraform/plan", handlers.HandlePlan)
	mux.HandleFunc("POST /api/terraform/apply", handlers.HandleApply)

	// Chat endpoint
	mux.HandleFunc("POST /api/chat", handlers.HandleChat)

	// Health check
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// CORS for frontend dev server
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})

	handler := c.Handler(mux)

	log.Println("Backend listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
