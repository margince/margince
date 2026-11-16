// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import "testing"

// declaredSchemaless names the sites that ask for JSON and send no response
// schema on purpose, against the sentence in their code that says why.
// gatekit:fixture the sentence each site's own code uses to say why its JSON reply carries no schema — expected data, checked to still exist, not a waived cost
var declaredSchemaless = map[string]string{
	"brief_ranking/rank": "carries no ResponseSchema: BoundToCandidates enforces",
	"cert_judge/judge":   "It carries no ResponseSchema: its digest stamps every judged",
}

// checkSchemaDeclared holds a site whose request asks for JSON to send the
// schema providers enforce at generation, or to say in its code why not.
func checkSchemaDeclared(t *testing.T, key string, unschemed bool) {
	t.Helper()
	sentence, declared := declaredSchemaless[key]
	switch {
	case unschemed && !declared:
		t.Errorf("site %s asks for JSON and sends no response schema: declare one from its reply struct, "+
			"or say in its code why not and add that sentence to declaredSchemaless", key)
	case declared && !unschemed:
		t.Errorf("site %s is declared schemaless but every request now carries a schema; remove it from declaredSchemaless", key)
	case declared && !declarationStillInTree(t, sentence):
		t.Errorf("site %s is declared schemaless on the strength of %q, which is no longer in backend/", key, sentence)
	}
}

func checkDeclaredSchemalessReached(t *testing.T, prompts []sitePrompt) {
	t.Helper()
	read := map[string]bool{}
	for _, p := range prompts {
		read[p.task+"/"+p.variant] = true
	}
	for key := range declaredSchemaless {
		if !read[key] {
			t.Errorf("declaredSchemaless names %s, which no corpus scenario reaches", key)
		}
	}
}
