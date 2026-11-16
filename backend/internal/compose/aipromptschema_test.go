// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// asksForJSON reads the whole request, since some sites put the answer shape
// in the user turn rather than the system prompt.
func asksForJSON(req model.Request) bool {
	if strings.Contains(req.System, "JSON") {
		return true
	}
	for _, msg := range req.Messages {
		if strings.Contains(msg.Content, "JSON") {
			return true
		}
	}
	return false
}

// declaredSchemaless names the sites that ask for JSON and send no response
// schema on purpose, against the sentence in their code that says why. The
// sentence is checked to still exist, as declaredIsolation's are.
// gatekit:fixture the sentence each site's own code uses to say why its JSON reply carries no schema — expected data, checked to still exist, not a waived cost
var declaredSchemaless = map[string]string{
	"brief_ranking/rank": "carries no ResponseSchema: BoundToCandidates enforces",
	"cert_judge/judge":   "It carries no ResponseSchema: this request's digest stamps every judged",
}

// TestEveryJSONPromptSendsASchema holds each site whose instruction asks for
// JSON to send a response schema, which providers enforce at generation, or
// to say in its code why not.
func TestEveryJSONPromptSendsASchema(t *testing.T) {
	t.Parallel()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	unschemed := map[string]bool{}
	read := map[string]bool{}
	for _, sc := range scenarios {
		key := sc.Task + "/" + sc.Site
		one, _, readErr := readSitePrompt(census, sc)
		if readErr != nil {
			t.Errorf("%s (%s): %v", key, sc.Name, readErr)
			continue
		}
		read[key] = true
		if one.unschemed > 0 {
			unschemed[key] = true
		}
	}
	if len(read) == 0 {
		t.Fatal("no site was read, so no prompt was checked")
	}
	var sites []string
	for key := range read {
		sites = append(sites, key)
	}
	sort.Strings(sites)
	for _, key := range sites {
		sentence, declared := declaredSchemaless[key]
		switch {
		case unschemed[key] && !declared:
			t.Errorf("site %s asks for JSON and sends no response schema: declare one from its reply struct, "+
				"or say in its code why not and add that sentence to declaredSchemaless", key)
		case declared && !unschemed[key]:
			t.Errorf("site %s is declared schemaless but every request now carries a schema; remove it from declaredSchemaless", key)
		case declared && !declarationStillInTree(t, sentence):
			t.Errorf("site %s is declared schemaless on the strength of %q, which is no longer in backend/", key, sentence)
		}
	}
	for key := range declaredSchemaless {
		if !read[key] {
			t.Errorf("declaredSchemaless names %s, which no corpus scenario reaches", key)
		}
	}
}
