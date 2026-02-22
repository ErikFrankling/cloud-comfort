package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud-comfort/backend/github"
	"cloud-comfort/backend/llm"
	"cloud-comfort/backend/terraform"
)

const maxLoopIterations = 5

type chatRequest struct {
	Message     string              `json:"message"`
	History     []llm.Message       `json:"history"`
	RepoContext *github.RepoContext `json:"repo_context,omitempty"`
}

var writeFileTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "write_file",
		Description: "Write or update a terraform .tf file in the working directory. Provide the COMPLETE file content — it fully overwrites the file.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"filename": {
					"type": "string",
					"description": "The .tf or .tfvars filename (e.g. main.tf, variables.tf)"
				},
				"content": {
					"type": "string",
					"description": "The complete file content to write"
				}
			},
			"required": ["filename", "content"]
		}`),
	},
}

var listRepoFilesTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "list_github_repo_files",
		Description: "List files and directories in a GitHub repository. Use this to explore the repo structure before reading specific files. Call this on subdirectories to explore deeper.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {
					"type": "string",
					"description": "Repository in owner/repo format (e.g., 'ErikFrankling/Cleversel-Website')"
				},
				"path": {
					"type": "string",
					"description": "Directory path to list (empty string for root)",
					"default": ""
				},
				"branch": {
					"type": "string",
					"description": "Branch name",
					"default": "main"
				},
				"recursive": {
					"type": "boolean",
					"description": "Whether to list recursively",
					"default": false
				}
			},
			"required": ["repo"]
		}`),
	},
}

var readRepoFileTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "read_github_file",
		Description: "Read the full content of a specific file from a GitHub repository. Use this to read package.json, README, source files, configuration files, etc. after exploring the structure.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {
					"type": "string",
					"description": "Repository in owner/repo format"
				},
				"path": {
					"type": "string",
					"description": "File path (e.g., 'package.json', 'src/App.tsx')"
				},
				"branch": {
					"type": "string",
					"description": "Branch name",
					"default": "main"
				}
			},
			"required": ["repo", "path"]
		}`),
	},
}

var checkGitHubActionsTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "check_github_actions_status",
		Description: "Check the status of GitHub Actions workflow runs for a repository. Use this to monitor CI/CD deployment status and get error details when deployments fail. Returns recent workflow runs with their status (in_progress, completed, failed) and conclusion (success, failure, cancelled).",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {
					"type": "string",
					"description": "Repository in owner/repo format (e.g., 'ErikFrankling/Cleversel-Website')"
				},
				"branch": {
					"type": "string",
					"description": "Branch to check workflow runs for",
					"default": "main"
				},
				"limit": {
					"type": "integer",
					"description": "Number of recent runs to fetch (1-30)",
					"default": 5
				}
			},
			"required": ["repo"]
		}`),
	},
}

var getWorkflowJobsTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "get_workflow_run_details",
		Description: "Get detailed information about a specific workflow run including all jobs and steps. Use this when a workflow failed to see which specific job/step failed and why. Shows the full execution breakdown.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {
					"type": "string",
					"description": "Repository in owner/repo format"
				},
				"run_id": {
					"type": "integer",
					"description": "The workflow run ID (from check_github_actions_status)"
				}
			},
			"required": ["repo", "run_id"]
		}`),
	},
}

var getLatestDeploymentLogsTool = llm.Tool{
	Type: "function",
	Function: llm.ToolFunction{
		Name:        "get_latest_deployment_logs",
		Description: "Fetch the actual error logs from the latest failed GitHub Actions deployment. This automatically finds the most recent failed workflow run and returns the error messages from the logs. Use this when the user asks about deployment failures or when check_github_actions_status shows a failed run.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {
					"type": "string",
					"description": "Repository in owner/repo format (e.g., 'ErikFrankling/Cleversel-Website')"
				},
				"branch": {
					"type": "string",
					"description": "Branch to check (usually 'main')",
					"default": "main"
				}
			},
			"required": ["repo"]
		}`),
	},
}

