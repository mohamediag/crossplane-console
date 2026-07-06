// Package graph builds an in-memory DAG of Crossplane resources (XRs, MRs,
// composed plain-Kubernetes objects and packages) from watch-cache contents,
// and diffs successive snapshots to produce streamable deltas.
package graph

import (
	"fmt"
	"time"
)

// Health states, ordered by severity for rollup (see severity()).
const (
	StateHealthy     = "Healthy"
	StateProgressing = "Progressing"
	StateUnknown     = "Unknown"
	StateUnhealthy   = "Unhealthy"
	StateNA          = "NA" // no conditions at all (plain K8s objects)
)

// Node types.
const (
	NodeXR            = "xr"
	NodeMR            = "mr"
	NodeK8s           = "k8s"
	NodeProvider      = "provider"
	NodeFunction      = "function"
	NodeConfiguration = "configuration"
	NodeMissing       = "missing" // referenced by resourceRefs but not found
)

// Condition is one entry of status.conditions, trimmed to what the UI shows.
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// Health is the per-object health derived from Ready/Synced conditions.
type Health struct {
	State  string     `json:"state"`
	Ready  *Condition `json:"ready,omitempty"`
	Synced *Condition `json:"synced,omitempty"`
}

// Ref points at another object by coordinates (refs carry no UID).
type Ref struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

// Node is one vertex of the resource graph.
type Node struct {
	ID           string    `json:"id"`
	UID          string    `json:"uid,omitempty"`
	APIVersion   string    `json:"apiVersion"`
	Kind         string    `json:"kind"`
	Namespace    string    `json:"namespace,omitempty"`
	Name         string    `json:"name"`
	NodeType     string    `json:"nodeType"`
	Health       Health    `json:"health"`
	Aggregate    string    `json:"aggregate"`
	ExternalName string    `json:"externalName,omitempty"`
	Composition  *Ref      `json:"compositionRef,omitempty"`
	CompositionRevision string `json:"compositionRevision,omitempty"`
	CreatedAt    time.Time `json:"createdAt,omitempty"`
}

// Edge is a parent→child relation extracted from spec.crossplane.resourceRefs.
type Edge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`      // "composes"
	Validated bool   `json:"validated"` // child ownerReferences point back at parent
}

// Snapshot is a complete, immutable build of the graph.
type Snapshot struct {
	Revision    int64            `json:"revision"`
	GeneratedAt time.Time        `json:"generatedAt"`
	Nodes       map[string]*Node `json:"-"`
	Edges       map[string]*Edge `json:"-"`
}

// Delta is the difference between two snapshots, streamed over SSE.
type Delta struct {
	Revision     int64    `json:"revision"`
	Upserts      []*Node  `json:"upserts,omitempty"`
	Removals     []string `json:"removals,omitempty"`
	EdgesAdded   []*Edge  `json:"edgesAdded,omitempty"`
	EdgesRemoved []string `json:"edgesRemoved,omitempty"`
}

// Empty reports whether the delta carries no changes.
func (d *Delta) Empty() bool {
	return len(d.Upserts) == 0 && len(d.Removals) == 0 &&
		len(d.EdgesAdded) == 0 && len(d.EdgesRemoved) == 0
}

// NodeID builds the canonical coordinate-based node identifier. Refs in
// resourceRefs carry no UID, so identity must be coordinates, and missing
// children still get stable IDs.
func NodeID(apiVersion, kind, namespace, name string) string {
	return fmt.Sprintf("%s|%s|%s|%s", apiVersion, kind, namespace, name)
}

// EdgeID is stable for a given (from, to) pair.
func EdgeID(from, to string) string { return from + ">" + to }

func severity(state string) int {
	switch state {
	case StateUnhealthy:
		return 4
	case StateUnknown:
		return 3
	case StateProgressing:
		return 2
	case StateHealthy:
		return 1
	default: // NA
		return 0
	}
}

// worse returns the more severe of two states.
func worse(a, b string) string {
	if severity(b) > severity(a) {
		return b
	}
	return a
}
