// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The ranking against the vendor's OWN payload rather than against a shape
// this package wrote for itself.
//
// Every other test in this file's neighbour builds its JSON by hand, which
// proves the parser agrees with the author's idea of the vendor. The way this
// read fails in production is the opposite: the vendor's real prices carry far
// more fractional digits than a rate sheet does, its real list is mostly
// billing-lane aliases of a handful of models, and a large share of it is never
// scored at all. Each of those, read too strictly, empties the list rather than
// erroring, and a short list looks exactly like a quiet vendor.
//
// `testdata/openrouter-models.json` is a cut of the real response: the highest
// scored models with their aliases left in, plus models the benchmark never
// scored. Refresh it by re-cutting from the live endpoint; the assertions below
// are about SHAPE and ordering rather than about which model is fastest this
// month, so a refresh should not need them rewritten.

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestTheRankingSurvivesTheVendorsOwnPayload(t *testing.T) {
	body, err := os.ReadFile("testdata/openrouter-models.json")
	if err != nil {
		t.Fatalf("read vendor fixture: %v", err)
	}

	models, err := parseOpenRouterCatalogue(body)
	if err != nil {
		t.Fatalf("parse vendor payload: %v", err)
	}

	// The cap, and the thing that actually breaks: a fixture holding well over
	// ten scored models must still yield ten. Any filter that is too strict
	// shows up here as a short list rather than as an error.
	const top = 10
	got, rankedBy := rankedAvailableModels(models, top)
	if len(got) != top {
		t.Fatalf("ranked %d models from the vendor's own payload, want %d", len(got), top)
	}
	if rankedBy == "" {
		t.Fatal("a payload that parsed reported no ranking measure")
	}

	var unscored map[string]bool
	if unscored, err = idsWithoutAScore(body); err != nil {
		t.Fatalf("read the fixture's own scores: %v", err)
	}

	previous := 0.0
	for i, entry := range got {
		if strings.Contains(entry.ID, ":") {
			t.Errorf("entry %d is a billing-lane alias (%s); the id offered has to be bindable", i, entry.ID)
		}
		if unscored[entry.ID] {
			t.Errorf("entry %d (%s) carries no benchmark score, so nothing ranked it", i, entry.ID)
		}
		if entry.InputPerMtok == nil || *entry.InputPerMtok == "" || entry.OutputPerMtok == nil || *entry.OutputPerMtok == "" {
			t.Errorf("entry %d (%s) reached the wire with a blank price", i, entry.ID)
		}
		if entry.RankScore == nil {
			t.Fatalf("entry %d (%s) carries no score, so the list cannot say why it is here", i, entry.ID)
		}
		score, convErr := strconv.ParseFloat(*entry.RankScore, 64)
		if convErr != nil {
			t.Fatalf("entry %d (%s) has an unreadable score %q", i, entry.ID, *entry.RankScore)
		}
		if i > 0 && score > previous {
			t.Errorf("entry %d (%s) scores %v, above the entry before it (%v): the order is not the ranking",
				i, entry.ID, score, previous)
		}
		previous = score
	}
}

// idsWithoutAScore names the fixture's unscored models, read from the fixture
// itself rather than listed here: a hard-coded list is a second copy of the
// file that stops agreeing with it the first time it is refreshed.
func idsWithoutAScore(body []byte) (map[string]bool, error) {
	var vendor openRouterCatalogue
	if err := json.Unmarshal(body, &vendor); err != nil {
		return nil, err
	}
	unscored := map[string]bool{}
	for _, m := range vendor.Data {
		if m.Benchmarks.ArtificialAnalysis.IntelligenceIndex == nil {
			unscored[m.ID] = true
		}
	}
	if len(unscored) == 0 {
		// The fixture is meant to carry some. Without any, the exclusion this
		// test claims to check is never exercised and the test passes vacuously.
		return nil, errors.New(
			"the vendor fixture holds no unscored models, so the exclusion it exists to prove is untested")
	}
	return unscored, nil
}