// buildSystemPrompt reads all .tf files from workDir and builds the system prompt.
// If repoContext is provided, includes repository exploration information.
func buildSystemPrompt(workDir string, repoContext *github.RepoContext) string {
	var sb strings.Builder

	sb.WriteString(`You are Cloud Comfort, a Terraform infrastructure assistant. You help users create and manage AWS infrastructure by writing .tf files.

## Rules — follow these strictly

### Writing files
- Use write_file to create or edit .tf/.tfvars files. You provide the COMPLETE file content — it fully overwrites the file.
- Split resources across files: providers.tf, variables.tf, main.tf, outputs.tf. Never dump everything into one giant file.

### Variables
- EVERY variable MUST have a default value. Users cannot provide variable values interactively, so variables without defaults will cause terraform plan to fail.

### Providers & credentials
- Only use the hashicorp/aws provider unless the user specifically asks for something else.
- Do NOT use archive, random, null, local, or other utility providers — they cause init failures in the automated pipeline.
- If you need a Lambda deployment package, use a placeholder S3 key instead of data "archive_file".
- NEVER pass credentials (access_key, secret_key, token) directly in a provider block. Cloud providers (AWS, GCP, Azure) automatically read credentials from environment variables — just configure the region and nothing else.
- Do NOT declare terraform variables for cloud credentials (aws_access_key_id, aws_secret_access_key, etc.) unless you specifically need to reference their values in resource arguments (e.g. github_actions_secret). The provider auth is handled entirely by env vars.

### Keep it simple
- Build incrementally. Start with the core resources (5-10 max), get them passing validation, then add more.
- Do NOT create a massive deployment all at once. If the user asks for a complex architecture, implement it in stages and check in between.

### Response style
- Be concise. Say what you are creating and why in 1-3 sentences, then write the files.
- Do NOT produce summaries, architecture diagrams, flowcharts, ASCII art, tables, or deployment step lists after writing files. The UI already renders infrastructure diagrams and shows file contents — repeating that information is redundant.
- Do NOT wrap up with a recap of every resource you created. Just say what to do next (e.g. "click Deploy" or "let me know if you want to add X").

### Handling errors
- If validation fails with "Missing required provider", that is normal — the pipeline runs terraform init automatically after validation. Do not rewrite your files in response to this specific error.
- If terraform plan fails (e.g. missing variable defaults, invalid resource arguments), fix the specific issue immediately.

## Current .tf files in the working directory
`)

	entries, err := os.ReadDir(workDir)
	if err != nil || len(entries) == 0 {
		sb.WriteString("\nNo .tf files exist yet. Create them using write_file.\n")
	} else {
		found := false
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tfvars") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(workDir, name))
			if err != nil {
				continue
			}
			found = true
			fmt.Fprintf(&sb, "\n### %s\n```hcl\n%s\n```\n", name, string(content))
		}
		if !found {
			sb.WriteString("\nNo .tf files exist yet. Create them using write_file.\n")
		}
	}

	sb.WriteString(`
## Automated pipeline
After you write files, the following pipeline runs automatically:
1. terraform fmt — auto-formats your files
2. terraform validate — checks for syntax/config errors
3. terraform init — downloads providers (if validation passes)
4. terraform plan — dry-run to check for errors (if init passes)

If ANY step fails, you will receive the error output. Fix the files and try again.
The user must manually approve terraform apply (deployment). You cannot run apply.

If a plan error cannot be fixed by editing .tf files (e.g. IAM permission denied,
missing provider credentials, resource quota limits), do NOT retry. Instead, explain
the root cause to the user and what they need to do manually (e.g. attach an IAM
policy, set an env var, request a quota increase). Then stop making tool calls.

## CI/CD capabilities
You can set up CI/CD pipelines using the GitHub Terraform provider. Available resources:
- github_repository_file — commit files (like GitHub Actions workflows) to a user's repo
- github_actions_secret — set secrets on a repo (for AWS credentials, etc.)

Provider authentication is handled automatically via environment variables — do NOT configure credentials in provider blocks.

When you need to reference credential VALUES inside terraform resources (e.g. to create github_actions_secret resources), you can declare them as sensitive variables. The following TF_VAR_ env vars are set automatically:
- TF_VAR_aws_access_key_id, TF_VAR_aws_secret_access_key, TF_VAR_aws_session_token, TF_VAR_aws_region, TF_VAR_aws_default_region
- TF_VAR_github_token
- TF_VAR_google_credentials, TF_VAR_google_project, TF_VAR_google_region
- TF_VAR_arm_client_id, TF_VAR_arm_client_secret, TF_VAR_arm_subscription_id, TF_VAR_arm_tenant_id

Example — ONLY declare these when you need the value in a resource:
  variable "aws_access_key_id" { type = string; sensitive = true }
  resource "github_actions_secret" "aws_key" {
    repository      = "my-repo"
    secret_name     = "AWS_ACCESS_KEY_ID"
    plaintext_value = var.aws_access_key_id
  }

When a user asks to deploy a site or set up CI/CD from a GitHub repo:
1. Add the GitHub provider: provider "github" { owner = "<github username or org>" }
2. Create the hosting infra (S3 + CloudFront, or similar) — provider "aws" { region = "..." } only, no credentials
3. Create a GitHub Actions workflow using github_repository_file that builds and deploys
4. Set cloud credentials as GitHub secrets using github_actions_secret (this is where you use the sensitive variables)
5. Add terraform output blocks for important values (site URL, bucket name, etc.)

### Lambda deployments with CI/CD

When creating Lambda functions that will be deployed via GitHub Actions, you MUST solve the bootstrap problem: Terraform creates the infrastructure, but the Lambda code ZIP doesn't exist in S3 until GitHub Actions uploads it AFTER deployment.

**The correct pattern (ALWAYS follow this):**

1. Create an S3 bucket for Lambda code
2. Bootstrap a placeholder ZIP using aws_s3_object with inline base64 content
3. Create the Lambda function referencing that S3 object, with lifecycle ignore_changes
4. In the GitHub Actions workflow, build the Lambda, zip it, upload to S3, and update the function code
5. Use lifecycle { ignore_changes = [s3_key, source_code_hash] } so Terraform doesn't revert the code after GitHub Actions updates it

**Example placeholder ZIP resource:**
resource "aws_s3_object" "lambda_placeholder" {
  bucket  = aws_s3_bucket.lambda_code.bucket
  key     = "lambda-function.zip"
  content_base64 = filebase64("${path.module}/placeholder.zip")
}

**Example Lambda with lifecycle:**
resource "aws_lambda_function" "booking_handler" {
  function_name = "${var.bucket_name}-booking-handler"
  role          = aws_iam_role.lambda_role.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  timeout       = 10
  
  s3_bucket = aws_s3_bucket.lambda_code.bucket
  s3_key    = aws_s3_object.lambda_placeholder.key
  
  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.bookings.name
    }
  }
  
  lifecycle {
    ignore_changes = [s3_key, source_code_hash]
  }
}

**CRITICAL RULE:** NEVER use data "archive_file" or null/local providers for Lambda packaging they are blocked in this environment. ALWAYS use S3 for Lambda code storage with the bootstrap pattern above.

### GitHub Repository Exploration

When a repository context is provided below, you MUST explore it to understand the codebase before writing Terraform infrastructure. Follow this workflow:

**Exploration Strategy:**
1. Review the provided file tree to understand the project structure
2. Use read_github_file to read critical configuration files:
   - package.json (Node.js dependencies and scripts)
   - requirements.txt or pyproject.toml (Python)
   - go.mod (Go)
   - Cargo.toml (Rust)
   - Dockerfile or docker-compose.yml
   - README.md for setup instructions
3. Read main entry points and source files to understand the architecture
4. Use list_github_repo_files to explore subdirectories if needed

**Infrastructure Analysis:**
Based on what you discover, determine:
- Frontend framework (React, Vue, etc.) → S3 + CloudFront
- Backend API (Express, FastAPI, etc.) → API Gateway + Lambda or ECS
- Database connections (Mongoose, Prisma, SQLAlchemy) → DynamoDB or RDS
- Build process → CI/CD workflow steps
- Environment variables → github_actions_secret resources

**Best Practice:**
Always read package.json (or equivalent) and the main entry file before making infrastructure decisions. The file tree shows you what's available - read the files to understand what infrastructure is needed.

**CRITICAL: You MUST create Terraform files**
You are a Terraform infrastructure assistant. Your primary purpose is to write .tf files that create AWS infrastructure. You CANNOT end a conversation without writing Terraform configuration files. 

**When a user asks you to deploy or host something:**
1. If a repository is provided: Explore it first (5-10 seconds max)
2. Immediately after exploration: Write the Terraform files using write_file
3. Create at minimum: providers.tf, main.tf, variables.tf
4. The validation pipeline will run automatically after you write files
5. DO NOT say "I'll set this up" or "Let me create..." and then stop - actually WRITE the files

**Never do this:**
- End with "Let me know if you need anything else" without writing files
- Say "I'll create the infrastructure" but not use write_file
- Ask clarifying questions when the user clearly wants infrastructure deployed

**Always do this:**
- Write files immediately when infrastructure is requested
- Create complete, working Terraform configurations
- Include all necessary providers, resources, and variables

### Monitoring GitHub Actions Deployments

### Monitoring GitHub Actions Deployments

When you create a CI/CD pipeline using github_repository_file, you MUST monitor the deployment status to ensure it succeeds. Follow this workflow:

**After Terraform Apply Completes:**
1. The GitHub Actions workflow will trigger automatically on the next push to main
2. Use check_github_actions_status to see recent workflow runs
3. If a run failed (conclusion: "failure") or the user reports deployment issues, use get_latest_deployment_logs to fetch the actual error messages from the logs
4. Analyze the failure and fix the Terraform configuration or GitHub Actions workflow
5. Common failures include:
   - Missing AWS credentials in GitHub secrets
   - Incorrect build commands in the workflow
   - Lambda handler path errors
   - Missing dependencies (npm install, pip install)

**Recommended Tools:**
- Use get_latest_deployment_logs when you need to see the actual error messages from a failed deployment - this automatically finds the latest run and extracts the relevant error logs
- Use check_github_actions_status for an overview of recent runs and their status
- Use get_workflow_run_details only if you need the exact step-by-step breakdown

**Deployment Verification:**
- status: "completed" + conclusion: "success" = deployment succeeded
- status: "in_progress" = deployment is still running, wait and check again
- status: "completed" + conclusion: "failure" = deployment failed, use get_latest_deployment_logs to see the error messages

Always check the deployment status after making infrastructure changes, especially when the user reports issues with the deployed application.

## Guidelines
- Always explain what you're doing before making changes.
- Use standard Terraform best practices (separate files for main, variables, outputs, providers when appropriate).
- When modifying a file, describe what changed and why.
- Be concise but thorough in your explanations.
- If you receive terraform plan errors (e.g. missing variable defaults), fix them immediately.
- Always add output blocks for URLs, endpoints, and resource names the user will need.
`)

	// Add repository context if provided
	if repoContext != nil && repoContext.Valid {
		sb.WriteString(fmt.Sprintf(`
## Active Repository Context
**Repository:** %s
**Branch:** %s
**Language:** %s
**Description:** %s

### Repository File Tree
The following files and directories are available in this repository:
`, repoContext.Repo, repoContext.Branch, repoContext.Metadata.Language, repoContext.Metadata.Description))

		// Group files by directory for better readability
		dirs := make(map[string][]github.FileInfo)
		rootFiles := []github.FileInfo{}

		for _, file := range repoContext.FileTree {
			parts := strings.Split(file.Path, "/")
			if len(parts) > 1 {
				dir := parts[0]
				dirs[dir] = append(dirs[dir], file)
			} else {
				rootFiles = append(rootFiles, file)
			}
		}

		// Show root files first
		if len(rootFiles) > 0 {
			sb.WriteString("\n**Root directory:**\n")
			for _, file := range rootFiles {
				icon := "📄"
				if file.Type == "dir" {
					icon = "📁"
				}
				sb.WriteString(fmt.Sprintf("%s %s\n", icon, file.Path))
			}
		}

		// Show directories
		for dir, files := range dirs {
			sb.WriteString(fmt.Sprintf("\n**%s/:**\n", dir))
			// Just show first 20 files per directory to avoid overwhelming
			showCount := len(files)
			if showCount > 20 {
				showCount = 20
			}
			for i := 0; i < showCount; i++ {
				file := files[i]
				icon := "📄"
				if file.Type == "dir" {
					icon = "📁"
				}
				sb.WriteString(fmt.Sprintf("  %s %s\n", icon, file.Path))
			}
			if len(files) > 20 {
				sb.WriteString(fmt.Sprintf("  ... and %d more files\n", len(files)-20))
			}
		}

		sb.WriteString(`
Use the GitHub exploration tools (list_github_repo_files, read_github_file) to explore further and read the files you need to understand the infrastructure requirements.
`)
	}

	return sb.String()
}

