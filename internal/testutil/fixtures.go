// Package testutil provides fixture loading and unstructured builders for
// graph and discovery tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// Load reads a YAML fixture from the module-level testdata directory.
func Load(t *testing.T, name string) *unstructured.Unstructured {
	t.Helper()
	// Tests run with the package dir as CWD; testdata sits at the module root.
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing fixture %s: %v", name, err)
	}
	return &unstructured.Unstructured{Object: m}
}

// Obj builds a minimal unstructured object; mutate the result for specifics.
func Obj(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name": name,
			"uid":  "uid-" + name,
		},
	}}
	if namespace != "" {
		u.SetNamespace(namespace)
	}
	return u
}

// WithConditions sets status.conditions from (type, status) pairs.
func WithConditions(u *unstructured.Unstructured, pairs ...[2]string) *unstructured.Unstructured {
	conds := make([]interface{}, 0, len(pairs))
	for _, p := range pairs {
		conds = append(conds, map[string]interface{}{
			"type": p[0], "status": p[1], "reason": "Test",
		})
	}
	_ = unstructured.SetNestedSlice(u.Object, conds, "status", "conditions")
	return u
}

// WithResourceRefs sets spec.crossplane.resourceRefs from ref maps.
func WithResourceRefs(u *unstructured.Unstructured, refs ...map[string]interface{}) *unstructured.Unstructured {
	list := make([]interface{}, 0, len(refs))
	for _, r := range refs {
		list = append(list, r)
	}
	_ = unstructured.SetNestedSlice(u.Object, list, "spec", "crossplane", "resourceRefs")
	return u
}

// WithOwner appends an ownerReference with the given UID.
func WithOwner(u *unstructured.Unstructured, apiVersion, kind, name, uid string) *unstructured.Unstructured {
	owners, _, _ := unstructured.NestedSlice(u.Object, "metadata", "ownerReferences")
	owners = append(owners, map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind, "name": name, "uid": uid,
		"controller": true,
	})
	_ = unstructured.SetNestedSlice(u.Object, owners, "metadata", "ownerReferences")
	return u
}
