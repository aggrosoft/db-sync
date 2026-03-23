package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"db-sync/internal/model"
	"db-sync/internal/profile"
)

type scriptedWizard struct {
	startNewProfile model.Profile
	startNewErr     error
	editProfiles    []model.Profile
	editErr         error
	startNewCalls   int
	startEditCalls  int
	startEditInputs []model.Profile
}

func (wizard *scriptedWizard) StartNew(context.Context) (model.Profile, error) {
	wizard.startNewCalls++
	if wizard.startNewErr != nil {
		return model.Profile{}, wizard.startNewErr
	}
	return wizard.startNewProfile, nil
}

func (wizard *scriptedWizard) StartEdit(_ context.Context, existing model.Profile) (model.Profile, error) {
	wizard.startEditCalls++
	wizard.startEditInputs = append(wizard.startEditInputs, existing)
	if wizard.editErr != nil {
		return model.Profile{}, wizard.editErr
	}
	if len(wizard.editProfiles) == 0 {
		return model.Profile{}, errors.New("unexpected StartEdit call")
	}
	profile := wizard.editProfiles[0]
	wizard.editProfiles = wizard.editProfiles[1:]
	return profile, nil
}

type scriptedValidator struct {
	reports []profile.ValidationReport
	errs    []error
	inputs  []model.Profile
}

func (validator *scriptedValidator) ValidateProfile(context.Context, model.Profile) (profile.ValidationReport, error) {
	return profile.ValidationReport{}, nil
}

func (validator *scriptedValidator) ValidateAndSave(_ context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	validator.inputs = append(validator.inputs, candidate)
	if len(validator.reports) == 0 {
		return profile.ValidationReport{}, nil
	}
	report := validator.reports[0]
	validator.reports = validator.reports[1:]
	var err error
	if len(validator.errs) > 0 {
		err = validator.errs[0]
		validator.errs = validator.errs[1:]
	}
	return report, err
}

type scriptedStore struct {
	profile model.Profile
	err     error
}

func (store scriptedStore) Save(context.Context, model.Profile) (string, error) {
	return "", nil
}

func (store scriptedStore) Load(context.Context, string) (model.Profile, error) {
	if store.err != nil {
		return model.Profile{}, store.err
	}
	return store.profile, nil
}

func (store scriptedStore) List(context.Context) ([]profile.StoredProfile, error) {
	return nil, nil
}

func (store scriptedStore) PathFor(string) (string, error) {
	return "", nil
}

func TestStartInteractiveProfileRetriesBlockedValidation(t *testing.T) {
	stdout := &bytes.Buffer{}
	initial := model.DefaultProfile("retry-me")
	wizard := &scriptedWizard{startNewProfile: initial}
	validator := &scriptedValidator{
		reports: []profile.ValidationReport{
			{Blocked: true, Summary: "source validation failed"},
			{SavedPath: "memory://retry-me", Summary: "Validation passed and profile was saved."},
		},
		errs: []error{errors.New("source validation failed"), nil},
	}
	app := NewApp(bytes.NewBuffer(nil), stdout, &bytes.Buffer{})
	app.SetWizard(wizard)
	app.SetValidator(validator)
	app.SetValidationFailurePrompt(func(context.Context, model.Profile, profile.ValidationReport) (validationFailureAction, error) {
		return validationFailureRetry, nil
	})

	if err := app.StartInteractiveProfile(context.Background()); err != nil {
		t.Fatalf("StartInteractiveProfile() error = %v", err)
	}
	if wizard.startNewCalls != 1 {
		t.Fatalf("StartNew calls = %d, want 1", wizard.startNewCalls)
	}
	if wizard.startEditCalls != 0 {
		t.Fatalf("StartEdit calls = %d, want 0", wizard.startEditCalls)
	}
	if len(validator.inputs) != 2 {
		t.Fatalf("ValidateAndSave calls = %d, want 2", len(validator.inputs))
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("Saved profile to memory://retry-me")) {
		t.Fatalf("stdout = %q, want saved profile message", got)
	}
}

func TestRunProfileEditAllowsModifyAfterBlockedValidation(t *testing.T) {
	stdout := &bytes.Buffer{}
	existing := model.DefaultProfile("edit-me")
	firstEdit := existing
	updated := existing
	updated.Source.Engine = model.EngineMySQL
	wizard := &scriptedWizard{editProfiles: []model.Profile{firstEdit, updated}}
	validator := &scriptedValidator{
		reports: []profile.ValidationReport{
			{Blocked: true, Summary: "target validation failed"},
			{SavedPath: "memory://edit-me", Summary: "Validation passed and profile was saved."},
		},
		errs: []error{errors.New("target validation failed"), nil},
	}
	app := NewApp(bytes.NewBuffer(nil), stdout, &bytes.Buffer{})
	app.SetStore(scriptedStore{profile: existing})
	app.SetWizard(wizard)
	app.SetValidator(validator)
	app.SetValidationFailurePrompt(func(context.Context, model.Profile, profile.ValidationReport) (validationFailureAction, error) {
		return validationFailureModify, nil
	})

	if err := app.RunProfileEdit(context.Background(), existing.Name); err != nil {
		t.Fatalf("RunProfileEdit() error = %v", err)
	}
	if wizard.startEditCalls != 2 {
		t.Fatalf("StartEdit calls = %d, want 2", wizard.startEditCalls)
	}
	if len(wizard.startEditInputs) != 2 {
		t.Fatalf("StartEdit inputs = %d, want 2", len(wizard.startEditInputs))
	}
	if wizard.startEditInputs[0].Source.Engine != existing.Source.Engine {
		t.Fatalf("initial StartEdit input = %+v, want original profile", wizard.startEditInputs[0])
	}
	if wizard.startEditInputs[1].Source.Engine != firstEdit.Source.Engine {
		t.Fatalf("modify StartEdit input = %+v, want in-progress profile", wizard.startEditInputs[1])
	}
	if len(validator.inputs) != 2 {
		t.Fatalf("ValidateAndSave calls = %d, want 2", len(validator.inputs))
	}
	if validator.inputs[1].Source.Engine != model.EngineMySQL {
		t.Fatalf("modified profile engine = %q, want %q", validator.inputs[1].Source.Engine, model.EngineMySQL)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("Saved profile to memory://edit-me")) {
		t.Fatalf("stdout = %q, want saved profile message", got)
	}
}
