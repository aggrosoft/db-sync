package cli

import (
	"fmt"
	"io"
	"strings"

	"db-sync/internal/model"
	"db-sync/internal/schema"
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

func RenderSelectionPreview(writer io.Writer, candidate model.Profile, preview schema.SelectionPreview) error {
	blocked := "none"
	if len(preview.BlockedExclusions) > 0 {
		parts := make([]string, 0, len(preview.BlockedExclusions))
		for _, exclusion := range preview.BlockedExclusions {
			parts = append(parts, fmt.Sprintf("%s (required by %s)", exclusion.Table.String(), strings.Join(schema.SelectionStrings(exclusion.RequiredBy), ", ")))
		}
		blocked = strings.Join(parts, "; ")
	}
	lines := []string{
		fmt.Sprintf("Explicit selections: %s", selectionSummary(schema.SelectionStrings(preview.ExplicitIncludes), "all discovered tables")),
		fmt.Sprintf("Required additions: %s", selectionSummary(schema.SelectionStrings(preview.RequiredTables), "none")),
		fmt.Sprintf("Blocked exclusions: %s", blocked),
		fmt.Sprintf("Final table order: %s", selectionSummary(schema.SelectionStrings(preview.FinalTables), "none")),
		fmt.Sprintf("Sync: mode=%s mirror_delete=%t", candidate.Sync.Mode, candidate.Sync.MirrorDelete),
	}
	if len(preview.IgnoredExclusions) > 0 {
		lines = append(lines, fmt.Sprintf("Ignored exclusions: %s", strings.Join(preview.IgnoredExclusions, ", ")))
	}
	if preview.Blocked {
		lines = append(lines, "Selection status: blocked until the required exclusions are removed or the seed list changes")
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}
	return nil
}

func selectionSummary(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}
