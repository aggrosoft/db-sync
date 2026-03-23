package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/secrets"

	"github.com/adrg/xdg"
)

type FilesystemStore struct {
	baseDir string
	appName string
}

func NewFilesystemStore(baseDir string, appName string) *FilesystemStore {
	if appName == "" {
		appName = "db-sync"
	}
	return &FilesystemStore{baseDir: baseDir, appName: appName}
}

func (store *FilesystemStore) Save(_ context.Context, profile model.Profile) (string, error) {
	normalized, err := NormalizeProfile(profile)
	if err != nil {
		return "", err
	}
	if normalized.Source.EffectiveConnectionMode() == model.ConnectionModeLegacyTemplate {
		if err := secrets.ValidateTemplatePolicy(normalized.Source.DSNTemplate); err != nil {
			return "", err
		}
	}
	if normalized.Target.EffectiveConnectionMode() == model.ConnectionModeLegacyTemplate {
		if err := secrets.ValidateTemplatePolicy(normalized.Target.DSNTemplate); err != nil {
			return "", err
		}
	}
	path, err := store.PathFor(normalized.Name)
	if err != nil {
		return "", err
	}
	envPath := EnvPathForProfilePath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		loaded, parseErr := UnmarshalProfile(existing)
		if parseErr == nil && !strings.EqualFold(strings.TrimSpace(loaded.Name), strings.TrimSpace(normalized.Name)) {
			return "", fmt.Errorf("profile slug collision for %q", normalized.Name)
		}
	}
	encoded, err := MarshalProfile(normalized)
	if err != nil {
		return "", err
	}
	envContent, err := marshalProfileEnv(normalized)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", err
	}
	if strings.TrimSpace(envContent) == "" {
		_ = os.Remove(envPath)
	} else if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (store *FilesystemStore) Load(_ context.Context, name string) (model.Profile, error) {
	path, err := store.PathFor(name)
	if err != nil {
		return model.Profile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Profile{}, ErrProfileNotFound
		}
		return model.Profile{}, err
	}
	loaded, err := UnmarshalProfile(data)
	if err != nil {
		return model.Profile{}, err
	}
	env, err := loadProfileEnv(EnvPathForProfilePath(path))
	if err != nil {
		return model.Profile{}, err
	}
	return hydrateProfileSecrets(loaded, env), nil
}

func (store *FilesystemStore) List(_ context.Context) ([]StoredProfile, error) {
	profilesDir, err := store.profilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []StoredProfile{}, nil
		}
		return nil, err
	}
	profiles := make([]StoredProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(profilesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		loaded, err := UnmarshalProfile(data)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, StoredProfile{Name: loaded.Name, Slug: Slugify(loaded.Name), Path: path})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (store *FilesystemStore) PathFor(name string) (string, error) {
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	profilesDir, err := store.profilesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(profilesDir, Slugify(name)+".yaml"), nil
}

func (store *FilesystemStore) profilesDir() (string, error) {
	if store.baseDir != "" {
		return filepath.Join(store.baseDir, store.appName, "profiles"), nil
	}
	return filepath.Join(xdg.ConfigHome, store.appName, "profiles"), nil
}

func marshalProfileEnv(profile model.Profile) (string, error) {
	values := map[string]string{}
	if err := collectEndpointEnv(values, profile.Source); err != nil {
		return "", err
	}
	if err := collectEndpointEnv(values, profile.Target); err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func collectEndpointEnv(values map[string]string, endpoint model.Endpoint) error {
	switch endpoint.EffectiveConnectionMode() {
	case model.ConnectionModeConnectionString:
		if endpoint.Connection.ConnectionString.EnvVar == "" || endpoint.Connection.ConnectionString.Value == "" {
			return fmt.Errorf("connection string env backing is incomplete for %s", endpoint.Engine)
		}
		values[endpoint.Connection.ConnectionString.EnvVar] = endpoint.Connection.ConnectionString.Value
	case model.ConnectionModeDetails:
		if endpoint.Connection.Details.PasswordEnv == "" || endpoint.Connection.Details.Password == "" {
			return fmt.Errorf("password env backing is incomplete for %s", endpoint.Engine)
		}
		values[endpoint.Connection.Details.PasswordEnv] = endpoint.Connection.Details.Password
	}
	return nil
}

func loadProfileEnv(path string) (map[string]string, error) {
	env, err := secrets.LoadEnvFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return env, nil
}

func hydrateProfileSecrets(profile model.Profile, env map[string]string) model.Profile {
	profile.Source = hydrateEndpointSecrets(profile.Source, env)
	profile.Target = hydrateEndpointSecrets(profile.Target, env)
	return profile
}

func hydrateEndpointSecrets(endpoint model.Endpoint, env map[string]string) model.Endpoint {
	switch endpoint.EffectiveConnectionMode() {
	case model.ConnectionModeConnectionString:
		endpoint.Connection.ConnectionString.Value = env[endpoint.Connection.ConnectionString.EnvVar]
	case model.ConnectionModeDetails:
		endpoint.Connection.Details.Password = env[endpoint.Connection.Details.PasswordEnv]
	}
	return endpoint
}
