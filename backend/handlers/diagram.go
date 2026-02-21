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
		if _, err := os.Stat(filepath.Join(workDir, ".terraform")); os.IsNotExist(err) {
			init := exec.CommandContext(r.Context(), "terraform", "init", "-input=false")
			init.Dir = workDir
			if out, err := init.CombinedOutput(); err != nil {
				http.Error(w, "terraform init failed: "+string(out), http.StatusInternalServerError)
				return
			}
		}

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

func humanLabel(node string) string {
	parts := strings.SplitN(node, ".", 2)
	if len(parts) != 2 {
		return node
	}
	resourceType, name := parts[0], parts[1]
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

func nodeDef(id, label, category string) string {
	human := humanLabel(label)
	switch category {
	case "database":
		return fmt.Sprintf("  %s[(\"%s\")]:::%s\n", id, human, category)
	case "compute":
		return fmt.Sprintf("  %s(\"%s\"):::%s\n", id, human, category)
	default:
		return fmt.Sprintf("  %s[\"%s\"]:::%s\n", id, human, category)
	}
}

func dotToMermaid(dot string) string {
	nodeIDs := make(map[string]string)
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
	// assocConnections captures edges involving noisy (association) resources
	// so we can infer subnet placement without rendering them
	assocConnections := make(map[string][]string)

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
			if strings.HasPrefix(from, "provider[") || strings.HasPrefix(to, "provider[") {
				continue
			}
			// For association resources: capture their connections for placement
			// inference but don't render them as nodes or edges
			if isNoisyResource(fromType) {
				assocConnections[from] = append(assocConnections[from], to)
				continue
			}
			if isNoisyResource(toType) {
				assocConnections[to] = append(assocConnections[to], from)
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

	// Build outgoing edge map for placement logic
	outEdges := make(map[string][]string)
	for _, e := range edges {
		outEdges[e.from] = append(outEdges[e.from], e.to)
	}

	isVPC := func(label string) bool { return strings.HasPrefix(label, "aws_vpc.") }
	isSubnet := func(label string) bool { return strings.HasPrefix(label, "aws_subnet.") }

	// Map each subnet to its parent VPC
	subnetVPC := make(map[string]string)
	for label := range nodeIDs {
		if isSubnet(label) {
			for _, dep := range outEdges[label] {
				if isVPC(dep) {
					subnetVPC[label] = dep
					break
				}
			}
		}
	}

	// Build incoming edge map too
	inEdges := make(map[string][]string)
	for _, e := range edges {
		inEdges[e.to] = append(inEdges[e.to], e.from)
	}

	// Determine placement for each non-VPC, non-subnet node
	type placement struct{ kind, parent string } // kind: "in-subnet" | "in-vpc" | "global"
	placements := make(map[string]placement)
	for label := range nodeIDs {
		if isVPC(label) || isSubnet(label) {
			continue
		}
		var foundSubnet, foundVPC string
		for _, dep := range outEdges[label] {
			if isSubnet(dep) && foundSubnet == "" {
				foundSubnet = dep
			}
			if isVPC(dep) && foundVPC == "" {
				foundVPC = dep
			}
		}
		switch {
		case foundSubnet != "":
			placements[label] = placement{"in-subnet", foundSubnet}
		case foundVPC != "":
			placements[label] = placement{"in-vpc", foundVPC}
		default:
			placements[label] = placement{"global", ""}
		}
	}

	// Second pass: propagate VPC placement via both edge directions.
	// Resources like aws_eip don't reference the VPC directly but are
	// connected to resources that do — pull them into the VPC.
	changed := true
	for changed {
		changed = false
		for label, p := range placements {
			if p.kind != "global" {
				continue
			}
			neighbors := append(outEdges[label], inEdges[label]...)
			for _, nb := range neighbors {
				nbP, ok := placements[nb]
				if !ok {
					continue
				}
				if nbP.kind == "in-vpc" {
					placements[label] = placement{"in-vpc", nbP.parent}
					changed = true
					break
				}
				if nbP.kind == "in-subnet" {
					placements[label] = placement{"in-vpc", subnetVPC[nbP.parent]}
					changed = true
					break
				}
			}
		}
	}

	// Third pass: use association edges to place resources into subnets.
	// e.g. aws_route_table_association links a route table to a subnet —
	// use that to put the route table inside the subnet subgraph.
	// Build resource -> []subnets map via association connections.
	resourceSubnetsViaAssoc := make(map[string][]string)
	for _, connected := range assocConnections {
		var subnets, others []string
		for _, label := range connected {
			if strings.HasPrefix(label, "aws_subnet.") {
				subnets = append(subnets, label)
			} else if _, exists := nodeIDs[label]; exists {
				others = append(others, label)
			}
		}
		for _, other := range others {
			for _, subnet := range subnets {
				resourceSubnetsViaAssoc[other] = append(resourceSubnetsViaAssoc[other], subnet)
			}
		}
	}
	for label, assocSubnets := range resourceSubnetsViaAssoc {
		p := placements[label]
		if p.kind == "in-subnet" {
			continue // already placed precisely
		}
		if len(assocSubnets) == 1 {
			placements[label] = placement{"in-subnet", assocSubnets[0]}
		} else if len(assocSubnets) > 1 {
			// Associated with multiple subnets — promote to VPC level
			if vpc := subnetVPC[assocSubnets[0]]; vpc != "" {
				placements[label] = placement{"in-vpc", vpc}
			}
		}
	}

	// Group subnets by VPC
	vpcSubnets := make(map[string][]string)
	for subnet, vpc := range subnetVPC {
		vpcSubnets[vpc] = append(vpcSubnets[vpc], subnet)
	}

	// Collect all VPCs
	var vpcs []string
	for label := range nodeIDs {
		if isVPC(label) {
			vpcs = append(vpcs, label)
		}
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString("  classDef networking fill:#0d47a1,stroke:#42a5f5,stroke-width:2px,color:#e3f2fd,font-weight:bold\n")
	sb.WriteString("  classDef compute fill:#bf360c,stroke:#ff7043,stroke-width:2px,color:#fbe9e7,font-weight:bold\n")
	sb.WriteString("  classDef storage fill:#1b5e20,stroke:#66bb6a,stroke-width:2px,color:#e8f5e9,font-weight:bold\n")
	sb.WriteString("  classDef database fill:#4a148c,stroke:#ab47bc,stroke-width:2px,color:#f3e5f5,font-weight:bold\n")
	sb.WriteString("  classDef security fill:#b71c1c,stroke:#ef5350,stroke-width:2px,color:#ffebee,font-weight:bold\n")
	sb.WriteString("  classDef other fill:#263238,stroke:#78909c,stroke-width:2px,color:#eceff1,font-weight:bold\n")

	// Render VPC subgraphs — nodes defined INSIDE subgraphs so Mermaid places them correctly
	for _, vpc := range vpcs {
		vpcID := nodeIDs[vpc]
		sb.WriteString(fmt.Sprintf("  subgraph %s_sg[\"%s\"]\n", vpcID, humanLabel(vpc)))

		for _, subnet := range vpcSubnets[vpc] {
			subnetID := nodeIDs[subnet]

			// Collect resources inside this subnet
			var subnetResources []string
			for label, p := range placements {
				if p.kind == "in-subnet" && p.parent == subnet {
					subnetResources = append(subnetResources, label)
				}
			}

			if len(subnetResources) > 0 {
				// Non-empty subnet: nested subgraph containing its resources
				sb.WriteString(fmt.Sprintf("    subgraph %s_sg[\"%s\"]\n", subnetID, humanLabel(subnet)))
				for _, label := range subnetResources {
					cat := resourceCategory(strings.SplitN(label, ".", 2)[0])
					sb.WriteString("      " + nodeDef(nodeIDs[label], label, cat))
				}
				sb.WriteString("    end\n")
			} else {
				// Empty subnet: plain node inside the VPC box
				sb.WriteString("    " + nodeDef(subnetID, subnet, "networking"))
			}
		}

		// VPC-level resources (not inside any subnet)
		for label, p := range placements {
			if p.kind == "in-vpc" && p.parent == vpc {
				cat := resourceCategory(strings.SplitN(label, ".", 2)[0])
				sb.WriteString("    " + nodeDef(nodeIDs[label], label, cat))
			}
		}

		sb.WriteString("  end\n")
	}

	// Global subgraph
	var globals []string
	for label, p := range placements {
		if p.kind == "global" {
			globals = append(globals, label)
		}
	}
	if len(globals) > 0 {
		sb.WriteString("  subgraph global[\"☁️ Global AWS Services\"]\n")
		for _, label := range globals {
			cat := resourceCategory(strings.SplitN(label, ".", 2)[0])
			sb.WriteString("    " + nodeDef(nodeIDs[label], label, cat))
		}
		sb.WriteString("  end\n")
	}

	// Edges — skip to VPCs/subnets since containment already shows the relationship
	for _, e := range edges {
		if isVPC(e.to) || isSubnet(e.to) {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s --> %s\n", nodeIDs[e.from], nodeIDs[e.to]))
	}

	return sb.String()
}

func cleanNode(label string) string {
	label = strings.TrimPrefix(label, "[root] ")
	label = strings.TrimSuffix(label, " (expand)")
	return label
}
