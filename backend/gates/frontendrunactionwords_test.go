// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A run's history says what it did in the reader's language, by action kind.
// It names a target the reader may not see by its record type. A kind or type
// without words reads as the server's English line. Both directions fail here.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	workflowport "github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const frontendRunWords = "../frontend/src/features/automations/automations.runwords.ts"

func TestEveryRunActionKindHasWords(t *testing.T) {
	t.Parallel()
	inGo := map[string]bool{}
	for _, kind := range workflowport.AllActionKinds() {
		inGo[string(kind)] = true
	}
	entries := tsObjectEntries(t, frontendRunWords, "export const RUN_ACTION_WORDS")
	for kind, words := range entries {
		if !strings.Contains(words, "did:") || !strings.Contains(words, "queued:") {
			t.Errorf("RUN_ACTION_WORDS words %q without both a did and a queued sentence", kind)
		}
	}
	wordsMatchVocabulary(t, "action kind", "RUN_ACTION_WORDS", inGo, tsKeySet(t, entries))
}

// A run targets the record its event is about. A scan or list check targets any
// record type, and an event rule its event's own entity.
func TestEveryRunTargetTypeHasWordsWhenUnnamed(t *testing.T) {
	t.Parallel()
	inGo := map[string]bool{}
	for _, record := range datasource.RecordTypes() {
		inGo[string(record)] = true
	}
	entities := map[string]bool{}
	for _, entity := range datasource.EntityTypes() {
		entities[string(entity)] = true
	}
	for _, entry := range automation.Catalog() {
		if entity, _, isEvent := strings.Cut(entry.Trigger, "."); isEvent && entities[entity] {
			inGo[entity] = true
		}
	}
	inTS := tsKeySet(t, tsObjectEntries(t, frontendRunWords, "export const UNNAMED_TARGET"))
	wordsMatchVocabulary(t, "run target type", "UNNAMED_TARGET", inGo, inTS)
}
