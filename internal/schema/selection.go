package schema

import (
	"fmt"
	"sort"
	"strings"
)

type BlockedExclusion struct {
	Table      TableID
	RequiredBy []TableID
}

type SelectionPreview struct {
	ExplicitIncludes   []TableID
	ExplicitExclusions []TableID
	IgnoredExclusions  []string
	RequiredTables     []TableID
	BlockedExclusions  []BlockedExclusion
	FinalTables        []TableID
	Blocked            bool
}

func PreviewSelection(graph Graph, selected []string, excluded []string) (SelectionPreview, error) {
	includeIDs, err := resolveSelectionIDs(graph, selected)
	if err != nil {
		return SelectionPreview{}, err
	}
	excludeIDs, ignoredExclusions, err := resolveExclusionIDs(graph, excluded)
	if err != nil {
		return SelectionPreview{}, err
	}
	if len(includeIDs) == 0 {
		includeIDs = graph.Tables()
	}
	closure := graph.Closure(includeIDs)
	closureSet := tableSet(closure)
	excludeSet := tableSet(excludeIDs)
	includeSet := tableSet(includeIDs)

	required := make([]TableID, 0)
	blocked := make(map[TableID]map[TableID]struct{})
	finalSet := make(map[TableID]struct{}, len(closureSet))
	for _, id := range closure {
		if _, excluded := excludeSet[id]; excluded {
			if _, explicit := includeSet[id]; explicit {
				finalSet[id] = struct{}{}
			}
			continue
		}
		finalSet[id] = struct{}{}
		if _, explicit := includeSet[id]; !explicit {
			required = append(required, id)
		}
	}

	for tableID := range closureSet {
		for _, dependency := range graph.DependenciesFor(tableID) {
			if _, inClosure := closureSet[dependency.To]; !inClosure {
				continue
			}
			if _, excluded := excludeSet[dependency.To]; !excluded {
				continue
			}
			if _, ok := blocked[dependency.To]; !ok {
				blocked[dependency.To] = map[TableID]struct{}{}
			}
			blocked[dependency.To][tableID] = struct{}{}
		}
	}

	blockedExclusions := make([]BlockedExclusion, 0, len(blocked))
	for tableID, requiredBy := range blocked {
		blockedExclusions = append(blockedExclusions, BlockedExclusion{Table: tableID, RequiredBy: orderedTableSet(requiredBy)})
	}
	sort.Slice(blockedExclusions, func(i, j int) bool {
		return blockedExclusions[i].Table.String() < blockedExclusions[j].Table.String()
	})

	return SelectionPreview{
		ExplicitIncludes:   includeIDs,
		ExplicitExclusions: excludeIDs,
		IgnoredExclusions:  ignoredExclusions,
		RequiredTables:     sortTableIDs(required),
		BlockedExclusions:  blockedExclusions,
		FinalTables:        graph.OrderTables(finalSet),
		Blocked:            len(blockedExclusions) > 0,
	}, nil
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

func resolveExclusionIDs(graph Graph, values []string) ([]TableID, []string, error) {
	resolved := make([]TableID, 0, len(values))
	ignored := make([]string, 0)
	seen := map[TableID]struct{}{}
	ignoredSeen := map[string]struct{}{}
	for _, value := range values {
		id, err := resolveSelectionID(graph, value)
		if err != nil {
			if strings.HasPrefix(err.Error(), "unknown table selection ") {
				if _, ok := ignoredSeen[value]; !ok {
					ignoredSeen[value] = struct{}{}
					ignored = append(ignored, value)
				}
				continue
			}
			return nil, nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}
	sort.Strings(ignored)
	return sortTableIDs(resolved), ignored, nil
}

func tableSet(values []TableID) map[TableID]struct{} {
	set := make(map[TableID]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func orderedTableSet(values map[TableID]struct{}) []TableID {
	result := make([]TableID, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return sortTableIDs(result)
}

func sortTableIDs(values []TableID) []TableID {
	sorted := append([]TableID(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})
	return sorted
}
