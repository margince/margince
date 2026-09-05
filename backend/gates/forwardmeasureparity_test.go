// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every forward measure must be spelled the same on all three sides.
//
// A landing is built from ONE of three readings, and the choice is stored: the
// installation setting says which, and ProjectLanding is what obeys it. So the
// value travels from a settings PATCH, into a stored column, back out through a
// forecast read — and the three enums that carry it were authored separately.
//
// The failure that shape produces is delayed and total: a measure the settings
// screen admits and ProjectLanding refuses saves cleanly, and then every
// forecast read for that installation errors. The setting is applied on read,
// so nothing fails at write time to warn anybody.
//
// All sides are DERIVED — the wire's from the YAML, the server's from
// forecasting.ForwardMeasures.

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// Where the contract spells a forward measure. Held honest by
// TestTheMeasureSiteListCoversEveryForwardMeasureEnum, which walks the document
// rather than trusting this list.
var forwardMeasureSites = []struct {
	what string
	path []string
}{
	{"the landing a read returns", []string{"components", "schemas", "ForecastLanding"}},
	{"the stored installation setting", []string{"components", "schemas", "InstallationSettings"}},
	{"the settings patch", []string{"components", "schemas", "UpdateInstallationSettingsRequest"}},
}

// TestEveryForwardMeasureIsOfferedOnTheWire is the gate forecasting.ForwardMeasures names.
//
// Both directions fail. A measure Go can build and the contract does not offer
// is unreachable; a measure the contract offers and Go cannot build is the
// save-then-error case above.
func TestEveryForwardMeasureIsOfferedOnTheWire(t *testing.T) {
	t.Parallel()
	inGo := make([]string, 0, len(forecasting.ForwardMeasures()))
	for _, measure := range forecasting.ForwardMeasures() {
		inGo = append(inGo, string(measure))
	}

	doc := loadContractDocument(t)
	for _, site := range forwardMeasureSites {
		onWire := forwardMeasureEnumAt(t, doc, site.path)
		for _, measure := range inGo {
			if !slices.Contains(onWire, measure) {
				t.Errorf("forecasting can build a landing from %q and %s does not offer it — "+
					"a measure no caller can name is a measure nobody uses", measure, site.what)
			}
		}
		for _, measure := range onWire {
			if !slices.Contains(inGo, measure) {
				t.Errorf("%s offers %q and forecasting.ProjectLanding refuses it — "+
					"an installation that saves this setting errors on every forecast read, "+
					"because the measure is applied when the forecast is READ and nothing "+
					"fails at write time to warn them", site.what, measure)
			}
		}
	}
}

// TestTheMeasureSiteListCoversEveryForwardMeasureEnum keeps the list above from
// becoming a second copy of the contract.
//
// The census fails SHORT if it is written as a list somebody maintains: a
// fourth site added later inherits no check, and the defect it can carry is
// exactly the one this file exists for. So the document is walked instead, and
// any enum carrying a measure that this gate does not name is an error.
func TestTheMeasureSiteListCoversEveryForwardMeasureEnum(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	found := map[string]bool{}
	walkForwardMeasureEnums(doc, nil, found)

	if len(found) == 0 {
		t.Fatal("no forward-measure enum found anywhere in the contract — this gate is " +
			"walking the wrong document and would report PASS over a contract with no " +
			"measures at all")
	}
	named := map[string]bool{}
	for _, site := range forwardMeasureSites {
		named[strings.Join(site.path, "/")] = true
	}
	for where := range found {
		if !named[where] {
			t.Errorf("the contract constrains a forward measure at %s and this gate does not "+
				"check it — a site it does not name can offer a measure the server refuses",
				where)
		}
	}
}

// walkForwardMeasureEnums finds every enum in the document that carries a
// forward measure, by the one value that is unambiguously ours.
//
// `weighted` and `manager_call` are words other schemas could reasonably use;
// `commit_evidence` names this specific reading and nothing else in the
// contract spells it.
func walkForwardMeasureEnums(node any, at []string, found map[string]bool) {
	switch shape := node.(type) {
	case map[string]any:
		if slices.Contains(enumOf(shape), "commit_evidence") {
			found[strings.Join(measureSitePath(at), "/")] = true
			return
		}
		for key, child := range shape {
			walkForwardMeasureEnums(child, append(append([]string{}, at...), key), found)
		}
	case []any:
		for _, child := range shape {
			walkForwardMeasureEnums(child, at, found)
		}
	}
}

// measureSitePath trims the walk's path back to the schema, not the property
// under it, so a site is named the way forwardMeasureSites spells it.
func measureSitePath(at []string) []string {
	for i, step := range at {
		if step == "properties" {
			return at[:i]
		}
	}
	return at
}

// forwardMeasureEnumAt reads the measure enum under a named schema, wherever in
// its properties it sits.
func forwardMeasureEnumAt(t *testing.T, doc map[string]any, path []string) []string {
	t.Helper()
	node := any(doc)
	for _, step := range path {
		branch, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("the contract has no %s — this gate names a site that no longer exists, "+
				"so it is checking nothing", strings.Join(path, "/"))
		}
		node = branch[step]
	}
	found := map[string]bool{}
	walkForwardMeasureEnums(node, nil, found)
	if len(found) == 0 {
		t.Fatalf("%s carries no forward-measure enum — either the field was renamed or its "+
			"values changed, and either way this gate stopped comparing anything",
			strings.Join(path, "/"))
	}
	branch, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a schema object", strings.Join(path, "/"))
	}
	return measureEnumUnder(branch)
}

// measureEnumUnder pulls the enum values themselves out of a schema's
// properties, having already established one is there.
func measureEnumUnder(schema map[string]any) []string {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return enumOf(schema)
	}
	for _, child := range properties {
		field, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if values := enumOf(field); slices.Contains(values, "commit_evidence") {
			return values
		}
	}
	return nil
}
