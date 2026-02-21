package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"cloud-comfort/backend/terraform"
)

type GraphNode struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Category     string `json:"category"`     // networking|compute|storage|database|security|other|vpc|subnet|global
	Kind         string `json:"kind"`         // "group" | "resource"
	ResourceType string `json:"resourceType"` // raw terraform resource type, e.g. "aws_s3_bucket"
	Parent       string `json:"parent,omitempty"`
}

type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type DiagramResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

var (
	edgeRe = regexp.MustCompile(`"([^"]+)"\s+->\s+"([^"]+)"`)
	nodeRe = regexp.MustCompile(`"([^"]+)"\s+\[label="([^"]+)"\]`)
)

func HandleDiagram(tfSvc *terraform.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tfSvc.IsInitialized() {
			if err := tfSvc.Init(r.Context(), io.Discard); err != nil {
				http.Error(w, "terraform init failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		dot, err := tfSvc.Graph(r.Context())
		if err != nil {
			http.Error(w, "terraform graph failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		result := dotToGraph(dot)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
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
	// Drop non-AWS resources: local_file, null_resource, random_*, data sources, etc.
	if !strings.HasPrefix(resourceType, "aws_") {
		return true
	}

	// Drop API Gateway wiring — keep only the top-level REST API
	if strings.HasPrefix(resourceType, "aws_api_gateway_") && resourceType != "aws_api_gateway_rest_api" {
		return true
	}

	// Drop CloudWatch log groups / metrics (operational noise)
	if resourceType == "aws_cloudwatch_log_group" || resourceType == "aws_cloudwatch_metric_alarm" {
		return true
	}

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

func cleanNode(label string) string {
	label = strings.TrimPrefix(label, "[root] ")
	label = strings.TrimSuffix(label, " (expand)")
	return label
}

func dotToGraph(dot string) DiagramResponse {
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

	outEdges := make(map[string][]string)
	for _, e := range edges {
		outEdges[e.from] = append(outEdges[e.from], e.to)
	}

	isVPC := func(label string) bool { return strings.HasPrefix(label, "aws_vpc.") }
	isSubnet := func(label string) bool { return strings.HasPrefix(label, "aws_subnet.") }

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

	inEdges := make(map[string][]string)
	for _, e := range edges {
		inEdges[e.to] = append(inEdges[e.to], e.from)
	}

	type placement struct{ kind, parent string }
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

	// Propagate VPC placement via both edge directions
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

	// Use association edges to place resources into subnets
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
			continue
		}
		if len(assocSubnets) == 1 {
			placements[label] = placement{"in-subnet", assocSubnets[0]}
		} else if len(assocSubnets) > 1 {
			if vpc := subnetVPC[assocSubnets[0]]; vpc != "" {
				placements[label] = placement{"in-vpc", vpc}
			}
		}
	}

	vpcSubnets := make(map[string][]string)
	for subnet, vpc := range subnetVPC {
		vpcSubnets[vpc] = append(vpcSubnets[vpc], subnet)
	}

	var vpcs []string
	for label := range nodeIDs {
		if isVPC(label) {
			vpcs = append(vpcs, label)
		}
	}

	var globals []string
	for label, p := range placements {
		if p.kind == "global" {
			globals = append(globals, label)
		}
	}

	// --- Build JSON output ---
	var jsonNodes []GraphNode
	var jsonEdges []GraphEdge

	// VPC group nodes + their subnet groups
	for _, vpc := range vpcs {
		vpcGroupID := "g_" + nodeIDs[vpc]
		jsonNodes = append(jsonNodes, GraphNode{
			ID:       vpcGroupID,
			Label:    humanLabel(vpc),
			Category: "vpc",
			Kind:     "group",
		})
		for _, subnet := range vpcSubnets[vpc] {
			jsonNodes = append(jsonNodes, GraphNode{
				ID:       "g_" + nodeIDs[subnet],
				Label:    humanLabel(subnet),
				Category: "subnet",
				Kind:     "group",
				Parent:   vpcGroupID,
			})
		}
	}

	// Global group
	if len(globals) > 0 {
		jsonNodes = append(jsonNodes, GraphNode{
			ID:       "global",
			Label:    "Global AWS Services",
			Category: "global",
			Kind:     "group",
		})
	}

	// Resource nodes
	for label, nid := range nodeIDs {
		if isVPC(label) || isSubnet(label) {
			continue
		}
		p := placements[label]
		var parentID string
		switch p.kind {
		case "in-subnet":
			parentID = "g_" + nodeIDs[p.parent]
		case "in-vpc":
			parentID = "g_" + nodeIDs[p.parent]
		default:
			parentID = "global"
		}
		rtype := strings.SplitN(label, ".", 2)[0]
		jsonNodes = append(jsonNodes, GraphNode{
			ID:           nid,
			Label:        humanLabel(label),
			Category:     resourceCategory(rtype),
			Kind:         "resource",
			ResourceType: rtype,
			Parent:       parentID,
		})
	}

	// Edges — skip edges involving VPCs or subnets (containment shows the relationship)
	for i, e := range edges {
		if isVPC(e.to) || isSubnet(e.to) || isVPC(e.from) || isSubnet(e.from) {
			continue
		}
		jsonEdges = append(jsonEdges, GraphEdge{
			ID:     fmt.Sprintf("e%d", i),
			Source: nodeIDs[e.from],
			Target: nodeIDs[e.to],
		})
	}

	return DiagramResponse{Nodes: jsonNodes, Edges: jsonEdges}
}