// executeToolCall runs a tool call and returns the result message.
// Supports write_file, list_github_repo_files, read_github_file, and GitHub Actions monitoring.
func executeToolCall(tc llm.ToolCall, workDir string, githubSvc *github.Service, ctx context.Context) llm.Message {
	switch tc.Function.Name {
	case "write_file":
		return executeWriteFile(tc, workDir)
	case "list_github_repo_files":
		return executeListRepoFiles(tc, githubSvc, ctx)
	case "read_github_file":
		return executeReadRepoFile(tc, githubSvc, ctx)
	case "check_github_actions_status":
		return executeCheckGitHubActions(tc, githubSvc, ctx)
	case "get_workflow_run_details":
		return executeGetWorkflowJobs(tc, githubSvc, ctx)
	case "get_latest_deployment_logs":
		return executeGetLatestDeploymentLogs(tc, githubSvc, ctx)
	default:
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Unknown tool: %s", tc.Function.Name),
		}
	}
}

func executeWriteFile(tc llm.ToolCall, workDir string) llm.Message {
	var args struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	// Validate filename
	if strings.Contains(args.Filename, "/") || strings.Contains(args.Filename, "..") || strings.Contains(args.Filename, "\\") {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "Invalid filename: must not contain path separators",
		}
	}
	if !strings.HasSuffix(args.Filename, ".tf") && !strings.HasSuffix(args.Filename, ".tfvars") {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "Invalid filename: must end with .tf or .tfvars",
		}
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error creating directory: %v", err),
		}
	}

	path := filepath.Join(workDir, args.Filename)
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error writing file: %v", err),
		}
	}

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("Successfully wrote %s (%d bytes)", args.Filename, len(args.Content)),
	}
}

