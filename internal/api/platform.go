package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/graph"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PlatformPackage is one pkg.crossplane.io Provider/Function/Configuration.
type PlatformPackage struct {
	Name            string       `json:"name"`
	Kind            string       `json:"kind"`
	Package         string       `json:"package,omitempty"`
	CurrentRevision string       `json:"currentRevision,omitempty"`
	Health          graph.Health `json:"health"`
}

// PipelineStep is one entry of a Composition's function pipeline.
type PipelineStep struct {
	Step     string `json:"step"`
	Function string `json:"function"`
}

// PlatformComposition summarizes a Composition and its revisions.
type PlatformComposition struct {
	Name               string         `json:"name"`
	CompositeAPI       string         `json:"compositeApiVersion,omitempty"`
	CompositeKind      string         `json:"compositeKind,omitempty"`
	Mode               string         `json:"mode,omitempty"`
	Pipeline           []PipelineStep `json:"pipeline,omitempty"`
	RevisionCount      int            `json:"revisionCount"`
	LatestRevision     int64          `json:"latestRevision,omitempty"`
	LatestRevisionName string         `json:"latestRevisionName,omitempty"`
}

// PlatformXRD summarizes a CompositeResourceDefinition.
type PlatformXRD struct {
	Name         string   `json:"name"`
	Group        string   `json:"group"`
	Kind         string   `json:"kind"`
	Scope        string   `json:"scope"`
	Versions     []string `json:"versions,omitempty"`
	Established  bool     `json:"established"`
	Compositions []string `json:"compositions,omitempty"`
}

// PlatformProviderConfig summarizes a (Cluster)ProviderConfig.
type PlatformProviderConfig struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Group             string `json:"group"`
	Namespace         string `json:"namespace,omitempty"`
	CredentialsSource string `json:"credentialsSource,omitempty"`
	UsedBy            int64  `json:"usedBy"`
}

// PlatformOperation summarizes an ops.crossplane.io Operation kind.
type PlatformOperation struct {
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	Namespace string       `json:"namespace,omitempty"`
	Schedule  string       `json:"schedule,omitempty"`
	Health    graph.Health `json:"health"`
}

// PlatformSummary is the response of GET /api/platform.
type PlatformSummary struct {
	Providers       []PlatformPackage        `json:"providers"`
	Functions       []PlatformPackage        `json:"functions"`
	Configurations  []PlatformPackage        `json:"configurations"`
	XRDs            []PlatformXRD            `json:"xrds"`
	Compositions    []PlatformComposition    `json:"compositions"`
	ProviderConfigs []PlatformProviderConfig `json:"providerConfigs"`
	Operations      []PlatformOperation      `json:"operations"`
}

// PlatformInput carries the cached object lists BuildPlatform reads.
type PlatformInput struct {
	Packages        []*unstructured.Unstructured
	Extensions      []*unstructured.Unstructured // XRDs, Compositions, CompositionRevisions
	ProviderConfigs []*unstructured.Unstructured
	Operations      []*unstructured.Unstructured
	MRs             []*unstructured.Unstructured // for providerConfig usage fallback counting
}

// compositionNameLabel is set by Crossplane on every CompositionRevision.
const compositionNameLabel = "crossplane.io/composition-name"

