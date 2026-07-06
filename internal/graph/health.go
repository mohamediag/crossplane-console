package graph

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// progressingWindow: objects younger than this with conditions but no Ready
// are considered Progressing rather than Unknown.
const progressingWindow = 5 * time.Minute

// ExtractConditions returns all status.conditions entries, tolerating any
// missing or oddly-typed fields (CRD statuses are not guaranteed shapes).
func ExtractConditions(u *unstructured.Unstructured) []Condition {
	raw, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found || err != nil {
		return nil
	}
	out := make([]Condition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, Condition{
			Type:               str(m["type"]),
			Status:             str(m["status"]),
			Reason:             str(m["reason"]),
			Message:            str(m["message"]),
			LastTransitionTime: str(m["lastTransitionTime"]),
		})
	}
	return out
}

// ComputeHealth derives the health state from Ready/Synced conditions:
//   - Ready=True                → Healthy
//   - Ready=False or Synced=False → Unhealthy
//   - conditions but no decisive Ready → Progressing while young, else Unknown
//   - no conditions at all      → NA
func ComputeHealth(u *unstructured.Unstructured, now time.Time) Health {
	conds := ExtractConditions(u)
	h := Health{State: StateNA}
	if len(conds) == 0 {
		return h
	}
	for i := range conds {
		switch conds[i].Type {
		case "Ready":
			c := conds[i]
			h.Ready = &c
		case "Synced":
			c := conds[i]
			h.Synced = &c
		}
	}
	switch {
	case h.Ready != nil && h.Ready.Status == "True":
		h.State = StateHealthy
	case h.Ready != nil && h.Ready.Status == "False":
		h.State = StateUnhealthy
	case h.Synced != nil && h.Synced.Status == "False":
		h.State = StateUnhealthy
	default:
		if now.Sub(u.GetCreationTimestamp().Time) < progressingWindow {
			h.State = StateProgressing
		} else {
			h.State = StateUnknown
		}
	}
	return h
}

// PackageHealth derives health for pkg.crossplane.io objects, which use
// Healthy/Installed conditions instead of Ready/Synced.
func PackageHealth(u *unstructured.Unstructured) Health {
	conds := ExtractConditions(u)
	h := Health{State: StateNA}
	var healthy, installed *Condition
	for i := range conds {
		switch conds[i].Type {
		case "Healthy":
			c := conds[i]
			healthy = &c
		case "Installed":
			c := conds[i]
			installed = &c
		}
	}
	// Reuse the Ready/Synced slots so the UI renders them uniformly.
	h.Ready, h.Synced = healthy, installed
	switch {
	case healthy != nil && healthy.Status == "True":
		h.State = StateHealthy
	case healthy != nil && healthy.Status == "False",
		installed != nil && installed.Status == "False":
		h.State = StateUnhealthy
	case len(conds) > 0:
		h.State = StateProgressing
	}
	return h
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}
