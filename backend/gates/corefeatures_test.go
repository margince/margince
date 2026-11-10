// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestTheFileSwitchAcceptsEveryContractCoreFeature(t *testing.T) {
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
	features := doc.Components.Schemas["CoreFeature"].Enum
	if len(features) == 0 {
		t.Fatal("the contract declares no core features")
	}
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
