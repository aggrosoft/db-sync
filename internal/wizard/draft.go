package wizard

import (
	"strconv"

	"db-sync/internal/model"
	profilepkg "db-sync/internal/profile"
)

type EndpointDraft struct {
	Engine           model.Engine
	Mode             model.ConnectionMode
	ConnectionString string
	ConnectionEnvVar string
	Host             string
	Port             string
	Database         string
	Username         string
	Password         string
	PasswordEnvVar   string
	SSLMode          string
	LegacyTemplate   string
}

type ProfileDraft struct {
	Name         string
	Source       EndpointDraft
	Target       EndpointDraft
	Selection    model.Selection
	SyncMode     string
	MirrorDelete bool
}

func NewDraft() ProfileDraft {
	profile := model.DefaultProfile("")
	return ProfileDraft{
		Source:       EndpointDraft{Mode: model.ConnectionModeConnectionString},
		Target:       EndpointDraft{Mode: model.ConnectionModeConnectionString},
		Selection:    profile.Selection,
		SyncMode:     profile.Sync.Mode,
		MirrorDelete: profile.Sync.MirrorDelete,
	}
}

func FromProfile(profile model.Profile) ProfileDraft {
	profile = profile.WithDefaults()
	return ProfileDraft{
		Name:         profile.Name,
		Source:       endpointDraftFromProfile(profile.Name, "source", profile.Source),
		Target:       endpointDraftFromProfile(profile.Name, "target", profile.Target),
		Selection:    profile.Selection,
		SyncMode:     profile.Sync.Mode,
		MirrorDelete: profile.Sync.MirrorDelete,
	}
}

func (draft ProfileDraft) ToProfile() model.Profile {
	profile := model.DefaultProfile(draft.Name)
	profile.Source = draft.Source.ToEndpoint(draft.Name, "source")
	profile.Target = draft.Target.ToEndpoint(draft.Name, "target")
	profile.Selection = draft.Selection
	profile.Sync.Mode = draft.SyncMode
	profile.Sync.MirrorDelete = draft.MirrorDelete
	return profile.WithDefaults()
}

func endpointDraftFromProfile(profileName string, role string, endpoint model.Endpoint) EndpointDraft {
	endpoint = endpoint.WithDefaults()
	draft := EndpointDraft{
		Engine:           endpoint.Engine,
		Mode:             endpoint.EffectiveConnectionMode(),
		ConnectionString: endpoint.Connection.ConnectionString.Value,
		ConnectionEnvVar: endpoint.Connection.ConnectionString.EnvVar,
		Host:             endpoint.Connection.Details.Host,
		Port:             profilepkg.PortString(endpoint.Connection.Details.Port),
		Database:         endpoint.Connection.Details.Database,
		Username:         endpoint.Connection.Details.Username,
		Password:         endpoint.Connection.Details.Password,
		PasswordEnvVar:   endpoint.Connection.Details.PasswordEnv,
		SSLMode:          endpoint.Connection.Details.SSLMode,
		LegacyTemplate:   endpoint.DSNTemplate,
	}
	if draft.ConnectionEnvVar == "" {
		draft.ConnectionEnvVar = profilepkg.ConnectionStringEnvVar(profileName, role)
	}
	if draft.PasswordEnvVar == "" {
		draft.PasswordEnvVar = profilepkg.PasswordEnvVar(profileName, role)
	}
	if draft.Mode == "" {
		draft.Mode = model.ConnectionModeConnectionString
	}
	return draft
}

func (draft EndpointDraft) ToEndpoint(profileName string, role string) model.Endpoint {
	endpoint := model.Endpoint{Engine: draft.Engine}
	switch draft.Mode {
	case model.ConnectionModeLegacyTemplate:
		endpoint.Connection.Mode = model.ConnectionModeLegacyTemplate
		endpoint.DSNTemplate = draft.LegacyTemplate
	case model.ConnectionModeDetails:
		port, _ := strconv.Atoi(draft.Port)
		endpoint.Connection.Mode = model.ConnectionModeDetails
		endpoint.Connection.Details = model.ConnectionDetails{
			Host:        draft.Host,
			Port:        port,
			Database:    draft.Database,
			Username:    draft.Username,
			Password:    draft.Password,
			PasswordEnv: firstNonEmpty(draft.PasswordEnvVar, profilepkg.PasswordEnvVar(profileName, role)),
			SSLMode:     draft.SSLMode,
		}
	default:
		endpoint.Connection.Mode = model.ConnectionModeConnectionString
		endpoint.Connection.ConnectionString = model.ConnectionString{
			Value:  draft.ConnectionString,
			EnvVar: firstNonEmpty(draft.ConnectionEnvVar, profilepkg.ConnectionStringEnvVar(profileName, role)),
		}
	}
	return endpoint.WithDefaults()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
