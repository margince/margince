// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The draft_reply/first site's model lane: an opening message written from an
// intent alone.
//
// Its one caller is the site's certification case. draft_email drafts a first
// message through the recipient's own engine instead (agentdraftseam.go).

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

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
	draft, err := d.firstVoicedDraft(ctx, data, voice)
	return data.greeted(draft), err
}

// firstVoicedDraft is completeFirstVoiced before the greeting repair, so the
// voice floor judges the model's own text.
func (d replyDrafter) firstVoicedDraft(ctx context.Context, data replyActivityData, voice draftvoice.Context) (replyDraft, error) {
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
