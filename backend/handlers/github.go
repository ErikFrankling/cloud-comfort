package handlers

import (
	"encoding/json"
	"net/http"

	"cloud-comfort/backend/github"
)

// GitHubExploreRequest is the request body for the explore endpoint
type GitHubExploreRequest struct {
	Repo   string `json:"repo"`   // owner/repo format
	Branch string `json:"branch"` // optional, defaults to main
}

// HandleGitHubExplore returns a handler that explores a GitHub repository
func HandleGitHubExplore(githubSvc *github.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if githubSvc == nil {
			http.Error(w, "GitHub service not configured - missing token", http.StatusServiceUnavailable)
			return
		}

		var req GitHubExploreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Repo == "" {
			http.Error(w, "Missing repo field", http.StatusBadRequest)
			return
		}

		if req.Branch == "" {
			req.Branch = "main"
		}

		ctx := r.Context()
		repoContext, err := githubSvc.ExploreRepo(ctx, req.Repo, req.Branch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(repoContext)
	}
}
