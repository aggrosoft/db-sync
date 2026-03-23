package postgres

import (
	"context"
	"fmt"

	"db-sync/internal/model"
	"db-sync/internal/profile"

	"github.com/jackc/pgx/v5"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (adapter *Adapter) ValidateSource(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error) {
	return adapter.validate(ctx, resolvedDSN, engine, "source", false)
}

func (adapter *Adapter) ValidateTarget(ctx context.Context, resolvedDSN string, engine model.Engine) (profile.EndpointValidation, error) {
	return adapter.validate(ctx, resolvedDSN, engine, "target", true)
}

func (adapter *Adapter) validate(ctx context.Context, resolvedDSN string, engine model.Engine, role string, requireWritable bool) (profile.EndpointValidation, error) {
	conn, err := pgx.Connect(ctx, resolvedDSN)
	if err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	defer conn.Close(ctx)

	checks := []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}}
	var tableCount int
	if err := conn.QueryRow(ctx, "select count(*) from information_schema.tables").Scan(&tableCount); err != nil {
		failedValidation := failed(role, engine, "metadata", err.Error())
		failedValidation.Checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusFailed, Detail: err.Error()})
		return failedValidation, err
	}
	checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusPassed, Detail: fmt.Sprintf("information_schema visible (%d rows)", tableCount)})
	if requireWritable {
		var readOnly string
		if err := conn.QueryRow(ctx, "show transaction_read_only").Scan(&readOnly); err != nil {
			return failed(role, engine, "target capability", err.Error()), err
		}
		var inRecovery bool
		if err := conn.QueryRow(ctx, "select pg_is_in_recovery()").Scan(&inRecovery); err != nil {
			return failed(role, engine, "target capability", err.Error()), err
		}
		if readOnly == "on" || inRecovery {
			validation := failed(role, engine, "target capability", "target is read-only or in recovery")
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: "target is read-only or in recovery"})
			return validation, fmt.Errorf("target is read-only or in recovery")
		}
		checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusPassed, Detail: "target accepts non-mutating probe"})
	}
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusPassed, Checks: checks}, nil
}

func failed(role string, engine model.Engine, name string, detail string) profile.EndpointValidation {
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusFailed, Checks: []profile.CheckResult{{Name: name, Status: profile.StatusFailed, Detail: detail}}, Message: detail}
}