func executeListRepoFiles(tc llm.ToolCall, githubSvc *github.Service, ctx context.Context) llm.Message {
	if githubSvc == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "GitHub service not configured - missing GITHUB_TOKEN",
		}
	}

	var args struct {
		Repo      string `json:"repo"`
		Path      string `json:"path"`
		Branch    string `json:"branch"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	if args.Branch == "" {
		args.Branch = "main"
	}

	files, err := githubSvc.ListRepoFiles(ctx, args.Repo, args.Path, args.Branch, args.Recursive)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to list files: %v", err),
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d files/directories in %s/%s:\n\n", len(files), args.Repo, args.Path))
	for _, f := range files {
		typeIcon := "📄"
		if f.Type == "dir" {
			typeIcon = "📁"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", typeIcon, f.Path))
	}

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    sb.String(),
	}
}

func executeReadRepoFile(tc llm.ToolCall, githubSvc *github.Service, ctx context.Context) llm.Message {
	if githubSvc == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "GitHub service not configured - missing GITHUB_TOKEN",
		}
	}

	var args struct {
		Repo   string `json:"repo"`
		Path   string `json:"path"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	if args.Branch == "" {
		args.Branch = "main"
	}

	content, err := githubSvc.ReadFile(ctx, args.Repo, args.Path, args.Branch)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to read file: %v", err),
		}
	}

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("Content of %s:\n\n```\n%s\n```", args.Path, content),
	}
}

