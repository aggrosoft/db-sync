package model

const (
	CurrentProfileVersion = 1
	SyncModeInsertMissing = "insert-missing"
)

type ConnectionMode string

const (
	ConnectionModeConnectionString ConnectionMode = "connection-string"
	ConnectionModeDetails          ConnectionMode = "details"
	ConnectionModeLegacyTemplate   ConnectionMode = "legacy-template"
)

type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
	EngineMariaDB  Engine = "mariadb"
)

type ConnectionString struct {
	Value  string `yaml:"-"`
	EnvVar string `yaml:"env,omitempty"`
}

type ConnectionDetails struct {
	Host        string `yaml:"host,omitempty"`
	Port        int    `yaml:"port,omitempty"`
	Database    string `yaml:"database,omitempty"`
	Username    string `yaml:"username,omitempty"`
	Password    string `yaml:"-"`
	PasswordEnv string `yaml:"password_env,omitempty"`
	SSLMode     string `yaml:"sslmode,omitempty"`
}

type Connection struct {
	Mode             ConnectionMode    `yaml:"mode,omitempty"`
	ConnectionString ConnectionString  `yaml:"connection_string,omitempty"`
	Details          ConnectionDetails `yaml:"details,omitempty"`
}

type Endpoint struct {
	Engine      Engine     `yaml:"engine"`
	Connection  Connection `yaml:"connection,omitempty"`
	DSNTemplate string     `yaml:"dsn_template,omitempty"`
}

type Selection struct {
	Tables         []string `yaml:"tables"`
	ExcludedTables []string `yaml:"excluded_tables,omitempty"`
}

type SyncOptions struct {
	Mode         string `yaml:"mode"`
	MirrorDelete bool   `yaml:"mirror_delete"`
}

type Profile struct {
	Version   int         `yaml:"version"`
	Name      string      `yaml:"name"`
	Source    Endpoint    `yaml:"source"`
	Target    Endpoint    `yaml:"target"`
	Selection Selection   `yaml:"selection"`
	Sync      SyncOptions `yaml:"sync"`
}

func DefaultProfile(name string) Profile {
	return Profile{
		Version: CurrentProfileVersion,
		Name:    name,
		Selection: Selection{
			Tables:         []string{},
			ExcludedTables: []string{},
		},
		Sync: SyncOptions{
			Mode:         SyncModeInsertMissing,
			MirrorDelete: false,
		},
	}
}

func (p Profile) WithDefaults() Profile {
	if p.Version == 0 {
		p.Version = CurrentProfileVersion
	}
	p.Source = p.Source.WithDefaults()
	p.Target = p.Target.WithDefaults()
	if p.Selection.Tables == nil {
		p.Selection.Tables = []string{}
	}
	if p.Selection.ExcludedTables == nil {
		p.Selection.ExcludedTables = []string{}
	}
	if p.Sync.Mode == "" {
		p.Sync.Mode = SyncModeInsertMissing
	}
	return p
}

func (endpoint Endpoint) WithDefaults() Endpoint {
	if endpoint.Connection.Details.Port == 0 {
		endpoint.Connection.Details.Port = endpoint.defaultPort()
	}
	if endpoint.Engine == EnginePostgres && endpoint.Connection.Details.SSLMode == "" {
		endpoint.Connection.Details.SSLMode = "disable"
	}
	return endpoint
}

func (endpoint Endpoint) EffectiveConnectionMode() ConnectionMode {
	if endpoint.Connection.Mode != "" {
		return endpoint.Connection.Mode
	}
	if endpoint.DSNTemplate != "" {
		return ConnectionModeLegacyTemplate
	}
	if endpoint.Connection.ConnectionString.Value != "" || endpoint.Connection.ConnectionString.EnvVar != "" {
		return ConnectionModeConnectionString
	}
	if endpoint.Connection.Details.Host != "" || endpoint.Connection.Details.Port != 0 || endpoint.Connection.Details.Database != "" || endpoint.Connection.Details.Username != "" || endpoint.Connection.Details.Password != "" || endpoint.Connection.Details.PasswordEnv != "" {
		return ConnectionModeDetails
	}
	return ""
}

func (endpoint Endpoint) defaultPort() int {
	switch endpoint.Engine {
	case EnginePostgres:
		return 5432
	case EngineMySQL, EngineMariaDB:
		return 3306
	default:
		return 0
	}
}
