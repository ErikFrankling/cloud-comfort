package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/hashicorp/terraform-exec/tfexec"
)

// Service wraps terraform-exec and provides safe, concurrent access
// to terraform operations on a single working directory.
type Service struct {
	workDir  string
	execPath string
	mu       sync.Mutex
}

// NewService creates a terraform service for the given working directory.
// It finds the terraform binary on PATH and ensures the workdir exists.
func NewService(workDir string) (*Service, error) {
	execPath, err := exec.LookPath("terraform")
	if err != nil {
		return nil, fmt.Errorf("terraform binary not found on PATH: %w", err)
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving workdir path: %w", err)
	}

	if err := os.MkdirAll(absWorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating workdir: %w", err)
	}

	return &Service{
		workDir:  absWorkDir,
		execPath: execPath,
	}, nil
}

// WorkDir returns the absolute path to the terraform working directory.
func (s *Service) WorkDir() string {
	return s.workDir
}

// newTF creates a fresh tfexec.Terraform instance with stdout piped to
// the provided writer. Environment variables are inherited from the process
// env (set by setCloudEnv in main.go) — we do NOT call tf.SetEnv because
// it replaces the entire parent env and blocks TF_VAR_ keys.
func (s *Service) newTF(output io.Writer) (*tfexec.Terraform, error) {
	tf, err := tfexec.NewTerraform(s.workDir, s.execPath)
	if err != nil {
		return nil, fmt.Errorf("creating terraform instance: %w", err)
	}

	if output != nil {
		tf.SetStdout(output)
		tf.SetStderr(output)
	}

	return tf, nil
}

// Init runs terraform init in the working directory.
func (s *Service) Init(ctx context.Context, output io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(output)
	if err != nil {
		return err
	}

	if err := tf.Init(ctx, tfexec.Upgrade(false)); err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}

	return nil
}

// Plan runs terraform plan and returns the structured plan JSON, whether
// changes are detected, and any error. Stdout/stderr are streamed to output.
func (s *Service) Plan(ctx context.Context, output io.Writer) (*tfjson.Plan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(output)
	if err != nil {
		return nil, false, err
	}

	planFile := filepath.Join(s.workDir, "plan.tfplan")

	hasChanges, err := tf.Plan(ctx, tfexec.Out(planFile), tfexec.Lock(false))
	if err != nil {
		return nil, false, fmt.Errorf("terraform plan: %w", err)
	}

	// Read the plan file back as structured JSON
	plan, err := tf.ShowPlanFile(ctx, planFile)
	if err != nil {
		return nil, hasChanges, fmt.Errorf("terraform show plan: %w", err)
	}

	return plan, hasChanges, nil
}

// Validate runs terraform validate and returns structured diagnostics.
// Requires Init() to have been run first (providers must be downloaded).
func (s *Service) Validate(ctx context.Context) (*tfjson.ValidateOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(nil)
	if err != nil {
		return nil, err
	}

	result, err := tf.Validate(ctx)
	if err != nil {
		return nil, fmt.Errorf("terraform validate: %w", err)
	}

	return result, nil
}

// Format runs terraform fmt on all files in the working directory, writing
// formatted output back in place.
func (s *Service) Format(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(nil)
	if err != nil {
		return err
	}

	if err := tf.FormatWrite(ctx); err != nil {
		return fmt.Errorf("terraform fmt: %w", err)
	}

	return nil
}

// IsInitialized checks if terraform init has been run by looking for the
// .terraform directory.
func (s *Service) IsInitialized() bool {
	info, err := os.Stat(filepath.Join(s.workDir, ".terraform"))
	return err == nil && info.IsDir()
}

// Graph runs terraform graph and returns the DOT output string.
func (s *Service) Graph(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(nil)
	if err != nil {
		return "", err
	}

	dot, err := tf.Graph(ctx)
	if err != nil {
		return "", fmt.Errorf("terraform graph: %w", err)
	}

	return dot, nil
}

// Output reads terraform output values. Returns a map of output names to their
// string values (sensitive values are redacted).
func (s *Service) Output(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(nil)
	if err != nil {
		return nil, err
	}

	meta, err := tf.Output(ctx)
	if err != nil {
		return nil, fmt.Errorf("terraform output: %w", err)
	}

	result := make(map[string]string, len(meta))
	for k, v := range meta {
		if v.Sensitive {
			result[k] = "(sensitive)"
			continue
		}
		// Value is JSON — try to unquote strings, otherwise use raw JSON
		var s string
		if err := json.Unmarshal(v.Value, &s); err == nil {
			result[k] = s
		} else {
			result[k] = string(v.Value)
		}
	}
	return result, nil
}

// Apply runs terraform apply in the working directory.
func (s *Service) Apply(ctx context.Context, output io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, err := s.newTF(output)
	if err != nil {
		return err
	}

	if err := tf.Apply(ctx, tfexec.Lock(false)); err != nil {
		return fmt.Errorf("terraform apply: %w", err)
	}

	return nil
}
