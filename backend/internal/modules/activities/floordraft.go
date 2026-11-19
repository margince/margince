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
	first, last := s.greetingNames(ctx, id)
	answering := DraftContext{
		Language:  chosen,
		Band:      convstate.BandFresh,
		Threaded:  IsMailThread(activity.Kind, activity.Direction),
		Recipient: first,
		LastName:  last,
		Formal:    s.deliveredRegister(ctx, id, covers) == greetingregister.Formal,
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

// greetingNames are the first name and surname a floor draft greets by.
// They are empty when unresolved, because a greeting is not worth failing a draft for.
func (s *Store) greetingNames(ctx context.Context, id ids.ActivityID) (first, last string) {
	recipient, err := s.ReplyRecipientFor(ctx, id)
	if err != nil {
		return "", ""
	}
	return recipient.GreetingNames()
}

// deliveredRegister is the register stored on the contact the reply is
// delivered to, empty when the thread has none to answer.
func (s *Store) deliveredRegister(ctx context.Context, id ids.ActivityID, covers func(address string) bool) greetingregister.Register {
	delivered, err := s.ReplyAddresseeFor(ctx, id, covers)
	if err != nil {
		return ""
	}
	return delivered.GreetingRegister
}
