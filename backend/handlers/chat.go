package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud-comfort/backend/llm"
	"cloud-comfort/backend/terraform"
)

type chatRequest struct {
	Message string        `json:"message"`
	History []llm.Message `json:"history"`
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

// buildSystemPrompt reads all .tf files from workDir and builds the system prompt.
func buildSystemPrompt(workDir string) string {
	var sb strings.Builder

	sb.WriteString(`You are Cloud Comfort, a Terraform infrastructure assistant.

## Your capabilities
- You help users create and manage cloud infrastructure using Terraform.
- You can create new .tf files and edit existing ones using the write_file tool.

## How to edit files
- When you use write_file, you provide the COMPLETE file content. It fully overwrites the file.
- To edit an existing file, take its current content shown below, make your changes, and write the entire modified file back using write_file.
- Never write partial files — always include the full content.
- You can create new files by using write_file with a new filename.

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
## Validation
- After you write files, terraform fmt and terraform validate are run automatically.
- If validation fails, you will see the errors in the tool result. Fix the files and try again.
- Common issues: typos in resource types, missing required arguments, invalid HCL syntax.

## Guidelines
- Always explain what you're doing before making changes.
- Use standard Terraform best practices (separate files for main, variables, outputs, providers when appropriate).
- When modifying a file, describe what changed and why.
- Be concise but thorough in your explanations.
`)

	return sb.String()
}

// executeToolCall runs a tool call and returns the result message.
func executeToolCall(tc llm.ToolCall, workDir string) llm.Message {
	if tc.Function.Name != "write_file" {
		return llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Unknown tool: %s", tc.Function.Name),
		}
	}

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

// runValidation formats files and runs terraform validate, returning a summary string.
func runValidation(ctx context.Context, tfSvc *terraform.Service) string {
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
		return sb.String()
	}

	result, err := tfSvc.Validate(ctx)
	if err != nil {
		fmt.Fprintf(&sb, "\nterraform validate: error (%v)", err)
		return sb.String()
	}

	if result.Valid {
		sb.WriteString("\nterraform validate: OK")
	} else {
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
	}

	return sb.String()
}

// HandleChat returns a handler that streams LLM responses via SSE with tool calling.
func HandleChat(client *llm.Client, tfSvc *terraform.Service) http.HandlerFunc {
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

		// Build messages with system prompt containing current files
		systemPrompt := buildSystemPrompt(workDir)
		messages := []llm.Message{
			{Role: "system", Content: systemPrompt},
		}
		messages = append(messages, req.History...)
		messages = append(messages, llm.Message{Role: "user", Content: req.Message})

		tools := []llm.Tool{writeFileTool}

		// SSE writer that forwards content to the frontend
		sseOut := &chatSSEWriter{w: w, f: flusher}

		// Tool call loop — keep going until the LLM responds with just content
		for {
			assistantMsg, err := client.ChatStream(r.Context(), messages, tools, sseOut)
			if err != nil {
				sendSSEEvent(w, flusher, map[string]any{"error": err.Error()})
				return
			}

			// If no tool calls, we're done
			if len(assistantMsg.ToolCalls) == 0 {
				break
			}

			// Append assistant message with tool calls to history
			messages = append(messages, *assistantMsg)

			// Execute each tool call and collect results
			for _, tc := range assistantMsg.ToolCalls {
				result := executeToolCall(tc, workDir)
				messages = append(messages, result)

				// Notify frontend about the file write
				sendSSEEvent(w, flusher, map[string]any{
					"tool_call": map[string]string{
						"name":   tc.Function.Name,
						"result": result.Content,
					},
				})
			}

			// After all file writes, run fmt + validate and append feedback
			validation := runValidation(r.Context(), tfSvc)
			// Add validation result as a system-like user message so LLM sees it
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "[System] Validation results after writing files:" + validation,
			})

			sendSSEEvent(w, flusher, map[string]any{
				"validation": validation,
			})

			// Continue the loop — LLM will respond to tool results + validation
		}

		sendSSEEvent(w, flusher, map[string]any{"done": true})
	}
}

// chatSSEWriter writes content deltas as SSE events.
type chatSSEWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (s *chatSSEWriter) Write(p []byte) (int, error) {
	text := string(p)
	if text == "" {
		return 0, nil
	}
	data, _ := json.Marshal(map[string]string{"content": text})
	fmt.Fprintf(s.w, "data: %s\n\n", data)
	s.f.Flush()
	return len(p), nil
}

func sendSSEEvent(w http.ResponseWriter, f http.Flusher, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}
