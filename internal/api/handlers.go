package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/graph"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// graphResponse is the wire form of a snapshot (maps flattened to slices).
type graphResponse struct {
	Revision    int64         `json:"revision"`
	GeneratedAt string        `json:"generatedAt"`
	Nodes       []*graph.Node `json:"nodes"`
	Edges       []*graph.Edge `json:"edges"`
}

func snapshotResponse(s *graph.Snapshot, keep func(*graph.Node) bool) graphResponse {
	resp := graphResponse{
		Revision:    s.Revision,
		GeneratedAt: s.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Nodes:       []*graph.Node{},
		Edges:       []*graph.Edge{},
	}
	kept := map[string]bool{}
	for id, n := range s.Nodes {
		if keep == nil || keep(n) {
			kept[id] = true
			resp.Nodes = append(resp.Nodes, n)
		}
	}
	for _, e := range s.Edges {
		if kept[e.From] && kept[e.To] {
			resp.Edges = append(resp.Edges, e)
		}
	}
	sort.Slice(resp.Nodes, func(i, j int) bool { return resp.Nodes[i].ID < resp.Nodes[j].ID })
	sort.Slice(resp.Edges, func(i, j int) bool { return resp.Edges[i].ID < resp.Edges[j].ID })
	return resp
}

// GET /api/graph?namespace=&kind=&root=
// root: node ID → only that subtree. namespace/kind: keep subtrees whose XR
// root matches; packages and orphans are filtered directly by the predicate.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	snap := s.Engine.Snapshot()
	q := r.URL.Query()
	ns, kind, root := q.Get("namespace"), q.Get("kind"), q.Get("root")

	if root == "" && ns == "" && kind == "" {
		writeJSON(w, http.StatusOK, snapshotResponse(snap, nil))
		return
	}

	children := map[string][]string{}
	for _, e := range snap.Edges {
		children[e.From] = append(children[e.From], e.To)
	}
	keep := map[string]bool{}
	var mark func(id string)
	mark = func(id string) {
		if keep[id] {
			return
		}
		keep[id] = true
		for _, c := range children[id] {
			mark(c)
		}
	}

	if root != "" {
		if _, ok := snap.Nodes[root]; !ok {
			writeError(w, http.StatusNotFound, "unknown root node")
			return
		}
		mark(root)
	} else {
		hasParent := map[string]bool{}
		for _, e := range snap.Edges {
			hasParent[e.To] = true
		}
		for id, n := range snap.Nodes {
			if hasParent[id] {
				continue
			}
			if ns != "" && n.Namespace != ns {
				continue
			}
			if kind != "" && !strings.EqualFold(n.Kind, kind) {
				continue
			}
			mark(id)
		}
	}
	writeJSON(w, http.StatusOK, snapshotResponse(snap, func(n *graph.Node) bool { return keep[n.ID] }))
}

