// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The model lane of the reply drafter: the request one draft call sends, the
// completion paths with their shared correct-and-retry loop, and the shape
// checks every served draft passes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/draftrules"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const replyDraftSystem = `Draft a professional email reply, for the sender to send under their own name.
Return ONLY a JSON object: {"subject":"...","body":"..."}.
- The selected activity and stated intent are the authoritative reason for this reply. Answer that exact message.
- No other thread message is supplied. Use the selected subject and body, steered by intent and rewrite_of. A selected message written by our side calls for a follow-up to its recipient, not an answer to ourselves.
- Where the message raises several distinct points, answer at least two by name, a clause each, before the ask.
- Use only facts present in the supplied data. Never invent customers, outcomes, prices, commitments, or capabilities.
- Do not claim a personal writing style or voice unless a separate voice profile is supplied.
` + draftrules.Rewrite

// replyDraftSchema is the answer's shape, subject first: the order the model
// writes it in.
//
// It carries no length bound: Anthropic refuses a schema holding minLength or
// maxLength and the strict profile does not take one. validateReplyDraft holds
// the bounds, from the constants below.
var replyDraftSchema = schema.Must(schema.Record(
	schema.Field("subject", schema.String()),
	schema.Field("body", schema.String()),
))

const (
	// replyDraftSubjectMaxRunes is RFC 5322's line limit: a Subject header is
	// one line, and a longer one is folded or refused by the sending server.
	replyDraftSubjectMaxRunes = 998
	// replyDraftBodyMaxRunes bounds a body far above any email a sender
	// writes, so it only ever catches a runaway generation.
	replyDraftBodyMaxRunes = 50_000
)

type replyDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// replyDraftSystemFor assembles the system turn for one call. It carries what
// this surface is for, the voice rule when voiced, the rules every drafting
// surface shares, and this call's data boundary (promptfence.Fence.Rule).
func replyDraftSystemFor(fence promptfence.Fence, voiced bool) string {
	system := replyDraftSystem
	if voiced {
		system += "\n\n" + draftvoice.SystemRule
	}
	return system + "\n\n" + draftrules.Shared + "\n" + fence.Rule("activity")
}

// replyDraftRequest builds the one request a draft call sends, in whichever of
// this site's two system variants the call is made under. The workspace's Voice
// DNA state selects the variant per call — a loaded profile supplies a block and
// takes the voice prompt, no profile takes the plain one — and both remain the
// same invocation site: same schema, same bounds, same data boundary.
//
//promptvoice:exempt the reply is an email sent to a customer under the user's own name, and draftrules carries that user's voice; Margince's register belongs to what it says TO the user, never to what it writes AS them.
func replyDraftRequest(activity replyActivityData, voiceBlock voiceBlockFor, correction string) (model.Request, error) {
	payload, err := json.Marshal(activity)
	if err != nil {
		return model.Request{}, fmt.Errorf("compose: encode reply activity context: %w", err)
	}
	// The activity is the counterparty's own text. It was safe here only by
	// accident — json.Marshal escapes "<" to \u003c, so a forged block marker
	// could not be spelled — and an accident of the encoder is not a boundary:
	// it goes the moment this block is rendered as text rather than JSON.
	fence := promptfence.New()
	content := fence.Wrap(string(payload))
	if voiceBlock != nil {
		content = voiceBlock(fence) + "\n\n" + content
	}
	// The correction rides the USER turn and never the variant choice: it is
	// feedback about one attempt, and a plain draft told to fix a phrase must
	// stay a plain draft rather than silently becoming a voiced one.
	content += correction
	return model.Request{
		System: replyDraftSystemFor(fence, voiceBlock != nil),
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
	return d.completeWith(ctx, activity, voiceBlock, "")
}

// completeWith is complete plus the correction a retry carries.
func (d replyDrafter) completeWith(ctx context.Context, activity replyActivityData, voiceBlock voiceBlockFor, correction string) (replyDraft, error) {
	req, err := replyDraftRequest(activity, voiceBlock, correction)
	if err != nil {
		return replyDraft{}, err
	}

	resp, err := ai.Ask(ctx, d.brain, req, replyDraftShapeValid)
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
func (d replyDrafter) completeChecked(ctx context.Context, data replyActivityData, voiceBlock voiceBlockFor) (replyDraft, error) {
	record := draftcheck.Grounds{
		Booked: data.Booked(), Met: draftcheck.IntentNamesMeeting(data.Intent),
		FirstName: data.Recipient, LastName: data.RecipientLastName,
	}
	draft, err := draftcore.CorrectOnce(ctx, data.Lang(), data.Band(), record,
		func(ctx context.Context, correction string) (replyDraft, error) {
			return d.completeWith(ctx, data, voiceBlock, correction)
		},
		// The reply site returns no reasoning: its schema is subject and body
		// alone, so there is no second channel to judge here.
		func(draft replyDraft) (string, []string) { return draft.Body, nil },
		func(draft replyDraft) (string, bool) { return draft.Subject, data.Threaded() },
		draftRetryLog{log: d.logger()},
	)
	return draft, err
}

// greeted is the draft with the floor's greeting added if it still opens
// without one. Applied once a reply draft is settled, after every voice check,
// so the floor never judges a line the model did not write.
//
// The greeting lengthens a body every earlier check already passed, so the
// served draft is held to the bounds again; a refusal sends the caller to its
// deterministic draft.
func (data replyActivityData) greeted(draft replyDraft) (replyDraft, error) {
	draft.Body = draftcheck.EnsureGreeting(draft.Body, data.Envelope,
		draftcheck.Grounds{FirstName: data.Recipient, LastName: data.RecipientLastName})
	if err := validateReplyDraft(draft); err != nil {
		return replyDraft{}, err
	}
	return draft, nil
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

func (l draftRetryLog) RetryDidNotClear(ctx context.Context, rule draftcheck.Rule, phrase string, remaining int, servedRetry bool) {
	// The SEVERITY is logged beside the rule, because "the retry did not clear"
	// is a different event for a false claim than for a phrasing tic, and an
	// operator scanning these lines should not have to know every rule by name
	// to tell them apart.
	l.log.WarnContext(ctx, "draft still carries a rejected phrase after one retry",
		"rule", rule.Name(), "severity", severityName(rule.Severity()),
		"phrase", phrase, "remaining_rules", remaining, "served", servedAttempt(servedRetry))
}

// servedAttempt names which draft the loop served, for the same log line.
func servedAttempt(retry bool) string {
	if retry {
		return "retry"
	}
	return "first"
}

// severityName renders a severity for a log line. A word rather than the
// integer, because an operator reading `severity=1` learns nothing and the
// ordering is an implementation detail of the comparison.
func severityName(s draftcheck.Severity) string {
	if s == draftcheck.Claim {
		return "claim"
	}
	return "style"
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
		return errors.New("compose: reply draft subject is empty")
	}
	if strings.ContainsAny(draft.Subject, "\r\n") {
		return errors.New("compose: reply draft subject contains a line break")
	}
	if strings.TrimSpace(draft.Body) == "" {
		return errors.New("compose: reply draft body is empty")
	}
	if len([]rune(draft.Subject)) > replyDraftSubjectMaxRunes || len([]rune(draft.Body)) > replyDraftBodyMaxRunes {
		return errors.New("compose: reply draft exceeds the supported length")
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
