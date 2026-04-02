package sync

import (
	"strings"
	"testing"

	"db-sync/internal/schema"
)

func TestSharedWritableColumnsSkipsNonPrimaryIdentityColumns(t *testing.T) {
	sourceTable := schema.Table{
		ID: schema.TableID{Name: "category"},
		Columns: []schema.Column{
			{Name: "id", Ordinal: 1, Writable: true},
			{Name: "auto_increment", Ordinal: 2, Writable: true, Identity: true},
			{Name: "name", Ordinal: 3, Writable: true},
		},
		PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
	}
	targetTable := sourceTable

	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		t.Fatalf("sharedWritableColumns() error = %v", err)
	}
	if got, want := testColumnNames(columns), []string{"id", "name"}; !sameColumnNames(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
}

func TestSharedWritableColumnsKeepsPrimaryKeyIdentityColumns(t *testing.T) {
	sourceTable := schema.Table{
		ID: schema.TableID{Name: "accounts"},
		Columns: []schema.Column{
			{Name: "id", Ordinal: 1, Writable: true, Identity: true},
			{Name: "email", Ordinal: 2, Writable: true},
		},
		PrimaryKey: schema.PrimaryKey{Columns: []string{"id"}},
	}
	targetTable := sourceTable

	columns, err := sharedWritableColumns(sourceTable, targetTable)
	if err != nil {
		t.Fatalf("sharedWritableColumns() error = %v", err)
	}
	if got, want := testColumnNames(columns), []string{"id", "email"}; !sameColumnNames(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
}

func testColumnNames(columns []schema.Column) []string {
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, column.Name)
	}
	return result
}

