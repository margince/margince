// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a waiting row carries when the message is an email, and what it does not
// carry when the message is a chat.
//
// The lane spans both (the waiting query's own `a.kind IN ('email', 'message')`),
// so the canonical email row rides the field rather than the kind word: a client
// branching on "this is a waiting row" would draw a mail icon and an email's
// access badge over a message that never travelled on one.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func emailRow(id ids.UUID) *crmcontracts.EmailSummary {
	return &crmcontracts.EmailSummary{
		ActivityId:    openapi_types.UUID(id),
		OccurredAt:    rankInstant.Add(-2 * 24 * time.Hour),
		Version:       4,
		Subject:       ptrTo("Re: the renewal quote"),
		Preview:       ptrTo("Can you hold the price until Friday?"),
		Counterparty:  ptrTo("Dana Buyer"),
		DisplayStatus: crmcontracts.EmailAccessStatusTeam,
		Move:          crmcontracts.EmailSummaryMoveNone,
	}
}

func ptrTo[T any](v T) *T { return &v }

// An email wait carries the canonical row, so the queue shows the same message
// the timeline shows rather than a bare sentence about it.
func TestAWaitingEmailCarriesTheCanonicalRow(t *testing.T) {
	id := ids.MustParse("01a05500-0000-7000-8000-0000000000e1")
	waiting := WaitingCustomer{
		ActivityID:   id,
		Subject:      "Re: the renewal quote",
		Since:        rankInstant.Add(-2 * 24 * time.Hour),
		EmailSummary: emailRow(id),
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.EmailSummary == nil {
		t.Fatal("a waiting email carried no email_summary; the canonical row is what the client draws")
	}
	if row.item.EmailSummary.ActivityId != openapi_types.UUID(id) {
		t.Errorf("the row's summary names activity %v, want the waiting message %v",
			row.item.EmailSummary.ActivityId, id)
	}
	// The title survives beside it. A surface that has not migrated yet still
	// draws a sentence, and a row that lost its title to gain a summary would
	// go blank on every one of them.
	if row.item.Title == nil || *row.item.Title == "" {
		t.Error("the row lost its title when it gained a summary")
	}
}

// A chat is not an email. The seam hands this lane no summary for one, and the
// row must not invent an empty one: a zero-valued summary would claim an empty
// subject and a `team` badge, which is a message rather than the absence of one.
func TestAWaitingChannelMessageCarriesNoEmailRow(t *testing.T) {
	waiting := WaitingCustomer{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000e2"),
		Subject:    "ping about the quote",
		Since:      rankInstant.Add(-2 * 24 * time.Hour),
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.EmailSummary != nil {
		t.Errorf("a waiting chat carried an email row: %+v", *row.item.EmailSummary)
	}
	if row.item.Title == nil || *row.item.Title != "ping about the quote" {
		t.Errorf("the chat row lost the title it renders from: %+v", row.item.Title)
	}
	// The verb is unchanged: a chat is still answered, just not through an
	// email's row.
	if row.item.Move == nil || row.item.Move.Action != crmcontracts.WorklistMoveActionDraftReply {
		t.Errorf("the chat row lost its reply verb: %+v", row.item.Move)
	}
}
