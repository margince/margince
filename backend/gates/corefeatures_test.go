// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const browserCoreFeatureRecords = "../frontend/src/lib/corefeature-records.json"

func coreFeatureEnums(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := gatekit.ReadFile(contractDocument)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `yaml:"enum"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	enums := map[string][]string{}
	for name, schema := range doc.Components.Schemas {
		enums[name] = schema.Enum
	}
	if len(enums["CoreFeature"]) == 0 || len(enums["RbacObject"]) < parityRoleObjectFloor {
		t.Fatal("the contract feature and object census went short")
	}
	return enums
}

func TestTheFileSwitchAcceptsEveryContractCoreFeature(t *testing.T) {
	t.Parallel()
	features := coreFeatureEnums(t)["CoreFeature"]
	if err := corefeatures.ValidateDisabled(features); err != nil {
		t.Fatal(err)
	}
	disabled := corefeatures.New(features)
	for _, feature := range features {
		if disabled.Enabled(crmcontracts.CoreFeature(feature)) {
			t.Errorf("disabled feature %s remains enabled", feature)
		}
	}
	if err := corefeatures.ValidateDisabled([]string{"future_misspelled_feature"}); err == nil {
		t.Fatal("an unknown feature was accepted")
	}
}

func browserCoreFeaturePolicy(t *testing.T) []byte {
	t.Helper()
	enums := coreFeatureEnums(t)
	policy := map[string][]string{}
	for _, kind := range slices.Concat(enums["RbacObject"], enums["RecordType"]) {
		var dependencies []string
		for _, feature := range enums["CoreFeature"] {
			if !corefeatures.New([]string{feature}).RecordEnabled(kind) {
				dependencies = append(dependencies, feature)
			}
		}
		if len(dependencies) != 0 {
			policy[kind] = dependencies
		}
	}
	if len(policy) == 0 {
		t.Fatal("the server exposes no feature-bound record kinds")
	}
	var lines []string
	for _, kind := range slices.Sorted(maps.Keys(policy)) {
		key, err := json.Marshal(kind)
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for _, feature := range policy[kind] {
			value, err := json.Marshal(feature)
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, string(value))
		}
		lines = append(lines, "  "+string(key)+": ["+strings.Join(values, ", ")+"]")
	}
	return []byte("{\n" + strings.Join(lines, ",\n") + "\n}\n")
}

func TestTheBrowserSharesTheServerRecordFeaturePolicy(t *testing.T) {
	t.Parallel()
	got, err := gatekit.ReadFile(browserCoreFeatureRecords)
	if err != nil {
		t.Fatalf("reading browser record policy: %v; regenerate with %s", err, mcpParityCommand)
	}
	if !bytes.Equal(got, browserCoreFeaturePolicy(t)) {
		t.Fatalf("browser record policy differs from the server; regenerate with %s", mcpParityCommand)
	}
}
