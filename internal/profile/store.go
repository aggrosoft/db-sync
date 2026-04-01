package profile

import (
	"context"
	"errors"
	"fmt"

	"db-sync/internal/model"
)

type Status string

const (
	StatusPassed  Status = "passed"
	StatusWarning Status = "warning"
	StatusFailed  Status = "failed"
)

type CheckResult struct {
	Name   string `yaml:"name"`
	Status Status `yaml:"status"`
	Detail string `yaml:"detail,omitempty"`
}

type EndpointValidation struct {
	Role    string        `yaml:"role"`
	Engine  model.Engine  `yaml:"engine"`
	Status  Status        `yaml:"status"`
	Checks  []CheckResult `yaml:"checks"`
	Message string        `yaml:"message,omitempty"`
}

type ValidationReport struct {
	Source     EndpointValidation `yaml:"source"`
	Target     EndpointValidation `yaml:"target"`
	MissingEnv []string           `yaml:"missing_env,omitempty"`
	Blocked    bool               `yaml:"blocked"`
	Summary    string             `yaml:"summary,omitempty"`
}

type ProfileValidator interface {
	ValidateProfile(ctx context.Context, profile model.Profile) (ValidationReport, error)
}

func (report ValidationReport) Error() error {
	if !report.Blocked {
		return nil
	}
	if len(report.MissingEnv) > 0 {
		return fmt.Errorf("missing required environment variables: %v", report.MissingEnv)
	}
	if report.Summary != "" {
		return errors.New(report.Summary)
	}
	return errors.New("profile validation failed")
}
