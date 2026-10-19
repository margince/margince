// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestMoveLanguageCannotChangeTheFiveSelectedEvents(t *testing.T) {
	langcopytest.Census(t, moveWords)
	langcopytest.Census(t, offerStatusWords)
	renders := []func(briefCopy) string{
		func(s briefCopy) string { return s.roomMoveLine("Amy") },
		func(s briefCopy) string { return s.offerMoveLine("Q1", 3, "sent") },
		func(s briefCopy) string { return s.stageMoveLine("", "StageZ") },
		func(s briefCopy) string { return s.roomMoveLine("Zed") },
		func(s briefCopy) string { return s.stageMoveLine("StageA", "StageB") },
		func(s briefCopy) string { return s.offerMoveLine("Q2", 4, "accepted") },
	}
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		var moves []DealMoveIn
		for _, render := range renders {
			moves = append(moves, said.wordedMove(at(8), ids.Nil, render))
		}
		selected := newestFirst(moves)
		if len(selected) != dealMovesCap {
			t.Fatalf("selected %d events", len(selected))
		}
		for i, token := range []string{"StageZ", "StageB", "Q2", "Q1", "Zed"} {
			if !strings.Contains(selected[i].Text, token) {
				t.Fatalf("%s changed event %d: %s", language, i, selected[i].Text)
			}
		}
		encoded, err := json.Marshal(selected)
		if err != nil {
			t.Fatal(err)
		}
		if language != "en" && strings.Contains(string(encoded), "Since then") {
			t.Fatal("internal English ordering leaked to model input")
		}
		if !strings.Contains(said.offerMoveLine("Q3", 5, "future_status"), "future_status") {
			t.Fatal("unknown status lost")
		}
	}
}