func sameColumnNames(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestDisplaySyncTable(t *testing.T) {
	if got := displaySyncTable(schema.TableID{Name: "customer"}, "explicit"); got != "customer [explicit]" {
		t.Fatalf("displaySyncTable() = %q, want customer [explicit]", got)
	}
	if got := displaySyncTable(schema.TableID{Schema: "public", Name: "users"}, "implicit"); strings.Contains(got, "public.") {
		t.Fatalf("displaySyncTable() = %q, want display name without schema prefix", got)
	}
}

func TestBuildDeleteBatchQueryMySQL(t *testing.T) {
	query, args := buildDeleteBatchQuery(mysqlDialect{}, schema.TableID{Name: "orders"}, []string{"id"}, [][]any{{int64(1)}, {int64(2)}, {int64(3)}})
	want := "delete from `orders` where (`id` = ?) or (`id` = ?) or (`id` = ?)"
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
	if len(args) != 3 || args[0] != int64(1) || args[1] != int64(2) || args[2] != int64(3) {
		t.Fatalf("args = %#v, want [1 2 3]", args)
	}
}

func TestBuildDeleteBatchQueryPostgresCompositeKey(t *testing.T) {
	query, args := buildDeleteBatchQuery(postgresDialect{}, schema.TableID{Schema: "public", Name: "category"}, []string{"id", "version_id"}, [][]any{{"a", "live"}, {"b", "draft"}})
	want := "delete from \"public\".\"category\" where (\"id\" = $1 and \"version_id\" = $2) or (\"id\" = $3 and \"version_id\" = $4)"
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
	if len(args) != 4 || args[0] != "a" || args[1] != "live" || args[2] != "b" || args[3] != "draft" {
		t.Fatalf("args = %#v, want composite key args", args)
	}
}

func TestBuildInsertBatchQueryMySQL(t *testing.T) {
	query, args := buildInsertBatchQuery(mysqlDialect{}, schema.TableID{Name: "tmp_keys"}, []schema.Column{{Name: "id"}, {Name: "version_id"}}, [][]any{{"a", "live"}, {"b", "draft"}})
	want := "insert into `tmp_keys` (`id`, `version_id`) values (?, ?), (?, ?)"
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
	if len(args) != 4 || args[0] != "a" || args[1] != "live" || args[2] != "b" || args[3] != "draft" {
		t.Fatalf("args = %#v, want insert args", args)
	}
}

func TestBuildMirrorDeleteQueries(t *testing.T) {
	countQuery := buildCountMirrorDeleteQuery(postgresDialect{}, schema.TableID{Schema: "public", Name: "orders"}, "tmp_orders", []string{"id"})
	wantCount := "select count(*) from \"public\".\"orders\" as target where not exists (select 1 from \"tmp_orders\" as source_keys where \"target\".\"id\" = \"source_keys\".\"id\")"
	if countQuery != wantCount {
		t.Fatalf("countQuery = %q, want %q", countQuery, wantCount)
	}
	deleteQuery := buildMirrorDeleteQuery(mysqlDialect{}, schema.TableID{Name: "orders"}, "tmp_orders", []string{"id"})
	wantDelete := "delete target from `orders` as target where not exists (select 1 from `tmp_orders` as source_keys where `target`.`id` = `source_keys`.`id`)"
	if deleteQuery != wantDelete {
		t.Fatalf("deleteQuery = %q, want %q", deleteQuery, wantDelete)
	}
}

func TestBuildCreateMirrorDeleteTempTableQuery(t *testing.T) {
	query := buildCreateMirrorDeleteTempTableQuery(mysqlDialect{}, schema.TableID{Name: "orders"}, "tmp_orders", []schema.Column{{Name: "id"}, {Name: "version_id"}})
	want := "create temporary table `tmp_orders` as select `id`, `version_id` from `orders` where 1 = 0"
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
}

func TestResolveConfiguredTableIDsRequiresExplicitSelection(t *testing.T) {
	available := []schema.TableID{{Name: "orders"}, {Name: "users"}}
	resolved, err := resolveConfiguredTableIDs(available, []string{"orders"}, "merge")
	if err != nil {
		t.Fatalf("resolveConfiguredTableIDs() error = %v", err)
	}
	if len(resolved) != 1 || resolved[0] != (schema.TableID{Name: "orders"}) {
		t.Fatalf("resolved = %#v, want orders", resolved)
	}
	if _, err := resolveConfiguredTableIDs(available, []string{"payments"}, "merge"); err == nil {
		t.Fatal("resolveConfiguredTableIDs() error = nil, want explicit-selection error")
	}
}

func TestBuildReplaceQueries(t *testing.T) {
	if got := buildCountMissingStageRowsQuery(mysqlDialect{}, schema.TableID{Name: "orders"}, "tmp_orders", []string{"id"}); got != "select count(*) from `tmp_orders` as source where not exists (select 1 from `orders` as target where `target`.`id` = `source`.`id`)" {
		t.Fatalf("buildCountMissingStageRowsQuery() = %q, want mysql missing-row count query", got)
	}
	if got := buildInsertMissingStageRowsQuery(postgresDialect{}, schema.TableID{Schema: "public", Name: "orders"}, "tmp_orders", []schema.Column{{Name: "id"}, {Name: "payload"}}, []string{"id"}); got != "insert into \"public\".\"orders\" (\"id\", \"payload\") select \"source\".\"id\", \"source\".\"payload\" from \"tmp_orders\" as source where not exists (select 1 from \"public\".\"orders\" as target where \"target\".\"id\" = \"source\".\"id\")" {
		t.Fatalf("buildInsertMissingStageRowsQuery() = %q, want postgres insert-missing query", got)
	}
	if got := buildCountChangedStageRowsQuery(postgresDialect{}, schema.TableID{Schema: "public", Name: "orders"}, "tmp_orders", []schema.Column{{Name: "payload"}}, []string{"id"}); got != "select count(*) from \"public\".\"orders\" as target join \"tmp_orders\" as source on \"target\".\"id\" = \"source\".\"id\" where \"target\".\"payload\" is distinct from \"source\".\"payload\"" {
		t.Fatalf("buildCountChangedStageRowsQuery() = %q, want postgres changed-row count query", got)
	}
	if got := buildUpdateChangedStageRowsQuery(mysqlDialect{}, schema.TableID{Name: "orders"}, "tmp_orders", []schema.Column{{Name: "payload"}, {Name: "status"}}, []string{"id"}); got != "update `orders` as target join `tmp_orders` as source on `target`.`id` = `source`.`id` set `target`.`payload` = `source`.`payload`, `target`.`status` = `source`.`status` where not (`target`.`payload` <=> `source`.`payload`) or not (`target`.`status` <=> `source`.`status`)" {
		t.Fatalf("buildUpdateChangedStageRowsQuery() = %q, want mysql update-changed query", got)
	}
}
