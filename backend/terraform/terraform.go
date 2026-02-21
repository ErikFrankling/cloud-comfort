package terraform

import (
	"context"
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
	env      map[string]string
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
		env:      make(map[string]string),
	}, nil
}

// SetEnv sets environment variables that will be passed to every terraform
// invocation. Use this for cloud auth secrets (AWS_ACCESS_KEY_ID, etc).
func (s *Service) SetEnv(env map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range env {
		s.env[k] = v
	}
}

// WorkDir returns the absolute path to the terraform working directory.
func (s *Service) WorkDir() string {
	return s.workDir
}

// newTF creates a fresh tfexec.Terraform instance with stdout piped to
// the provided writer and environment variables applied.
func (s *Service) newTF(output io.Writer) (*tfexec.Terraform, error) {
	tf, err := tfexec.NewTerraform(s.workDir, s.execPath)
	if err != nil {
		return nil, fmt.Errorf("creating terraform instance: %w", err)
	}

	if output != nil {
		tf.SetStdout(output)
		tf.SetStderr(output)
	}

	if len(s.env) > 0 {
		if err := tf.SetEnv(s.env); err != nil {
			return nil, fmt.Errorf("setting env vars: %w", err)
		}
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
