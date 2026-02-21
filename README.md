# cloud-comfort

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
