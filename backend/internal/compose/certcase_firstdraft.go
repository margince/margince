// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The first-message site's certification case.
//
// ADR-0074 requires every shipped site to be declared, registered and certified
// through its PRODUCTION path, and this site has a prompt of its own — the
// reply's is not true here, because it names an activity that does not exist.
// A prompt nobody certifies is a prompt free to change under a record still
// claiming to describe it, which is the failure the census exists to prevent.
//
// The fixture is the DRAFTER'S OWN INPUT, as it is on the two composers: a
// fixture that does not decode into what the site is handed describes a request
// this site cannot send, and it is refused at Prepare rather than scored later.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// firstDraftSite is the site name, as ai-tasks.yaml declares it.
const firstDraftSite = "draft_reply/first"

// firstDraftCases serves the message that opens a conversation.
type firstDraftCases struct{}

func (firstDraftCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskDraftReply,
		Variant: "first",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare reads the fixture as the data block this site carries and refuses a
// scenario it cannot answer.
//
// It reuses refuseUnsendableActivity for the same reason the reply site does:
// the bound is read back out of the ENCODED block, so a field added to
// replyActivityData is bounded here on the day it is added. What it additionally
// refuses is a fixture carrying thread evidence — a subject, a body, or an
// inbound thread flag — because a first message has none of those in existence,
// and a scenario supplying them would certify a call the product never makes.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (firstDraftCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var data replyActivityData
	if err := json.Unmarshal(fixture, &data); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", firstDraftSite, err)
	}
	if err := refuseUnsendableActivity(data); err != nil {
		return nil, err
	}
	for what, supplied := range map[string]string{
		"a subject being answered": data.Subject,
		"a body being answered":    data.Body,
		"an inbound mail thread":   data.Thread,
	} {
		if supplied != "" {
			return nil, fmt.Errorf(
				"%s: the fixture carries %s (%q), and a first message has none — this site is the one that "+
					"opens a conversation, so a scenario supplying thread evidence certifies a call the "+
					"product never makes", firstDraftSite, what, supplied)
		}
	}
	if data.Intent == "" {
		return nil, fmt.Errorf(
			"%s: the fixture states no intent, and the intent is the whole of this site's subject material — "+
				"a scenario without one is asking the model to invent a reason to write", firstDraftSite)
	}
	if err := refuseUnanswerableComposerCase(firstDraftSite, expected); err != nil {
		return nil, err
	}
	return &firstDraftCase{data: data}, nil
}

// firstDraftCase is one first-message request ready to be answered.
type firstDraftCase struct{ data replyActivityData }

// Run drives the production lane — the same completeFirstVoiced, correction loop
// and validators DraftFirstEmail drives — and records every request it issued.
//
// The drafter is built with a brain and nothing else because this path does no
// I/O at all: unlike the reply, there is no activity to read and no voice signal
// to record, which is the same property that lets the seam take an intent and
// nothing else.
//
// The voice state is the EMPTY one, which certifies the plain variant. That is
// this site's only certified register today and saying so here is the point: a
// scenario cannot yet state a profile for a first message, so nothing measures
// the voiced turn. Certifying the voiced variant needs a fixture that carries a
// voice artifact, the way the reply site's does.
func (c *firstDraftCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	recorder := &replyDraftRecorder{completer: completer}
	_, err := replyDrafter{brain: recorder}.completeFirstVoiced(ctx, c.data, draftvoice.Context{})
	trace := aitasks.Trace{Requests: recorder.requests}
	if recorder.failed != nil {
		return trace, fmt.Errorf("%s: the model call did not complete: %w", firstDraftSite, recorder.failed)
	}
	if err != nil {
		if recorder.last == "" {
			return trace, fmt.Errorf("%s: no reply reached the drafter to be measured: %w", firstDraftSite, err)
		}
		// The draft the drafter REFUSED, which is what a human would have been
		// spared: the served answer in that case is the deterministic floor,
		// and scoring the floor would certify text no model wrote.
		trace.Output = recorder.last
		return trace, nil
	}
	trace.Output = recorder.served
	return trace, nil
}

// Evaluate re-reads the draft with the same validators the served path uses, so
// the record's verdict is measured rather than inferred from the absence of an
// error. The rubric measures the prose; what is checked here is that a draft was
// written at all, and that it is one this product would send.
func (c *firstDraftCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	draft, err := parseReplyDraft(trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if err := validateReplyDraft(draft); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
