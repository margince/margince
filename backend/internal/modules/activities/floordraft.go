// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// FloorDraft is the no-model draft of a reply to one activity. The HTTP route
// and the agent tool both fall back to it, so the greeting and the thread they
// answer cannot differ by transport. chosen is the language the rep picked, or
// Unknown to read it from the activity; lang is the language the draft is in.
//
// covers names the addresses that are ours by domain. The register then comes
// from the contact the reply is delivered to. Nil treats nobody as ours.
func (s *Store) FloorDraft(ctx context.Context, anchor ids.UUID, activity crmcontracts.Activity, chosen textlang.Lang, covers func(address string) bool) (subject, body string, lang textlang.Lang) {
	id := ids.From[ids.ActivityKind](anchor)
	recipient := s.floorRecipient(ctx, id, covers)
	first, last := recipient.GreetingNames()
	answering := DraftContext{
		Language:   chosen,
		Band:       convstate.BandFresh,
		Threaded:   IsMailThread(activity.Kind, activity.Direction),
		Recipient:  first,
		LastName:   last,
		Salutation: recipient.Salutation,
		Formal:     recipient.GreetingRegister == greetingregister.Formal,
	}
	if activity.Subject != nil {
		answering.Topic = *activity.Subject
	}
	if activity.Body != nil {
		answering.Body = *activity.Body
	}
	subject, body = DeterministicEmailDraft(answering)
	return subject, body, answering.language()
}

// floorRecipient is the contact a floor draft greets, empty when unresolved,
// because a greeting is not worth failing a draft for.
func (s *Store) floorRecipient(ctx context.Context, id ids.ActivityID, covers func(address string) bool) ReplyRecipient {
	recipient, err := s.ReplyRecipientFor(ctx, id, covers)
	if err != nil {
		return ReplyRecipient{}
	}
	return recipient
}
