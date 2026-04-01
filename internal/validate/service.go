package validate

import (
	"context"
	"fmt"
	"sort"

	"db-sync/internal/model"
	"db-sync/internal/profile"
)

type Adapter interface {
	ValidateSource(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error)
	ValidateTarget(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error)
}

type Registry map[model.Engine]Adapter

type Service struct {
	envProvider func() map[string]string
	registry    Registry
}

func NewService(envProvider func() map[string]string, registry Registry) *Service {
	return &Service{envProvider: envProvider, registry: registry}
}

func (service *Service) ValidateProfile(ctx context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	normalized, err := profile.NormalizeProfile(candidate)
	if err != nil {
		return profile.ValidationReport{}, err
	}
	env := service.envProvider()
	sourceDSN, sourceMissing, err := ResolveEndpoint(normalized.Source, env)
	sourceErr := err
	targetDSN, targetMissing, err := ResolveEndpoint(normalized.Target, env)
	targetErr := err
	if sourceErr != nil || targetErr != nil {
		missing := dedupe(append(sourceMissing, targetMissing...))
		switch {
		case sourceErr != nil:
			return profile.ValidationReport{MissingEnv: missing, Blocked: true, Summary: sourceErr.Error()}, sourceErr
		default:
			return profile.ValidationReport{MissingEnv: missing, Blocked: true, Summary: targetErr.Error()}, targetErr
		}
	}
	sourceAdapter, err := service.adapterFor(normalized.Source.Engine)
	if err != nil {
		return profile.ValidationReport{}, err
	}
	targetAdapter, err := service.adapterFor(normalized.Target.Engine)
	if err != nil {
		return profile.ValidationReport{}, err
	}
	sourceReport, err := sourceAdapter.ValidateSource(ctx, sourceDSN, normalized.Source.Engine)
	if err != nil {
		return blockedReport(sourceReport, profile.EndpointValidation{}, err), err
	}
	targetReport, err := targetAdapter.ValidateTarget(ctx, targetDSN, normalized.Target.Engine)
	if err != nil {
		return blockedReport(sourceReport, targetReport, err), err
	}
	report := profile.ValidationReport{Source: sourceReport, Target: targetReport, Summary: "Validation passed for both endpoints."}
	report.Blocked = report.Source.Status == profile.StatusFailed || report.Target.Status == profile.StatusFailed
	return report, nil
}

func (service *Service) adapterFor(engine model.Engine) (Adapter, error) {
	adapter, ok := service.registry[engine]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for engine %q", engine)
	}
	return adapter, nil
}

func blockedReport(source profile.EndpointValidation, target profile.EndpointValidation, err error) profile.ValidationReport {
	return profile.ValidationReport{Source: source, Target: target, Blocked: true, Summary: err.Error()}
}

func dedupe(values []string) []string {
	set := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func firstConfiguredValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
