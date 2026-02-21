# Cloud Comfort

LLM-powered Terraform infrastructure assistant. Chat with an AI to create, edit, and deploy `.tf` files. The LLM writes files via tool calling, and terraform fmt/validate run automatically after each write.

**Stack:** Go backend, React/Vite frontend, OpenAI-compatible LLM API, HashiCorp terraform-exec.

## File Hierarchy

```
cloud-comfort/
├── backend/
│   ├── main.go                  # HTTP server, routing, env config
│   ├── go.mod
│   ├── handlers/
│   │   ├── chat.go              # LLM chat endpoint, tool calling loop, SSE streaming
│   │   ├── files.go             # CRUD for .tf files
│   │   └── terraform.go         # init/plan/apply SSE handlers
│   ├── llm/
│   │   └── client.go            # OpenAI-compatible API client (streaming + non-streaming)
│   └── terraform/
│       └── terraform.go         # Terraform service (wraps terraform-exec)
├── frontend/
│   ├── src/
│   │   ├── App.tsx              # Single-page app: chat panel + file browser
│   │   ├── App.css
│   │   └── main.tsx             # React entry point
│   ├── index.html
│   ├── vite.config.ts
│   └── package.json             # react 19, vite 6, typescript 5.6
├── Makefile                     # dev-backend, dev-frontend, install, build
├── flake.nix                    # Nix devshell: node 20, go 1.23, terraform
├── .env                         # LLM and cloud provider credentials
└── .envrc                       # direnv → nix develop
```

## API Endpoints

All endpoints served on `:8080`. Frontend proxied via Vite at `:5173`.

### Terraform Operations (SSE streaming)

| Method | Path                   | Description           | Response                                                      |
| ------ | ---------------------- | --------------------- | ------------------------------------------------------------- |
| `POST` | `/api/terraform/init`  | Run `terraform init`  | SSE `{line}` events, final `{done:true}`                      |
| `POST` | `/api/terraform/plan`  | Run `terraform plan`  | SSE lines, final `{done, has_changes, plan}` (full plan JSON) |
| `POST` | `/api/terraform/apply` | Run `terraform apply` | SSE lines, final `{done:true}`                                |

### File Management (JSON)

| Method   | Path                          | Description                    | Body / Response                              |
| -------- | ----------------------------- | ------------------------------ | -------------------------------------------- |
| `GET`    | `/api/terraform/files`        | List all `.tf`/`.tfvars` files | `[{name, size}]`                             |
| `GET`    | `/api/terraform/files/{name}` | Get file content               | `text/plain`                                 |
| `PUT`    | `/api/terraform/files/{name}` | Create/overwrite file          | Raw text body (1MB limit) → `{status, name}` |
| `DELETE` | `/api/terraform/files/{name}` | Delete file                    | `{status, name}`                             |

### Chat (SSE streaming + tool calling)

| Method | Path        | Description                                             |
| ------ | ----------- | ------------------------------------------------------- |
| `POST` | `/api/chat` | Send message, get streamed LLM response with tool calls |

**Request body:**

```json
{ "message": "Create an S3 bucket", "history": [{"role":"user","content":"..."}, ...] }
```

**SSE events sent to frontend:**
| Event key | When | Payload |
|-----------|------|---------|
| `content` | LLM generates text | `{content: "chunk"}` |
| `tool_call` | LLM wrote a file | `{tool_call: {name, result}}` |
| `validation` | After file writes | `{validation: "fmt: ok\nvalidate: OK"}` |
| `error` | Something failed | `{error: "message"}` |
| `done` | Stream complete | `{done: true}` |

### Health

| Method | Path          | Response         |
| ------ | ------------- | ---------------- |
| `GET`  | `/api/health` | `{status: "ok"}` |

## LLM Connection

Configured via env vars in `.env`:

| Var             | Default                        | Purpose                       |
| --------------- | ------------------------------ | ----------------------------- |
| `LLM_BASE_URL`  | `https://openrouter.ai/api/v1` | OpenAI-compatible API base    |
| `LLM_API_KEY`   | —                              | Bearer token                  |
| `LLM_MODEL`     | `openai/gpt-4o`                | Model identifier              |
| `LLM_STREAMING` | `true`                         | Enable SSE streaming from LLM |

The client (`llm/client.go`) speaks the OpenAI chat completions protocol: `POST {base_url}/chat/completions` with `Authorization: Bearer {key}`. Works with OpenRouter, OpenAI, Ollama, or any compatible endpoint.

Two modes:

- **Streaming** (`ChatStream`): Reads SSE `data:` lines, accumulates content deltas and tool call deltas across chunks, forwards content to the frontend in real time.
- **Non-streaming** (`Chat`): Standard JSON request/response.

## Tool Calling

One tool is registered: **`write_file`**.

```json
{
  "name": "write_file",
  "description": "Write or update a terraform .tf file. Provide COMPLETE file content.",
  "parameters": {
    "filename": ".tf or .tfvars filename",
    "content": "complete file content"
  }
}
```

