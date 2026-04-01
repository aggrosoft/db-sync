//go:build integration

package sync

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	mysqladapter "db-sync/internal/db/mysql"
	"db-sync/internal/model"
	"db-sync/internal/schema"
	"db-sync/internal/testkit"
)

type integrationAnalysis struct {
	preview   schema.SelectionPreview
	discovery schema.DiscoveryReport
	drift     schema.DriftReport
}

func (analysis integrationAnalysis) SelectionPreview() schema.SelectionPreview {
	return analysis.preview
}

func (analysis integrationAnalysis) DiscoveryReport() schema.DiscoveryReport {
	return analysis.discovery
}

func (analysis integrationAnalysis) DriftReport() schema.DriftReport {
	return analysis.drift
}

func TestRunProfileIntegrationUpsertAndMirrorDelete(t *testing.T) {
	ctx := context.Background()
	sourceContainer := testkit.StartMySQLContainer(ctx, t)
	defer sourceContainer.Cleanup()
	targetContainer := testkit.StartMySQLContainer(ctx, t)
	defer targetContainer.Cleanup()

	sourceDSN := strings.ReplaceAll(sourceContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	targetDSN := strings.ReplaceAll(targetContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	sourceDB := openMySQLForTest(t, sourceDSN)
	defer sourceDB.Close()
	targetDB := openMySQLForTest(t, targetDSN)
	defer targetDB.Close()

	applyStatements(t, sourceDB,
		`create table authors (id int primary key, name varchar(255) not null, status varchar(32) not null)`,
		`create table books (id int primary key, author_id int not null, title varchar(255) not null, constraint books_authors_fk foreign key (author_id) references authors(id))`,
		`insert into authors (id, name, status) values (1, 'Alice Updated', 'active'), (2, 'Bob', 'active')`,
		`insert into books (id, author_id, title) values (20, 1, 'Fresh Title'), (21, 2, 'Bob Book')`,
	)
	applyStatements(t, targetDB,
		`create table authors (id int primary key, name varchar(255) not null, status varchar(32) not null)`,
		`create table books (id int primary key, author_id int not null, title varchar(255) not null, constraint books_authors_fk foreign key (author_id) references authors(id))`,
		`insert into authors (id, name, status) values (1, 'Alice Old', 'active'), (3, 'Ghost', 'inactive')`,
		`insert into books (id, author_id, title) values (10, 3, 'Ghost Book'), (20, 1, 'Old Title')`,
	)

	adapter := mysqladapter.NewAdapter()
	sourceSnapshot, err := adapter.DiscoverSourceSchema(ctx, sourceDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	targetSnapshot, err := adapter.DiscoverTargetSchema(ctx, targetDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverTargetSchema() error = %v", err)
	}

	candidate := model.DefaultProfile("integration")
	candidate.Source.Engine = model.EngineMySQL
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.Value = sourceDSN
	candidate.Target.Engine = model.EngineMySQL
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString.Value = targetDSN
	candidate.Selection.Tables = []string{"books"}
	candidate.Sync.MirrorDelete = true

	preview, err := schema.PreviewSelection(schema.BuildDependencyGraph(sourceSnapshot), candidate.Selection.Tables, candidate.Selection.ExcludedTables)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	analysis := integrationAnalysis{
		preview: preview,
		discovery: schema.DiscoveryReport{
			Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMySQL, Snapshot: sourceSnapshot},
			Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMySQL, Snapshot: targetSnapshot},
		},
		drift: schema.CompareSnapshots(filterSnapshot(sourceSnapshot, preview.FinalTables), filterSnapshot(targetSnapshot, preview.FinalTables)),
	}

	service := NewService(func() map[string]string { return map[string]string{} })
	report, err := service.RunProfile(ctx, candidate, analysis, false, nil)
	if err != nil {
		t.Fatalf("RunProfile() error = %v", err)
	}
	if report.InsertedRows != 2 {
		t.Fatalf("InsertedRows = %d, want 2", report.InsertedRows)
	}
	if report.UpdatedRows != 1 {
		t.Fatalf("UpdatedRows = %d, want 1", report.UpdatedRows)
	}
	if report.DeletedRows != 1 {
		t.Fatalf("DeletedRows = %d, want 1", report.DeletedRows)
	}
	if len(report.Tables) != 2 {
		t.Fatalf("Tables = %d, want 2", len(report.Tables))
	}
	if report.Tables[0].Scope != "implicit" || report.Tables[1].Scope != "explicit" {
		t.Fatalf("table scopes = %#v, want implicit then explicit", report.Tables)
	}

	assertStringRows(t, targetDB, `select cast(id as char), name, status from authors order by id`, [][]string{{"1", "Alice Old", "active"}, {"2", "Bob", "active"}, {"3", "Ghost", "inactive"}})
	assertStringRows(t, targetDB, `select cast(id as char), cast(author_id as char), title from books order by id`, [][]string{{"20", "1", "Fresh Title"}, {"21", "2", "Bob Book"}})
}

func TestRunProfileIntegrationDryRunReportsPlannedChanges(t *testing.T) {
	ctx := context.Background()
	sourceContainer := testkit.StartMySQLContainer(ctx, t)
	defer sourceContainer.Cleanup()
	targetContainer := testkit.StartMySQLContainer(ctx, t)
	defer targetContainer.Cleanup()

	sourceDSN := strings.ReplaceAll(sourceContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	targetDSN := strings.ReplaceAll(targetContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	sourceDB := openMySQLForTest(t, sourceDSN)
	defer sourceDB.Close()
	targetDB := openMySQLForTest(t, targetDSN)
	defer targetDB.Close()

	applyStatements(t, sourceDB,
		`create table authors (id int primary key, name varchar(255) not null, status varchar(32) not null)`,
		`create table books (id int primary key, author_id int not null, title varchar(255) not null, constraint books_authors_fk foreign key (author_id) references authors(id))`,
		`insert into authors (id, name, status) values (1, 'Alice Updated', 'active'), (2, 'Bob', 'active')`,
		`insert into books (id, author_id, title) values (20, 1, 'Fresh Title'), (21, 2, 'Bob Book')`,
	)
	applyStatements(t, targetDB,
		`create table authors (id int primary key, name varchar(255) not null, status varchar(32) not null)`,
		`create table books (id int primary key, author_id int not null, title varchar(255) not null, constraint books_authors_fk foreign key (author_id) references authors(id))`,
		`insert into authors (id, name, status) values (1, 'Alice Old', 'active'), (3, 'Ghost', 'inactive')`,
		`insert into books (id, author_id, title) values (10, 3, 'Ghost Book'), (20, 1, 'Old Title')`,
	)

	adapter := mysqladapter.NewAdapter()
	sourceSnapshot, err := adapter.DiscoverSourceSchema(ctx, sourceDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	targetSnapshot, err := adapter.DiscoverTargetSchema(ctx, targetDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverTargetSchema() error = %v", err)
	}

	candidate := model.DefaultProfile("integration")
	candidate.Source.Engine = model.EngineMySQL
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.Value = sourceDSN
	candidate.Target.Engine = model.EngineMySQL
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString.Value = targetDSN
	candidate.Selection.Tables = []string{"books"}
	candidate.Sync.MirrorDelete = true

	preview, err := schema.PreviewSelection(schema.BuildDependencyGraph(sourceSnapshot), candidate.Selection.Tables, candidate.Selection.ExcludedTables)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	analysis := integrationAnalysis{
		preview: preview,
		discovery: schema.DiscoveryReport{
			Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMySQL, Snapshot: sourceSnapshot},
			Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMySQL, Snapshot: targetSnapshot},
		},
		drift: schema.CompareSnapshots(filterSnapshot(sourceSnapshot, preview.FinalTables), filterSnapshot(targetSnapshot, preview.FinalTables)),
	}

	service := NewService(func() map[string]string { return map[string]string{} })
	report, err := service.RunProfile(ctx, candidate, analysis, true, nil)
	if err != nil {
		t.Fatalf("RunProfile() error = %v", err)
	}
	if report.InsertedRows != 2 {
		t.Fatalf("InsertedRows = %d, want 2", report.InsertedRows)
	}
	if report.UpdatedRows != 1 {
		t.Fatalf("UpdatedRows = %d, want 1", report.UpdatedRows)
	}
	if report.DeletedRows != 1 {
		t.Fatalf("DeletedRows = %d, want 1", report.DeletedRows)
	}

	assertStringRows(t, targetDB, `select cast(id as char), name, status from authors order by id`, [][]string{{"1", "Alice Old", "active"}, {"3", "Ghost", "inactive"}})
	assertStringRows(t, targetDB, `select cast(id as char), cast(author_id as char), title from books order by id`, [][]string{{"10", "3", "Ghost Book"}, {"20", "1", "Old Title"}})
}

func TestRunProfileIntegrationMirrorDeleteRunsBeforeInsert(t *testing.T) {
	ctx := context.Background()
	sourceContainer := testkit.StartMySQLContainer(ctx, t)
	defer sourceContainer.Cleanup()
	targetContainer := testkit.StartMySQLContainer(ctx, t)
	defer targetContainer.Cleanup()

	sourceDSN := strings.ReplaceAll(sourceContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	targetDSN := strings.ReplaceAll(targetContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	sourceDB := openMySQLForTest(t, sourceDSN)
	defer sourceDB.Close()
	targetDB := openMySQLForTest(t, targetDSN)
	defer targetDB.Close()

	applyStatements(t, sourceDB,
		`create table entries (id int primary key, external_number varchar(64) not null, payload varchar(255) not null, unique key entries_external_number_uq (external_number))`,
		`insert into entries (id, external_number, payload) values (1, 'A-100', 'alpha'), (2, 'B-200', 'beta')`,
	)
	applyStatements(t, targetDB,
		`create table entries (id int primary key, external_number varchar(64) not null, payload varchar(255) not null, unique key entries_external_number_uq (external_number))`,
		`insert into entries (id, external_number, payload) values (9, 'B-200', 'stale test row')`,
	)

	adapter := mysqladapter.NewAdapter()
	sourceSnapshot, err := adapter.DiscoverSourceSchema(ctx, sourceDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	targetSnapshot, err := adapter.DiscoverTargetSchema(ctx, targetDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverTargetSchema() error = %v", err)
	}

	candidate := model.DefaultProfile("integration")
	candidate.Source.Engine = model.EngineMySQL
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.Value = sourceDSN
	candidate.Target.Engine = model.EngineMySQL
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString.Value = targetDSN
	candidate.Selection.Tables = []string{"entries"}
	candidate.Sync.MirrorDelete = true

	preview, err := schema.PreviewSelection(schema.BuildDependencyGraph(sourceSnapshot), candidate.Selection.Tables, candidate.Selection.ExcludedTables)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	analysis := integrationAnalysis{
		preview: preview,
		discovery: schema.DiscoveryReport{
			Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMySQL, Snapshot: sourceSnapshot},
			Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMySQL, Snapshot: targetSnapshot},
		},
		drift: schema.CompareSnapshots(filterSnapshot(sourceSnapshot, preview.FinalTables), filterSnapshot(targetSnapshot, preview.FinalTables)),
	}

	service := NewService(func() map[string]string { return map[string]string{} })
	report, err := service.RunProfile(ctx, candidate, analysis, false, nil)
	if err != nil {
		t.Fatalf("RunProfile() error = %v", err)
	}
	if report.InsertedRows != 2 {
		t.Fatalf("InsertedRows = %d, want 2", report.InsertedRows)
	}
	if report.DeletedRows != 1 {
		t.Fatalf("DeletedRows = %d, want 1", report.DeletedRows)
	}

	assertStringRows(t, targetDB, `select cast(id as char), external_number, payload from entries order by id`, [][]string{{"1", "A-100", "alpha"}, {"2", "B-200", "beta"}})
}

func TestRunProfileIntegrationSkipsNonPrimaryAutoIncrementColumns(t *testing.T) {
	ctx := context.Background()
	sourceContainer := testkit.StartMySQLContainer(ctx, t)
	defer sourceContainer.Cleanup()
	targetContainer := testkit.StartMySQLContainer(ctx, t)
	defer targetContainer.Cleanup()

	sourceDSN := strings.ReplaceAll(sourceContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	targetDSN := strings.ReplaceAll(targetContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	sourceDB := openMySQLForTest(t, sourceDSN)
	defer sourceDB.Close()
	targetDB := openMySQLForTest(t, targetDSN)
	defer targetDB.Close()

	applyStatements(t, sourceDB,
		`create table category (id varchar(36) primary key, auto_increment int not null auto_increment, name varchar(255) not null, unique key category_auto_increment_uq (auto_increment))`,
		`insert into category (id, name) values ('cat-1', 'alpha source'), ('cat-2', 'beta source')`,
	)
	applyStatements(t, targetDB,
		`create table category (id varchar(36) primary key, auto_increment int not null auto_increment, name varchar(255) not null, unique key category_auto_increment_uq (auto_increment))`,
		`insert into category (id, name) values ('cat-1', 'alpha target'), ('cat-legacy', 'legacy target')`,
	)

	adapter := mysqladapter.NewAdapter()
	sourceSnapshot, err := adapter.DiscoverSourceSchema(ctx, sourceDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	targetSnapshot, err := adapter.DiscoverTargetSchema(ctx, targetDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverTargetSchema() error = %v", err)
	}

	candidate := model.DefaultProfile("integration")
	candidate.Source.Engine = model.EngineMySQL
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.Value = sourceDSN
	candidate.Target.Engine = model.EngineMySQL
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString.Value = targetDSN
	candidate.Selection.Tables = []string{"category"}

	preview, err := schema.PreviewSelection(schema.BuildDependencyGraph(sourceSnapshot), candidate.Selection.Tables, candidate.Selection.ExcludedTables)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	analysis := integrationAnalysis{
		preview: preview,
		discovery: schema.DiscoveryReport{
			Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMySQL, Snapshot: sourceSnapshot},
			Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMySQL, Snapshot: targetSnapshot},
		},
		drift: schema.CompareSnapshots(filterSnapshot(sourceSnapshot, preview.FinalTables), filterSnapshot(targetSnapshot, preview.FinalTables)),
	}

	service := NewService(func() map[string]string { return map[string]string{} })
	report, err := service.RunProfile(ctx, candidate, analysis, false, nil)
	if err != nil {
		t.Fatalf("RunProfile() error = %v", err)
	}
	if report.InsertedRows != 1 {
		t.Fatalf("InsertedRows = %d, want 1", report.InsertedRows)
	}
	if report.UpdatedRows != 1 {
		t.Fatalf("UpdatedRows = %d, want 1", report.UpdatedRows)
	}

	assertStringRows(t, targetDB, `select id, cast(auto_increment as char), name from category order by id`, [][]string{{"cat-1", "1", "alpha source"}, {"cat-2", "3", "beta source"}, {"cat-legacy", "2", "legacy target"}})
}

func TestRunProfileIntegrationOrdersSelfReferencingRows(t *testing.T) {
	ctx := context.Background()
	sourceContainer := testkit.StartMySQLContainer(ctx, t)
	defer sourceContainer.Cleanup()
	targetContainer := testkit.StartMySQLContainer(ctx, t)
	defer targetContainer.Cleanup()

	sourceDSN := strings.ReplaceAll(sourceContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	targetDSN := strings.ReplaceAll(targetContainer.DSN, "${MYSQL_PASSWORD}", "app-secret")
	sourceDB := openMySQLForTest(t, sourceDSN)
	defer sourceDB.Close()
	targetDB := openMySQLForTest(t, targetDSN)
	defer targetDB.Close()

	applyStatements(t, sourceDB,
		`create table category (id varchar(36) not null, version_id varchar(36) not null, parent_id varchar(36) null, parent_version_id varchar(36) null, name varchar(255) not null, primary key (id, version_id), constraint fk_category_parent foreign key (parent_id, parent_version_id) references category(id, version_id) on delete cascade on update cascade)`,
		`insert into category (id, version_id, parent_id, parent_version_id, name) values ('root', 'live', null, null, 'root source')`,
		`insert into category (id, version_id, parent_id, parent_version_id, name) values ('child', 'live', 'root', 'live', 'child source')`,
	)
	applyStatements(t, targetDB,
		`create table category (id varchar(36) not null, version_id varchar(36) not null, parent_id varchar(36) null, parent_version_id varchar(36) null, name varchar(255) not null, primary key (id, version_id), constraint fk_category_parent foreign key (parent_id, parent_version_id) references category(id, version_id) on delete cascade on update cascade)`,
	)

	adapter := mysqladapter.NewAdapter()
	sourceSnapshot, err := adapter.DiscoverSourceSchema(ctx, sourceDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverSourceSchema() error = %v", err)
	}
	targetSnapshot, err := adapter.DiscoverTargetSchema(ctx, targetDSN, model.EngineMySQL)
	if err != nil {
		t.Fatalf("DiscoverTargetSchema() error = %v", err)
	}

	candidate := model.DefaultProfile("integration")
	candidate.Source.Engine = model.EngineMySQL
	candidate.Source.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Source.Connection.ConnectionString.Value = sourceDSN
	candidate.Target.Engine = model.EngineMySQL
	candidate.Target.Connection.Mode = model.ConnectionModeConnectionString
	candidate.Target.Connection.ConnectionString.Value = targetDSN
	candidate.Selection.Tables = []string{"category"}

	preview, err := schema.PreviewSelection(schema.BuildDependencyGraph(sourceSnapshot), candidate.Selection.Tables, candidate.Selection.ExcludedTables)
	if err != nil {
		t.Fatalf("PreviewSelection() error = %v", err)
	}
	analysis := integrationAnalysis{
		preview: preview,
		discovery: schema.DiscoveryReport{
			Source: schema.EndpointDiscovery{Role: "source", Engine: model.EngineMySQL, Snapshot: sourceSnapshot},
			Target: schema.EndpointDiscovery{Role: "target", Engine: model.EngineMySQL, Snapshot: targetSnapshot},
		},
		drift: schema.CompareSnapshots(filterSnapshot(sourceSnapshot, preview.FinalTables), filterSnapshot(targetSnapshot, preview.FinalTables)),
	}

	service := NewService(func() map[string]string { return map[string]string{} })
	report, err := service.RunProfile(ctx, candidate, analysis, false, nil)
	if err != nil {
		t.Fatalf("RunProfile() error = %v", err)
	}
	if report.InsertedRows != 2 {
		t.Fatalf("InsertedRows = %d, want 2", report.InsertedRows)
	}

	assertStringRows(t, targetDB, `select id, version_id, coalesce(parent_id, ''), coalesce(parent_version_id, ''), name from category order by id`, [][]string{{"child", "live", "root", "live", "child source"}, {"root", "live", "", "", "root source"}})
}

func openMySQLForTest(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("Ping() error = %v", err)
	}
	return db
}

func applyStatements(t *testing.T, db *sql.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("Exec(%q) error = %v", statement, err)
		}
	}
}

func filterSnapshot(snapshot schema.Snapshot, tableIDs []schema.TableID) schema.Snapshot {
	selected := make(map[schema.TableID]struct{}, len(tableIDs))
	for _, tableID := range tableIDs {
		selected[tableID] = struct{}{}
	}
	filtered := make([]schema.Table, 0, len(tableIDs))
	for _, table := range snapshot.Tables {
		if _, ok := selected[table.ID]; ok {
			filtered = append(filtered, table)
		}
	}
	snapshot.Tables = filtered
	return schema.NormalizeSnapshot(snapshot)
}

func assertStringRows(t *testing.T, db *sql.DB, query string, want [][]string) {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("Query(%q) error = %v", query, err)
	}
	defer rows.Close()
	got := make([][]string, 0)
	for rows.Next() {
		values := make([]sql.NullString, len(want[0]))
		scanArgs := make([]any, len(values))
		for index := range values {
			scanArgs[index] = &values[index]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		row := make([]string, 0, len(values))
		for _, value := range values {
			row = append(row, value.String)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for rowIndex := range want {
		if strings.Join(got[rowIndex], "|") != strings.Join(want[rowIndex], "|") {
			t.Fatalf("row %d = %v, want %v", rowIndex, got[rowIndex], want[rowIndex])
		}
	}
}
