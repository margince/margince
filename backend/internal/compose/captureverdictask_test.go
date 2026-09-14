// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The verdict payload validator is a security control, not a parsing
// convenience: it is what stops a model — or a sender who talked one into
// obliging — from answering about an address nobody asked about, answering
// twice, or inventing a verdict outside the closed set. Each rejection below is
// a distinct way the one-sender contract can be broken.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The request is the whole security perimeter of this site: the sender's own
// bytes reach the model unedited, and the only thing that stops them ending
// their span is a marker minted for THIS call and named in THIS call's system
// prompt. A request that fences under some other marker, or repeats the sender
// outside the span, hands the instruction region to whoever wrote the mail.
func TestVerdictRequestFencesTheSenderUnderTheMarkerItDeclares(t *testing.T) {
	row := capture.PendingCounterparty{
		ID:          ids.NewV7(),
		Email:       "stranger@prospect.example",
		DisplayName: "A Stranger",
		Subject:     "quote please",
		Body:        "We need forty seats by March.",
	}

	req := verdictRequest(row)

	marker, declared := promptfence.MarkerIn(req.System)
	if !declared {
		t.Fatalf("the verdict system prompt declares no data boundary: %q", req.System)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("got %d messages, want the single user turn", len(req.Messages))
	}
	content := req.Messages[0].Content
	openTag, closeTag := "<"+marker+` id="`+row.ID.String()+`">`, "</"+marker+">"
	openAt, closeAt := strings.Index(content, openTag), strings.Index(content, closeTag)
	if openAt < 0 || closeAt < openAt {
		t.Fatalf("the sender is not wrapped in the declared marker keyed by the row id:\n%s", content)
	}
	span := content[openAt+len(openTag) : closeAt]
	for _, sender := range []string{row.DisplayName, row.Email, row.Subject, row.Body} {
		if !strings.Contains(span, sender) {
			t.Errorf("sender text %q never reached the fenced span:\n%s", sender, content)
		}
		// Containment is a question of counts, not membership: a prompt that keeps
		// the fence and ALSO repeats the sender beside it puts that copy in the
		// instruction region while "is it inside?" stays true.
		if n := strings.Count(content, sender); n != 1 {
			t.Errorf("sender text %q appears %d times, want only the fenced one:\n%s", sender, n, content)
		}
	}
}

// A fence's scope is one call. A marker a previous sender was shown is a marker
// they can spell, so reusing one would give away the only thing they cannot
// forge.
func TestVerdictRequestMintsAFreshBoundaryPerCall(t *testing.T) {
	row := capture.PendingCounterparty{ID: ids.NewV7(), Email: "stranger@prospect.example"}

	first, declared := promptfence.MarkerIn(verdictRequest(row).System)
	if !declared {
		t.Fatal("the verdict system prompt declares no data boundary")
	}
	second, declared := promptfence.MarkerIn(verdictRequest(row).System)
	if !declared {
		t.Fatal("the second verdict system prompt declares no data boundary")
	}
	if first == second {
		t.Errorf("two verdict requests share the boundary %q", first)
	}
}

func TestValidateVerdictPayloadRejectsEveryBrokenBatchContract(t *testing.T) {
	asked := capture.PendingCounterparty{ID: ids.NewV7()}
	other := ids.NewV7()
	ok := verdictResult{ID: asked.ID.String(), Verdict: capture.KindContact, Confidence: 0.9}

	cases := []struct {
		name    string
		results []verdictResult
		wantMsg string
	}{
		{
			name:    "an exact answer is accepted",
			results: []verdictResult{ok},
		},
		{
			// An answer about someone who was not the subject of this call must
			// never be applied.
			name:    "an id nobody asked about",
			results: []verdictResult{{ID: other.String(), Verdict: capture.KindContact, Confidence: 0.9}},
			wantMsg: "was not requested",
		},
		{
			name:    "the same id answered twice",
			results: []verdictResult{ok, ok},
			wantMsg: "appears twice",
		},
		{
			// `unsure` is deliberately not in the vocabulary — abstention is
			// derived from the floor, never self-declared.
			name:    "a verdict outside the closed set",
			results: []verdictResult{{ID: asked.ID.String(), Verdict: capture.PendingStatusUnsure, Confidence: 0.9}},
			wantMsg: "is not one of " + strings.Join(verdictKindNames(), "|"),
		},
		{
			name:    "confidence above one",
			results: []verdictResult{{ID: asked.ID.String(), Verdict: capture.KindContact, Confidence: 1.5}},
			wantMsg: "outside [0,1]",
		},
		{
			name:    "confidence below zero",
			results: []verdictResult{{ID: asked.ID.String(), Verdict: capture.KindContact, Confidence: -0.1}},
			wantMsg: "outside [0,1]",
		},
		{
			// A silently dropped id would leave its row claimed but unjudged.
			name:    "a requested id left out",
			results: []verdictResult{},
			wantMsg: "is missing from the results",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateVerdictPayload(verdictPayload{Results: tc.results}, asked)
			if tc.wantMsg == "" {
				if got != "" {
					t.Fatalf("a valid payload was rejected: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantMsg) {
				t.Fatalf("got %q, want a message containing %q", got, tc.wantMsg)
			}
		})
	}
}

// Whatever the validator echoes back is MODEL output, which a sender who got the
// model to obey can choose. It reaches the operator's log, so it must be bounded
// — otherwise the log is a writing surface.
func TestValidationMessagesDoNotEchoUnboundedModelText(t *testing.T) {
	asked := capture.PendingCounterparty{ID: ids.NewV7()}
	flood := strings.Repeat("A", 100_000)
	msg := validateVerdictPayload(verdictPayload{
		Results: []verdictResult{{ID: flood, Verdict: capture.KindContact, Confidence: 0.9}},
	}, asked)

	if msg == "" {
		t.Fatal("an unrequested id was accepted")
	}
	if len(msg) > 500 {
		t.Fatalf("the validation message is %d bytes — model-chosen text must be clamped before it reaches a log", len(msg))
	}
}

// The bound model intermittently returns a confidence declared type: number as
// a JSON string. A verdict is a single-answer decision — there is no partial
// result to keep — so a reply that is right but quoted must be read, not
// deferred.
func TestVerdictPayloadReadsAQuotedConfidence(t *testing.T) {
	const reply = `{"results":[{"id":"0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a61","verdict":"spam","confidence":"0.94"}]}`

	var payload verdictPayload
	if err := json.Unmarshal([]byte(reply), &payload); err != nil {
		t.Fatalf("a quoted confidence failed to decode: %v", err)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(payload.Results))
	}
	if got := float64(payload.Results[0].Confidence); got != 0.94 {
		t.Errorf("Confidence = %v, want 0.94", got)
	}
}

