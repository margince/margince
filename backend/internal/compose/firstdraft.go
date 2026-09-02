// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Drafting the message that starts a conversation.
//
// The reply path has been model-backed since it shipped; the first-message path
// was not, and the difference was not a decision about first messages — it was
// that activities.EmailDrafter took an anchor by signature, so a draft with no
// activity behind it could not reach the routed lane at all. What a caller got
// instead was their own intent string placed between two fixed lines: a
// template to rewrite, at the exact moment (a hand shaken at a conference, a
// reply that has to go out while it is fresh) the product is trying to serve.
//
// This is the SAME lane the reply uses — same task, same anti-AI floor, same
// one critic retry, same Voice DNA — with the evidence a first message actually
// has. What it does not have is an anchor, and that is what the two differ by:
// no message being answered, so no subject, no body, no silence to measure, and
// no activity for a learning signal to hang on.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

var _ activities.FirstEmailDrafter = replyDrafter{}

// DraftFirstEmail composes an opening message from the caller's intent.
//
// The intent is the whole of the subject material, and that is a bound rather
// than an oversight: the surfaces that call this name records the message will
// be FILED under, not records it is written FROM, and reading a company's
// fields into an opening line would make the draft's content depend on data the
// approving human is not looking at. A surface that does have the account in
// front of it drafts through accountdraft, which is grounded in that read.
//
// The envelope is resolved from the intent because the intent is the only text
// in existence — so the draft comes back in the language the request was
// written in, which is the best available answer and a visible one: a caller
// who wants German asks in German.
//
// A model that is down, over budget or answering nonsense must not cost the
// caller their draft. The deterministic floor is a real message they can edit,
// and it is what this path returned before the lane was reachable at all.
func (d replyDrafter) DraftFirstEmail(ctx context.Context, intent string) (string, string, error) {
	intent = strings.TrimSpace(intent)
	// BandFresh, not the band a silence would classify to: this IS the opening,
	// and there is no last message to count days from. It is what the
	// deterministic floor below has always rendered, and the two must agree —
	// a model draft and its fallback describing the correspondence differently
	// would make which writer answered visible in the prose.
	state := convstate.State{Band: convstate.BandFresh}
	fallbackSubject, fallbackBody := activities.DeterministicEmailDraft(activities.DraftContext{
		Band:     state.Band,
		Threaded: false,
	}, intent)
	if d.brain == nil {
		return fallbackSubject, fallbackBody, nil
	}
	data := replyActivityData{
		Envelope: d.envelope.Resolve(ctx, intent, state),
		// Threaded is FALSE and the subject and body are empty, which is what
		// makes this a first message rather than a reply with the evidence
		// missing: nothing is being answered, so nothing may carry "Re:".
		Thread: threadFlag(false),
		Intent: boundedRunes(intent, replyActivityMaxRunes),
	}
	// The message goes out under a person's name, so it is written in that
	// person's voice — the same rule the reply path obeys, and the reason this
	// surface may not opt out of it: one rep, one mailbox, two writers is
	// exactly the split Voice DNA exists to close.
	//
	// What it does NOT do is record a learning signal. A rep answers for a
	// served draft through a draft_ref keyed on the activity it replied to,
	// and there is no activity here — so a signal written now is one no
	// feedback could ever land on.
	draft, err := d.completeFirstVoiced(ctx, data, d.loadVoice(ctx))
	if err != nil {
		d.logger().WarnContext(ctx, "model first-message draft unavailable; using deterministic draft", "err", err)
		return fallbackSubject, fallbackBody, nil
	}
	return draft.Subject, draft.Body, nil
}

// completeFirstVoiced drafts the opening message under the sender's voice when
// they have one, holding the same deterministic anti-AI floor the reply path
// holds: detect on the raw draft, one critic retry, sanitize, and on surviving
// violations serve the plain draft instead.
//
// It is separate from completeVoiced because that one's every failure path
// writes a learning signal against an anchor, and this site has no anchor. What
// counts as a violation is not restated here: both methods ask
// draftvoice.Violations and draftvoice.Sanitize, so this one decides only what
// to do when the floor trips.
func (d replyDrafter) completeFirstVoiced(ctx context.Context, data replyActivityData, voice draftvoice.Context) (replyDraft, error) {
	if !voice.OK {
		return d.completeChecked(ctx, firstDraftSystem, data, nil)
	}
	block := voice.Block
	draft, err := d.completeChecked(ctx, firstDraftSystem, data, block)
	if err != nil {
		return replyDraft{}, err
	}
	if violations := voiceDraftViolations(draft); len(violations) > 0 {
		withFeedback := func(fence promptfence.Fence) string {
			return block(fence) + draftvoice.Feedback(violations)
		}
		retried, retryErr := d.completeWith(ctx, firstDraftSystem, data, withFeedback, "")
		if retryErr == nil {
			draft = retried
		}
	}
	draft.Subject, draft.Body = draftvoice.Sanitize(draft.Subject, draft.Body)
	// The sanitizer edits text, so the floor and the shape are re-checked on
	// what would actually be served.
	if len(voiceDraftViolations(draft)) > 0 || validateReplyDraft(draft) != nil {
		return d.completeChecked(ctx, firstDraftSystem, data, nil)
	}
	return draft, nil
}
