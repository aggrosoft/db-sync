package schema

import "sort"

type Dependency struct {
	From              TableID
	To                TableID
	ForeignKey        string
	Columns           []string
	ReferencedColumns []string
}

type Graph struct {
	tables       []TableID
	dependencies map[TableID][]Dependency
}

func BuildDependencyGraph(snapshot Snapshot) Graph {
	snapshot = NormalizeSnapshot(snapshot)
	graph := Graph{
		tables:       make([]TableID, 0, len(snapshot.Tables)),
		dependencies: make(map[TableID][]Dependency, len(snapshot.Tables)),
	}
	for _, table := range snapshot.Tables {
		graph.tables = append(graph.tables, table.ID)
		for _, foreignKey := range table.ForeignKeys {
			graph.dependencies[table.ID] = append(graph.dependencies[table.ID], Dependency{
				From:              table.ID,
				To:                foreignKey.ReferencedTable,
				ForeignKey:        foreignKey.Name,
				Columns:           append([]string(nil), foreignKey.Columns...),
				ReferencedColumns: append([]string(nil), foreignKey.ReferencedColumns...),
			})
		}
		sort.Slice(graph.dependencies[table.ID], func(i, j int) bool {
			if graph.dependencies[table.ID][i].To == graph.dependencies[table.ID][j].To {
				return graph.dependencies[table.ID][i].ForeignKey < graph.dependencies[table.ID][j].ForeignKey
			}
			return graph.dependencies[table.ID][i].To.String() < graph.dependencies[table.ID][j].To.String()
		})
	}
	sort.Slice(graph.tables, func(i, j int) bool {
		return graph.tables[i].String() < graph.tables[j].String()
	})
	return graph
}

func (graph Graph) Tables() []TableID {
	return append([]TableID(nil), graph.tables...)
}

func (graph Graph) DependenciesFor(id TableID) []Dependency {
	return append([]Dependency(nil), graph.dependencies[id]...)
}

func (graph Graph) HasTable(id TableID) bool {
	for _, candidate := range graph.tables {
		if candidate == id {
			return true
		}
	}
	return false
}

func (graph Graph) Closure(seeds []TableID) []TableID {
	if len(graph.tables) == 0 {
		return []TableID{}
	}
	visited := make(map[TableID]struct{}, len(graph.tables))
	stack := append([]TableID(nil), seeds...)
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		for _, dependency := range graph.dependencies[current] {
			stack = append(stack, dependency.To)
		}
	}
	return graph.OrderTables(visited)
}

func (graph Graph) OrderTables(set map[TableID]struct{}) []TableID {
	if len(set) == 0 {
		return []TableID{}
	}
	reverse := make(map[TableID][]TableID, len(set))
	indegree := make(map[TableID]int, len(set))
	for id := range set {
		indegree[id] = 0
	}
	for child := range set {
		for _, dependency := range graph.dependencies[child] {
			if _, ok := set[dependency.To]; !ok {
				continue
			}
			reverse[dependency.To] = append(reverse[dependency.To], child)
			indegree[child]++
		}
	}
	ready := make([]TableID, 0, len(set))
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].String() < ready[j].String()
	})
	ordered := make([]TableID, 0, len(set))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		ordered = append(ordered, current)
		children := reverse[current]
		sort.Slice(children, func(i, j int) bool {
			return children[i].String() < children[j].String()
		})
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			return ready[i].String() < ready[j].String()
		})
	}
	if len(ordered) == len(set) {
		return ordered
	}
	remaining := make([]TableID, 0, len(set)-len(ordered))
	seen := make(map[TableID]struct{}, len(ordered))
	for _, id := range ordered {
		seen[id] = struct{}{}
	}
	for id := range set {
		if _, ok := seen[id]; ok {
			continue
		}
		remaining = append(remaining, id)
	}
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].String() < remaining[j].String()
	})
	return append(ordered, remaining...)
}
