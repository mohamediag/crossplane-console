# Demo: a Crossplane v2 XR tree on kind

Creates a namespaced XRD + Composition that composes two ConfigMaps via
provider-kubernetes `Object` MRs — no cloud credentials needed — plus one
sample XR, so the console has a real tree to draw.

```sh
kind create cluster --name crossplane-console-demo

# Crossplane v2
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system --create-namespace --wait

# provider-kubernetes + in-cluster ProviderConfig + demo XRD/Composition/XR
kubectl apply -f provider.yaml
kubectl wait provider.pkg.crossplane.io/provider-kubernetes \
  --for=condition=Healthy --timeout=120s
kubectl apply -f providerconfig.yaml
kubectl apply -f xrd.yaml
kubectl wait xrd/demoapps.demo.crossplane.io --for=condition=Established
kubectl apply -f composition.yaml
kubectl apply -f xr.yaml
```

Then run the console against the kind cluster (from `apps/crossplane-console/`):

```sh
go run ./cmd/console --kubeconfig ~/.kube/config
# in another shell:
cd web && npm install && npm run dev   # → http://localhost:5173
```

Or deploy it in-cluster: `kubectl apply -f ../manifests/` and
`kubectl -n crossplane-console port-forward svc/crossplane-console 8080:8080`.
