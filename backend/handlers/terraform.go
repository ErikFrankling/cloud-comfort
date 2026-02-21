package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud-comfort/backend/terraform"
)

// sseWriter adapts an http.ResponseWriter + http.Flusher into an io.Writer
// that sends each written chunk as a Server-Sent Event.
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (s *sseWriter) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte("\n"))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		data, _ := json.Marshal(map[string]string{"line": string(line)})
		fmt.Fprintf(s.w, "data: %s\n\n", data)
		s.f.Flush()
	}
	return len(p), nil
}

func setupSSE(w http.ResponseWriter) (*sseWriter, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil, false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	return &sseWriter{w: w, f: flusher}, true
}

func sendDone(w http.ResponseWriter, f http.Flusher, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func sendError(w http.ResponseWriter, f http.Flusher, err error) {
	data, _ := json.Marshal(map[string]any{"error": err.Error()})
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

// HandleInit returns a handler that runs terraform init with SSE streaming.
func HandleInit(svc *terraform.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse, ok := setupSSE(w)
		if !ok {
			return
		}

		if err := svc.Init(r.Context(), sse); err != nil {
			sendError(w, sse.f, err)
			return
		}

		sendDone(w, sse.f, map[string]any{"done": true})
	}
}

// HandlePlan returns a handler that runs terraform plan with SSE streaming.
// The final event includes the full plan JSON.
func HandlePlan(svc *terraform.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse, ok := setupSSE(w)
		if !ok {
			return
		}

		plan, hasChanges, err := svc.Plan(r.Context(), sse)
		if err != nil {
			sendError(w, sse.f, err)
			return
		}

		sendDone(w, sse.f, map[string]any{
			"done":        true,
			"has_changes": hasChanges,
			"plan":        plan,
		})
	}
}

// HandleApply returns a handler that runs terraform apply with SSE streaming.
func HandleApply(svc *terraform.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse, ok := setupSSE(w)
		if !ok {
			return
		}

		if err := svc.Apply(r.Context(), sse); err != nil {
			sendError(w, sse.f, err)
			return
		}

		sendDone(w, sse.f, map[string]any{"done": true})
	}
}
