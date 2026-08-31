// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// The anti-AI floor a voiced draft passes through, and the guarantee it owes:
// every step may improve the draft and none may replace it with a worse one.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
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

func voiced() draftvoice.Context {
	return draftvoice.Context{
		Profile: ai.VoiceProfile{PersonalityMD: "I write short."},
		Version: ai.VoiceProfileVersion{ProfileVersion: 1, VoiceProfileMD: "Sentences stay under twelve words."},
		OK:      true,
	}
}

// A draft the sanitizer would empty is served unsanitized rather than blank.
//
// The sanitizer deletes characters. A subject made only of what it deletes
// comes back as the empty string, and an empty subject is not a message the
// contract can carry — so the pre-sanitizer draft, imperfect but real, is what
// the rep gets. Without this the composer answers a rep's "Write email" with a
// draft that has no subject at all.
func TestASanitizerThatWouldEmptyTheDraftIsNotApplied(t *testing.T) {
	// A subject that is NOTHING but a spaced em dash: it survives the parse,
	// which only refuses an empty string, and the sanitizer removes the whole
	// of it. A subject that merely CONTAINS one is not this case — the
	// sanitizer rewrites it to a comma and the draft is never empty, so a
	// fixture built from one cannot fail however the code behaves.
	const answer = `{"subject":" — ","body":"Hi Sarah,\n\nShall we pick this up?"}`
	lane := &sequencedLane{answers: []string{answer, answer}}

	draft, _, err := Write(context.Background(), lane, sampleInput(), voiced())
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

// A retry that clears nothing is discarded, and the first attempt stands.
//
// A retry is one more model answer, not a better one by definition: it is
// written without the draftcheck findings the first pass already cleared, so
// keeping it unconditionally can trade a clean draft for a dirty one.
func TestARetryThatClearsNothingIsDiscarded(t *testing.T) {
	const first = `{"subject":"Rollout — Phase 2","body":"Hi Sarah,\n\nShall we pick this up?"}`
	// The retry keeps the violation AND loses the recipient's name, so a test
	// that could not tell the two apart would pass on either being served.
	const retry = `{"subject":"Rollout — Phase 2","body":"Hi there,\n\nAnything to add?"}`
	lane := &sequencedLane{answers: []string{first, retry}}

	draft, _, err := Write(context.Background(), lane, sampleInput(), voiced())
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

// An unvoiced draft costs no second model call.
//
// The floor exists for the voiced path. Running it over every draft would
// double the model spend of the common case to fix the rare one.
func TestAnUnvoicedDraftRunsNoVoiceRetry(t *testing.T) {
	const answer = `{"subject":"Rollout — Phase 2","body":"Hi Sarah,\n\nShall we pick this up?"}`
	lane := &sequencedLane{answers: []string{answer}}

	if _, _, err := Write(context.Background(), lane, sampleInput(), draftvoice.Context{}); err != nil {
		t.Fatalf("the unvoiced draft failed: %v", err)
	}
	if lane.calls != 1 {
		t.Errorf("an unvoiced draft made %d model calls; it must make exactly one", lane.calls)
	}
}