When the LLM returns a tool call:

1. Arguments are parsed (`filename`, `content`)
2. Filename is validated — no path separators, must end in `.tf` or `.tfvars`
3. File is written to `./workdir/{filename}`
4. Result message sent back to the LLM as a `tool` role message

## LLM Agent Loop

Defined in `handlers/chat.go` `HandleChat`. This is a **tool-calling loop** that keeps running until the LLM stops requesting tools:

```
1. Build messages: [system prompt (with current .tf files)] + history + user message
2. Send to LLM with tools=[write_file]
3. Stream response to frontend (content chunks via SSE)
4. If response contains tool_calls:
   a. Execute each tool call (write file to disk)
   b. Send tool_call SSE event to frontend
   c. Run terraform fmt + terraform validate
   d. Append validation results to messages
   e. Send validation SSE event to frontend
   f. → Go to step 2 (LLM sees tool results + validation, may call tools again)
5. If no tool_calls → send {done: true}, loop ends
```

The system prompt is rebuilt each iteration and includes the full content of every `.tf`/`.tfvars` file currently on disk, so the LLM always has the latest state.

## Terraform Service

`terraform/terraform.go` wraps `hashicorp/terraform-exec`. All operations hold a mutex for safe concurrent access.

| Method               | Terraform Command    | Details                                                                                                    |
| -------------------- | -------------------- | ---------------------------------------------------------------------------------------------------------- |
| `Init(ctx, output)`  | `terraform init`     | Downloads providers. `Upgrade(false)`.                                                                     |
| `Plan(ctx, output)`  | `terraform plan`     | Writes `plan.tfplan`, reads it back as structured `tfjson.Plan` JSON. Returns `(plan, hasChanges, error)`. |
| `Apply(ctx, output)` | `terraform apply`    | Auto-approve. Streams stdout/stderr to `output`.                                                           |
| `Validate(ctx)`      | `terraform validate` | Returns `tfjson.ValidateOutput` with diagnostics (errors, warnings, line numbers).                         |
| `Format(ctx)`        | `terraform fmt`      | Formats all files in-place (`FormatWrite`).                                                                |
| `IsInitialized()`    | —                    | Checks if `.terraform/` directory exists.                                                                  |

Cloud provider credentials are passed to terraform via `SetEnv()`:

- **AWS:** `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_REGION`, `AWS_DEFAULT_REGION`
- **GCP:** `GOOGLE_CREDENTIALS`, `GOOGLE_PROJECT`, `GOOGLE_REGION`
- **Azure:** `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`, `ARM_SUBSCRIPTION_ID`, `ARM_TENANT_ID`

## Frontend → Backend Communication

The frontend (`App.tsx`) uses the native `fetch` API. No axios or API client library.

**Chat flow:**

1. `POST /api/chat` with `{message, history}`
2. Read response as `ReadableStream`, parse SSE `data:` lines
3. `content` events → append to assistant message in real time
4. `tool_call` events → append result, trigger `fetchFiles()` to refresh sidebar
5. `done` event → update conversation history for next request

**File operations:**

- `fetch('/api/terraform/files')` → list files
- `fetch('/api/terraform/files/{name}')` → view file content
- `fetch('/api/terraform/files/{name}', {method:'PUT', body: text})` → upload
- `fetch('/api/terraform/files/{name}', {method:'DELETE'})` → delete

## Running

```bash
# Enter nix devshell
direnv allow .
# or: nix develop

# Terminal 1 — backend
make dev-backend        # go run . on :8080

# Terminal 2 — frontend
make install            # npm install (first time)
make dev-frontend       # vite dev on :5173
```

Set credentials in `.env`:

```bash
LLM_API_KEY=sk-or-...
LLM_MODEL=openai/gpt-4o
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
AWS_REGION=us-east-1
```

### Terraform to Diagram.

Upload your Terraform `.tf` files and instantly visualize your infrastructure as an interactive diagram.

## How it works

1. Upload `.tf` files via the **Files** tab
2. Switch to the **Flow Chart** tab and click **Generate Diagram**
3. The backend runs `terraform graph` on your files and converts the output to a Mermaid diagram

No LLM involved — the diagram is generated directly from Terraform's own dependency graph.

## Stack

- **Frontend**: React + TypeScript + Mermaid.js
- **Backend**: Go
- **Diagramming**: `terraform graph` → DOT → Mermaid

## Running locally

**Requirements:** Go, Node.js, Terraform

```bash
# Backend
cd backend && go run .

# Frontend (new terminal)
cd frontend && npm install && npm run dev
```

Open `http://localhost:5173`

## Test file

A sample Terraform config is included at `backend/workdir/network.tf` with a VPC, subnets, internet gateway, security group, EC2 instance, RDS, and S3 bucket — useful for testing the diagram output locally.
