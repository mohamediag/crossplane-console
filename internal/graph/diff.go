package graph

import "reflect"

// Diff computes the delta from prev to next. A nil prev yields a delta that
// upserts everything (callers normally send a full snapshot instead).
func Diff(prev, next *Snapshot) *Delta {
	d := &Delta{Revision: next.Revision}
	for id, n := range next.Nodes {
		if prev == nil {
			d.Upserts = append(d.Upserts, n)
			continue
		}
		if old, ok := prev.Nodes[id]; !ok || !reflect.DeepEqual(old, n) {
			d.Upserts = append(d.Upserts, n)
		}
	}
	for id, e := range next.Edges {
		if prev == nil {
			d.EdgesAdded = append(d.EdgesAdded, e)
			continue
		}
		if old, ok := prev.Edges[id]; !ok || !reflect.DeepEqual(old, e) {
			d.EdgesAdded = append(d.EdgesAdded, e)
		}
	}
	if prev != nil {
		for id := range prev.Nodes {
			if _, ok := next.Nodes[id]; !ok {
				d.Removals = append(d.Removals, id)
			}
		}
		for id := range prev.Edges {
			if _, ok := next.Edges[id]; !ok {
				d.EdgesRemoved = append(d.EdgesRemoved, id)
			}
		}
	}
	return d
}
