// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The forwardable introduction note's certification case.
//
// This is the OUTWARD half of the introduction pair, and the one with the
// larger blast radius. Its sibling asks a colleague for a favour: a sentence
// that overclaims there is read by a teammate who shrugs. This note is
// forwarded to a customer over that colleague's own name, so the same slip
// reaches the counterparty — and the colleague, whose reputation is spent,
// cannot take it back.
//
// It also carries two facts the ask does not: an intermediary on an indirect
// route, and the rep's own free-text reason, which arrives straight off a
// request body and is the most obvious injection surface on the call.
//
// Run calls network.IntroNoteRequestFor and Evaluate calls
// network.CheckIntroNote, both the production path. A case that rebuilt either
// would measure a copy.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// introNoteSite is the site name, as ai-tasks.yaml declares it.
const introNoteSite = "draft_reply/intro_note"

// introNoteCases serves the note a colleague forwards to a customer.
type introNoteCases struct{}

func (introNoteCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskDraftReply,
		Variant: "intro_note",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare reads the fixture as the facts this site is handed, and refuses a
// scenario it cannot answer.
//
// The three it insists on are the three the note cannot be written without: the
// recipient it is addressed to, the rep it is about, and the colleague whose
// voice it goes out in. A fixture missing any of them describes a note the
// endpoint never produces, so certifying it would measure a call the product
// does not make.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (introNoteCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var in network.IntroNoteFixture
	if err := json.Unmarshal(fixture, &in); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", introNoteSite, err)
	}
	// A SLICE, so a fixture missing two of them names the same one every run:
	// a map here reported whichever the runtime happened to walk first, and a
	// scenario author fixing one error to be shown another is being told the
	// truth twice as slowly.
	for _, required := range []struct{ what, supplied string }{
		{"the recipient the note is addressed to", in.Contact},
		{"the colleague who forwards it", in.Colleague},
		{"the rep being introduced", in.Requester},
	} {
		if strings.TrimSpace(required.supplied) == "" {
			return nil, fmt.Errorf(
				"%s: the fixture names %s nowhere, so it describes a note the endpoint never "+
					"produces — a scenario without it certifies a call the product does not make",
				introNoteSite, required.what)
		}
	}
	// What the note must NOT say. An empty list is a real expectation: it means
	// the scenario is checking only that a forwardable note came back.
	var forbidden []string
	if err := json.Unmarshal(expected, &forbidden); err != nil {
		return nil, fmt.Errorf(
			"%s: the expected value is not a list of phrases the note must avoid: %w",
			introNoteSite, err)
	}
	for i, phrase := range forbidden {
		// A blank phrase is contained by EVERY string, so one in the list scores
		// every reply a wrong answer and the site reads as never able to write a
		// sendable note. Nothing upstream refuses it: the corpus loader checks
		// the scenario's shape, not the meaning of a phrase inside it.
		if strings.TrimSpace(phrase) == "" {
			return nil, fmt.Errorf(
				"%s: the phrase at position %d is blank, and a blank phrase appears in every reply — "+
					"the scenario would report a wrong answer on every run whatever the model wrote",
				introNoteSite, i)
		}
	}
	// The request is built HERE rather than in Run, so a scenario the endpoint
	// cannot be handed — a strength bucket from the wrong contract's vocabulary
	// — is refused before a model is paid to answer a prompt nobody sends.
	req, err := network.IntroNoteRequestFor(in)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", introNoteSite, err)
	}
	return &introNoteCase{in: in, forbidden: forbidden, req: req}, nil
}

// introNoteCase is one forwardable note ready to be answered.
type introNoteCase struct {
	in        network.IntroNoteFixture
	forbidden []string
	req       model.Request
}

// Run issues the one request this site sends, through the production builder.
func (c *introNoteCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	trace := aitasks.Trace{Requests: []model.Request{c.req}}
	res, err := completer.Complete(ctx, c.req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", introNoteSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate runs the production check, then asks whether the note said something
// the record does not hold.
//
// A reply the checker refuses is INVALID — the reader would have got the
// template instead, and scoring the template would certify text no model wrote.
//
// The subject is searched alongside the body, which is where this differs from
// the colleague-facing case: this site asks for a subject line naming the rep,
// so the subject is prose a customer reads rather than a field nobody looks at,
// and an overclaim placed there would go out unexamined.
func (c *introNoteCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	subject, body, err := network.CheckIntroNote(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	folded := strings.ToLower(subject + "\n" + body)
	var claimed []string
	for _, phrase := range c.forbidden {
		if strings.Contains(folded, strings.ToLower(phrase)) {
			claimed = append(claimed, phrase)
		}
	}
	if len(claimed) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: "claimed what the record does not hold: " + strings.Join(claimed, ", "),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
