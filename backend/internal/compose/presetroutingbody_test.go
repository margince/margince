// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The body is what PUT /v1/ai/routing takes, so it must decode as the
// contract's AiRouting with nothing left over.
func TestAPresetBecomesTheBodyTheRoutingEndpointTakes(t *testing.T) {
	raw, err := os.ReadFile("../../../config/presets/openrouter_cloud_eu.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := PresetRoutingBody(raw)
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(got))
	decoder.DisallowUnknownFields()
	var routing crmcontracts.AiRouting
	if err := decoder.Decode(&routing); err != nil {
		t.Fatalf("the body is not an AiRouting: %v\n%s", err, got)
	}
	if routing.Embeddings.Model != "mistralai/mistral-embed-2312" || routing.Profile != "eu_hosted" {
		t.Errorf("embeddings %q, profile %q: the body lost what the preset binds", routing.Embeddings.Model, routing.Profile)
	}
}

func TestAFileWithNoRoutingIsRefused(t *testing.T) {
	if _, err := PresetRoutingBody([]byte("version: 1\n")); err == nil {
		t.Fatal("a config with no seeds.ai_routing produced a body")
	}
}
