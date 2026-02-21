package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"cloud-comfort/backend/handlers"
	"cloud-comfort/backend/llm"
	"cloud-comfort/backend/terraform"

	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	// Load .env file — check current dir and parent (project root)
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	workDir := filepath.Join("workdir")
	tfSvc, err := terraform.NewService(workDir)
	if err != nil {
		log.Fatal(err)
	}

	tfSvc.SetEnv(collectCloudEnv())

	// Create LLM client
	llmClient := llm.NewClient(llm.Config{
		BaseURL:   getEnvOr("LLM_BASE_URL", "https://openrouter.ai/api/v1"),
		APIKey:    os.Getenv("LLM_API_KEY"),
		Model:     getEnvOr("LLM_MODEL", "openai/gpt-4o"),
		Streaming: getEnvOr("LLM_STREAMING", "true") == "true",
	})

	absWorkDir := tfSvc.WorkDir()

	mux := http.NewServeMux()

	// Terraform operations (SSE streaming)
	mux.HandleFunc("POST /api/terraform/init", handlers.HandleInit(tfSvc))
	mux.HandleFunc("POST /api/terraform/plan", handlers.HandlePlan(tfSvc))
	mux.HandleFunc("POST /api/terraform/apply", handlers.HandleApply(tfSvc))

	// CORS preflight for terraform endpoints
	mux.HandleFunc("OPTIONS /api/terraform/init", handlers.HandlePreflight)
	mux.HandleFunc("OPTIONS /api/terraform/plan", handlers.HandlePreflight)
	mux.HandleFunc("OPTIONS /api/terraform/apply", handlers.HandlePreflight)

	// File management
	mux.HandleFunc("GET /api/terraform/files", handlers.HandleListFiles(absWorkDir))
	mux.HandleFunc("GET /api/terraform/files/{name}", handlers.HandleGetFile(absWorkDir))
	mux.HandleFunc("PUT /api/terraform/files/{name}", handlers.HandleUploadFile(absWorkDir))
	mux.HandleFunc("DELETE /api/terraform/files/{name}", handlers.HandleDeleteFile(absWorkDir))

	// Chat endpoint (SSE streaming with LLM + validation)
	mux.HandleFunc("POST /api/chat", handlers.HandleChat(llmClient, tfSvc))
	// Diagram generation
	mux.HandleFunc("POST /api/diagram", handlers.HandleDiagram(tfSvc))

	// Health check
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})

	log.Println("Backend listening on :8080")
	if err := http.ListenAndServe(":8080", c.Handler(mux)); err != nil {
		log.Fatal(err)
	}
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// collectCloudEnv reads common cloud auth env vars and returns them as a map.
func collectCloudEnv() map[string]string {
	keys := []string{
		// AWS
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION",
		// GCP
		"GOOGLE_CREDENTIALS", "GOOGLE_PROJECT", "GOOGLE_REGION",
		// Azure
		"ARM_CLIENT_ID", "ARM_CLIENT_SECRET", "ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID",
	}
	env := make(map[string]string)
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return env
}
