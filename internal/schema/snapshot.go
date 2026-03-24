package schema

import (
	"fmt"
	"sort"
	"strings"

	"db-sync/internal/model"
)

type TableID struct {
	Schema string
	Name   string
}

func ParseTableID(value string) TableID {
	parts := strings.SplitN(strings.TrimSpace(value), ".", 2)
	if len(parts) == 2 {
		return TableID{Schema: parts[0], Name: parts[1]}
	}
	return TableID{Name: strings.TrimSpace(value)}
}

func (id TableID) String() string {
	if id.Schema == "" {
		return id.Name
	}
	return id.Schema + "." + id.Name
}

type Snapshot struct {
	Role   string
	Engine model.Engine
	Tables []Table
}

type Table struct {
	ID          TableID
	Columns     []Column
	PrimaryKey  PrimaryKey
	ForeignKeys []ForeignKey
}

type Column struct {
	Name             string
	Ordinal          int
	DataType         string
	NativeType       string
	Nullable         bool
	DefaultSQL       string
	HasProvenDefault bool
	Identity         bool
	Generated        bool
	Writable         bool
}

type PrimaryKey struct {
	Name    string
	Columns []string
}

type ForeignKey struct {
	Name              string
	Columns           []string
	ReferencedTable   TableID
	ReferencedColumns []string
	UpdateRule        string
	DeleteRule        string
}

type BlockedError struct {
	Role        string
	Engine      model.Engine
	Summary     string
	Remediation []string
	Cause       error
}

func NewBlockedError(role string, engine model.Engine, summary string, remediation []string, cause error) error {
	return &BlockedError{Role: role, Engine: engine, Summary: summary, Remediation: remediation, Cause: cause}
}

func (err *BlockedError) Error() string {
	if err == nil {
		return ""
	}
	if len(err.Remediation) == 0 {
		if err.Cause == nil {
			return err.Summary
		}
		return fmt.Sprintf("%s: %v", err.Summary, err.Cause)
	}
	if err.Cause == nil {
		return fmt.Sprintf("%s. remediation: %s", err.Summary, strings.Join(err.Remediation, "; "))
	}
	return fmt.Sprintf("%s: %v. remediation: %s", err.Summary, err.Cause, strings.Join(err.Remediation, "; "))
}

func (err *BlockedError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func NormalizeSnapshot(snapshot Snapshot) Snapshot {
	normalized := Snapshot{Role: snapshot.Role, Engine: snapshot.Engine, Tables: make([]Table, len(snapshot.Tables))}
	for index, table := range snapshot.Tables {
		normalizedTable := Table{
			ID:          table.ID,
			Columns:     append([]Column(nil), table.Columns...),
			PrimaryKey:  PrimaryKey{Name: table.PrimaryKey.Name, Columns: append([]string(nil), table.PrimaryKey.Columns...)},
			ForeignKeys: append([]ForeignKey(nil), table.ForeignKeys...),
		}
		sort.Slice(normalizedTable.Columns, func(i, j int) bool {
			if normalizedTable.Columns[i].Ordinal == normalizedTable.Columns[j].Ordinal {
				return normalizedTable.Columns[i].Name < normalizedTable.Columns[j].Name
			}
			return normalizedTable.Columns[i].Ordinal < normalizedTable.Columns[j].Ordinal
		})
		sort.Strings(normalizedTable.PrimaryKey.Columns)
		for foreignKeyIndex := range normalizedTable.ForeignKeys {
			foreignKey := &normalizedTable.ForeignKeys[foreignKeyIndex]
			foreignKey.Columns = append([]string(nil), foreignKey.Columns...)
			foreignKey.ReferencedColumns = append([]string(nil), foreignKey.ReferencedColumns...)
		}
		sort.Slice(normalizedTable.ForeignKeys, func(i, j int) bool {
			if normalizedTable.ForeignKeys[i].Name == normalizedTable.ForeignKeys[j].Name {
				return normalizedTable.ForeignKeys[i].ReferencedTable.String() < normalizedTable.ForeignKeys[j].ReferencedTable.String()
			}
			return normalizedTable.ForeignKeys[i].Name < normalizedTable.ForeignKeys[j].Name
		})
		normalized.Tables[index] = normalizedTable
	}
	sort.Slice(normalized.Tables, func(i, j int) bool {
		return normalized.Tables[i].ID.String() < normalized.Tables[j].ID.String()
	})
	return normalized
}

func (snapshot Snapshot) TableByID(id TableID) (Table, bool) {
	for _, table := range snapshot.Tables {
		if table.ID == id {
			return table, true
		}
	}
	return Table{}, false
}
