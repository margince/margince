// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
)

func residentRouting(profile, premium, embeddings string) string {
	return "profile: " + profile + "\ntiers:\n  premium: " + premium + "\n  local_small: {provider: ollama, model: gemma3}\nembeddings: " + embeddings + "\n"
}

func vertexAtLocation(location string) string {
	return "{provider: gemini_vertex, location: " + location + ", model: gemini-3.5-flash}"
}

func TestEUHostedAdmitsVertexAtAnEULocation(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"eu", "europe-west4", "europe-west1", "europe-north1"} {
		doc := residentRouting("eu_hosted", vertexAtLocation(location), vertexAtLocation(location))
		if _, err := ParseRouting([]byte(doc)); err != nil {
			t.Errorf("location %s keeps processing in the EU and was refused: %v", location, err)
		}
	}
}

func TestEUHostedRefusesAVertexLocationOutsideTheEU(t *testing.T) {
	t.Parallel()
	const resident = "{provider: gemini_vertex, location: europe-west4, model: gemini-embedding-001}"
	for name, tc := range map[string]struct {
		premium, embeddings string
		names               []string
	}{
		"London":           {vertexAtLocation("europe-west2"), resident, []string{"tier premium", "europe-west2", "London is outside the EU"}},
		"Zürich":           {vertexAtLocation("europe-west6"), resident, []string{"tier premium", "europe-west6", "Zürich is outside the EU"}},
		"global":           {vertexAtLocation("global"), resident, []string{"tier premium", "may process the prompt anywhere"}},
		"the US":           {vertexAtLocation("us"), resident, []string{"tier premium", "processes in the US"}},
		"a US region":      {vertexAtLocation("us-central1"), resident, []string{"tier premium", "the EU locations are eu, europe-central2"}},
		"embeddings alone": {vertexAtLocation("eu"), vertexAtLocation("europe-west2"), []string{"the embeddings lane", "London is outside the EU"}},
		"no location":      {"{provider: gemini_vertex, model: gemini-3.5-flash}", resident, []string{"tier premium", "needs a `location`"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRouting([]byte(residentRouting("eu_hosted", tc.premium, tc.embeddings)))
			if err == nil {
				t.Fatal("eu_hosted admitted a Vertex binding that processes outside the EU")
			}
			for _, want := range tc.names {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal %q does not say %q", err, want)
				}
			}
		})
	}
}

// The other arm: the EU is the profile's promise, not the provider's, so a
// profile that makes none admits any location.
func TestOnlyEUHostedHoldsVertexToAnEULocation(t *testing.T) {
	t.Parallel()
	doc := residentRouting("cloud_frontier", vertexAtLocation("europe-west2"), vertexAtLocation("us"))
	if _, err := ParseRouting([]byte(doc)); err != nil {
		t.Errorf("cloud_frontier makes no residency promise and refused a location: %v", err)
	}
	doc = residentRouting("sovereign", "{provider: ollama, model: gemma3}", vertexAtLocation("eu"))
	if _, err := ParseRouting([]byte(doc)); err == nil || !strings.Contains(err.Error(), `forbids cloud provider "gemini_vertex"`) {
		t.Errorf("sovereign admits no cloud provider, even one in the EU, got %v", err)
	}
}

// The certification lane and the routing preview read the gap rather than the
// parser's error, so a Vertex location outside the EU must surface there too.
func TestTheEUGapNamesAVertexLocationOutsideTheEU(t *testing.T) {
	t.Parallel()
	vertex := func(location string) ProviderConfig {
		return ProviderConfig{Provider: providerGeminiVertex, Location: location}
	}
	if gap := EURegionPinGap(vertex("europe-west4")); gap != "" {
		t.Errorf("an EU location reported a gap: %s", gap)
	}
	for _, binding := range []ProviderConfig{vertex("europe-west2"), vertex("global"), vertex("")} {
		if EURegionPinGap(binding) == "" {
			t.Errorf("%+v reported no gap", binding)
		}
	}
	if gap := EURegionPinGap(ProviderConfig{Provider: providerGemini}); gap != "" {
		t.Errorf("a binding with no location to read reported a gap: %s", gap)
	}
}

// Degrading only walks to tiers that are bound, and every bound tier passed
// the location rule, so a budget-pressed ladder stays inside the EU.
func TestADegradedLadderUnderEUHostedStaysInTheEU(t *testing.T) {
	t.Parallel()
	cfg, err := ParseRouting([]byte(`profile: eu_hosted
tiers:
  frontier: {provider: gemini_vertex, location: eu, model: gemini-3.5-flash}
  premium: {provider: gemini_vertex, location: eu, model: gemini-3.5-flash}
  cheap_cloud: {provider: gemini_vertex, location: europe-west1, model: gemini-3.1-flash-lite}
  local_small: {provider: ollama, model: gemma3}
embeddings: {provider: gemini_vertex, location: europe-west4, model: gemini-embedding-001}
`))
	if err != nil {
		t.Fatal(err)
	}
	planned := 0
	for _, task := range append(AllTasks(), TaskEmbeddings) {
		plan, _ := boundPlan(cfg, task, BandDegraded)
		for _, binding := range plan {
			planned++
			if gap := vertexLocationGap(binding.config); gap != "" {
				t.Errorf("task %s degrades onto tier %s, outside the EU: %s", task, binding.tier, gap)
			}
		}
	}
	if planned == 0 {
		t.Fatal("no task planned a binding, so nothing was checked")
	}
}
