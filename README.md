# Crossplane Console

A **read-only observability console for Crossplane v2**. It connects to a
cluster, discovers Crossplane types at runtime, and renders a live, graph-based
view of every composite resource tree — XRs, their composed managed resources,
plain Kubernetes children and nested XRs — with health status, condition
details, raw YAML and Kubernetes events. No kubectl required, nothing is ever
mutated.

```
┌───────────────────────────────────────────────────────────────┐
│  Graph  Resources                        ● live               │
├───────────────────────────────────────────────────────────────┤
│                    ┌──────────────┐                           │
│                    │ XR  ●        │  ← aggregate health ring  │
│                    │ sample-service│                          │
│                    └──────┬───────┘                           │
│         ┌────────┬────────┼──────────┐                        │
│     ┌───┴───┐┌───┴───┐┌───┴───┐┌─────┴─────┐                  │
│     │MR ●   ││MR ●   ││MR ●   ││MR ●       │   click a node   │
│     │config ││deploy ││service││extsecret  │   → detail drawer│
│     └───────┘└───────┘└───────┘└───────────┘   (conditions,   │
│                                                YAML, events)  │
└───────────────────────────────────────────────────────────────┘
```

## How it works

- **Discovery, not hardcoding.** One informer on CRDs classifies every type by
  Crossplane's CRD categories (`managed`, `composite`) and `pkg.crossplane.io`
  kinds. Types appear/disappear with providers and XRDs; the console follows.
- **Watch cache.** Each discovered type gets its own dynamic informer
  (started once the CRD is Established, stopped on CRD delete). Objects are
  cached with `managedFields` stripped.
- **Graph.** Edges are read top-down from `spec.crossplane.resourceRefs`
  (with a legacy `spec.resourceRefs` fallback for v1 XRs) and validated
  bottom-up via `ownerReferences`. Health = Ready/Synced conditions
  (Healthy / Progressing / Unknown / Unhealthy / NA), rolled up worst-first
  from leaves to roots. Referenced-but-absent children render as "missing"
  placeholder nodes — that's what deletion-in-progress looks like.
- **Live.** A 500ms debounced full rebuild diffs against the previous snapshot
  and streams deltas over SSE (`/api/stream`). Revisions are consecutive; the
  frontend detects gaps and resyncs. Changes land in the browser well under 2s.
- **Plain K8s children** (anything composed that isn't an MR/XR) are rendered
  from the ref alone and fetched live on demand in the detail drawer — the
  console never starts informers for arbitrary kinds.

## API

| Endpoint | Description |
|---|---|
| `GET /api/graph?namespace=&kind=&root=` | full graph or filtered subtrees |
| `GET /api/resources?type=xr\|mr\|pkg&kind=&namespace=&health=&limit=&offset=` | paginated list |
| `GET /api/resource?id=<apiVersion\|kind\|namespace\|name>` | conditions, YAML, owners, children |
| `GET /api/events?involvedUid=&type=&limit=` | Kubernetes events, recency-sorted |
| `GET /api/meta` | detected types, sync state, filter values |
| `GET /api/stream` | SSE: `snapshot`, `delta`, `k8sevent` |
| `GET /healthz`, `GET /readyz` | probes |

## Local development

```sh
# backend (uses --kubeconfig, else in-cluster, else ~/.kube/config)
go run ./cmd/console --kubeconfig /path/to/kubeconfig --log-level debug

# frontend with hot reload (proxies /api to :8080)
cd web && npm install && npm run dev    # → http://localhost:5173

# tests
make test
```

The production binary embeds the built SPA (`go build -tags embedui` after
`npm run build`); without the tag it serves the API only.

## Try it on kind

See [`deploy/demo/`](deploy/demo/README.md) for a self-contained walkthrough:
kind cluster → Crossplane v2 → a demo XRD/Composition composing ConfigMap
`Object` MRs (no cloud credentials) → one XR → open the console.

## Deploy in-cluster

```sh
docker build -t <registry>/crossplane-console:<tag> .
kubectl apply -f deploy/manifests/   # namespace, SA, RBAC, deployment, service
```

**RBAC note:** the shipped ClusterRole grants cluster-wide read (`get/list/watch`
on `*/*`) because MR/XR API groups are created at runtime and cannot be
enumerated statically. This includes Secret reads — for hardened installs,
replace the wildcard with your actual MR/XR groups (commented example in
`deploy/manifests/rbac.yaml`).

In this repo, the console deploys as an ArgoCD platform add-on:
`gitops/apps/templates/crossplane-console.yaml` + `gitops/crossplane-console/`
(reachable at `https://crossplane-console.homelab.local`). Release = push the
image (`make docker-push`), then bump the pinned tag in
`gitops/crossplane-console/deployment.yaml`.

## Design notes / limits (MVP)

- Read-only by construction: no write verb in RBAC, no mutating API.
- Single cluster, no auth of its own (put it behind your gateway/SSO).
- Scales comfortably to hundreds of MRs (full graph rebuilds are ~ms;
  lists are virtualized; SSE is debounced).
- Degrades gracefully: "Crossplane not detected" banner, per-type sync
  status while providers install.
- Stretch ideas already accommodated by the data model: provider health
  dashboard (pkg conditions are cached), composition pipeline visualization
  (`compositionRevisionRef` is surfaced), search, drift indicators.
