// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func storedReason(t *testing.T, in StageInput, reader textlang.Lang) *crmcontracts.WorkGrounds {
	t.Helper()
	stored, err := marshalSystemSummary(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return readWhy(systemtext.WithLocale(context.Background(), reader), stored)
}

// Margince's own reason reads in the approver's language, and the summary
// keeps its sentence beside it.
func TestAMarginceReasonReadsInTheApproversLanguage(t *testing.T) {
	why := systemtext.Sentence("coach.why.no_next_step", systemtext.Text("Depot rollout"), systemtext.Date("2026-11-12"))
	in := StageInput{Summary: "Move Depot rollout to Proposal", Why: Grounds{Sentence: why}}
	got := storedReason(t, in, textlang.German)
	if got == nil || !strings.HasPrefix(got.Text, "Für Depot rollout ist kein nächster Schritt geplant") {
		t.Fatalf("a German approver reads %+v", got)
	}
	stored, err := marshalSystemSummary(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if parts := readSummaryParts(stored); parts == nil || parts.Sentence != in.Summary {
		t.Fatalf("the reason displaced the summary: %+v", parts)
	}
}

// An agent's reason is its own words, shown as written on one line.
func TestAnAgentsReasonIsShownAsWritten(t *testing.T) {
	got := storedReason(t, StageInput{Why: Grounds{Text: "  Anna Weber wrote on 14 Nov\nthat the rollout moves to March.  "}}, textlang.German)
	if got == nil || got.Text != "Anna Weber wrote on 14 Nov that the rollout moves to March." {
		t.Fatalf("the approver reads %+v", got)
	}
}

// A proposal with no reason carries none, rather than a stock sentence.
func TestAProposalWithNoReasonCarriesNone(t *testing.T) {
	if got := storedReason(t, StageInput{Summary: "Rename the company"}, textlang.English); got != nil {
		t.Fatalf("a proposal with no reason reads %+v", got)
	}
}
