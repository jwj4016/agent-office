package domain

import "sort"

// Graph answers structural questions about a validated workflow.
type Graph struct {
	spec     *WorkflowSpec
	byID     map[string]*Node
	children map[string][]string
	order    []string
}

// NewGraph indexes a workflow. The workflow must have passed
// ValidateStructure (in particular it must be acyclic).
func NewGraph(w *WorkflowSpec) *Graph {
	g := &Graph{spec: w, byID: map[string]*Node{}, children: map[string][]string{}}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		g.byID[n.ID] = n
	}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		for _, d := range n.DependsOn {
			g.children[d] = append(g.children[d], n.ID)
		}
	}
	for _, c := range g.children {
		sort.Strings(c)
	}
	// Stable topological order: spec order among ready nodes.
	indeg := map[string]int{}
	for _, n := range w.Nodes {
		indeg[n.ID] = len(n.DependsOn)
	}
	done := map[string]bool{}
	for len(g.order) < len(w.Nodes) {
		progressed := false
		for _, n := range w.Nodes {
			if done[n.ID] || indeg[n.ID] > 0 {
				continue
			}
			done[n.ID] = true
			g.order = append(g.order, n.ID)
			for _, c := range g.children[n.ID] {
				indeg[c]--
			}
			progressed = true
		}
		if !progressed {
			break // cyclic input; callers must validate first
		}
	}
	return g
}

func (g *Graph) Node(id string) *Node        { return g.byID[id] }
func (g *Graph) Order() []string             { return g.order }
func (g *Graph) Children(id string) []string { return g.children[id] }

// Descendants returns every node downstream of any of ids, excluding ids.
func (g *Graph) Descendants(ids ...string) map[string]bool {
	out := map[string]bool{}
	stack := append([]string(nil), ids...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range g.children[id] {
			if !out[c] {
				out[c] = true
				stack = append(stack, c)
			}
		}
	}
	for _, id := range ids {
		delete(out, id)
	}
	return out
}

// BranchMembers returns the nodes of the branch that starts at target
// under a condition, up to but excluding its join node. An empty branch
// (target is the join) has no members.
func (g *Graph) BranchMembers(condition, target string) map[string]bool {
	c := g.byID[condition]
	if c == nil || c.Routing == nil || target == c.Routing.JoinStep {
		return map[string]bool{}
	}
	join := c.Routing.JoinStep
	set := map[string]bool{target: true}
	stack := []string{target}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, ch := range g.children[id] {
			if ch != join && !set[ch] {
				set[ch] = true
				stack = append(stack, ch)
			}
		}
	}
	return set
}

// BranchTargets lists a condition's distinct branch starts, including
// the default target.
func (g *Graph) BranchTargets(condition string) []string {
	c := g.byID[condition]
	if c == nil || c.Routing == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range append(branchTargets(c.Routing), c.Routing.DefaultTarget) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
