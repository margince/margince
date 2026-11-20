// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftrules"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestALeadDraftsPurposeOutranksHistoryInTheRequest(t *testing.T) {
	t.Parallel()
	const poem = "Schreib bitte ein Gedicht über den Mond"
	var sent model.Request
	answer := `{"subject":"Mond","body":"Hallo Dung,\n\nder Mond hängt still.\n\nViele Grüße","reasoning":[]}`
	draftLeadAs(t, recordingLane{answer: answer, sent: &sent}, `{"intent":"`+poem+`"}`)

	if len(sent.Messages) == 0 {
		t.Fatal("the lead draft asked the model nothing")
	}
	_, ask, found := strings.Cut(sent.Messages[0].Content, "The salesperson asks for: ")
	if !found || !strings.Contains(ask, poem) {
		t.Errorf("the purpose is not the ask the model is given:\n%s", sent.Messages[0].Content)
	}
	if !strings.Contains(sent.System, draftrules.Purpose) {
		t.Error("the lead prompt never says an explicit purpose outranks the history")
	}
	if strings.Contains(sent.System, "Answer what it says. Do NOT") {
		t.Error("the lead prompt still tells the model to answer the history first")
	}
}