// BuildPlatform assembles the platform-layer summary from cached objects.
// Pure function over PlatformInput lists.
func BuildPlatform(in PlatformInput) PlatformSummary {
	packages, extensions := in.Packages, in.Extensions
	s := PlatformSummary{
		Providers:       []PlatformPackage{},
		Functions:       []PlatformPackage{},
		Configurations:  []PlatformPackage{},
		XRDs:            []PlatformXRD{},
		Compositions:    []PlatformComposition{},
		ProviderConfigs: []PlatformProviderConfig{},
		Operations:      []PlatformOperation{},
	}

	for _, u := range packages {
		p := PlatformPackage{
			Name:            u.GetName(),
			Kind:            u.GetKind(),
			Package:         nestedStr(u, "spec", "package"),
			CurrentRevision: nestedStr(u, "status", "currentRevision"),
			Health:          graph.PackageHealth(u),
		}
		switch u.GetKind() {
		case "Provider":
			s.Providers = append(s.Providers, p)
		case "Function":
			s.Functions = append(s.Functions, p)
		case "Configuration":
			s.Configurations = append(s.Configurations, p)
		}
	}

	// Split extension objects by kind.
	var xrds, comps, revs []*unstructured.Unstructured
	for _, u := range extensions {
		switch u.GetKind() {
		case "CompositeResourceDefinition":
			xrds = append(xrds, u)
		case "Composition":
			comps = append(comps, u)
		case "CompositionRevision":
			revs = append(revs, u)
		}
	}

	// Revisions grouped by owning composition name (label set by Crossplane).
	revCount := map[string]int{}
	latestRev := map[string]int64{}
	latestRevName := map[string]string{}
	for _, u := range revs {
		comp := u.GetLabels()[compositionNameLabel]
		if comp == "" {
			continue
		}
		revCount[comp]++
		if n, ok := nestedNumber(u, "spec", "revision"); ok && n >= latestRev[comp] {
			latestRev[comp] = n
			latestRevName[comp] = u.GetName()
		}
	}

	// Compositions, indexed by the composite type they serve for XRD matching.
	compsByType := map[string][]string{} // "group|Kind" -> composition names
	for _, u := range comps {
		api := nestedStr(u, "spec", "compositeTypeRef", "apiVersion")
		kind := nestedStr(u, "spec", "compositeTypeRef", "kind")
		c := PlatformComposition{
			Name:               u.GetName(),
			CompositeAPI:       api,
			CompositeKind:      kind,
			Mode:               nestedStr(u, "spec", "mode"),
			Pipeline:           pipelineSteps(u),
			RevisionCount:      revCount[u.GetName()],
			LatestRevision:     latestRev[u.GetName()],
			LatestRevisionName: latestRevName[u.GetName()],
		}
		s.Compositions = append(s.Compositions, c)
		key := groupOfAPIVersion(api) + "|" + kind
		compsByType[key] = append(compsByType[key], u.GetName())
	}

	for _, u := range xrds {
		group := nestedStr(u, "spec", "group")
		kind := nestedStr(u, "spec", "names", "kind")
		scope := nestedStr(u, "spec", "scope")
		if scope == "" {
			scope = "Namespaced" // v2 default
		}
		x := PlatformXRD{
			Name:         u.GetName(),
			Group:        group,
			Kind:         kind,
			Scope:        scope,
			Versions:     servedVersions(u),
			Established:  xrdEstablished(u),
			Compositions: compsByType[group+"|"+kind],
		}
		sort.Strings(x.Compositions)
		s.XRDs = append(s.XRDs, x)
	}

	for _, u := range in.ProviderConfigs {
		pc := PlatformProviderConfig{
			Name:              u.GetName(),
			Kind:              u.GetKind(),
			Group:             groupOfAPIVersion(u.GetAPIVersion()),
			Namespace:         u.GetNamespace(),
			CredentialsSource: nestedStr(u, "spec", "credentials", "source"),
		}
		if users, ok := nestedNumber(u, "status", "users"); ok {
			pc.UsedBy = users
		} else {
			pc.UsedBy = countConfigRefs(in.MRs, pc.Group, pc.Name)
		}
		s.ProviderConfigs = append(s.ProviderConfigs, pc)
	}

	for _, u := range in.Operations {
		s.Operations = append(s.Operations, PlatformOperation{
			Name:      u.GetName(),
			Kind:      u.GetKind(),
			Namespace: u.GetNamespace(),
			Schedule:  nestedStr(u, "spec", "schedule"),
			Health:    graph.ComputeHealth(u, time.Now()),
		})
	}

	sortPackages := func(list []PlatformPackage) {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	sortPackages(s.Providers)
	sortPackages(s.Functions)
	sortPackages(s.Configurations)
	sort.Slice(s.XRDs, func(i, j int) bool { return s.XRDs[i].Name < s.XRDs[j].Name })
	sort.Slice(s.Compositions, func(i, j int) bool { return s.Compositions[i].Name < s.Compositions[j].Name })
	sort.Slice(s.ProviderConfigs, func(i, j int) bool { return s.ProviderConfigs[i].Name < s.ProviderConfigs[j].Name })
	sort.Slice(s.Operations, func(i, j int) bool { return s.Operations[i].Name < s.Operations[j].Name })
	return s
}

// countConfigRefs counts MRs referencing a ProviderConfig by name, scoped to
// the config's provider family: an MR in s3.aws.m.upbound.io references
// configs in aws.m.upbound.io, never same-named configs of other providers.
func countConfigRefs(mrs []*unstructured.Unstructured, pcGroup, pcName string) int64 {
	var n int64
	for _, u := range mrs {
		if nestedStr(u, "spec", "providerConfigRef", "name") != pcName {
			continue
		}
		mrGroup := groupOfAPIVersion(u.GetAPIVersion())
		if mrGroup == pcGroup || strings.HasSuffix(mrGroup, "."+pcGroup) {
			n++
		}
	}
	return n
}

// GET /api/platform
func (s *Server) handlePlatform(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, BuildPlatform(PlatformInput{
		Packages:        s.Manager.ListByCategory(discovery.CategoryPackage),
		Extensions:      s.Manager.ListByCategory(discovery.CategoryExtension),
		ProviderConfigs: s.Manager.ListByCategory(discovery.CategoryProviderConfig),
		Operations:      s.Manager.ListByCategory(discovery.CategoryOperation),
		MRs:             s.Manager.ListByCategory(discovery.CategoryManaged),
	}))
}

func pipelineSteps(u *unstructured.Unstructured) []PipelineStep {
	raw, found, _ := unstructured.NestedSlice(u.Object, "spec", "pipeline")
	if !found {
		return nil
	}
	out := make([]PipelineStep, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		step := PipelineStep{}
		step.Step, _ = m["step"].(string)
		if ref, ok := m["functionRef"].(map[string]interface{}); ok {
			step.Function, _ = ref["name"].(string)
		}
		out = append(out, step)
	}
	return out
}

func servedVersions(u *unstructured.Unstructured) []string {
	raw, found, _ := unstructured.NestedSlice(u.Object, "spec", "versions")
	if !found {
		return nil
	}
	var out []string
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if served, _ := m["served"].(bool); !served {
			continue
		}
		if name, _ := m["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func xrdEstablished(u *unstructured.Unstructured) bool {
	conds := graph.ExtractConditions(u)
	for _, c := range conds {
		if c.Type == "Established" && c.Status == "True" {
			return true
		}
	}
	return false
}

func nestedStr(u *unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(u.Object, fields...)
	return v
}

// nestedNumber tolerates both int64 (API server / informer objects) and
// float64 (JSON/YAML-decoded fixtures) numeric representations.
func nestedNumber(u *unstructured.Unstructured, fields ...string) (int64, bool) {
	v, found, _ := unstructured.NestedFieldNoCopy(u.Object, fields...)
	if !found {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func groupOfAPIVersion(apiVersion string) string {
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		return apiVersion[:i]
	}
	return ""
}
