// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// Asking the model what the account needs, and holding its answer to the
// records it was given.
//
// The reply is a closed shape: a finding names a kind from four, cites ONE
// message by an id the model was handed, and quotes that message verbatim.
// Everything the reader sees is checked here — the id against the input, the
// quote against the message's own text — and a finding that fails is dropped
// whole. What survives is the same suggestion shape the 360's rules produce,
// so the page draws both with one component and a dismissal holds across
// both.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// maxFindings bounds what one read may raise. Past four the list is an
// inventory of the inbox, and the card has three rule rows to seat as well.
const maxFindings = 4

// The read kinds — the four a finding may carry — derived from the contract
// so a rename upstream fails here rather than laundering a string past the
// enum. The rule kinds are the 360's own and the model may not raise them.
var readKinds = []crmcontracts.Organization360SuggestionKind{
	crmcontracts.Organization360SuggestionKindCommitmentUnmet,
	crmcontracts.Organization360SuggestionKindQuestionUnanswered,
	crmcontracts.Organization360SuggestionKindRiskRaised,
	crmcontracts.Organization360SuggestionKindNeedRaised,
}

// What performing a finding means, in the reply's own words. `none` is a
// finding that advises without a button, the honest answer when neither a
// reply nor a task is the move.
const (
	actionDraftReply = "draft_reply"
	actionAddTask    = "add_task"
	actionNone       = "none"
)

const scanSystem = `You read one account's records for the rep who works it and say what needs a person.

The data is one JSON object. "account" is how the account stands: its contacts, open deals, open tasks and recent activity by subject. "messages" are the recent exchanges, oldest first, each with its own words; "direction" says who wrote it — "outbound" is us, "inbound" is them — and "unread_chars" says how much of a body was cut.

Raise a finding ONLY in these kinds:
- commitment_unmet — WE said we would do something, in an outbound message, and nothing in the account — no later message of ours, no open task — says it happened.
- question_unanswered — THEY asked something, in an inbound message, and no later outbound message answers it.
- risk_raised — they wrote something that puts the relationship or a deal at risk: a budget cut, a competitor, a decision-maker leaving, a delay on their side.
- need_raised — they wrote about a need, a plan or a purchase that nothing in the account has picked up: no open deal about it, no task.

Return ONLY a JSON object: {"findings":[{"kind":"commitment_unmet|question_unanswered|risk_raised|need_raised","title":"...","reason":"...","message_id":"...","quote":"...","action":"draft_reply|add_task|none"}]}.

Rules:
- At most four findings, the one that most needs a person first. Return {"findings":[]} when nothing does — that is a good answer, not a failure.
- "message_id" is the id of the ONE message the finding rests on, and "quote" is a verbatim excerpt of that message's "text", between 30 and 200 characters, copied exactly. Never paraphrase a quote and never quote a message you were not given; a finding whose quote is not in its message is dropped.
- "title" says what to do, in under eight words, starting with a verb. "reason" is one sentence saying what the message says and why it needs a person now. Plain words, addressed to the reader. Never put an id in a title or a reason.
- "action" is draft_reply when the move is writing back on that message, add_task when it is something to do that is not a reply, and none otherwise.
- Never invent a fact. A finding rests on words in a message, never on what a message does not say. If the account names sections_omitted, say nothing about those subjects at all — the reader is not allowed to see them.`

// scanSystemFor is the whole system prompt: the task, the house voice, the
// output language, and the fence rule that makes every message body data.
func scanSystemFor(fence promptfence.Fence, lang string) string {
	return scanSystem + "\n" + promptvoice.Rule + "\n" + promptlang.Rule(lang) + "\n" +
		fence.Rule("account records")
}

// ScanRequest is the one request the scan sends. Exported so the
// certification case issues the request production sends rather than a copy.
//
// The reply schema is built per call: the citation enum is THIS input's
// message ids, so a fabricated id fails the provider's own validation before
// the reply reaches the parser. With no messages there is nothing to cite and
// the caller does not ask at all.
func ScanRequest(in Input, lang string) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:                scanSystemFor(fence, lang),
		Messages:              []model.Message{{Role: "user", Content: fence.Wrap(encodeInput(in))}},
		IncludeCompanyContext: true,
		MaxTokens:             ai.ReasoningOutputMaxTokens,
		ResponseSchema:        findingsSchema(in),
		SecretStripper:        ai.NewSecretStripper(),
	}
}

