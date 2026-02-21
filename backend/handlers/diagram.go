package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type DiagramResponse struct {
	Mermaid string `json:"mermaid"`
}

var (
	edgeRe = regexp.MustCompile(`"([^"]+)"\s+->\s+"([^"]+)"`)
	nodeRe = regexp.MustCompile(`"([^"]+)"\s+\[label="([^"]+)"\]`)
)

func HandleDiagram(workDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Run terraform init if not already done
		if _, err := os.Stat(filepath.Join(workDir, ".terraform")); os.IsNotExist(err) {
			init := exec.CommandContext(r.Context(), "terraform", "init", "-input=false")
			init.Dir = workDir
			if out, err := init.CombinedOutput(); err != nil {
				http.Error(w, "terraform init failed: "+string(out), http.StatusInternalServerError)
				return
			}
		}

		// Run terraform graph
		var buf bytes.Buffer
		graph := exec.CommandContext(r.Context(), "terraform", "graph")
		graph.Dir = workDir
		graph.Stdout = &buf
		var errBuf bytes.Buffer
		graph.Stderr = &errBuf
		if err := graph.Run(); err != nil {
			http.Error(w, "terraform graph failed: "+errBuf.String(), http.StatusInternalServerError)
			return
		}

		mermaid := dotToMermaid(buf.String())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DiagramResponse{Mermaid: mermaid})
	}
}

// noisySuffixes are resource type suffixes that represent glue/sub-resources
// and add visual noise without meaningful architectural information.
var noisySuffixes = []string{
	"_association",
	"_attachment",
	"_versioning",
	"_public_access_block",
	"_server_side_encryption_configuration",
	"_acl",
	"_policy",
	"_rule",
	"_permission",
}

func isNoisyResource(resourceType string) bool {
	for _, suffix := range noisySuffixes {
		if strings.HasSuffix(resourceType, suffix) {
			return true
		}
	}
	return false
}

// resourceCategory maps an AWS resource type prefix to a style class.
func resourceCategory(resourceType string) string {
	networking := []string{"aws_vpc", "aws_subnet", "aws_internet_gateway", "aws_route", "aws_nat_gateway", "aws_network_acl", "aws_eip"}
	compute := []string{"aws_instance", "aws_lambda", "aws_ecs", "aws_eks", "aws_autoscaling", "aws_launch"}
	storage := []string{"aws_s3", "aws_ebs", "aws_efs", "aws_fsx"}
	database := []string{"aws_db", "aws_rds", "aws_dynamodb", "aws_elasticache", "aws_redshift"}
	security := []string{"aws_security_group", "aws_iam", "aws_kms", "aws_waf", "aws_acm"}

	for _, prefix := range networking {
		if strings.HasPrefix(resourceType, prefix) {
			return "networking"
		}
	}
	for _, prefix := range compute {
		if strings.HasPrefix(resourceType, prefix) {
			return "compute"
		}
	}
	for _, prefix := range storage {
		if strings.HasPrefix(resourceType, prefix) {
			return "storage"
		}
	}
	for _, prefix := range database {
		if strings.HasPrefix(resourceType, prefix) {
			return "database"
		}
	}
	for _, prefix := range security {
		if strings.HasPrefix(resourceType, prefix) {
			return "security"
		}
	}
	return "other"
}

// humanLabel converts "aws_s3_bucket.my_assets" to "S3 Bucket: my_assets".
func humanLabel(node string) string {
	parts := strings.SplitN(node, ".", 2)
	if len(parts) != 2 {
		return node
	}
	resourceType, name := parts[0], parts[1]

	// Strip aws_ prefix, replace underscores with spaces, title case
	display := strings.TrimPrefix(resourceType, "aws_")
	display = strings.ReplaceAll(display, "_", " ")
	words := strings.Fields(display)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}

	return strings.Join(words, " ") + ": " + name
}

