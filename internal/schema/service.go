package schema

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
)

type Adapter interface {
	DiscoverSourceSchema(ctx context.Context, resolvedDSN string, engine model.Engine) (Snapshot, error)
	DiscoverTargetSchema(ctx context.Context, resolvedDSN string, engine model.Engine) (Snapshot, error)
}

type Registry map[model.Engine]Adapter

type EndpointResolver func(endpoint model.Endpoint, env map[string]string) (string, []string, error)

type EndpointDiscovery struct {
	Role        string
	Engine      model.Engine
	Snapshot    Snapshot
	Blocked     bool
	Summary     string
	Remediation []string
}

type DiscoveryReport struct {
	Source     EndpointDiscovery
	Target     EndpointDiscovery
	MissingEnv []string
	Blocked    bool
	Summary    string
}

type Service struct {
	envProvider func() map[string]string
	registry    Registry
	resolver    EndpointResolver
}

func NewService(envProvider func() map[string]string, registry Registry, resolver EndpointResolver) *Service {
	return &Service{envProvider: envProvider, registry: registry, resolver: resolver}
}

func (service *Service) DiscoverProfile(ctx context.Context, candidate model.Profile) (DiscoveryReport, error) {
	normalized, err := profilepkg.NormalizeProfile(candidate)
	if err != nil {
		return DiscoveryReport{}, err
	}
	return service.DiscoverEndpoints(ctx, normalized.Source, normalized.Target)
}

func (service *Service) DiscoverEndpoints(ctx context.Context, source model.Endpoint, target model.Endpoint) (DiscoveryReport, error) {
	env := service.envProvider()
	if service.resolver == nil {
		return DiscoveryReport{}, errors.New("endpoint resolver is not configured")
	}
	sourceDSN, sourceMissing, err := service.resolver(source, env)
	sourceErr := err
	targetDSN, targetMissing, err := service.resolver(target, env)
	targetErr := err
	if sourceErr != nil || targetErr != nil {
		missing := dedupe(append(sourceMissing, targetMissing...))
		report := DiscoveryReport{
			Source:     EndpointDiscovery{Role: "source", Engine: source.Engine},
			Target:     EndpointDiscovery{Role: "target", Engine: target.Engine},
			MissingEnv: missing,
			Blocked:    true,
		}
		summary := sourceErr
		if summary == nil {
			summary = targetErr
		}
		if summary != nil {
			report.Summary = summary.Error()
		}
		return report, summary
	}
	sourceAdapter, err := service.adapterFor(source.Engine)
	if err != nil {
		return DiscoveryReport{}, err
	}
	targetAdapter, err := service.adapterFor(target.Engine)
	if err != nil {
		return DiscoveryReport{}, err
	}
	sourceSnapshot, err := sourceAdapter.DiscoverSourceSchema(ctx, sourceDSN, source.Engine)
	if err != nil {
		endpoint := blockedEndpoint("source", source.Engine, err)
		return DiscoveryReport{Source: endpoint, Target: EndpointDiscovery{Role: "target", Engine: target.Engine}, Blocked: true, Summary: endpoint.Summary}, err
	}
	targetSnapshot, err := targetAdapter.DiscoverTargetSchema(ctx, targetDSN, target.Engine)
	if err != nil {
		endpoint := blockedEndpoint("target", target.Engine, err)
		return DiscoveryReport{Source: EndpointDiscovery{Role: "source", Engine: source.Engine, Snapshot: sourceSnapshot}, Target: endpoint, Blocked: true, Summary: endpoint.Summary}, err
	}
	return DiscoveryReport{
		Source:  EndpointDiscovery{Role: "source", Engine: source.Engine, Snapshot: NormalizeSnapshot(sourceSnapshot)},
		Target:  EndpointDiscovery{Role: "target", Engine: target.Engine, Snapshot: NormalizeSnapshot(targetSnapshot)},
		Blocked: false,
		Summary: "Schema discovery succeeded for both endpoints.",
	}, nil
}

func (service *Service) adapterFor(engine model.Engine) (Adapter, error) {
	adapter, ok := service.registry[engine]
	if !ok {
		return nil, fmt.Errorf("no discovery adapter registered for engine %q", engine)
	}
	return adapter, nil
}

func blockedEndpoint(role string, engine model.Engine, err error) EndpointDiscovery {
	endpoint := EndpointDiscovery{Role: role, Engine: engine, Blocked: true, Summary: err.Error()}
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		endpoint.Summary = blocked.Summary
		endpoint.Remediation = append([]string(nil), blocked.Remediation...)
	}
	return endpoint
}

func dedupe(values []string) []string {
	set := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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
