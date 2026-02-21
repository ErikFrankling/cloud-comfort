package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type fileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func isValidTFFilename(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return false
	}
	return strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tfvars")
}

// HandleListFiles returns a handler that lists .tf files in the workdir.
func HandleListFiles(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(workDir)
		if err != nil {
			if os.IsNotExist(err) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]fileInfo{})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		files := []fileInfo{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tfvars") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, fileInfo{Name: name, Size: info.Size()})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	}
}

// HandleGetFile returns a handler that serves a .tf file's content.
func HandleGetFile(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !isValidTFFilename(name) {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}

		content, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(content)
	}
}

// HandleUploadFile returns a handler that writes a .tf file to the workdir.
func HandleUploadFile(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !isValidTFFilename(name) {
			http.Error(w, "invalid filename: must end with .tf or .tfvars and contain no path separators", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		if err := os.MkdirAll(workDir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(filepath.Join(workDir, name), body, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "uploaded", "name": name})
	}
}

// HandleDeleteFile returns a handler that deletes a .tf file from the workdir.
func HandleDeleteFile(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !isValidTFFilename(name) {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}

		path := filepath.Join(workDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}

		if err := os.Remove(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
	}
}