func findingsSchema(in Input) json.RawMessage {
	messageIDs := make([]string, 0, len(in.Messages))
	for _, message := range in.Messages {
		messageIDs = append(messageIDs, message.ID.String())
	}
	kinds := make([]string, 0, len(readKinds))
	for _, kind := range readKinds {
		kinds = append(kinds, string(kind))
	}
	return schema.Must(schema.Object(map[string]schema.Node{
		"findings": schema.Array(schema.Object(map[string]schema.Node{
			"kind":       schema.Enum(kinds...),
			"title":      schema.String(),
			"reason":     schema.String(),
			"message_id": schema.Enum(messageIDs...),
			"quote":      schema.String(),
			"action":     schema.Enum(actionDraftReply, actionAddTask, actionNone),
		}, "kind", "title", "reason", "message_id", "quote", "action")),
	}, "findings"))
}

// encodeInput is the input as the model reads it. Marshalling a struct of
// strings, slices and times cannot fail, and a wrapped error here would be
// caught by nothing a caller could act on.
func encodeInput(in Input) string {
	encoded, _ := json.Marshal(in) //nolint:errchkjson // plain fields, cannot fail
	return string(encoded)
}

// LaneError is the lane breaking on an answer: it did not reply, or replied in
// a form the records do not support. Distinct from "no lane wired", which is a
// deployment reading its floor by design, and from a budget deferral, which
// is neither. A caller must not cache a lane failure over a good answer, and
// Cause says what the lane did so the log can, without the finding becoming
// the reader's problem.
type LaneError struct {
	Cause error
}

func (e *LaneError) Error() string { return "account scan lane: " + e.Cause.Error() }

func (e *LaneError) Unwrap() error { return e.Cause }

// Read asks the model and returns the grounded findings. A lane that breaks
// is returned as *LaneError; a budget deferral as the typed error the job
// carrier snoozes on; no lane or nothing to read as the deterministic floor.
func Read(
	ctx context.Context, lane Completer, orgID ids.OrganizationID, in Input, lang string,
) ([]crmcontracts.Organization360Suggestion, crmcontracts.WrittenBy, error) {
	if lane == nil || len(in.Messages) == 0 {
		return nil, crmcontracts.Deterministic, nil
	}
	resp, err := ai.Ask(ctx, lane, ScanRequest(in, lang), func(text string) error {
		_, refused, parseErr := ParseFindings(text, orgID, in)
		if parseErr != nil {
			return parseErr
		}
		if len(refused) > 0 {
			return errors.New(strings.Join(refused, "; "))
		}
		return nil
	})
	var deferral *ai.BudgetDeferralError
	if errors.As(err, &deferral) {
		return nil, crmcontracts.Deterministic, err
	}
	if err != nil {
		return nil, crmcontracts.Deterministic, &LaneError{Cause: err}
	}
	kept, _, err := ParseFindings(resp.Text, orgID, in)
	if err != nil {
		return nil, crmcontracts.Deterministic, &LaneError{Cause: err}
	}
	return kept, crmcontracts.Model, nil
}

// rawFinding is the reply's shape before anything is checked.
type rawFinding struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
	MessageID string `json:"message_id"`
	Quote     string `json:"quote"`
	Action    string `json:"action"`
}

// ParseFindings reads a reply and keeps what the records support.
//
// It returns the findings that ground, and — separately — one sentence per
// finding it refused, in the validator's words. The re-ask sends those back
// to the model; the final read keeps what grounded and drops the rest, so a
// reply that half-fabricated still yields its honest half. An unparseable
// reply, or one with no findings key, is an error: the model did not answer
// the question.
func ParseFindings(
	reply string, orgID ids.OrganizationID, in Input,
) (kept []crmcontracts.Organization360Suggestion, refused []string, err error) {
	var parsed struct {
		Findings *[]rawFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse the scan reply: %w", err)
	}
	if parsed.Findings == nil {
		return nil, nil, errors.New("the reply carries no findings key, so it did not answer the question")
	}
	seen := map[string]bool{}
	for i, raw := range *parsed.Findings {
		if len(kept) == maxFindings {
			break
		}
		suggestion, why := ground(raw, orgID, in)
		if why != "" {
			refused = append(refused, fmt.Sprintf("finding %d: %s", i+1, why))
			continue
		}
		if seen[suggestion.Fingerprint] {
			continue
		}
		seen[suggestion.Fingerprint] = true
		kept = append(kept, suggestion)
	}
	return kept, refused, nil
}