// Tolerating the wrapper must not tolerate the value. The range gate lives in
// the validator, where the refusal can name what was wrong, so a quoted 1.4 is
// read and then rejected rather than being read as a number the floor accepts.
func TestVerdictPayloadStillRefusesAnOutOfRangeConfidence(t *testing.T) {
	asked := capture.PendingCounterparty{ID: ids.NewV7()}
	reply := `{"results":[{"id":"` + asked.ID.String() + `","verdict":"spam","confidence":"1.4"}]}`

	var payload verdictPayload
	if err := json.Unmarshal([]byte(reply), &payload); err != nil {
		t.Fatalf("a quoted confidence failed to decode: %v", err)
	}
	msg := validateVerdictPayload(payload, asked)
	if msg == "" {
		t.Fatal("a confidence of 1.4 was accepted; want a refusal")
	}
	if !strings.Contains(msg, "outside [0,1]") {
		t.Errorf("refusal %q does not say what was wrong with the value", msg)
	}
}

// The prompt says WHO REACHED WHOM, and it is the fact the judgment turns on.
//
// Every sender used to be rendered "From:", including addresses the mailbox
// owner had written TO. That is how a property manager the founder emailed was
// judged as a stranger writing in and became a workspace-visible contact: the
// model was told the desk had contacted the business, which it never had.
func TestTheVerdictPromptSaysWhichWayTheMessageWent(t *testing.T) {
	t.Parallel()
	outbound := capture.PendingCounterparty{
		ID: ids.NewV7(), Email: "desk@example.com", DisplayName: "Some Desk",
		Direction: "outbound", Subject: "Access card", Body: "Could you reissue it?",
	}

	prompt := promptTextOf(verdictRequest(outbound))

	if strings.Contains(prompt, "From: Some Desk") {
		t.Error("an address the owner WROTE TO is described as having written in")
	}
	if !strings.Contains(prompt, "To: Some Desk") {
		t.Errorf("the prompt does not say the owner wrote to this address:\n%s", prompt)
	}
	if !strings.Contains(prompt, "never written back") {
		t.Error("the prompt does not say the address has never answered, which is what " +
			"separates an intention from a relationship")
	}
}

// An address that HAS answered is described as one, so the model is not told to
// discount a real correspondent.
func TestTheVerdictPromptSaysWhenAnAddressHasAnswered(t *testing.T) {
	t.Parallel()
	answered := capture.PendingCounterparty{
		ID: ids.NewV7(), Email: "desk@example.com", DisplayName: "Some Desk",
		Direction: "outbound", WroteBack: true,
	}

	prompt := promptTextOf(verdictRequest(answered))

	if strings.Contains(prompt, "never written back") {
		t.Error("an address that answered us is reported as never having written back")
	}
	if !strings.Contains(prompt, "has written back") {
		t.Errorf("the prompt does not report the answer we already have:\n%s", prompt)
	}
}

// A row written before the direction was recorded says nothing rather than
// guessing. Reading an empty direction as inbound would put the old defect back
// for exactly the rows that carry it.
func TestAnUnknownDirectionClaimsNeitherWay(t *testing.T) {
	t.Parallel()
	old := capture.PendingCounterparty{
		ID: ids.NewV7(), Email: "desk@example.com", DisplayName: "Some Desk",
	}

	prompt := promptTextOf(verdictRequest(old))

	if strings.Contains(prompt, "From: Some Desk") || strings.Contains(prompt, "To: Some Desk") {
		t.Errorf("a row with no recorded direction is claimed to have one:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Some Desk") {
		t.Error("the sender is not named at all")
	}
}

// promptTextOf reads the user message out of a built request.
func promptTextOf(req model.Request) string {
	var out strings.Builder
	for _, message := range req.Messages {
		out.WriteString(message.Content)
	}
	return out.String()
}
