// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a deployment's declared binding is what `make e2e-ai ROUTING=` acts
// on, so every way it can go wrong costs either a paid run against the wrong
// models or an error that sends an operator to the wrong file.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
	return path
}

const boundConfig = `
seeds:
  ai_routing:
    profile: eu_hosted
    tiers:
      local_small: { provider: openai_compatible, model: vendor/small-1, base_url: https://broker.example/api }
      premium:     { provider: openai_compatible, model: vendor/big-1, base_url: https://broker.example/api }
    embeddings:
      provider: openai_compatible
      model: vendor/embed-1
      base_url: https://broker.example/api
      dimensions: 1024
`

// The happy path: the binding an operator declared is the binding the cert lane
// gets, tier for tier, with the profile the file states — the profile is part of
// a record's identity, so taking it from anywhere else would file the record
// under an environment class nobody declared.
func TestRoutingFromDeployConfigReadsTheDeclaredBinding(t *testing.T) {
	routing, err := RoutingFromDeployConfig(writeConfig(t, boundConfig), runtimeenv.Development)
	if err != nil {
		t.Fatalf("reading a config that declares a binding: %v", err)
	}
	if routing.Profile != ai.ProfileEUHosted {
		t.Errorf("profile = %q, want the file's own eu_hosted", routing.Profile)
	}
	small, bound := routing.Tiers[ai.TierLocalSmall]
	if !bound || small.Model != "vendor/small-1" {
		t.Errorf("local_small = %+v, want the declared vendor/small-1", small)
	}
	if premium := routing.Tiers[ai.TierPremium]; premium.Model != "vendor/big-1" {
		t.Errorf("premium = %+v, want the declared vendor/big-1", premium)
	}
}

// A path that is not there is reported AS a missing file. deployconfig.Load
// tolerates an absent layer, which is right for an optional overlay and wrong
// here — a typo would otherwise be reported as "declares no seeds.ai_routing"
// and send an operator to edit a file that does not exist.
func TestRoutingFromDeployConfigNamesAMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-here.yaml")
	_, err := RoutingFromDeployConfig(missing, runtimeenv.Development)
	if err == nil {
		t.Fatal("an absent config was accepted; the run would have failed later and blamed the file's contents")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error must name the path that is missing, got %q", err)
	}
	if strings.Contains(err.Error(), "declares no seeds.ai_routing") {
		t.Errorf("a missing file must not be reported as an empty one, got %q", err)
	}
}

// A config that parses but declares nothing is an error here, unlike at
// bootstrap where binding nothing is the ordinary case: a certification run has
// no default to fall back on, and there is no model to measure.
func TestRoutingFromDeployConfigRefusesAConfigThatBindsNothing(t *testing.T) {
	_, err := RoutingFromDeployConfig(writeConfig(t, "# a deployment that binds no model, which is the ordinary case at bootstrap\n"), runtimeenv.Development)
	if err == nil {
		t.Fatal("a config declaring no binding was accepted; the run would have spent its setup before failing on the first call")
	}
	if !strings.Contains(err.Error(), "seeds.ai_routing") {
		t.Errorf("the refusal must name the key that is missing, got %q", err)
	}
	// It must also say what to do instead, or an operator has to guess.
	if !strings.Contains(err.Error(), "MODEL=") {
		t.Errorf("the refusal must name the alternative, got %q", err)
	}
}

// A declared binding the parser REFUSES is reported as the file's error, not read
// as "nothing declared" — the two send an operator to opposite fixes.
//
// The case is an incomplete block: ai_routing without an embeddings lane, which
// ParseRouting rejects. Note what this does NOT test — an unknown provider name
// is not refused here at all, because ValidateTierBinding only enforces the
// profile rule and a bad provider surfaces later, when SelectBrain tries to build
// a client. An earlier version of this test asserted "unknown provider" and
// passed on THIS error instead, which is a test passing for a reason it did not
// name.
func TestRoutingFromDeployConfigReportsARefusedBinding(t *testing.T) {
	_, err := RoutingFromDeployConfig(writeConfig(t, "seeds:\n  ai_routing:\n    profile: eu_hosted\n    tiers:\n      local_small: { provider: openai_compatible, model: x, base_url: https://broker.example/api }\n"), runtimeenv.Development)
	if err == nil {
		t.Fatal("a routing block with no embeddings lane was accepted; the router requires one and would have failed at assembly")
	}
	if !strings.Contains(err.Error(), "seeds.ai_routing") {
		t.Errorf("the error must name the key whose contents were refused, got %q", err)
	}
	if strings.Contains(err.Error(), "declares no seeds.ai_routing") {
		t.Errorf("a block that IS declared but invalid must not be reported as absent, got %q", err)
	}
}
