package api

import (
	"fmt"
	"strings"

	"github.com/mohamediag/crossplane-console/internal/discovery"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// PrinterValue is one evaluated additionalPrinterColumns cell.
type PrinterValue struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// EvaluatePrinterColumns renders a CRD's additionalPrinterColumns against one
// object — the same columns `kubectl get <type>` shows. Wide-only columns
// (priority > 0) are skipped; evaluation errors yield an empty value (CRD
// authors write JSONPaths against fields that may not exist yet).
func EvaluatePrinterColumns(cols []discovery.PrinterColumn, u *unstructured.Unstructured) []PrinterValue {
	if len(cols) == 0 || u == nil {
		return nil
	}
	out := make([]PrinterValue, 0, len(cols))
	for _, c := range cols {
		if c.Priority > 0 {
			continue
		}
		out = append(out, PrinterValue{
			Name:  c.Name,
			Type:  c.Type,
			Value: evalJSONPath(c.JSONPath, u.Object),
		})
	}
	return out
}

func evalJSONPath(path string, obj map[string]interface{}) string {
	jp := jsonpath.New("col")
	jp.AllowMissingKeys(true)
	// CRD printer columns use bare paths (".spec.x"); the jsonpath parser
	// wants template syntax ("{.spec.x}").
	if err := jp.Parse("{" + path + "}"); err != nil {
		return ""
	}
	results, err := jp.FindResults(obj)
	if err != nil {
		return ""
	}
	var parts []string
	for _, group := range results {
		for _, v := range group {
			if !v.IsValid() || !v.CanInterface() {
				continue
			}
			parts = append(parts, fmt.Sprintf("%v", v.Interface()))
		}
	}
	return strings.Join(parts, ",")
}
