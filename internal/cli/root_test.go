package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"db-sync/internal/model"
	"db-sync/internal/profile"
	"db-sync/internal/schema"
)

type fakeWizard struct {
	startNewCalled bool
	profile        model.Profile
	err            error
}

func (wizard *fakeWizard) StartNew(context.Context) (model.Profile, error) {
	wizard.startNewCalled = true
	if wizard.err != nil {
		return model.Profile{}, wizard.err
	}
	return wizard.profile, nil
}

func (wizard *fakeWizard) StartEdit(context.Context, model.Profile) (model.Profile, error) {
	return model.Profile{}, errors.New("unexpected StartEdit call")
}

func (wizard *fakeWizard) SelectTables(context.Context, model.Profile, schema.DiscoveryReport) (model.Profile, error) {
	return wizard.profile, nil
}

type fakeDiscoverer struct{}

func (fakeDiscoverer) DiscoverProfile(context.Context, model.Profile) (schema.DiscoveryReport, error) {
	return schema.DiscoveryReport{}, nil
}

type fakeValidator struct{}

func (fakeValidator) ValidateProfile(context.Context, model.Profile) (profile.ValidationReport, error) {
	return profile.ValidationReport{}, nil
}

func (fakeValidator) ValidateAndSave(_ context.Context, candidate model.Profile) (profile.ValidationReport, error) {
	return profile.ValidationReport{SavedPath: "memory://" + candidate.Name}, nil
}

type fakeStore struct{}

func (fakeStore) Save(context.Context, model.Profile) (string, error) {
	return "", nil
}

func (fakeStore) Load(context.Context, string) (model.Profile, error) {
	return model.Profile{}, profile.ErrProfileNotFound
}

func (fakeStore) List(context.Context) ([]profile.StoredProfile, error) {
	return []profile.StoredProfile{}, nil
}

func (fakeStore) PathFor(string) (string, error) {
	return "", nil
}

type fakeFileInfo struct {
	mode fs.FileMode
}

func (info fakeFileInfo) Name() string       { return "stdin" }
func (info fakeFileInfo) Size() int64        { return 0 }
func (info fakeFileInfo) Mode() fs.FileMode  { return info.mode }
func (info fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (info fakeFileInfo) IsDir() bool        { return false }
func (info fakeFileInfo) Sys() any           { return nil }

type fakeReader struct {
	info fs.FileInfo
	err  error
}

func (reader fakeReader) Read(_ []byte) (int, error) {
	return 0, nil
}

func (reader fakeReader) Stat() (fs.FileInfo, error) {
	return reader.info, reader.err
}

func TestInteractivityProbe(t *testing.T) {
	tests := []struct {
		name       string
		stdin      fakeReader
		isTerminal bool
		env        map[string]string
		want       bool
	}{
		{
			name:       "windows signal keeps pseudo terminal interactive",
			stdin:      fakeReader{info: fakeFileInfo{mode: 0}},
			isTerminal: false,
			env:        map[string]string{"TERM_PROGRAM": "vscode"},
			want:       false,
		},
		{
			name:       "character device starts wizard even when x-term probe fails",
			stdin:      fakeReader{info: fakeFileInfo{mode: fs.ModeCharDevice}},
			isTerminal: false,
			env:        map[string]string{"WT_SESSION": "1"},
			want:       true,
		},
		{
			name:       "redirected stdin falls back to help",
			stdin:      fakeReader{info: fakeFileInfo{mode: 0}},
			isTerminal: false,
			env:        map[string]string{"WT_SESSION": "1"},
			want:       false,
		},
		{
			name:       "headless env stays non interactive",
			stdin:      fakeReader{info: fakeFileInfo{mode: fs.ModeCharDevice}},
			isTerminal: true,
			env:        map[string]string{"CI": "true"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := interactivityProbe{
				stdin:      tt.stdin,
				isTerminal: func(int) bool { return tt.isTerminal },
				lookupEnv:  func(key string) string { return tt.env[key] },
			}
			if got := probe.isInteractive(); got != tt.want {
				t.Fatalf("isInteractive() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRootCommandNoArgsStartsWizardWhenInteractive(t *testing.T) {
	stdout := &bytes.Buffer{}
	wizard := &fakeWizard{profile: model.DefaultProfile("interactive")}
	app := NewApp(fakeReader{info: fakeFileInfo{mode: fs.ModeCharDevice}}, stdout, &bytes.Buffer{})
	app.SetWizard(wizard)
	app.SetValidator(fakeValidator{})
	app.SetStore(fakeStore{})
	app.SetDiscoverer(fakeDiscoverer{})

	cmd := NewRootCommand(app)
	cmd.SetArgs([]string{})
	cmd.SetIn(strings.NewReader(""))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !wizard.startNewCalled {
		t.Fatal("wizard StartNew was not called")
	}
	if !strings.Contains(stdout.String(), "Saved profile to memory://interactive") {
		t.Fatalf("stdout = %q, want saved profile message", stdout.String())
	}
}

func TestRootCommandNoArgsFallsBackToHelpWhenNonInteractive(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := NewApp(fakeReader{info: fakeFileInfo{mode: 0}}, stdout, &bytes.Buffer{})
	app.SetWizard(&fakeWizard{profile: model.DefaultProfile("ignored")})
	app.SetValidator(fakeValidator{})
	app.SetDiscoverer(nil)

	cmd := NewRootCommand(app)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Create, validate, and reuse safe database sync profiles") {
		t.Fatalf("stdout = %q, want help output", stdout.String())
	}
}

func TestRootCommandSubcommandsStayAvailable(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := NewApp(fakeReader{info: fakeFileInfo{mode: 0}}, stdout, &bytes.Buffer{})
	app.SetStore(fakeStore{})

	cmd := NewRootCommand(app)
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"profile", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "No saved profiles found.") {
		t.Fatalf("stdout = %q, want list output", stdout.String())
	}
}
