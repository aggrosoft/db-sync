package wizard

const (
	ConnectionModeHelp   = "Choose a pasted connection string for the fastest path or guided details when you want the CLI to build the DSN for you."
	ConnectionStringHelp = "This value is stored in the profile .env file, not in the YAML profile."
	DetailsHelp          = "Only the password is treated as a secret; it will be written to the profile .env file and kept out of YAML."
	FutureTablesHelp     = "Table selection happens in a later phase; Phase 1 stores an empty list by design."
	MirrorDeleteHelp     = "Mirror delete removes target rows that are missing from the source. Leave this off until you explicitly need it."
)