// ground turns one raw finding into a suggestion, or says why it cannot.
func ground(raw rawFinding, orgID ids.OrganizationID, in Input) (crmcontracts.Organization360Suggestion, string) {
	kind, ok := readKind(raw.Kind)
	if !ok {
		return crmcontracts.Organization360Suggestion{}, fmt.Sprintf("kind %q is not one the scan raises", clamp(raw.Kind))
	}
	message, ok := in.message(raw.MessageID)
	if !ok {
		return crmcontracts.Organization360Suggestion{}, fmt.Sprintf("message %q was not among the exchanges given", clamp(raw.MessageID))
	}
	if !claims.Quoted(message.Text, raw.Quote) {
		return crmcontracts.Organization360Suggestion{}, "the quote is not in the message it cites, verbatim"
	}
	title, reason := strings.TrimSpace(raw.Title), strings.TrimSpace(raw.Reason)
	if title == "" || reason == "" {
		return crmcontracts.Organization360Suggestion{}, "a finding needs both a title and a reason"
	}
	if claims.SpellsRecordID(title) || claims.SpellsRecordID(reason) {
		return crmcontracts.Organization360Suggestion{}, "an id appears in the title or the reason; ids belong in message_id only"
	}
	if raw.Action != actionDraftReply && raw.Action != actionAddTask && raw.Action != actionNone {
		return crmcontracts.Organization360Suggestion{}, fmt.Sprintf("action %q is not one the page performs", clamp(raw.Action))
	}
	quote := claims.CollapseSpace(raw.Quote)
	evidence := []crmcontracts.OrganizationBriefEvidence{{
		EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeActivity,
		EntityId:   openapi_types.UUID(message.ID),
		Name:       nonEmpty(message.Subject),
		Quote:      &quote,
		At:         &message.At,
		Origin:     ptr(origin(message)),
	}}
	by := crmcontracts.Model
	out := crmcontracts.Organization360Suggestion{
		Kind:        kind,
		Title:       &title,
		Reason:      reason,
		Evidence:    evidence,
		Fingerprint: org360.SuggestionFingerprint(string(kind), orgID.String(), evidence),
		WrittenBy:   &by,
		// The message's own date, like the no-reply rule's: when the words
		// were written, never a deadline the system chose.
		DueAt: &message.At,
	}
	switch raw.Action {
	case actionDraftReply:
		out.Action = org360.NewSuggestionAction(crmcontracts.Organization360SuggestionActionKindDraftReply)
		out.Action.ActivityId = &evidence[0].EntityId
	case actionAddTask:
		// The page writes the step from the body the finding carries, so the
		// sentence the reader accepted is the task they get. It hangs on the
		// account: the one record the scan is certain the finding is about.
		out.Action = org360.NewSuggestionAction(crmcontracts.Organization360SuggestionActionKindAddTask)
		body := org360.TaskBody(title, "organization", orgID.UUID)
		out.Action.Task = &body
	}
	return out, ""
}

func readKind(name string) (crmcontracts.Organization360SuggestionKind, bool) {
	for _, kind := range readKinds {
		if string(kind) == name {
			return kind, true
		}
	}
	return "", false
}

// origin says where a quote came from in the reader's terms: the channel,
// and who spoke. Server-authored like the rules' own, so the receipt under a
// model's finding and the one under a rule's row use one voice.
func origin(message MessageIn) string {
	channel := map[string]string{
		"email": "Email", "message": "Message", "call": "Call", "meeting": "Meeting",
	}[message.Kind]
	if channel == "" {
		channel = "Exchange"
	}
	switch message.Direction {
	case string(crmcontracts.ActivityDirectionOutbound):
		return channel + " you sent"
	case string(crmcontracts.ActivityDirectionInbound):
		return channel + " they sent"
	}
	return channel
}

// clamp bounds a model-authored token before it is echoed into a refusal, so
// a reply cannot inflate the message it is refused with.
func clamp(token string) string {
	const at = 60
	if len(token) <= at {
		return token
	}
	return token[:at] + "…"
}

func nonEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func ptr(v string) *string { return &v }
