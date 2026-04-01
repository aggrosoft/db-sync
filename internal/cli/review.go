package cli

import (
	"fmt"
	"io"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/schema"

	prettytable "github.com/jedib0t/go-pretty/v6/table"
	prettytext "github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/term"
)

func RenderDiscoveryReport(writer io.Writer, report schema.DiscoveryReport) error {
	for _, endpoint := range []schema.EndpointDiscovery{report.Source, report.Target} {
		if endpoint.Role == "" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s schema [%s]: %d tables discovered\n", strings.Title(endpoint.Role), endpoint.Engine, len(endpoint.Snapshot.Tables)); err != nil {
			return err
		}
		if endpoint.Summary != "" && endpoint.Summary != report.Summary {
			if _, err := fmt.Fprintln(writer, endpoint.Summary); err != nil {
				return err
			}
		}
		for _, remediation := range endpoint.Remediation {
			if _, err := fmt.Fprintf(writer, "  - %s\n", remediation); err != nil {
				return err
			}
		}
	}
	if len(report.MissingEnv) > 0 {
		if _, err := fmt.Fprintf(writer, "Missing required environment variables: %s\n", strings.Join(report.MissingEnv, ", ")); err != nil {
			return err
		}
	}
	if report.Summary != "" {
		_, err := fmt.Fprintln(writer, report.Summary)
		return err
	}
	return nil
}

func PreviewConfiguredSelection(candidate model.Profile, discovery schema.DiscoveryReport) (schema.SelectionPreview, error) {
	graph := schema.BuildDependencyGraph(discovery.Source.Snapshot)
	return schema.PreviewSelection(graph, candidate.Selection.Tables, candidate.Selection.ExcludedTables)
}

func RenderSelectionPreview(writer io.Writer, candidate model.Profile, preview schema.SelectionPreview, drift schema.DriftReport) error {
	if _, err := fmt.Fprintln(writer, "Selected tables"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%d total, %d explicit, %d implicit\n\n", len(preview.FinalTables), len(preview.ExplicitIncludes), len(preview.RequiredTables)); err != nil {
		return err
	}
	if err := renderSelectionImpactTable(writer, candidate, preview, drift); err != nil {
		return err
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func selectionSummary(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}

func renderSelectionImpactTable(writer io.Writer, candidate model.Profile, preview schema.SelectionPreview, drift schema.DriftReport) error {
	explicitSet := tableIDSet(preview.ExplicitIncludes)
	driftByTable := map[schema.TableID]schema.TableDrift{}
	for _, table := range drift.Tables {
		driftByTable[table.TableID] = table
	}
	tableWriter := newCLIPrettyTable(writer)
	tableWriter.AppendHeader(prettytable.Row{"#", "Table", "Scope", "Warnings", "Blockers"})
	tableWriter.SetColumnConfigs([]prettytable.ColumnConfig{
		{Name: "#", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 4},
		{Name: "Table", WidthMax: 34, WidthMaxEnforcer: prettytext.WrapSoft},
		{Name: "Scope", WidthMax: 10},
		{Name: "Warnings", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
		{Name: "Blockers", Align: prettytext.AlignRight, AlignHeader: prettytext.AlignRight, WidthMax: 8},
	})
	finalTables := orderedSelectionImpactTables(preview)
	for index, tableID := range finalTables {
		scope := "implicit"
		if _, ok := explicitSet[tableID]; ok {
			scope = "explicit"
		}
		tableDrift := driftByTable[tableID]
		tableWriter.AppendRow(prettytable.Row{index + 1, displayTableID(tableID), scope, severityCount(writer, len(tableDrift.Warnings), false), severityCount(writer, len(tableDrift.Blockers), true)})
	}
	_, err := fmt.Fprintln(writer, tableWriter.Render())
	return err
}

func orderedSelectionImpactTables(preview schema.SelectionPreview) []schema.TableID {
	explicitSet := tableIDSet(preview.ExplicitIncludes)
	values := preview.FinalTables
	explicit := make([]schema.TableID, 0, len(values))
	implicit := make([]schema.TableID, 0, len(values))
	for _, value := range values {
		if _, ok := explicitSet[value]; ok {
			explicit = append(explicit, value)
			continue
		}
		implicit = append(implicit, value)
	}
	return append(explicit, implicit...)
}
func displayTableID(value schema.TableID) string {
	if value.Name != "" {
		return value.Name
	}
	return value.String()
}

func newCLIPrettyTable(writer io.Writer) prettytable.Writer {
	tableWriter := prettytable.NewWriter()
	tableWriter.SetStyle(prettytable.StyleLight)
	return tableWriter
}

func isTTYWriter(writer io.Writer) bool {
	type fdWriter interface {
		Fd() uintptr
	}
	fd, ok := writer.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(fd.Fd()))
}

func severityCount(writer io.Writer, count int, blocker bool) string {
	value := fmt.Sprintf("%d", count)
	if count == 0 || !isTTYWriter(writer) {
		return value
	}
	if blocker {
		return prettytext.Colors{prettytext.FgRed}.Sprint(value)
	}
	return prettytext.Colors{prettytext.FgYellow}.Sprint(value)
}