func dotToMermaid(dot string) string {
	nodeIDs := make(map[string]string) // label -> safe id
	idCounter := 0

	nodeID := func(label string) string {
		if id, ok := nodeIDs[label]; ok {
			return id
		}
		id := fmt.Sprintf("n%d", idCounter)
		idCounter++
		nodeIDs[label] = id
		return id
	}

	type edge struct{ from, to string }
	var edges []edge
	edgeSet := make(map[string]bool)

	for _, line := range strings.Split(dot, "\n") {
		if n := nodeRe.FindStringSubmatch(line); n != nil {
			label := cleanNode(n[2])
			rtype := strings.SplitN(label, ".", 2)[0]
			if label != "root" && !isNoisyResource(rtype) && !strings.HasPrefix(label, "provider[") {
				nodeID(label)
			}
			continue
		}
		if m := edgeRe.FindStringSubmatch(line); m != nil {
			from := cleanNode(m[1])
			to := cleanNode(m[2])
			fromType := strings.SplitN(from, ".", 2)[0]
			toType := strings.SplitN(to, ".", 2)[0]
			if from == "root" || to == "root" {
				continue
			}
			if isNoisyResource(fromType) || isNoisyResource(toType) {
				continue
			}
			if strings.HasPrefix(from, "provider[") || strings.HasPrefix(to, "provider[") {
				continue
			}
			key := from + "->" + to
			if !edgeSet[key] {
				edgeSet[key] = true
				nodeID(from)
				nodeID(to)
				edges = append(edges, edge{from, to})
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	// Style classes — vibrant colours, thick glowing-style borders
	sb.WriteString("  classDef networking fill:#0d47a1,stroke:#42a5f5,stroke-width:2px,color:#e3f2fd,font-weight:bold\n")
	sb.WriteString("  classDef compute fill:#bf360c,stroke:#ff7043,stroke-width:2px,color:#fbe9e7,font-weight:bold\n")
	sb.WriteString("  classDef storage fill:#1b5e20,stroke:#66bb6a,stroke-width:2px,color:#e8f5e9,font-weight:bold\n")
	sb.WriteString("  classDef database fill:#4a148c,stroke:#ab47bc,stroke-width:2px,color:#f3e5f5,font-weight:bold\n")
	sb.WriteString("  classDef security fill:#b71c1c,stroke:#ef5350,stroke-width:2px,color:#ffebee,font-weight:bold\n")
	sb.WriteString("  classDef other fill:#263238,stroke:#78909c,stroke-width:2px,color:#eceff1,font-weight:bold\n")

	// Nodes — shape varies by category
	for label, id := range nodeIDs {
		rtype := strings.SplitN(label, ".", 2)[0]
		category := resourceCategory(rtype)
		human := humanLabel(label)
		var nodeDef string
		switch category {
		case "database":
			nodeDef = fmt.Sprintf("  %s[(\"%s\")]:::%s\n", id, human, category) // cylinder
		case "compute":
			nodeDef = fmt.Sprintf("  %s(\"%s\"):::%s\n", id, human, category) // rounded
		default:
			nodeDef = fmt.Sprintf("  %s[\"%s\"]:::%s\n", id, human, category) // rectangle
		}
		sb.WriteString(nodeDef)
	}

	// Find nodes that have no edges
	connected := make(map[string]bool)
	for _, e := range edges {
		connected[e.from] = true
		connected[e.to] = true
	}

	// Isolated nodes go into a Global AWS Services subgraph
	var isolated []string
	for label := range nodeIDs {
		if !connected[label] {
			isolated = append(isolated, label)
		}
	}
	if len(isolated) > 0 {
		sb.WriteString("  subgraph global[\"☁️ Global AWS Services\"]\n")
		for _, label := range isolated {
			sb.WriteString(fmt.Sprintf("    %s\n", nodeIDs[label]))
		}
		sb.WriteString("  end\n")
	}

	// Edges
	for _, e := range edges {
		sb.WriteString(fmt.Sprintf("  %s --> %s\n", nodeIDs[e.from], nodeIDs[e.to]))
	}

	return sb.String()
}

// cleanNode strips the "[root] " prefix and " (expand)" suffix terraform adds.
func cleanNode(label string) string {
	label = strings.TrimPrefix(label, "[root] ")
	label = strings.TrimSuffix(label, " (expand)")
	return label
}
