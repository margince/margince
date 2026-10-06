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

// A location is the operator's choice, whatever profile the document declares:
// the save does not second-guess where a Vertex binding processes its prompts.
func TestAnyProfileAdmitsAVertexLocationOutsideTheEU(t *testing.T) {
	t.Parallel()
	for _, profile := range []string{"eu_hosted", "cloud_frontier"} {
		doc := residentRouting(profile, vertexAtLocation("europe-west2"), vertexAtLocation("us"))
		if _, err := ParseRouting([]byte(doc)); err != nil {
			t.Errorf("profile %s refused a Vertex location: %v", profile, err)
		}
	}
}

func TestAVertexBindingWithNoLocationIsRefused(t *testing.T) {
	t.Parallel()
	const bare = "{provider: gemini_vertex, model: gemini-3.5-flash}"
	_, err := ParseRouting([]byte(residentRouting("eu_hosted", bare, vertexAtLocation("eu"))))
	if err == nil || !strings.Contains(err.Error(), "needs a `location`") {
		t.Errorf("a Vertex binding with no location was admitted or refused without naming the field: %v", err)
	}
}

func TestSovereignAdmitsNoCloudProviderEvenInTheEU(t *testing.T) {
	t.Parallel()
	doc := residentRouting("sovereign", "{provider: ollama, model: gemma3}", vertexAtLocation("eu"))
	if _, err := ParseRouting([]byte(doc)); err == nil || !strings.Contains(err.Error(), `forbids cloud provider "gemini_vertex"`) {
		t.Errorf("sovereign admits no cloud provider, even one in the EU, got %v", err)
	}
}
