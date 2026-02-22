package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

// Service wraps the GitHub API client for repo exploration
type Service struct {
	client *github.Client
	token  string
}

// FileInfo represents a file or directory in the repo
type FileInfo struct {
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "dir"
	Size int    `json:"size,omitempty"`
}

// RepoContext holds all information about an explored repository
type RepoContext struct {
	Repo     string        `json:"repo"`
	Branch   string        `json:"branch"`
	Valid    bool          `json:"valid"`
	Error    string        `json:"error,omitempty"`
	Metadata *RepoMetadata `json:"metadata,omitempty"`
	FileTree []FileInfo    `json:"file_tree"`
}

// RepoMetadata holds repository information
type RepoMetadata struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Language      string `json:"language"`
	Stars         int    `json:"stars"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

// NewService creates a GitHub service with the provided token
func NewService(token string) *Service {
	if token == "" {
		return nil
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &Service{
		client: client,
		token:  token,
	}
}

// ExploreRepo loads the full file tree and metadata for a repository
func (s *Service) ExploreRepo(ctx context.Context, repo, branch string) (*RepoContext, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	if branch == "" {
		branch = "main"
	}

	// Get repo metadata
	repository, resp, err := s.client.Repositories.Get(ctx, owner, repoName)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &RepoContext{
				Repo:   repo,
				Branch: branch,
				Valid:  false,
				Error:  "Repository not found or not accessible",
			}, nil
		}
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return &RepoContext{
				Repo:   repo,
				Branch: branch,
				Valid:  false,
				Error:  "Authentication failed - check GitHub token",
			}, nil
		}
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// Use repo's default branch if none specified
	if branch == "" && repository.DefaultBranch != nil {
		branch = *repository.DefaultBranch
	}

	// Get file tree recursively
	tree, resp, err := s.client.Git.GetTree(ctx, owner, repoName, branch, true)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			// Try listing contents instead for root level
			return s.exploreFromRoot(ctx, owner, repoName, branch, repository)
		}
		return nil, fmt.Errorf("failed to get file tree: %w", err)
	}

	// Convert tree entries to FileInfo
	fileTree := make([]FileInfo, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		fileType := "file"
		if entry.GetType() == "tree" {
			fileType = "dir"
		}
		fileTree = append(fileTree, FileInfo{
			Path: entry.GetPath(),
			Type: fileType,
			Size: entry.GetSize(),
		})
	}

	return &RepoContext{
		Repo:   repo,
		Branch: branch,
		Valid:  true,
		Metadata: &RepoMetadata{
			Name:          repository.GetName(),
			Description:   repository.GetDescription(),
			Language:      repository.GetLanguage(),
			Stars:         repository.GetStargazersCount(),
			Private:       repository.GetPrivate(),
			DefaultBranch: repository.GetDefaultBranch(),
		},
		FileTree: fileTree,
	}, nil
}

// exploreFromRoot is a fallback that lists directory contents recursively
func (s *Service) exploreFromRoot(ctx context.Context, owner, repo, branch string, repository *github.Repository) (*RepoContext, error) {
	var fileTree []FileInfo

	err := s.listContentsRecursive(ctx, owner, repo, branch, "", &fileTree)
	if err != nil {
		return nil, err
	}

	return &RepoContext{
		Repo:   fmt.Sprintf("%s/%s", owner, repo),
		Branch: branch,
		Valid:  true,
		Metadata: &RepoMetadata{
			Name:          repository.GetName(),
			Description:   repository.GetDescription(),
			Language:      repository.GetLanguage(),
			Stars:         repository.GetStargazersCount(),
			Private:       repository.GetPrivate(),
			DefaultBranch: repository.GetDefaultBranch(),
		},
		FileTree: fileTree,
	}, nil
}

// listContentsRecursive recursively lists all files in the repository
func (s *Service) listContentsRecursive(ctx context.Context, owner, repo, branch, path string, fileTree *[]FileInfo) error {
	_, contents, resp, err := s.client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{
		Ref: branch,
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil // Directory doesn't exist, skip
		}
		return err
	}

	for _, content := range contents {
		fileType := "file"
		if content.GetType() == "dir" {
			fileType = "dir"
		}

		*fileTree = append(*fileTree, FileInfo{
			Path: content.GetPath(),
			Type: fileType,
			Size: content.GetSize(),
		})

		// Recurse into directories (but skip common large dirs)
		if content.GetType() == "dir" {
			switch content.GetName() {
			case "node_modules", ".git", "vendor", "dist", "build", ".terraform":
				// Skip these directories
			default:
				if err := s.listContentsRecursive(ctx, owner, repo, branch, content.GetPath(), fileTree); err != nil {
					// Log but continue
					continue
				}
			}
		}
	}

	return nil
}

// ListRepoFiles lists files in a specific directory of the repository
func (s *Service) ListRepoFiles(ctx context.Context, repo, path, branch string, recursive bool) ([]FileInfo, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	if branch == "" {
		branch = "main"
	}

	if recursive {
		var fileTree []FileInfo
		err := s.listContentsRecursive(ctx, owner, repoName, branch, path, &fileTree)
		return fileTree, err
	}

	_, contents, _, err := s.client.Repositories.GetContents(ctx, owner, repoName, path, &github.RepositoryContentGetOptions{
		Ref: branch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list contents: %w", err)
	}

	result := make([]FileInfo, 0, len(contents))
	for _, content := range contents {
		fileType := "file"
		if content.GetType() == "dir" {
			fileType = "dir"
		}
		result = append(result, FileInfo{
			Path: content.GetPath(),
			Type: fileType,
			Size: content.GetSize(),
		})
	}

	return result, nil
}

// ReadFile reads the content of a specific file from the repository
func (s *Service) ReadFile(ctx context.Context, repo, path, branch string) (string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	if branch == "" {
		branch = "main"
	}

	content, _, _, err := s.client.Repositories.GetContents(ctx, owner, repoName, path, &github.RepositoryContentGetOptions{
		Ref: branch,
	})
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if content == nil {
		return "", fmt.Errorf("file not found or is a directory")
	}

	decoded, err := content.GetContent()
	if err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return decoded, nil
}

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Branch     string `json:"branch"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"url"`
}

// GetWorkflowRuns fetches recent GitHub Actions workflow runs for a repository
func (s *Service) GetWorkflowRuns(ctx context.Context, repo, branch string, limit int) ([]WorkflowRun, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	if branch == "" {
		branch = "main"
	}

	if limit == 0 || limit > 30 {
		limit = 10 // Default to 10, max 30
	}

	opts := &github.ListWorkflowRunsOptions{
		Branch: branch,
		ListOptions: github.ListOptions{
			PerPage: limit,
		},
	}

	runs, _, err := s.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repoName, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	result := make([]WorkflowRun, 0, len(runs.WorkflowRuns))
	for _, run := range runs.WorkflowRuns {
		result = append(result, WorkflowRun{
			ID:         run.GetID(),
			Name:       run.GetName(),
			Status:     run.GetStatus(),
			Conclusion: run.GetConclusion(),
			Branch:     run.GetHeadBranch(),
			CreatedAt:  run.GetCreatedAt().Format("2006-01-02 15:04:05"),
			URL:        run.GetHTMLURL(),
		})
	}

	return result, nil
}

// WorkflowJob represents a job in a workflow run
type WorkflowJob struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Steps      []Step `json:"steps"`
	LogsURL    string `json:"logs_url"`
}

// Step represents a step in a workflow job
type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Number     int64  `json:"number"`
}

// GetWorkflowRunJobs fetches jobs for a specific workflow run
func (s *Service) GetWorkflowRunJobs(ctx context.Context, repo string, runID int64) ([]WorkflowJob, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	opts := &github.ListWorkflowJobsOptions{
		Filter: "latest",
	}

	jobs, _, err := s.client.Actions.ListWorkflowJobs(ctx, owner, repoName, runID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow jobs: %w", err)
	}

	result := make([]WorkflowJob, 0, len(jobs.Jobs))
	for _, job := range jobs.Jobs {
		steps := make([]Step, 0, len(job.Steps))
		for _, step := range job.Steps {
			steps = append(steps, Step{
				Name:       step.GetName(),
				Status:     step.GetStatus(),
				Conclusion: step.GetConclusion(),
				Number:     step.GetNumber(),
			})
		}

		result = append(result, WorkflowJob{
			Name:       job.GetName(),
			Status:     job.GetStatus(),
			Conclusion: job.GetConclusion(),
			Steps:      steps,
			LogsURL:    job.GetHTMLURL(),
		})
	}

	return result, nil
}

// GetWorkflowRunLogs fetches and parses the logs for a failed workflow run
// Returns the last 200 lines of error messages from the logs
func (s *Service) GetWorkflowRunLogs(ctx context.Context, repo string, runID int64) (string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repo format, expected owner/repo")
	}
	owner, repoName := parts[0], parts[1]

	// Get the log URL (true = follow redirects)
	url, resp, err := s.client.Actions.GetWorkflowRunLogs(ctx, owner, repoName, runID, true)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("logs not available yet - workflow may still be running")
		}
		return "", fmt.Errorf("failed to get workflow logs: %w", err)
	}

	if url == nil {
		return "", fmt.Errorf("no logs available")
	}

	// Download the zip file
	httpClient := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	zipResp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download logs: %w", err)
	}
	defer zipResp.Body.Close()

	if zipResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download logs: HTTP %d", zipResp.StatusCode)
	}

	// Read the zip file into memory
	zipData, err := io.ReadAll(zipResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read zip data: %w", err)
	}

	// Parse the zip file
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to parse zip: %w", err)
	}

	// Extract error messages from log files
	var allErrors []string
	errorPattern := regexp.MustCompile(`(?i)(error|failed|failure|exception|fatal|panic)`)

	for _, file := range zipReader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Open the file in the zip
		rc, err := file.Open()
		if err != nil {
			continue
		}

		// Read file content
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			// Look for error lines
			if errorPattern.MatchString(line) {
				// Get context around the error (2 lines before and after)
				start := i - 2
				if start < 0 {
					start = 0
				}
				end := i + 3
				if end > len(lines) {
					end = len(lines)
				}

				context := strings.Join(lines[start:end], "\n")
				allErrors = append(allErrors, fmt.Sprintf("[%s]:\n%s\n", file.Name, context))
			}
		}
	}

	if len(allErrors) == 0 {
		return "No error messages found in logs. The workflow may have completed successfully or logs may not be available yet.", nil
	}

	// Return the last errors (most recent), limited to avoid overwhelming the LLM
	result := "=== Latest Deployment Error Logs ===\n\n"
	if len(allErrors) > 20 {
		result += fmt.Sprintf("(Showing %d of %d error contexts)\n\n", 20, len(allErrors))
		allErrors = allErrors[len(allErrors)-20:]
	}
	result += strings.Join(allErrors, "\n---\n")

	return result, nil
}

// GetLatestWorkflowRunStatus gets the most recent workflow run and its status
func (s *Service) GetLatestWorkflowRunStatus(ctx context.Context, repo, branch string) (*WorkflowRun, []WorkflowJob, error) {
	runs, err := s.GetWorkflowRuns(ctx, repo, branch, 1)
	if err != nil {
		return nil, nil, err
	}

	if len(runs) == 0 {
		return nil, nil, fmt.Errorf("no workflow runs found")
	}

	latestRun := runs[0]

	// Get jobs for this run
	jobs, err := s.GetWorkflowRunJobs(ctx, repo, latestRun.ID)
	if err != nil {
		return &latestRun, nil, err
	}

	return &latestRun, jobs, nil
}