// runValidation formats files and runs terraform validate, returning a summary
// string and whether validation passed.
func runValidation(ctx context.Context, tfSvc *terraform.Service) (string, bool) {
	var sb strings.Builder

	// Auto-format
	if err := tfSvc.Format(ctx); err != nil {
		fmt.Fprintf(&sb, "\nterraform fmt: error (%v)", err)
	} else {
		sb.WriteString("\nterraform fmt: formatted")
	}

	// Validate (only if init has been run)
	if !tfSvc.IsInitialized() {
		sb.WriteString("\nterraform validate: skipped (run terraform init first)")
		return sb.String(), true // treat as passed — init will run next
	}

	result, err := tfSvc.Validate(ctx)
	if err != nil {
		fmt.Fprintf(&sb, "\nterraform validate: error (%v)", err)
		return sb.String(), false
	}

	if result.Valid {
		sb.WriteString("\nterraform validate: OK")
		return sb.String(), true
	}

	fmt.Fprintf(&sb, "\nterraform validate: FAILED (%d errors, %d warnings)", result.ErrorCount, result.WarningCount)
	for _, d := range result.Diagnostics {
		loc := ""
		if d.Range != nil {
			loc = fmt.Sprintf(" (%s line %d)", d.Range.Filename, d.Range.Start.Line)
		}
		severity := string(d.Severity)
		if len(severity) > 0 {
			severity = strings.ToUpper(severity[:1]) + severity[1:]
		}
		fmt.Fprintf(&sb, "\n  %s: %s%s", severity, d.Summary, loc)
		if d.Detail != "" {
			fmt.Fprintf(&sb, "\n    %s", d.Detail)
		}
	}

	return sb.String(), false
}

