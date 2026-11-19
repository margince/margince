// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The import screen checks a source name before the server does, and knows a
// draft has ended by the run-state code. Each is a mirror of the server's own
// value, and the contract states the source-name rule too. A rule that drifts
// lets the screen pass a name the server refuses, or refuse one it takes.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/importwire"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	frontendImportFlow  = "../frontend/src/features/import/importflow.ts"
	frontendImportDraft = "../frontend/src/features/import/importdraft.ts"
)

var (
	tsSourceNamePattern = regexp.MustCompile(`(?m)^export const SOURCE_NAME = /(.*)/;$`)
	tsSourceNameMax     = regexp.MustCompile(`(?m)^export const SOURCE_NAME_MAX = (\d+);$`)
	tsRunStateCode      = regexp.MustCompile(`(?m)^export const RUN_STATE = "([^"]*)";$`)
)

func TestTheSourceNameRuleIsOneRuleOnTheScreenTheServerAndTheContract(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendImportFlow)
	if err != nil {
		t.Fatalf("reading %s: %v", frontendImportFlow, err)
	}
	pattern := tsSourceNamePattern.FindSubmatch(source)
	limit := tsSourceNameMax.FindSubmatch(source)
	if pattern == nil || limit == nil {
		t.Fatalf("%s no longer declares SOURCE_NAME and SOURCE_NAME_MAX as literals", frontendImportFlow)
	}
	if got, want := string(pattern[1]), importwire.SourceNamePattern.String(); got != want {
		t.Errorf("%s SOURCE_NAME is /%s/, the server's is /%s/", frontendImportFlow, got, want)
	}
	if got := string(limit[1]); got != strconv.Itoa(importwire.SourceNameMax) {
		t.Errorf("%s SOURCE_NAME_MAX is %s, the server's is %d", frontendImportFlow, got, importwire.SourceNameMax)
	}
	patterned := 0
	walkSourceNameSchemas(loadContractDocument(t), func(name string, schema map[string]any) {
		if schema["pattern"] != nil {
			patterned++
		}
		if got, ok := schema["maxLength"].(int); !ok || got != importwire.SourceNameMax {
			t.Errorf("api/crm.yaml declares %s with maxLength %v, the server's is %d", name, schema["maxLength"], importwire.SourceNameMax)
		}
		if got, ok := schema["pattern"].(string); name == "source_name" && (!ok || got != importwire.SourceNamePattern.String()) {
			t.Errorf("api/crm.yaml declares source_name with pattern %v, the server's is %s", schema["pattern"], importwire.SourceNamePattern)
		}
	})
	if patterned == 0 {
		t.Fatal("api/crm.yaml declares no source_name pattern, so this gate read nothing")
	}
}

// walkSourceNameSchemas visits each schema of a source name in the contract,
// the one a run is filed under and the one an upload proposes.
//
//craft:ignore naked-any a decoded YAML document is an arbitrary tree, so the node this walks is any
func walkSourceNameSchemas(node any, visit func(name string, schema map[string]any)) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if schema, ok := child.(map[string]any); ok && (key == "source_name" || key == "suggested_source_name") {
				if _, bounded := schema["maxLength"]; bounded || schema["pattern"] != nil {
					visit(key, schema)
				}
			}
			walkSourceNameSchemas(child, visit)
		}
	case []any:
		for _, child := range value {
			walkSourceNameSchemas(child, visit)
		}
	}
}

func TestTheScreenKnowsAnEndedDraftByTheServersRunStateCode(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendImportDraft)
	if err != nil {
		t.Fatalf("reading %s: %v", frontendImportDraft, err)
	}
	code := tsRunStateCode.FindSubmatch(source)
	if code == nil {
		t.Fatalf("%s no longer declares RUN_STATE as a string literal", frontendImportDraft)
	}
	if got := string(code[1]); got != importwire.RunStateCode {
		t.Errorf("%s RUN_STATE is %q, the server refuses an ended draft with %q", frontendImportDraft, got, importwire.RunStateCode)
	}
}
