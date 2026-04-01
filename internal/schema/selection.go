package schema

import (
	"fmt"
	"sort"
	"strings"
)

type SelectionPreview struct {
	ExplicitIncludes []TableID
	RequiredTables   []TableID
	FinalTables      []TableID
}

func PreviewSelection(graph Graph, selected []string, excluded []string) (SelectionPreview, error) {
	includeIDs, err := resolveSelectionIDs(graph, selected)
	if err != nil {
		return SelectionPreview{}, err
	}
	excludedIDs, err := resolveSelectionIDs(graph, excluded)
	if err != nil {
		return SelectionPreview{}, err
	}
	if len(includeIDs) == 0 {
		includeIDs = graph.Tables()
	}
	excludedSet := tableSet(excludedIDs)
	includeIDs = filterTableIDs(includeIDs, excludedSet)
	closure := graph.Closure(includeIDs)
	includeSet := tableSet(includeIDs)

	required := make([]TableID, 0)
	finalSet := make(map[TableID]struct{}, len(closure))
	for _, id := range closure {
		if _, excluded := excludedSet[id]; excluded {
			continue
		}
		finalSet[id] = struct{}{}
		if _, explicit := includeSet[id]; !explicit {
			required = append(required, id)
		}
	}

	return SelectionPreview{
		ExplicitIncludes: includeIDs,
		RequiredTables:   sortTableIDs(required),
		FinalTables:      graph.OrderTables(finalSet),
	}, nil
}

func filterTableIDs(values []TableID, blocked map[TableID]struct{}) []TableID {
	filtered := make([]TableID, 0, len(values))
	for _, value := range values {
		if _, excluded := blocked[value]; excluded {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func SelectionStrings(ids []TableID) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return values
}

func ParseSelectionInput(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	})
	result := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		normalized := strings.TrimSpace(field)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func resolveSelectionIDs(graph Graph, values []string) ([]TableID, error) {
	resolved := make([]TableID, 0, len(values))
	seen := map[TableID]struct{}{}
	for _, value := range values {
		id, err := resolveSelectionID(graph, value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}
	return sortTableIDs(resolved), nil
}

func resolveSelectionID(graph Graph, value string) (TableID, error) {
	id := ParseTableID(value)
	if graph.HasTable(id) {
		return id, nil
	}
	if id.Schema != "" {
		return TableID{}, fmt.Errorf("unknown table selection %q", value)
	}
	matches := make([]TableID, 0)
	for _, candidate := range graph.Tables() {
		if candidate.Name == id.Name {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return TableID{}, fmt.Errorf("unknown table selection %q", value)
	case 1:
		return matches[0], nil
	default:
		return TableID{}, fmt.Errorf("ambiguous table selection %q; qualify one of: %s", value, strings.Join(SelectionStrings(matches), ", "))
	}
}

func tableSet(values []TableID) map[TableID]struct{} {
	set := make(map[TableID]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func sortTableIDs(values []TableID) []TableID {
	sorted := append([]TableID(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})
	return sorted
}
