package graph

import (
	"testing"
)

func snapWith(rev int64, nodes []*Node, edges []*Edge) *Snapshot {
	s := &Snapshot{Revision: rev, Nodes: map[string]*Node{}, Edges: map[string]*Edge{}}
	for _, n := range nodes {
		s.Nodes[n.ID] = n
	}
	for _, e := range edges {
		s.Edges[e.ID] = e
	}
	return s
}

func TestDiff(t *testing.T) {
	nodeA := &Node{ID: "a", Health: Health{State: StateHealthy}, Aggregate: StateHealthy}
	nodeB := &Node{ID: "b", Health: Health{State: StateHealthy}, Aggregate: StateHealthy}
	edgeAB := &Edge{ID: "a>b", From: "a", To: "b", Type: "composes"}

	t.Run("no changes yields empty delta", func(t *testing.T) {
		prev := snapWith(1, []*Node{nodeA, nodeB}, []*Edge{edgeAB})
		next := snapWith(2, []*Node{copyNode(nodeA), copyNode(nodeB)}, []*Edge{copyEdge(edgeAB)})
		d := Diff(prev, next)
		if !d.Empty() {
			t.Fatalf("delta not empty: %+v", d)
		}
	})

	t.Run("health-only change upserts the node", func(t *testing.T) {
		prev := snapWith(1, []*Node{nodeA}, nil)
		changed := copyNode(nodeA)
		changed.Health.State = StateUnhealthy
		next := snapWith(2, []*Node{changed}, nil)
		d := Diff(prev, next)
		if len(d.Upserts) != 1 || d.Upserts[0].ID != "a" {
			t.Fatalf("upserts = %+v, want node a", d.Upserts)
		}
	})

	t.Run("removals and edge removals", func(t *testing.T) {
		prev := snapWith(1, []*Node{nodeA, nodeB}, []*Edge{edgeAB})
		next := snapWith(2, []*Node{copyNode(nodeA)}, nil)
		d := Diff(prev, next)
		if len(d.Removals) != 1 || d.Removals[0] != "b" {
			t.Fatalf("removals = %v, want [b]", d.Removals)
		}
		if len(d.EdgesRemoved) != 1 || d.EdgesRemoved[0] != "a>b" {
			t.Fatalf("edgesRemoved = %v, want [a>b]", d.EdgesRemoved)
		}
	})

	t.Run("nil prev upserts everything", func(t *testing.T) {
		next := snapWith(1, []*Node{nodeA, nodeB}, []*Edge{edgeAB})
		d := Diff(nil, next)
		if len(d.Upserts) != 2 || len(d.EdgesAdded) != 1 {
			t.Fatalf("delta = %+v, want full upsert", d)
		}
	})

	t.Run("edge validation flip re-adds the edge", func(t *testing.T) {
		prev := snapWith(1, []*Node{nodeA, nodeB}, []*Edge{edgeAB})
		validated := copyEdge(edgeAB)
		validated.Validated = true
		next := snapWith(2, []*Node{copyNode(nodeA), copyNode(nodeB)}, []*Edge{validated})
		d := Diff(prev, next)
		if len(d.EdgesAdded) != 1 {
			t.Fatalf("edgesAdded = %+v, want the re-validated edge", d.EdgesAdded)
		}
	})
}

func copyNode(n *Node) *Node { c := *n; return &c }
func copyEdge(e *Edge) *Edge { c := *e; return &c }
