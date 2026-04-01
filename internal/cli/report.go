package cli

import (
	"fmt"
	"io"
	"strings"

	"db-sync/internal/schema"
	syncapp "db-sync/internal/sync"

	prettytable "github.com/jedib0t/go-pretty/v6/table"
	prettytext "github.com/jedib0t/go-pretty/v6/text"
)

func RenderDriftReport(writer io.Writer, preview schema.SelectionPreview, report schema.DriftReport) error {
	if len(report.Tables) == 0 {
		_, err := fmt.Fprintln(writer, "Schema drift: no tables selected for sync.")
		return err
	}
	if len(report.Warnings) == 0 && len(report.Blockers) == 0 {
		_, err := fmt.Fprintln(writer, "Drift details: no warnings or blockers.")
		return err
	}
	if _, err := fmt.Fprintf(writer, "Drift details: %d table(s) with warnings/blockers\n", countAffectedTables(report)); err != nil {
		return err
	}
	explicitSet := tableIDSet(preview.ExplicitIncludes)
	for _, table := range orderedProblemTables(report, explicitSet) {
		if len(table.Blockers) == 0 && len(table.Warnings) == 0 {
			continue
		}
		scope := "implicit"
		if _, ok := explicitSet[table.TableID]; ok {
			scope = "explicit"
		}
		if _, err := fmt.Fprintf(writer, "\n%s (%s)\n", displayTableID(table.TableID), scope); err != nil {
			return err
		}
		detailTable := newCLIPrettyTable(writer)
		detailTable.AppendHeader(prettytable.Row{"Level", "Code", "Column", "Detail"})
		detailTable.SetColumnConfigs([]prettytable.ColumnConfig{
			{Name: "Level", WidthMax: 8},
			{Name: "Code", WidthMax: 24, WidthMaxEnforcer: prettytext.WrapSoft},
			{Name: "Column", WidthMax: 20, WidthMaxEnforcer: prettytext.WrapSoft},
			{Name: "Detail", WidthMax: 68, WidthMaxEnforcer: prettytext.WrapSoft},
		})
		for _, reason := range table.Blockers {
			detailTable.AppendRow(prettytable.Row{severityLabel(writer, "blocker", true), reason.Code, emptyCell(reason.Column), sanitizeReasonDetail(reason)})
		}
		for _, reason := range table.Warnings {
			detailTable.AppendRow(prettytable.Row{severityLabel(writer, "warning", false), reason.Code, emptyCell(reason.Column), sanitizeReasonDetail(reason)})
		}
		if _, err := fmt.Fprintln(writer, detailTable.Render()); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func RenderSyncReport(writer io.Writer, report syncapp.Report) error {
	mode := "executed"
	if report.DryRun {
		mode = "dry-run"
	}
	if _, err := fmt.Fprintf(writer, "Sync %s: %d table(s), %d missing row(s), %d inserted row(s), %d updated row(s), %d deleted row(s)\n", mode, len(report.Tables), report.MissingRows, report.InsertedRows, report.UpdatedRows, report.DeletedRows); err != nil {
		return err
	}
	tableWriter := newCLIPrettyTable(writer)
	tableWriter.AppendHeader(prettytable.Row{"Scope", "Table", "Source", "Missing", "Inserted", "Updated", "Deleted"})
	tableWriter.SetColumnConfigs([]prettytable.ColumnConfig{
		{Name: "Scope", WidthMax: 10},
		{Name: "Table", WidthMax: 34, WidthMaxEnforcer: prettytext.WrapSoft},
		{Name: "Source", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
		{Name: "Missing", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
		{Name: "Inserted", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
		{Name: "Updated", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
		{Name: "Deleted", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
	})
	for _, table := range report.Tables {
		tableWriter.AppendRow(prettytable.Row{table.Scope, displayTableID(table.TableID), table.SourceRows, table.MissingRows, table.InsertedRows, table.UpdatedRows, table.DeletedRows})
	}
	if _, err := fmt.Fprintln(writer, tableWriter.Render()); err != nil {
		return err
	}
	if report.Summary != "" {
		_, err := fmt.Fprintf(writer, "\n%s\n", report.Summary)
		return err
	}
	return nil
}

func tableIDSet(values []schema.TableID) map[schema.TableID]struct{} {
	set := make(map[schema.TableID]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func countAffectedTables(report schema.DriftReport) int {
	count := 0
	for _, table := range report.Tables {
		if len(table.Warnings) > 0 || len(table.Blockers) > 0 {
			count++
		}
	}
	return count
}

func emptyCell(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func severityLabel(writer io.Writer, label string, blocker bool) string {
	if !isTTYWriter(writer) {
		return label
	}
	if blocker {
		return prettytext.Colors{prettytext.FgRed}.Sprint(label)
	}
	return prettytext.Colors{prettytext.FgYellow}.Sprint(label)
}

func sanitizeReasonDetail(reason schema.DriftReason) string {
	value := reason.Detail
	tableName := reason.TableID.String()
	displayName := displayTableID(reason.TableID)
	if tableName == "" || tableName == displayName {
		return value
	}
	value = strings.ReplaceAll(value, tableName+".", displayName+".")
	value = strings.ReplaceAll(value, tableName, displayName)
	return value
}

func orderedProblemTables(report schema.DriftReport, explicitSet map[schema.TableID]struct{}) []schema.TableDrift {
	explicit := make([]schema.TableDrift, 0, len(report.Tables))
	implicit := make([]schema.TableDrift, 0, len(report.Tables))
	for _, table := range report.Tables {
		if len(table.Blockers) == 0 && len(table.Warnings) == 0 {
			continue
		}
		if _, ok := explicitSet[table.TableID]; ok {
			explicit = append(explicit, table)
			continue
		}
		implicit = append(implicit, table)
	}
	return append(explicit, implicit...)
}