// HandleChat returns a handler that streams LLM responses via SSE with tool calling.
func HandleChat(client *llm.Client, tfSvc *terraform.Service, githubSvc *github.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Set up SSE
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		workDir := tfSvc.WorkDir()

		// Build tools list
		tools := []llm.Tool{writeFileTool}
		if githubSvc != nil {
			tools = append(tools,
				listRepoFilesTool,
				readRepoFileTool,
				checkGitHubActionsTool,
				getWorkflowJobsTool,
				getLatestDeploymentLogsTool,
			)
		}

		// Streaming callbacks that forward deltas to the frontend as SSE events
		cb := &llm.StreamCallbacks{
			OnContent: func(text string) {
				sendSSEEvent(w, flusher, map[string]any{"content": text})
			},
			OnReasoning: func(text string) {
				sendSSEEvent(w, flusher, map[string]any{"reasoning": text})
			},
		}

		// Messages accumulated during the tool-call loop (assistant replies,
		// tool results, validation feedback). Kept separate so we can rebuild
		// the system prompt with fresh file contents each iteration.
		var loopMessages []llm.Message

		// Tool call loop — keep going until the LLM responds with just content
		hitLimit := true
		for i := 0; i < maxLoopIterations; i++ {
			if r.Context().Err() != nil {
				return
			}
			// Rebuild system prompt each iteration so the LLM always sees
			// the current file contents, even after write_file calls.
			// Include repo context if provided (only on first iteration, then cached in loopMessages)
			var repoContext *github.RepoContext
			if i == 0 {
				repoContext = req.RepoContext
			}
			systemPrompt := buildSystemPrompt(workDir, repoContext)
			messages := []llm.Message{
				{Role: "system", Content: systemPrompt},
			}
			messages = append(messages, req.History...)
			messages = append(messages, llm.Message{Role: "user", Content: req.Message})
			messages = append(messages, loopMessages...)

			assistantMsg, err := client.ChatStream(r.Context(), messages, tools, cb)
			if err != nil {
				sendSSEEvent(w, flusher, map[string]any{"error": err.Error()})
				return
			}

			// If no tool calls, we're done
			if len(assistantMsg.ToolCalls) == 0 {
				hitLimit = false
				break
			}

			// Track assistant message with tool calls
			loopMessages = append(loopMessages, *assistantMsg)

			// Execute each tool call and collect results
			for _, tc := range assistantMsg.ToolCalls {
				result := executeToolCall(tc, workDir, githubSvc, r.Context())
				loopMessages = append(loopMessages, result)

				// Notify frontend about the tool call
				toolCallData := map[string]string{
					"name":   tc.Function.Name,
					"result": result.Content,
				}

				// Extract filename for write_file, or repo/path for GitHub tools
				var tcArgs map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &tcArgs); err == nil {
					if filename, ok := tcArgs["filename"].(string); ok {
						toolCallData["filename"] = filename
					} else if path, ok := tcArgs["path"].(string); ok {
						toolCallData["filename"] = path
					} else if repo, ok := tcArgs["repo"].(string); ok {
						toolCallData["filename"] = repo
					}
				}

				sendSSEEvent(w, flusher, map[string]any{
					"tool_call": toolCallData,
				})
			}

			if r.Context().Err() != nil {
				return
			}

			// After all file writes, run fmt + validate and append feedback
			validation, valid := runValidation(r.Context(), tfSvc)

			sendSSEEvent(w, flusher, map[string]any{
				"validation": validation,
			})

			if !valid {
				// Validation failed — feed errors to LLM to fix
				loopMessages = append(loopMessages, llm.Message{
					Role:    "user",
					Content: "[System] Validation results after writing files:" + validation,
				})
				continue
			}

			// Validation passed — check if we have any files before running terraform
			entries, _ := os.ReadDir(workDir)
			hasFiles := false
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".tf") || strings.HasSuffix(e.Name(), ".tfvars")) {
					hasFiles = true
					break
				}
			}

			if !hasFiles {
				// No Terraform files exist yet - remind the LLM to write them
				loopMessages = append(loopMessages, llm.Message{
					Role:    "user",
					Content: "[System] No .tf files exist yet. You MUST create Terraform configuration files to deploy infrastructure. Use write_file to create providers.tf, main.tf, and other necessary files.",
				})
				continue
			}

			// Validation passed and we have files — run init + plan automatically
			sendSSEEvent(w, flusher, map[string]any{"phase": "init"})

			var initBuf bytes.Buffer
			if err := tfSvc.Init(r.Context(), &initBuf); err != nil {
				initErr := fmt.Sprintf("[System] terraform init failed:\n%s\n%v", initBuf.String(), err)
				loopMessages = append(loopMessages, llm.Message{
					Role:    "user",
					Content: initErr,
				})
				sendSSEEvent(w, flusher, map[string]any{"phase_error": "init", "error": err.Error()})
				continue
			}

			sendSSEEvent(w, flusher, map[string]any{"phase": "plan"})

			var planBuf bytes.Buffer
			plan, hasChanges, err := tfSvc.Plan(r.Context(), &planBuf)
			if err != nil {
				planErr := fmt.Sprintf("[System] terraform plan failed:\n%s\n%v", planBuf.String(), err)
				loopMessages = append(loopMessages, llm.Message{
					Role:    "user",
					Content: planErr,
				})
				sendSSEEvent(w, flusher, map[string]any{"phase_error": "plan", "error": err.Error()})
				continue
			}

			// Plan succeeded — build a summary for the LLM
			planSummary := fmt.Sprintf("[System] terraform plan succeeded (has_changes=%v).\n%s", hasChanges, planBuf.String())
			loopMessages = append(loopMessages, llm.Message{
				Role:    "user",
				Content: planSummary,
			})

			// Notify frontend that plan is ready
			planData := map[string]any{
				"plan_ready":  true,
				"has_changes": hasChanges,
				"plan_output": planBuf.String(),
			}
			if plan != nil {
				planJSON, _ := json.Marshal(plan)
				planData["plan_json"] = string(planJSON)
			}
			sendSSEEvent(w, flusher, planData)

			// Continue loop — LLM will summarize the plan for the user
		}

		if hitLimit {
			sendSSEEvent(w, flusher, map[string]any{
				"error": fmt.Sprintf("Stopped after %d fix attempts. The error likely requires manual intervention (e.g. IAM permissions, provider credentials, resource quotas).", maxLoopIterations),
			})
		}
		sendSSEEvent(w, flusher, map[string]any{"done": true})
	}
}

