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

	"cloud-comfort/backend/llm"
	"cloud-comfort/backend/terraform"
)

const maxLoopIterations = 5

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

	sb.WriteString(`You are Cloud Comfort, a Terraform infrastructure assistant. You help users create and manage AWS infrastructure by writing .tf files.

## Rules — follow these strictly

### Writing files
- Use write_file to create or edit .tf/.tfvars files. You provide the COMPLETE file content — it fully overwrites the file.
- Split resources across files: providers.tf, variables.tf, main.tf, outputs.tf. Never dump everything into one giant file.

### Variables
- EVERY variable MUST have a default value. Users cannot provide variable values interactively, so variables without defaults will cause terraform plan to fail.

### Providers
- Only use the hashicorp/aws provider unless the user specifically asks for something else.
- Do NOT use archive, random, null, local, or other utility providers — they cause init failures in the automated pipeline.
- If you need a Lambda deployment package, use a placeholder S3 key instead of data "archive_file".

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

## Guidelines
- Always explain what you're doing before making changes.
- Use standard Terraform best practices (separate files for main, variables, outputs, providers when appropriate).
- When modifying a file, describe what changed and why.
- Be concise but thorough in your explanations.
- If you receive terraform plan errors (e.g. missing variable defaults), fix them immediately.
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
		tools := []llm.Tool{writeFileTool}

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
			systemPrompt := buildSystemPrompt(workDir)
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
				result := executeToolCall(tc, workDir)
				loopMessages = append(loopMessages, result)

				// Notify frontend about the file write
				var tcArgs struct {
					Filename string `json:"filename"`
				}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &tcArgs)
				sendSSEEvent(w, flusher, map[string]any{
					"tool_call": map[string]string{
						"name":     tc.Function.Name,
						"filename": tcArgs.Filename,
						"result":   result.Content,
					},
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

			// Validation passed — run init + plan automatically
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
