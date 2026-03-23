package validate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/secrets"

	mysqlcfg "github.com/go-sql-driver/mysql"
)

type Adapter interface {
	ValidateSource(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error)
	ValidateTarget(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error)
}

type Registry map[model.Engine]Adapter

type Service struct {
	store       profile.ProfileStore
	envProvider func() map[string]string
	registry    Registry
}

func NewService(store profile.ProfileStore, envProvider func() map[string]string, registry Registry) *Service {
	return &Service{store: store, envProvider: envProvider, registry: registry}
}

func (service *Service) ValidateProfile(ctx context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	normalized, err := profile.NormalizeProfile(candidate)
	if err != nil {
		return profile.ValidationReport{}, err
	}
	if normalized.Source.EffectiveConnectionMode() == model.ConnectionModeLegacyTemplate {
		if err := secrets.ValidateTemplatePolicy(normalized.Source.DSNTemplate); err != nil {
			report := policyFailureReport("source", normalized.Source.Engine, err)
			return report, err
		}
	}
	if normalized.Target.EffectiveConnectionMode() == model.ConnectionModeLegacyTemplate {
		if err := secrets.ValidateTemplatePolicy(normalized.Target.DSNTemplate); err != nil {
			report := policyFailureReport("target", normalized.Target.Engine, err)
			return report, err
		}
	}
	env := service.envProvider()
	sourceDSN, sourceMissing, err := resolveEndpoint(normalized.Source, env)
	sourceErr := err
	targetDSN, targetMissing, err := resolveEndpoint(normalized.Target, env)
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

func (service *Service) ValidateAndSave(ctx context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	report, err := service.ValidateProfile(ctx, candidate)
	if err != nil {
		return report, err
	}
	if report.Blocked {
		return report, report.Error()
	}
	savedPath, err := service.store.Save(ctx, candidate)
	if err != nil {
		return report, err
	}
	report.SavedPath = savedPath
	report.Summary = "Validation passed and profile was saved."
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

func resolveEndpoint(endpoint model.Endpoint, env map[string]string) (string, []string, error) {
	switch endpoint.EffectiveConnectionMode() {
	case model.ConnectionModeLegacyTemplate:
		resolved, err := secrets.ResolveTemplate(endpoint.DSNTemplate, env)
		if err != nil {
			return "", resolved.Missing(), err
		}
		return resolved.Value(), nil, nil
	case model.ConnectionModeConnectionString:
		value := firstConfiguredValue(env[endpoint.Connection.ConnectionString.EnvVar], endpoint.Connection.ConnectionString.Value)
		if value == "" {
			return "", []string{endpoint.Connection.ConnectionString.EnvVar}, fmt.Errorf("missing required environment variables: %s", endpoint.Connection.ConnectionString.EnvVar)
		}
		return value, nil, nil
	case model.ConnectionModeDetails:
		password := firstConfiguredValue(env[endpoint.Connection.Details.PasswordEnv], endpoint.Connection.Details.Password)
		if password == "" {
			return "", []string{endpoint.Connection.Details.PasswordEnv}, fmt.Errorf("missing required environment variables: %s", endpoint.Connection.Details.PasswordEnv)
		}
		return buildDetailsDSN(endpoint, password)
	default:
		return "", nil, errors.New("unsupported connection mode")
	}
}

func buildDetailsDSN(endpoint model.Endpoint, password string) (string, []string, error) {
	details := endpoint.Connection.Details
	switch endpoint.Engine {
	case model.EnginePostgres:
		query := url.Values{}
		if details.SSLMode != "" {
			query.Set("sslmode", details.SSLMode)
		}
		return (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(details.Username, password),
			Host:     fmt.Sprintf("%s:%d", details.Host, details.Port),
			Path:     details.Database,
			RawQuery: query.Encode(),
		}).String(), nil, nil
	case model.EngineMySQL, model.EngineMariaDB:
		cfg := mysqlcfg.NewConfig()
		cfg.User = details.Username
		cfg.Passwd = password
		cfg.Net = "tcp"
		cfg.Addr = fmt.Sprintf("%s:%d", details.Host, details.Port)
		cfg.DBName = details.Database
		return cfg.FormatDSN(), nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported engine %q", endpoint.Engine)
	}
}

func policyFailureReport(role string, engine model.Engine, err error) profile.ValidationReport {
	endpoint := profile.EndpointValidation{
		Role:    role,
		Engine:  engine,
		Status:  profile.StatusFailed,
		Checks:  []profile.CheckResult{{Name: "placeholder policy", Status: profile.StatusFailed, Detail: err.Error()}},
		Message: err.Error(),
	}
	report := profile.ValidationReport{Blocked: true, Summary: err.Error()}
	if role == "source" {
		report.Source = endpoint
	} else {
		report.Target = endpoint
	}
	return report
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

func failedValidation(role string, engine model.Engine, detail string) profile.EndpointValidation {
	return profile.EndpointValidation{
		Role:    role,
		Engine:  engine,
		Status:  profile.StatusFailed,
		Checks:  []profile.CheckResult{{Name: "connection", Status: profile.StatusFailed, Detail: detail}},
		Message: detail,
	}
}

var ErrBlockedSave = errors.New("blocked save")

func firstConfiguredValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
