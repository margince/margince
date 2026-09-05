// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every period the contract offers must be a window the server can resolve.
//
// Three transports carry a period — GET /forecast, GET /forecast/calls and the
// agent tool — and each read it as an if against `month` falling through to
// quarter. That shape fails in the one direction nothing on the page can show:
// an unrecognised value answers as a QUARTER under the word the caller sent, so
// a week would have read as three months labelled "week".
//
// Both sides are DERIVED. The wire's values come from the YAML, the server's
// from forecasting.PeriodKindOf. A value added to the contract and not to the
// mapping is a period a client may ask for and the server silently reinterprets.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// Where the contract spells a forecasting period. Each is a separate authoring
// site, and the defect this gate exists for is one of them gaining a value the
// others and the server never learn about.
var forecastPeriodSites = []struct {
	what string
	path []string
}{
	{"the readings query", []string{"paths", "/forecast", "get"}},
	{"the calls query", []string{"paths", "/forecast/calls", "get"}},
	{"the call body", []string{"components", "schemas", "NewForecastCall"}},
}

// The census this gate is derived from: every place in the contract that
// constrains a forecasting period.
//
// Written as a SEARCH rather than a list, because the list is what failed. An
// earlier cut of this gate named two sites, and the two it did not name — the
// call body and the agent tool's own schema — each kept an if-chain that read
// an unknown period as a quarter. A gate whose corpus is hand-maintained is a
// second copy of the thing it protects.
func TestTheSiteListCoversEveryPeriodEnumInTheContract(t *testing.T) {
	t.Parallel()
	// Walked STRUCTURALLY rather than counted as text. An earlier cut of this
	// grepped for two exact spellings, which is a census that fails short: a
	// site written in block style, or with its values reordered, is invisible,
	// and an invisible site is one that can read an unknown period as a quarter
	// — the defect this whole gate exists for. Under-recognition is the one way
	// a gate must not break.
	doc := loadContractDocument(t)
	found := map[string]bool{}
	walkPeriodEnums(doc, nil, found)

	named := map[string]bool{}
	for _, site := range forecastPeriodSites {
		named[strings.Join(site.path, "/")] = true
	}
	for where := range found {
		if !named[where] {
			t.Errorf("the contract constrains a period at %s and this gate does not check it — "+
				"a site it does not name can read an unknown period as a quarter", where)
		}
	}
	if len(found) != len(forecastPeriodSites) {
		t.Errorf("the contract has %d period enum(s) and this gate names %d",
			len(found), len(forecastPeriodSites))
	}
}

// walkPeriodEnums records every path whose enum admits `quarter`, which in this
// contract is only ever a forecasting window.
//
// The path recorded is the one this gate's own site list spells, so a site the
// walk finds and the list omits names itself in the failure.
func walkPeriodEnums(node any, at []string, found map[string]bool) {
	switch shape := node.(type) {
	case map[string]any:
		if slices.Contains(enumOf(shape), "quarter") {
			found[strings.Join(periodSitePath(at), "/")] = true
			return
		}
		for key, child := range shape {
			walkPeriodEnums(child, append(append([]string{}, at...), key), found)
		}
	case []any:
		for _, child := range shape {
			walkPeriodEnums(child, at, found)
		}
	}
}

