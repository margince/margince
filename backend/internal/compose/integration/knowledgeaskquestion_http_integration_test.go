// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A question the contract does not accept is refused, never answered as "not
// covered": that outcome is a statement about the corpus.
func TestAskingACorpusHoldsTheContractsQuestionLimits(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithCorpusAsk(unaskableEmbedder{}, nil, quietAskLog()))
	e.BootstrapWorkspace(t)
	corpusID := createCorpusOverHTTP(t, e)
	path := "/v1/knowledge/corpora/" + corpusID + "/ask"

	for name, body := range map[string]AnyMap{
		"no question":       {},
		"a null question":   {"question": nil},
		"an empty question": {"question": ""},
		"a blank question":  {"question": "  \n "},
		"a 1001-character":  {"question": strings.Repeat("q", 1001)},
	} {
		if status := e.Call(t, "POST", path, body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("asking with %s → %d, want 422", name, status)
		}
	}
	if status, _ := askCorpusOverHTTP(t, e, corpusID, strings.Repeat("q", 1000)); status != http.StatusOK {
		t.Errorf("asking a 1000-character question → %d, want 200", status)
	}
}
