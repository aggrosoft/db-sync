package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"db-sync/internal/model"
	"db-sync/internal/profile"

	_ "github.com/go-sql-driver/mysql"
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
	db, err := sql.Open("mysql", resolvedDSN)
	if err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return failed(role, engine, "authentication", err.Error()), err
	}
	checks := []profile.CheckResult{{Name: "authentication", Status: profile.StatusPassed, Detail: "connection established"}}
	var tableCount int
	if err := db.QueryRowContext(ctx, "select count(*) from information_schema.tables").Scan(&tableCount); err != nil {
		validation := failed(role, engine, "metadata", err.Error())
		validation.Checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusFailed, Detail: err.Error()})
		return validation, err
	}
	checks = append(checks, profile.CheckResult{Name: "metadata", Status: profile.StatusPassed, Detail: fmt.Sprintf("information_schema visible (%d rows)", tableCount)})
	if requireWritable {
		var readOnly int
		if err := db.QueryRowContext(ctx, "select @@global.read_only").Scan(&readOnly); err != nil {
			return failed(role, engine, "target capability", err.Error()), err
		}
		if readOnly == 1 {
			validation := failed(role, engine, "target capability", "target is read-only")
			validation.Checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusFailed, Detail: "target is read-only"})
			return validation, fmt.Errorf("target is read-only")
		}
		checks = append(checks, profile.CheckResult{Name: "target capability", Status: profile.StatusPassed, Detail: "target accepts non-mutating probe"})
	}
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusPassed, Checks: checks}, nil
}

func failed(role string, engine model.Engine, name string, detail string) profile.EndpointValidation {
	return profile.EndpointValidation{Role: role, Engine: engine, Status: profile.StatusFailed, Checks: []profile.CheckResult{{Name: name, Status: profile.StatusFailed, Detail: detail}}, Message: detail}
}
