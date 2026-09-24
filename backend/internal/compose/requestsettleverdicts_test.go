// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Every verdict this site will ACCEPT is a verdict its prompt TEACHES.
//
// `unsure` was accepted in four places and taught in none: the response
// schema's enum offered the token, the reply validator admitted it, the
// database CHECK stores it, and a certification scenario is built entirely
// around expecting it — while the system prompt defined "settled" and
// "still_owed" and never mentioned it at all.
//
// A model cannot answer a question it was not asked. Worse, the prompt pushed
// the other way: still_owed is defined as "did not address what they asked at
// all", which is exactly what a bare "Ok." looks like, so the one reading the
// prompt supports is the one the scenario marks wrong. Three of Gemma 4's nine
// failures on this task were that, and no model could have done better from
// this prompt.
//
// A gate rather than a fixed sentence, because the next verdict added to the
// enum will be added for the same reason and forgotten in the same place.
func TestEveryVerdictTheSiteAcceptsIsDefinedInItsPrompt(t *testing.T) {
	t.Parallel()
	prompt := settleSystem
	for verdict := range settleVerdicts {
		// Quoted, as the prompt names the other two: a verdict mentioned in
		// passing is not a verdict defined, and the quoted form is how this
		// prompt introduces one.
		if !strings.Contains(prompt, `"`+verdict+`"`) {
			t.Errorf("the reply validator accepts verdict %q and the prompt never defines it, so a "+
				"model is being asked for a token it was never taught", verdict)
		}
	}
}

// And the three the enum offers are exactly the three the validator admits.
//
// Held separately because the enum and the map are two lists of one thing: a
// verdict added to the schema and not the map reaches the model, comes back,
// and is refused as an unknown word after the call is paid for.
func TestTheSchemasVerdictsAreTheOnesTheValidatorAdmits(t *testing.T) {
	t.Parallel()
	declared := string(settleSchema())
	for verdict := range settleVerdicts {
		if !strings.Contains(declared, `"`+verdict+`"`) {
			t.Errorf("the validator admits %q and the response schema does not offer it", verdict)
		}
	}
	for _, verdict := range []string{activities.RequestSettled, activities.RequestStillOwed, activities.RequestUnsure} {
		if !settleVerdicts[verdict] {
			t.Errorf("the schema offers %q and the validator refuses it", verdict)
		}
	}
}

// A still_owed verdict names what is owed, or it is not an answer.
//
// The check was one-directional: prose on a settled or unsure verdict was
// refused, and a still_owed carrying NOTHING was accepted. So was everything
// else in the stack — the schema's required list was id/verdict/confidence, and
// the column's CHECK is `verdict = 'still_owed' OR (remaining IS NULL AND
// due_at IS NULL)`, which constrains the same one direction. Only the
// certification scenario asked for the phrase, and five of nine failures on
// this task were models answering still_owed with an empty remaining that
// nothing in production would have stopped.
//
// What that ships is a worklist row telling a rep they owe something and not
// what. The retry loop is the point of refusing it here: the model is told what
// is missing and asked again, which is the path a schema-invalid reply already
// takes.
func TestAStillOwedVerdictMustNameWhatIsOwed(t *testing.T) {
	t.Parallel()
	requestID := ids.NewV7()
	batch := []settleCandidate{{Request: activities.RepliedRequest{RequestID: requestID}}}
	id := requestID.String()

	for name, tc := range map[string]struct {
		result  settleResult
		refused bool
	}{
		"still_owed naming it": {
			settleResult{ID: id, Verdict: activities.RequestStillOwed, Remaining: "Send the quote", Confidence: 1}, false,
		},
		"still_owed naming nothing": {
			settleResult{ID: id, Verdict: activities.RequestStillOwed, Remaining: "", Confidence: 1}, true,
		},
		"still_owed with only spaces": {
			settleResult{ID: id, Verdict: activities.RequestStillOwed, Remaining: "   ", Confidence: 1}, true,
		},
		"settled needs no phrase": {
			settleResult{ID: id, Verdict: activities.RequestSettled, Remaining: "", Confidence: 1}, false,
		},
		"unsure needs no phrase": {
			settleResult{ID: id, Verdict: activities.RequestUnsure, Remaining: "", Confidence: 1}, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			msg := validateSettlePayload(settlePayload{Results: []settleResult{tc.result}}, batch)
			if tc.refused && msg == "" {
				t.Error("the reply was accepted and it names nothing this rep can act on")
			}
			if !tc.refused && msg != "" {
				t.Errorf("a well-formed reply was refused: %s", msg)
			}
		})
	}
}

// A reply that leaves `remaining` out violates the schema, not only the
// validator. A key the schema makes optional is one a constrained decoder may
// skip, and a still_owed that skipped it is then refused after the call is
// paid for; required, the decoder writes it — empty where the verdict is not
// still_owed, as the prompt says.
func TestAReplyMissingRemainingViolatesTheSchema(t *testing.T) {
	t.Parallel()
	missing := `{"results":[{"id":"r1","verdict":"still_owed","due_at":"","confidence":0.9}]}`
	if err := schema.ValidateJSON(settleSchema(), missing); err == nil {
		t.Error("the response schema admits a verdict with no remaining key, which the validator then refuses on still_owed")
	}
	present := `{"results":[{"id":"r1","verdict":"settled","remaining":"","due_at":"","confidence":0.9}]}`
	if err := schema.ValidateJSON(settleSchema(), present); err != nil {
		t.Errorf("a settled verdict with an empty remaining violates the schema: %v", err)
	}
}