func sendSSEEvent(w http.ResponseWriter, f http.Flusher, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func executeCheckGitHubActions(tc llm.ToolCall, githubSvc *github.Service, ctx context.Context) llm.Message {
	if githubSvc == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "GitHub service not configured - missing GITHUB_TOKEN",
		}
	}

	var args struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	if args.Branch == "" {
		args.Branch = "main"
	}
	if args.Limit == 0 {
		args.Limit = 5
	}

	runs, err := githubSvc.GetWorkflowRuns(ctx, args.Repo, args.Branch, args.Limit)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to fetch workflow runs: %v", err),
		}
	}

	if len(runs) == 0 {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "No workflow runs found. Make sure the GitHub Actions workflow file has been committed to the repository.",
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent GitHub Actions workflow runs for %s (%s branch):\n\n", args.Repo, args.Branch))

	for _, run := range runs {
		statusEmoji := "⏳"
		if run.Status == "completed" {
			if run.Conclusion == "success" {
				statusEmoji = "✅"
			} else if run.Conclusion == "failure" {
				statusEmoji = "❌"
			} else {
				statusEmoji = "⚠️"
			}
		}

		sb.WriteString(fmt.Sprintf("%s **%s** (Run #%d)\n", statusEmoji, run.Name, run.ID))
		sb.WriteString(fmt.Sprintf("   Status: %s | Conclusion: %s\n", run.Status, run.Conclusion))
		sb.WriteString(fmt.Sprintf("   Created: %s\n", run.CreatedAt))
		sb.WriteString(fmt.Sprintf("   URL: %s\n\n", run.URL))
	}

	// Add guidance for next steps
	latest := runs[0]
	sb.WriteString("**Latest Run Analysis:**\n")
	if latest.Status == "in_progress" {
		sb.WriteString("The latest workflow is still running. Wait a few minutes and check again.\n")
	} else if latest.Conclusion == "failure" {
		sb.WriteString(fmt.Sprintf("The latest workflow failed. Use get_workflow_run_details with run_id=%d to see which job/step failed and get error details.\n", latest.ID))
	} else if latest.Conclusion == "success" {
		sb.WriteString("The latest workflow completed successfully! The deployment should be live.\n")
	}

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    sb.String(),
	}
}

