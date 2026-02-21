package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
)

const workDir = "./workdir"

func getTerraform() (*tfexec.Terraform, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}

	execPath, err := findTerraformBinary()
	if err != nil {
		return nil, err
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}

	return tfexec.NewTerraform(absWorkDir, execPath)
}

func findTerraformBinary() (string, error) {
	path, err := exec.LookPath("terraform")
	if err != nil {
		return "", err
	}
	return path, nil
}

func HandleInit(w http.ResponseWriter, r *http.Request) {
	tf, err := getTerraform()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tf.Init(context.Background(), tfexec.Upgrade(false)); err != nil {
		log.Printf("terraform init error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "initialized"})
}

func HandlePlan(w http.ResponseWriter, r *http.Request) {
	tf, err := getTerraform()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	planFile := filepath.Join(workDir, "plan.tfplan")

	hasChanges, err := tf.Plan(context.Background(), tfexec.Out(planFile))
	if err != nil {
		log.Printf("terraform plan error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	planJSON, err := tf.ShowPlanFile(context.Background(), planFile)
	if err != nil {
		log.Printf("terraform show error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"has_changes": hasChanges,
		"plan":        planJSON,
	})
}

func HandleApply(w http.ResponseWriter, r *http.Request) {
	tf, err := getTerraform()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tf.Apply(context.Background()); err != nil {
		log.Printf("terraform apply error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "applied"})
}
