// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The model lane of the reply drafter: the request one draft call sends, the
// completion paths with their shared correct-and-retry loop, and the shape
// checks every served draft passes.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/draftrules"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// draftSystem names which of this task's system prompts a call is made under.
//
// A TYPE rather than a bare string, because this value chooses what the model
// is told it is doing: a first message sent under the reply prompt is told an
// activity is the authoritative reason for it, and there is no activity. The
// two are one task — same schema, same bounds, same rules block, same data
// boundary — differing in what evidence exists to write from, which is what a
// site is.
type draftSystem string

const replyDraftSystem draftSystem = `Draft a professional email reply on behalf of the CRM user's company.
Return ONLY a JSON object: {"subject":"...","body":"..."}.
- The activity and stated intent are the authoritative reason for this reply.
- Company context may improve positioning, relevant proof, and language, but never overrides the activity.
- Use only facts present in the supplied data. Never invent customers, outcomes, prices, commitments, or capabilities.
- Do not claim a personal writing style or voice unless a separate voice profile is supplied.`

// firstDraftSystem is the draft_reply/first site: a message that OPENS a
// conversation, written from the caller's stated intent and nothing else.
//
// It is a separate prompt rather than the reply's because the reply's is not
// true here. That one tells the model "the activity and stated intent are the
// authoritative reason for this reply" — and on this site there is no activity,
// no thread and no prior message. A prompt that names evidence which does not
// exist invites the model to supply it, which on a first message to a stranger
// is the one failure that cannot be edited out afterwards: an opening line
// referring to a conversation nobody had.
//
// The intent is stated as the WHOLE brief, positively, for the same reason. A
// caller's sentence is thin material and the honest instruction is to write
// from it rather than around it.
const firstDraftSystem draftSystem = `Draft the FIRST email of a new conversation, on behalf of the CRM user's company.
Return ONLY a JSON object: {"subject":"...","body":"..."}.
- Nothing has been sent or received yet. There is no thread, no earlier message and no shared history: never refer to one, and never open with a follow-up phrase.
- The stated intent is the whole brief. Write the message it describes; if it is thin, keep the message short rather than inventing a reason for it.
- Use only facts present in the supplied data. Never invent customers, outcomes, prices, commitments, or capabilities — and never a prior meeting, call or email.
- Do NOT write a sign-off or a sender name. A name you guessed would go out over the wrong signature.
- Say one thing and ask for one thing. Three short paragraphs at most.
- Do not claim a personal writing style or voice unless a separate voice profile is supplied.`

var replyDraftSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "required":["subject","body"],
  "properties":{
    "subject":{"type":"string","minLength":1,"maxLength":998},
    "body":{"type":"string","minLength":1,"maxLength":50000}
  }
}`)

type replyDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// replyDraftSystemFor assembles this call's system turn: what this surface is
// for, the rules every drafting surface shares, and THIS call's data boundary
// (see promptfence.Fence.Rule).
func replyDraftSystemFor(system draftSystem, fence promptfence.Fence) string {
	return string(system) + "\n\n" + draftrules.Shared + "\n" + fence.Rule("activity")
}

// voicedSite is a site's prompt with the voice rule added — the system turn a
// call carrying a profile block is made under.
//
// A function rather than a second constant per site, because the voiced turn is
// DERIVED from the plain one. Two spelled-out variants drift the moment a site's
// own rules change, and the first-message site's variant would then be a copy of
// the reply's that nobody remembered to update — which is precisely the shape
// this replaced.
func voicedSite(site draftSystem) draftSystem {
	return draftSystem(string(site) + "\n\n" + draftvoice.SystemRule)
}

// replyDraftRequest builds the one request a draft call sends, in whichever of
// this site's two system variants the call is made under. The workspace's Voice
// DNA state selects the variant per call — a loaded profile supplies a block and
// takes the voice prompt, no profile takes the plain one — and both remain the
// same invocation site: same schema, same bounds, same data boundary.
//
//promptvoice:exempt the reply is an email sent to a customer under the user's own name, and draftrules carries that user's voice; Margince's register belongs to what it says TO the user, never to what it writes AS them.
func replyDraftRequest(site draftSystem, activity replyActivityData, voiceBlock voiceBlockFor, correction string) (model.Request, error) {
	payload, err := json.Marshal(activity)
	if err != nil {
		return model.Request{}, fmt.Errorf("compose: encode reply activity context: %w", err)
	}
	// The activity is the counterparty's own text. It was safe here only by
	// accident — json.Marshal escapes "<" to \u003c, so a forged block marker
	// could not be spelled — and an accident of the encoder is not a boundary:
	// it goes the moment this block is rendered as text rather than JSON.
	fence := promptfence.New()
	system := site
	content := fence.Wrap(string(payload))
	if voiceBlock != nil {
		// The voice rule is ADDED to whichever site this call is made under,
		// never substituted for it. A swap carried the reply site's evidence
		// claim onto every voiced call, and on the first-message site that
		// claim is false: telling a model "the activity is the authoritative
		// reason for this reply" when there is no activity is an invitation to
		// invent the thread, which is the one error a first message to a
		// stranger cannot be edited back from.
		system = voicedSite(site)
		content = voiceBlock(fence) + "\n\n" + content
	}
	// The correction rides the USER turn and never the variant choice: it is
	// feedback about one attempt, and a plain draft told to fix a phrase must
	// stay a plain draft rather than silently becoming a voiced one.
	content += correction
	return model.Request{
		System: replyDraftSystemFor(system, fence),
		Messages: []model.Message{{
			Role:    chatRoleUser,
			Content: content,
		}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: replyDraftSchema,
		SecretStripper: ai.NewSecretStripper(),
	}, nil
}

func (d replyDrafter) complete(ctx context.Context, activity replyActivityData, voiceBlock voiceBlockFor) (replyDraft, error) {
	return d.completeWith(ctx, replyDraftSystem, activity, voiceBlock, "")
}

// completeWith is complete plus the correction a retry carries.
func (d replyDrafter) completeWith(ctx context.Context, site draftSystem, activity replyActivityData, voiceBlock voiceBlockFor, correction string) (replyDraft, error) {
	req, err := replyDraftRequest(site, activity, voiceBlock, correction)
	if err != nil {
		return replyDraft{}, err
	}

	resp, err := draftcore.CompleteChecked(ctx, d.brain, req, replyDraftShapeValid)
	if err != nil {
		return replyDraft{}, err
	}
	draft, err := parseReplyDraft(resp.Text)
	if err != nil {
		return replyDraft{}, err
	}
	if err := validateReplyDraft(draft); err != nil {
		return replyDraft{}, err
	}
	return draft, nil
}

// completeChecked drafts through the shared correct-and-retry loop, so the
// reply surface cannot drift from the two composers about what a rejected
// phrase is or how many chances the model gets to fix one.
//
// The voice block rides along unchanged: it selects the system variant, and the
// correction rides the user turn, so a plain draft told to fix a phrase stays a
// plain draft rather than silently becoming a voiced one.
func (d replyDrafter) completeChecked(ctx context.Context, site draftSystem, data replyActivityData, voiceBlock voiceBlockFor) (replyDraft, error) {
	return draftcore.CorrectOnce(ctx, data.Lang(), data.Band(),
		func(ctx context.Context, correction string) (replyDraft, error) {
			return d.completeWith(ctx, site, data, voiceBlock, correction)
		},
		// The reply site returns no reasoning: its schema is subject and body
		// alone, so there is no second channel to judge here.
		func(draft replyDraft) (string, []string) { return draft.Body, nil },
		func(draft replyDraft) (string, bool) { return draft.Subject, data.Threaded() },
		draftRetryLog{log: d.logger()},
	)
}

// draftRetryLog reports what the correction loop decided. A retry that does not
// help is invisible from the outside — the caller gets a draft either way — and
// "the model kept producing rejected phrasing" is the signal that says a phrase
// list or a prompt rule needs work.
type draftRetryLog struct{ log *slog.Logger }

func (l draftRetryLog) RetryFailed(ctx context.Context, findings int, err error) {
	l.log.WarnContext(ctx, "draft correction retry failed; serving the first draft",
		"findings", findings, "err", err)
}

func (l draftRetryLog) RetryDidNotClear(ctx context.Context, rule, phrase string, remaining int) {
	l.log.WarnContext(ctx, "draft still carries rejected phrasing after one retry",
		"rule", rule, "phrase", phrase, "remaining", remaining)
}

// parseReplyDraft reads one model reply as the draft it claims to be. The
// provider's own envelope comes off first: a reply is not malformed for having
// been returned inside one.
func parseReplyDraft(text string) (replyDraft, error) {
	var draft replyDraft
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &draft); err != nil {
		return replyDraft{}, fmt.Errorf("compose: reply draft response is not valid JSON: %w", err)
	}
	draft.Body = ai.PlainText(draft.Body)
	return draft, nil
}

// replyDraftShapeValid judges the answer the CALLER will get, so it reads it
// through the same parse. Judging the raw text instead would accept a body
// that is only non-empty because of markup the caller then strips away.
func replyDraftShapeValid(text string) error {
	draft, err := parseReplyDraft(text)
	if err != nil {
		return fmt.Errorf(`output must be {"subject":"...","body":"..."}: %w`, err)
	}
	return validateReplyDraft(draft)
}

func validateReplyDraft(draft replyDraft) error {
	if strings.TrimSpace(draft.Subject) == "" {
		return fmt.Errorf("compose: reply draft subject is empty")
	}
	if strings.ContainsAny(draft.Subject, "\r\n") {
		return fmt.Errorf("compose: reply draft subject contains a line break")
	}
	if strings.TrimSpace(draft.Body) == "" {
		return fmt.Errorf("compose: reply draft body is empty")
	}
	if len([]rune(draft.Subject)) > 998 || len([]rune(draft.Body)) > 50_000 {
		return fmt.Errorf("compose: reply draft exceeds the supported length")
	}
	return nil
}

func boundedRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
