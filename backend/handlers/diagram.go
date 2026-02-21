package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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

		dot := buf.String()
		log.Printf("terraform graph output:\n%s", dot)
		mermaid := dotToMermaid(dot)
		log.Printf("mermaid output:\n%s", mermaid)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DiagramResponse{Mermaid: mermaid})
	}
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
		// Node declaration
		if n := nodeRe.FindStringSubmatch(line); n != nil {
			label := cleanNode(n[2])
			if label != "root" {
				nodeID(label)
			}
			continue
		}
		// Edge
		if m := edgeRe.FindStringSubmatch(line); m != nil {
			from := cleanNode(m[1])
			to := cleanNode(m[2])
			if from == "root" || to == "root" {
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
	for label, id := range nodeIDs {
		sb.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", id, label))
	}
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