// GET /api/resources?type=xr|mr|pkg&kind=&namespace=&health=&limit=&offset=
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	snap := s.Engine.Snapshot()
	q := r.URL.Query()
	typ, kind, ns, health := q.Get("type"), q.Get("kind"), q.Get("namespace"), q.Get("health")
	limit := intParam(q.Get("limit"), 200)
	offset := intParam(q.Get("offset"), 0)

	var items []*graph.Node
	for _, n := range snap.Nodes {
		switch typ {
		case "xr":
			if n.NodeType != graph.NodeXR {
				continue
			}
		case "mr":
			if n.NodeType != graph.NodeMR {
				continue
			}
		case "pkg":
			if n.NodeType != graph.NodeProvider && n.NodeType != graph.NodeFunction &&
				n.NodeType != graph.NodeConfiguration {
				continue
			}
		case "":
			// no type filter
		default:
			writeError(w, http.StatusBadRequest, "type must be xr, mr or pkg")
			return
		}
		if kind != "" && !strings.EqualFold(n.Kind, kind) {
			continue
		}
		if ns != "" && n.Namespace != ns {
			continue
		}
		if health != "" && !strings.EqualFold(n.Health.State, health) {
			continue
		}
		items = append(items, n)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := items[offset:end]
	resp := map[string]any{"total": total, "items": page}

	// Kind-filtered listing of a watched type: include that CRD's printer
	// columns evaluated for each row (what kubectl get <kind> shows).
	if kind != "" && len(page) > 0 {
		if info, ok := s.Registry.InfoFor(page[0].APIVersion, page[0].Kind); ok && len(info.PrinterColumns) > 0 {
			values := make(map[string][]PrinterValue, len(page))
			for _, n := range page {
				if u := s.Manager.GetByCoordinates(n.APIVersion, n.Kind, n.Namespace, n.Name); u != nil {
					values[n.ID] = EvaluatePrinterColumns(info.PrinterColumns, u)
				}
			}
			cols := []map[string]string{}
			for _, c := range info.PrinterColumns {
				if c.Priority == 0 {
					cols = append(cols, map[string]string{"name": c.Name, "type": c.Type})
				}
			}
			resp["columns"] = cols
			resp["printerValues"] = values
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type detailResponse struct {
	Node       *graph.Node       `json:"node"`
	Conditions []graph.Condition `json:"conditions"`
	YAML       string            `json:"yaml"`
	Owners     []graph.Ref       `json:"owners"`
	Children   []*graph.Node     `json:"children"`
	// PrinterColumns are the CRD's additionalPrinterColumns evaluated for
	// this object (what kubectl get would show).
	PrinterColumns []PrinterValue `json:"printerColumns,omitempty"`
	// Pipeline is the function pipeline of the CompositionRevision that
	// rendered this XR (XR nodes only).
	Pipeline []PipelineStep `json:"pipeline,omitempty"`
	// ProviderConfigRef is the MR's spec.providerConfigRef (MR nodes only).
	ProviderConfigRef *graph.Ref `json:"providerConfigRef,omitempty"`
}

// GET /api/resource?id=<node id>
// Node IDs contain '/' and '|', so the ID travels as a query parameter
// instead of a path segment.
func (s *Server) handleResourceDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}
	parts := strings.SplitN(id, "|", 4)
	if len(parts) != 4 {
		writeError(w, http.StatusBadRequest, "malformed id, want apiVersion|kind|namespace|name")
		return
	}
	apiVersion, kind, ns, name := parts[0], parts[1], parts[2], parts[3]

	snap := s.Engine.Snapshot()
	node := snap.Nodes[id]

	u := s.Manager.GetByCoordinates(apiVersion, kind, ns, name)
	if u == nil && node != nil && node.NodeType == graph.NodeComposition {
		// Composition nodes are projected into their XR's namespace for the
		// graph, but the real object is cluster-scoped: retry without a ns.
		u = s.Manager.GetByCoordinates(apiVersion, kind, "", name)
		if u == nil {
			u = s.liveGet(r, apiVersion, kind, "", name)
		}
	}
	if u == nil {
		u = s.liveGet(r, apiVersion, kind, ns, name)
	}
	if node == nil && u == nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}

	resp := detailResponse{Node: node, Owners: []graph.Ref{}, Children: []*graph.Node{}, Conditions: []graph.Condition{}}
	if u != nil {
		if conds := graph.ExtractConditions(u); conds != nil {
			resp.Conditions = conds
		}
		if y, err := yaml.Marshal(u.Object); err == nil {
			resp.YAML = string(y)
		}
		for _, ref := range u.GetOwnerReferences() {
			resp.Owners = append(resp.Owners, graph.Ref{
				APIVersion: ref.APIVersion, Kind: ref.Kind,
				Namespace: u.GetNamespace(), Name: ref.Name,
			})
		}
		if info, ok := s.Registry.InfoFor(apiVersion, kind); ok {
			resp.PrinterColumns = EvaluatePrinterColumns(info.PrinterColumns, u)
		}
		if refName := nestedStr(u, "spec", "providerConfigRef", "name"); refName != "" {
			resp.ProviderConfigRef = &graph.Ref{
				Kind: nestedStr(u, "spec", "providerConfigRef", "kind"),
				Name: refName,
			}
		}
	}
	// XRs: surface the exact function pipeline that rendered this object,
	// from its pinned CompositionRevision (already in the extension cache).
	if node != nil && node.NodeType == graph.NodeXR && node.CompositionRevision != "" {
		if rev := s.Manager.GetByCoordinates(
			"apiextensions.crossplane.io/v1", "CompositionRevision", "", node.CompositionRevision); rev != nil {
			resp.Pipeline = pipelineSteps(rev)
		}
	}
	if node != nil {
		for _, e := range snap.Edges {
			if e.From == id {
				if child, ok := snap.Nodes[e.To]; ok {
					resp.Children = append(resp.Children, child)
				}
			}
		}
		sort.Slice(resp.Children, func(i, j int) bool { return resp.Children[i].ID < resp.Children[j].ID })
	}
	writeJSON(w, http.StatusOK, resp)
}

// liveGet fetches an unwatched object (plain-K8s composed child) on demand.
// Never watched: starting informers for arbitrary referenced kinds is
// unbounded, so the detail view pays one live GET instead.
func (s *Server) liveGet(r *http.Request, apiVersion, kind, ns, name string) *unstructured.Unstructured {
	if s.Mapper == nil || s.Dynamic == nil {
		return nil
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil
	}
	mapping, err := s.Mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
	if err != nil {
		return nil
	}
	u, err := s.Dynamic.Resource(mapping.Resource).Namespace(ns).
		Get(r.Context(), name, metaGetOptions)
	if err != nil {
		return nil
	}
	u.SetManagedFields(nil)
	return u
}

// GET /api/events?involvedUid=&namespace=&kind=&name=&type=&limit=
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.Events == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	q := r.URL.Query()
	items := s.Events.Query(
		q.Get("involvedUid"), q.Get("namespace"), q.Get("kind"), q.Get("name"),
		q.Get("type"), intParam(q.Get("limit"), 100))
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/meta
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	snap := s.Engine.Snapshot()
	namespaces := map[string]bool{}
	kinds := map[string]bool{}
	for _, n := range snap.Nodes {
		if n.Namespace != "" {
			namespaces[n.Namespace] = true
		}
		kinds[n.Kind] = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":            s.Version,
		"crossplaneDetected": s.Registry.CrossplaneDetected(),
		"revision":           snap.Revision,
		"types":              s.Manager.Status(),
		"namespaces":         sortedKeys(namespaces),
		"kinds":              sortedKeys(kinds),
		"typeCounts": map[string]int{
			"xr":  len(s.Manager.ListByCategory(discovery.CategoryComposite)),
			"mr":  len(s.Manager.ListByCategory(discovery.CategoryManaged)),
			"pkg": len(s.Manager.ListByCategory(discovery.CategoryPackage)),
		},
	})
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}