func executeGetWorkflowJobs(tc llm.ToolCall, githubSvc *github.Service, ctx context.Context) llm.Message {
	if githubSvc == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "GitHub service not configured - missing GITHUB_TOKEN",
		}
	}

	var args struct {
		Repo  string `json:"repo"`
		RunID int64  `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	jobs, err := githubSvc.GetWorkflowRunJobs(ctx, args.Repo, args.RunID)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to fetch workflow jobs: %v", err),
		}
	}

	if len(jobs) == 0 {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "No jobs found for this workflow run. The workflow may still be starting up.",
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workflow Run Details for %s (Run #%d):\n\n", args.Repo, args.RunID))

	for _, job := range jobs {
		statusEmoji := "⏳"
		if job.Status == "completed" {
			if job.Conclusion == "success" {
				statusEmoji = "✅"
			} else if job.Conclusion == "failure" {
				statusEmoji = "❌"
			} else {
				statusEmoji = "⚠️"
			}
		}

		sb.WriteString(fmt.Sprintf("%s **Job: %s**\n", statusEmoji, job.Name))
		sb.WriteString(fmt.Sprintf("   Status: %s | Conclusion: %s\n", job.Status, job.Conclusion))

		if len(job.Steps) > 0 {
			sb.WriteString("   Steps:\n")
			for _, step := range job.Steps {
				stepEmoji := "⏳"
				if step.Status == "completed" {
					if step.Conclusion == "success" {
						stepEmoji = "✅"
					} else if step.Conclusion == "failure" {
						stepEmoji = "❌"
					} else {
						stepEmoji = "⚠️"
					}
				}
				sb.WriteString(fmt.Sprintf("     %s %d. %s (%s)\n", stepEmoji, step.Number, step.Name, step.Status))
			}
		}

		// Highlight failures
		if job.Conclusion == "failure" {
			sb.WriteString("   ⚠️ This job FAILED. Check the logs for error details.\n")
		}

		sb.WriteString("\n")
	}

	// Add guidance
	sb.WriteString("**Troubleshooting:**\n")
	sb.WriteString("- If a step shows ❌, that's where the failure occurred\n")
	sb.WriteString("- Common failures: missing secrets, build errors, deployment errors\n")
	sb.WriteString("- Check the GitHub Actions logs in the repository for full error messages\n")

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    sb.String(),
	}
}

func executeGetLatestDeploymentLogs(tc llm.ToolCall, githubSvc *github.Service, ctx context.Context) llm.Message {
	if githubSvc == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "GitHub service not configured - missing GITHUB_TOKEN",
		}
	}

	var args struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Invalid arguments: %v", err),
		}
	}

	if args.Branch == "" {
		args.Branch = "main"
	}

	// Get recent workflow runs
	runs, err := githubSvc.GetWorkflowRuns(ctx, args.Repo, args.Branch, 5)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to fetch workflow runs: %v", err),
		}
	}

	if len(runs) == 0 {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "No workflow runs found. Make sure the GitHub Actions workflow file has been committed to the repository.",
		}
	}

	// Find the latest failed or completed run
	var targetRun *github.WorkflowRun
	for _, run := range runs {
		if run.Status == "completed" {
			targetRun = &run
			break
		}
	}

	if targetRun == nil {
		// No completed runs yet, check if any are running
		for _, run := range runs {
			if run.Status == "in_progress" || run.Status == "queued" {
				return llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Latest workflow run (ID: %d) is still %s. Wait a few minutes and try again.", run.ID, run.Status),
				}
			}
		}

		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    "No completed workflow runs found.",
		}
	}

	// Get the logs for this run
	logs, err := githubSvc.GetWorkflowRunLogs(ctx, args.Repo, targetRun.ID)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Failed to fetch logs for run %d: %v", targetRun.ID, err),
		}
	}

	// Build response with status and logs
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Latest Workflow Run: %s (ID: %d)\n", targetRun.Name, targetRun.ID))
	sb.WriteString(fmt.Sprintf("Status: %s | Conclusion: %s | Created: %s\n\n", targetRun.Status, targetRun.Conclusion, targetRun.CreatedAt))

	if targetRun.Conclusion == "success" {
		sb.WriteString("✅ The deployment completed successfully!\n\n")
	} else if targetRun.Conclusion == "failure" {
		sb.WriteString("❌ The deployment FAILED. Here are the error logs:\n\n")
	}

	sb.WriteString(logs)

	return llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    sb.String(),
	}
}
