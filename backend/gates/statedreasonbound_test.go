// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The written reason on a decision that narrows what the installation keeps is
// bounded by one number. The contract's RetentionOverrideRequest says it, and
// statedreason.Max is the Go spelling both the controller's overrides and the
// undo of a project filing use. This holds that pair together; it does not claim
// to find another reason bound elsewhere in the contract.

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/kernel/statedreason"
)

func TestTheStatedReasonBoundIsTheContractsMaxLength(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MaxLength *int `yaml:"maxLength"` //nolint:tagliatelle // the contract spells its keywords in camelCase
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("decoding the contract: %v", err)
	}
	reason, found := contract.Components.Schemas["RetentionOverrideRequest"].Properties["reason"]
	if !found || reason.MaxLength == nil {
		t.Fatal("RetentionOverrideRequest.reason declares no maxLength; the Go bound has nothing to mirror")
	}
	if *reason.MaxLength != statedreason.Max {
		t.Errorf("statedreason.Max = %d, the contract's reason maxLength = %d", statedreason.Max, *reason.MaxLength)
	}
}
