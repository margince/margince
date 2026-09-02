// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Voice DNA lane of the reply drafter: loading the actor's active profile,
// drafting under it with the deterministic anti-AI floor, and feeding served
// and rejected drafts back as learning signals.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// loadVoice resolves the actor's active voice through the same seam the two
// composers use, so a reply and a composed mail cannot disagree about whose
// voice they are written in.
//
// Held by: TestEveryDraftingSurfaceLoadsTheSenderVoice
// (backend/internal/compose/draftvoiceparity_test.go), which sweeps the tree
// for every surface composing draftrules.Shared and fails when one of them
// reaches a model without calling draftvoice.Load.
//
// A nil store is a deployment with no voice lane; a lookup failure degrades to
// the plain draft with the reason logged, because a broken voice read must
// never take reply drafting down with it.
func (d replyDrafter) loadVoice(ctx context.Context) draftvoice.Context {
	if d.voice == nil {
		return draftvoice.Context{}
	}
	return draftvoice.Load(ctx, d.voice, d.logger())
}

// completeVoiced drafts with the voice block when one is loaded, enforcing
// the deterministic anti-AI floor: detect → one critic retry → sanitize →
// on surviving violations fall back to the plain draft and record the
// failure as a rejected learning signal.
func (d replyDrafter) completeVoiced(ctx context.Context, anchor ids.UUID, data replyActivityData, voice draftvoice.Context) (replyDraft, *int, *string, error) {
	if !voice.OK {
		draft, err := d.completeChecked(ctx, replyDraftSystem, data, nil)
		return draft, nil, nil, err
	}
	block := voice.Block
	draft, err := d.completeChecked(ctx, replyDraftSystem, data, block)
	if err != nil {
		return replyDraft{}, nil, nil, err
	}
	// Detect on the RAW draft (subject and body separately — the
	// canned-opener rule anchors at text start): a violation the sanitizer
	// could mechanically remove still earns the critic retry, because the
	// retry fixes the sentence, not just the punctuation.
	if violations := voiceDraftViolations(draft); len(violations) > 0 {
		withFeedback := func(fence promptfence.Fence) string {
			return block(fence) + draftvoice.Feedback(violations)
		}
		retried, retryErr := d.complete(ctx, data, withFeedback)
		if retryErr == nil {
			draft = retried
		}
	}
	draft.Subject, draft.Body = draftvoice.Sanitize(draft.Subject, draft.Body)
	version := voice.Version.ProfileVersion
	// The sanitizer edits text, so the floor AND the shape are re-checked on
	// what would actually be served.
	if len(voiceDraftViolations(draft)) > 0 || validateReplyDraft(draft) != nil {
		// The voice-styled draft kept tripping the floor: serve the plain
		// draft instead and let the failure feed the learning panel.
		d.recordVoiceRejection(ctx, voice, anchor, draft)
		plain, plainErr := d.completeChecked(ctx, replyDraftSystem, data, nil)
		return plain, nil, nil, plainErr
	}
	d.recordVoiceDraft(ctx, voice, anchor, draft)
	ref := voiceDraftRef(voice, anchor, draft)
	return draft, &version, &ref, nil
}

// voiceDraftViolations runs the deterministic floor over subject and body
// independently; concatenation would hide a canned opener inside the body.
func voiceDraftViolations(draft replyDraft) []ai.VoiceViolation {
	return draftvoice.Violations(draft.Subject, draft.Body)
}

// voiceDraftRef keys one served draft for learning-signal feedback. It
// covers profile, version, anchor, and the full draft: two drafts for the
// same activity with the same body but different subjects — or from
// different profile versions — never collide.
func voiceDraftRef(voice draftvoice.Context, anchor ids.UUID, draft replyDraft) string {
	sum := sha256.Sum256([]byte(draft.Subject + "\n" + draft.Body))
	return fmt.Sprintf("replydraft:%s:%s:v%d:%s",
		voice.Profile.ID, anchor, voice.Version.ProfileVersion, hex.EncodeToString(sum[:8]))
}

func (d replyDrafter) recordVoiceDraft(ctx context.Context, voice draftvoice.Context, anchor ids.UUID, draft replyDraft) {
	if d.voice == nil {
		return
	}
	if err := d.voice.RecordDraftedSignal(ctx, voice.Profile.ID, voice.Version.ProfileVersion,
		voiceDraftRef(voice, anchor, draft), draft.Body); err != nil {
		d.logger().WarnContext(ctx, "voice draft signal not recorded", "err", err)
	}
}

func (d replyDrafter) recordVoiceRejection(ctx context.Context, voice draftvoice.Context, anchor ids.UUID, draft replyDraft) {
	if d.voice == nil {
		return
	}
	ref := voiceDraftRef(voice, anchor, draft)
	if err := d.voice.RecordDraftedSignal(ctx, voice.Profile.ID, voice.Version.ProfileVersion, ref, draft.Body); err != nil {
		d.logger().WarnContext(ctx, "voice rejection signal not recorded", "err", err)
		return
	}
	if _, err := d.voice.RejectDraft(ctx, voice.Profile.ID, ref); err != nil {
		d.logger().WarnContext(ctx, "voice rejection signal not recorded", "err", err)
	}
}

// replyDraftVoiceSystem replaces the no-voice guard when a profile block is
// supplied: the profile controls expression, never facts.
const replyDraftVoiceSystem draftSystem = `Draft a professional email reply on behalf of the CRM user's company, written in the user's own voice.
Return ONLY a JSON object: {"subject":"...","body":"..."}.
- The activity and stated intent are the authoritative reason for this reply.
- The supplied voice profile controls expression — rhythm, vocabulary, directness, structure — never facts.
- Use only facts present in the supplied data. Never invent customers, outcomes, prices, commitments, or capabilities.
- Obey the profile's avoid rules and the universal anti-AI rules; treat its style metrics as limits, not targets.`

// voiceBlockFor renders the voice profile block under the CALLING call's fence.
// The block is prepended to that call's user turn, so it must be bounded by the
// marker that call's system prompt declares — not one of its own.
type voiceBlockFor func(promptfence.Fence) string
