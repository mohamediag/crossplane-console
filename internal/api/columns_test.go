package api

import (
	"testing"

	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/testutil"
)

// TestEvaluatePrinterColumnsLiveFixtures runs the live App CRD's printer
// columns against the live sample-service XR: kubectl shows
// SYNCED=True READY=False COMPOSITION=app-kubernetes for this object.
func TestEvaluatePrinterColumnsLiveFixtures(t *testing.T) {
	crd := testutil.Load(t, "crd-app-composite.yaml")
	info, ok := discovery.Classify(crd)
	if !ok {
		t.Fatal("live XR CRD did not classify")
	}
	xr := testutil.Load(t, "xr-sample-service.yaml")

	vals := EvaluatePrinterColumns(info.PrinterColumns, xr)
	byName := map[string]string{}
	for _, v := range vals {
		byName[v.Name] = v.Value
	}
	if byName["SYNCED"] != "True" {
		t.Errorf("SYNCED = %q, want True", byName["SYNCED"])
	}
	if byName["READY"] != "False" {
		t.Errorf("READY = %q, want False", byName["READY"])
	}
	if byName["COMPOSITION"] != "app-kubernetes" {
		t.Errorf("COMPOSITION = %q, want app-kubernetes", byName["COMPOSITION"])
	}
	// COMPOSITIONREVISION has priority 1 → must be skipped.
	if _, present := byName["COMPOSITIONREVISION"]; present {
		t.Error("priority>0 column should be skipped")
	}
	// AGE is a date column: value is the raw creationTimestamp for the
	// client to render as age.
	if byName["AGE"] == "" {
		t.Error("AGE column empty, want creationTimestamp")
	}
}

func TestEvaluatePrinterColumnsEdgeCases(t *testing.T) {
	obj := testutil.Obj("g/v1", "K", "ns", "x")
	cols := []discovery.PrinterColumn{
		{Name: "MISSING", Type: "string", JSONPath: ".status.nope.deeper"},
		{Name: "BADPATH", Type: "string", JSONPath: ".status.conditions[?(@"},
		{Name: "NAME", Type: "string", JSONPath: ".metadata.name"},
	}
	vals := EvaluatePrinterColumns(cols, obj)
	byName := map[string]string{}
	for _, v := range vals {
		byName[v.Name] = v.Value
	}
	if byName["MISSING"] != "" || byName["BADPATH"] != "" {
		t.Errorf("missing/broken paths must yield empty, got %v", byName)
	}
	if byName["NAME"] != "x" {
		t.Errorf("NAME = %q, want x", byName["NAME"])
	}
	if EvaluatePrinterColumns(nil, obj) != nil {
		t.Error("nil columns must yield nil")
	}
}
