// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// The use-case lane names its candidates in a JSON table the Python bridge
// reads; these hold that table to the product it mirrors.
const (
	e2eLLMCandidatesFile = "../../../e2e/llm/candidates.json"
	// The preset whose Mistral routing the lane's OpenRouter route mirrors: the
	// EU-hosted one, because Mistral's own consumer app is served from the EU.
	e2eLLMMistralPreset = "../../../config/presets/openrouter_cloud_eu.yaml"
	e2eLLMMistralModel  = "mistralai/mistral-medium-3-5"
	// The bridge names every tool mcp__<SERVER>__<tool>, SERVER read from the
	// one file that writes it, so this census follows a rename.
	e2eLLMTranscriptWriter = "../../../e2e/llm/transcript.py"
	openAIFunctionName     = 64
)

type e2eLLMCandidateTable map[string]struct {
	Routes map[string]struct {
		Model   string                `json:"model"`
		Routing *ai.OpenRouterRouting `json:"routing"`
	} `json:"routes"`
}

func readE2ELLMCandidates(t *testing.T) e2eLLMCandidateTable {
	t.Helper()
	raw, err := os.ReadFile(e2eLLMCandidatesFile)
	if err != nil {
		t.Fatalf("reading %s: %v", e2eLLMCandidatesFile, err)
	}
	var table e2eLLMCandidateTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("%s: %v", e2eLLMCandidatesFile, err)
	}
	return table
}

// The lane's OpenRouter route for Mistral is a declared mirror of the routing
// the committed preset binds for the same model, so a lane verdict measures
// the endpoints a deployment would reach. Either side moving alone fails here.
func TestTheLanesMistralRoutingMirrorsThePreset(t *testing.T) {
	raw, err := os.ReadFile(e2eLLMMistralPreset)
	if err != nil {
		t.Fatalf("reading %s: %v", e2eLLMMistralPreset, err)
	}
	cfg, err := ai.ParsePreset(raw)
	if err != nil {
		t.Fatalf("%s: %v", e2eLLMMistralPreset, err)
	}
	var bound *ai.OpenRouterRouting
	for _, binding := range cfg.Tiers {
		if binding.Model == e2eLLMMistralModel {
			bound = binding.Routing
		}
	}
	if bound == nil {
		t.Fatalf("%s binds no tier to %s with a routing block; the lane's mirror has nothing to mirror", e2eLLMMistralPreset, e2eLLMMistralModel)
	}
	route := readE2ELLMCandidates(t)["mistral"].Routes["openrouter"]
	if route.Model != e2eLLMMistralModel {
		t.Fatalf("%s routes mistral over OpenRouter as %q, the preset binds %q", e2eLLMCandidatesFile, route.Model, e2eLLMMistralModel)
	}
	if route.Routing == nil || !reflect.DeepEqual(*route.Routing, *bound) {
		t.Errorf("%s mistral/openrouter routing = %+v, %s binds %+v: change both or neither",
			e2eLLMCandidatesFile, route.Routing, e2eLLMMistralPreset, *bound)
	}
}

// The bridge offers every served tool under the lane's prefix, and OpenAI
// refuses a whole request whose function name runs past its limit.
func TestEveryServedToolFitsAFunctionNameUnderTheLanesPrefix(t *testing.T) {
	prefix := e2eLLMToolPrefix(t)
	specs := servedSurface(t).Specs()
	if len(specs) == 0 {
		t.Fatal("the served surface lists no tools; this census would pass having read nothing")
	}
	for _, spec := range specs {
		if name := prefix + spec.Name; len(name) > openAIFunctionName {
			t.Errorf("%s is %d characters; OpenAI refuses a function name over %d, so the GPT lane could not offer it",
				name, len(name), openAIFunctionName)
		}
	}
}

// A model id carrying a '/' files its verdicts one folder too deep, where the
// coverage page would skip them and publish no row for a model that ran.
func TestANestedVerdictFolderIsRefusedNotSkipped(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "openai", "gpt-5.6-sol")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	verdict := `{"scenario":"case1_log_it","passed":1,"runs":1,"pass_at":1}`
	if err := os.WriteFile(filepath.Join(nested, "case1_log_it.json"), []byte(verdict), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readE2ELLMVerdicts(dir); err == nil {
		t.Fatal("a verdict filed one folder too deep was read as no verdict at all")
	}
}

var e2eLLMServerLine = regexp.MustCompile(`(?m)^SERVER = "([A-Za-z0-9_]+)"$`)

func e2eLLMToolPrefix(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(e2eLLMTranscriptWriter)
	if err != nil {
		t.Fatalf("reading %s: %v", e2eLLMTranscriptWriter, err)
	}
	got := e2eLLMServerLine.FindSubmatch(raw)
	if got == nil {
		t.Fatalf("%s declares no `SERVER = \"...\"` line; the tool prefix cannot be derived", e2eLLMTranscriptWriter)
	}
	return "mcp__" + string(got[1]) + "__"
}