// enumOf reads a node's enum, whatever else the node carries.
func enumOf(node map[string]any) []string {
	raw, ok := node["enum"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// periodSitePath trims the walk's path back to the shape forecastPeriodSites
// spells: the operation or the schema, not the parameter or property under it.
func periodSitePath(at []string) []string {
	for i, step := range at {
		if step == "parameters" || step == "properties" {
			return at[:i]
		}
		if step == "schema" {
			return at[:i]
		}
	}
	return at
}

// The agent tool publishes its own JSON Schema, hand-written rather than
// generated, so nothing else holds it to the contract beside it. A tool
// offering fewer windows than the HTTP surface is a window an agent cannot ask
// for on a product where it can.
func TestTheAgentToolOffersEveryWindowTheWireDoes(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	raw, err := os.ReadFile(filepath.Join("internal", "modules", "agents", "tools_forecast.go"))
	if err != nil {
		t.Fatalf("reading the forecast tool: %v", err)
	}
	tool := string(raw)
	for _, asked := range periodEnumAt(t, doc, []string{"paths", "/forecast", "get"}) {
		if !strings.Contains(tool, `"`+asked+`"`) {
			t.Errorf("the wire offers period %q and the agent tool's schema does not — "+
				"an agent cannot ask for a window a person can", asked)
		}
	}
}

func TestEveryContractPeriodValueMapsToAKind(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)

	for _, site := range forecastPeriodSites {
		values := periodEnumAt(t, doc, site.path)
		if len(values) == 0 {
			t.Fatalf("%s declares no period enum; this gate is reading the wrong place "+
				"and would pass over a contract that had stopped constraining the value", site.what)
		}
		for _, asked := range values {
			if _, known := forecasting.PeriodKindOf(asked); !known {
				t.Errorf("%s offers period %q and forecasting.PeriodKindOf does not know it — "+
					"a caller asking for it gets a quarter wearing their word", site.what, asked)
			}
		}
	}
}

// And the other way: a kind the server resolves that no transport offers is a
// window nobody can ask for, which is either a dead branch or a contract that
// forgot to publish it.
func TestEveryResolvableKindIsOfferedOnTheWire(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	offered := map[string]bool{}
	for _, site := range forecastPeriodSites {
		for _, asked := range periodEnumAt(t, doc, site.path) {
			offered[asked] = true
		}
	}
	for _, kind := range []forecasting.PeriodKind{
		forecasting.PeriodQuarter, forecasting.PeriodMonth, forecasting.PeriodWeek,
	} {
		if !offered[string(kind)] {
			t.Errorf("the server resolves period %q and no transport offers it — "+
				"either a client cannot reach a window that works, or the branch is dead", kind)
		}
	}
}

// A call names the window it is a call FOR, so the authoring shape has to carry
// every window a reading is taken over. Without that a weekly reading exists
// with no way to author the number it is compared against.
func TestAForecastCallCanNameEveryWindowAReadingIsTakenOver(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	body := periodEnumAt(t, doc, []string{"components", "schemas", "NewForecastCall"})
	if len(body) == 0 {
		t.Fatal("NewForecastCall declares no period enum")
	}
	for _, site := range forecastPeriodSites {
		for _, asked := range periodEnumAt(t, doc, site.path) {
			if !slices.Contains(body, asked) {
				t.Errorf("a reading can be taken over %q and a call cannot name it — "+
					"the reading exists with no way to author what it is compared against", asked)
			}
		}
	}
}

// periodEnumAt finds the `period` enum under one path, whether it is spelled as
// a query parameter or as a body property.
func periodEnumAt(t *testing.T, doc map[string]any, path []string) []string {
	t.Helper()
	node := any(doc)
	for _, step := range path {
		mapped, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("the contract has no %v", path)
		}
		node, ok = mapped[step]
		if !ok {
			t.Fatalf("the contract has no %v", path)
		}
	}
	mapped, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%v is not a mapping", path)
	}
	if params, ok := mapped["parameters"].([]any); ok {
		for _, one := range params {
			param, ok := one.(map[string]any)
			if !ok || param["name"] != "period" {
				continue
			}
			schema, isMap := param["schema"].(map[string]any)
			if !isMap {
				t.Fatalf("the period parameter under %v carries no schema", path)
			}
			return enumStrings(t, schema, "the period parameter")
		}
	}
	if props, ok := mapped["properties"].(map[string]any); ok {
		schema, isMap := props["period"].(map[string]any)
		if !isMap {
			return nil
		}
		return enumStrings(t, schema, "the period property")
	}
	return nil
}
