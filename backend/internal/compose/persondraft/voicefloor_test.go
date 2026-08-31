// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package persondraft

// The anti-AI floor a voiced draft passes through, and the guarantee it owes:
// every step may improve the draft and none may replace it with a worse one.
//
// The account composer carries the same floor and the same tests. Both, on
// purpose: the two were written from one template, and a fix applied to one of
// them is exactly the drift a pair of tests exists to catch.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// sequencedLane answers each call with the next scripted response, so a test
// can describe a first attempt and the retry that follows it.
type sequencedLane struct {
	answers []string
	err     error
	calls   int
}

func (l *sequencedLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	l.calls++
	if l.err != nil {
		return model.Response{}, l.err
	}
	if l.calls > len(l.answers) {
		return model.Response{}, errors.New("the lane was called more times than the test scripted")
	}
	return model.Response{Text: l.answers[l.calls-1]}, nil
}

func floorInput() Input {
	return Input{
		Recipient: RecipientIn{
			ID: "019fe7ae-0000-7000-8000-000000000001", Name: "Sarah Cole",
			FirstName: "Sarah", Email: "sarah@glazedfrog.example",
		},
	}
}

func voiced() draftvoice.Context {
	return draftvoice.Context{
		Profile: ai.VoiceProfile{PersonalityMD: "I write short."},
		Version: ai.VoiceProfileVersion{ProfileVersion: 1, VoiceProfileMD: "Sentences stay under twelve words."},
		OK:      true,
	}
}

// A retry that clears a voice violation may not smuggle in a phrasing one.
//
// The floor runs AFTER draftcore.CorrectOnce, so the draft it receives has
// already cleared the phrasing rules. The retry it may substitute has not: it
// is a fresh answer, written to a prompt that names the voice violations and
// says nothing about the phrasing findings the first pass fixed. Counting only
// voice violations lets a retry that drops an em dash and invents a shared
// history — "we help companies", to somebody we have never written to —
// replace a draft that was clean by both measures.
func TestARetryThatTradesAVoiceViolationForAPhrasingOneIsDiscarded(t *testing.T) {
	const first = `{"subject":"Rollout","body":"Hi Sarah,\n\nThe rollout — Phase 2 — is ready. Shall we pick this up?"}`
	const retry = `{"subject":"Rollout","body":"Hi Sarah,\n\nWe help companies like yours. Shall we pick this up?"}`
	lane := &sequencedLane{answers: []string{first, retry}}

	// A FIRST-TOUCH envelope, which is what makes "we help companies" a
	// finding: the invention rules only bind where there is no prior
	// correspondence to have established anything.
	in := floorInput()
	in.Envelope = draftfloor.Envelope{ConversationState: string(convstate.BandNone)}

	draft, _, err := Write(context.Background(), lane, in, voiced())
	if err != nil {
		t.Fatalf("the voiced draft failed: %v", err)
	}
	if strings.Contains(strings.ToLower(draft.Body), "we help companies") {
		t.Errorf("the floor served a retry carrying a phrasing rule the first draft had cleared; "+
			"body = %q", draft.Body)
	}
}

// A retry that clears nothing is discarded, and the first attempt stands.
func TestARetryThatClearsNothingIsDiscarded(t *testing.T) {
	const first = `{"subject":"Rollout — Phase 2","body":"Hi Sarah,\n\nShall we pick this up?"}`
	// The retry keeps the violation AND loses the recipient's name, so a test
	// that could not tell the two apart would pass on either being served.
	const retry = `{"subject":"Rollout — Phase 2","body":"Hi there,\n\nAnything to add?"}`
	lane := &sequencedLane{answers: []string{first, retry}}

	draft, _, err := Write(context.Background(), lane, floorInput(), voiced())
	if err != nil {
		t.Fatalf("the voiced draft failed: %v", err)
	}
	if lane.calls != 2 {
		t.Fatalf("the floor made %d model calls; a violation earns exactly one retry", lane.calls)
	}
	if !strings.Contains(draft.Body, "Shall we pick this up?") {
		t.Errorf("the composer kept a retry that cleared no violation; body = %q", draft.Body)
	}
}

// A draft the sanitizer would empty is served unsanitized rather than blank.
//
// The sanitizer deletes characters. A subject made only of what it deletes
// comes back as the empty string, and an empty subject is not a message the
// contract can carry — so the pre-sanitizer draft, imperfect but real, is what
// the rep gets.
func TestASanitizerThatWouldEmptyTheDraftIsNotApplied(t *testing.T) {
	// A subject that is NOTHING but a spaced em dash: it survives the parse,
	// which only refuses an empty string, and the sanitizer removes the whole
	// of it. A subject that merely CONTAINS one is not this case — the
	// sanitizer rewrites it to a comma and the draft is never empty.
	const answer = `{"subject":" — ","body":"Hi Sarah,\n\nShall we pick this up?"}`
	lane := &sequencedLane{answers: []string{answer, answer}}

	draft, _, err := Write(context.Background(), lane, floorInput(), voiced())
	if err != nil {
		t.Fatalf("the voiced draft failed: %v", err)
	}
	if strings.TrimSpace(draft.Subject) == "" {
		t.Error("the composer served a draft with an empty subject, which is not a message a rep can send")
	}
	if strings.TrimSpace(draft.Body) == "" {
		t.Error("the composer served a draft with an empty body")
	}
}

// An unvoiced draft costs no second model call.
func TestAnUnvoicedDraftRunsNoVoiceRetry(t *testing.T) {
	const answer = `{"subject":"Rollout — Phase 2","body":"Hi Sarah,\n\nShall we pick this up?"}`
	lane := &sequencedLane{answers: []string{answer}}

	if _, _, err := Write(context.Background(), lane, floorInput(), draftvoice.Context{}); err != nil {
		t.Fatalf("the unvoiced draft failed: %v", err)
	}
	if lane.calls != 1 {
		t.Errorf("an unvoiced draft made %d model calls; it must make exactly one", lane.calls)
	}
}
