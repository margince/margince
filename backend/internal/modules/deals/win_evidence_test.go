// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The closed vocabulary is what makes the escape hatch honest: a free-text
// answer cannot be counted, and counting them is the entire reason the exit
// exists rather than a hard requirement nobody could satisfy.
func TestAReasonOutsideTheVocabularyIsRefused(t *testing.T) {
	err := validateWonReason("because I said so", nil)

	var invalid *InvalidWonReasonError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidWonReasonError", err)
	}
	for _, allowed := range WonWithoutContractReasons {
		if !strings.Contains(invalid.Error(), allowed) {
			t.Errorf("the refusal does not name %q, so a caller cannot tell what to send", allowed)
		}
	}
}

// "Other" is the member that explains nothing on its own, which is the state
// this whole feature exists to refuse.
func TestOtherWithoutDetailIsRefused(t *testing.T) {
	blank := "   "
	for name, detail := range map[string]*string{
		"absent": nil,
		"blank":  &blank,
	} {
		t.Run(name, func(t *testing.T) {
			var needsDetail *WonReasonDetailRequiredError
			if !errors.As(validateWonReason("other", detail), &needsDetail) {
				t.Error("an unexplained \"other\" was accepted; it answers the report with nothing")
			}
		})
	}
}

func TestEveryVocabularyMemberIsAccepted(t *testing.T) {
	detail := "closed on a framework call-off"
	for _, reason := range WonWithoutContractReasons {
		var supplied *string
		if reason == reasonRequiringDetail {
			supplied = &detail
		}
		if err := validateWonReason(reason, supplied); err != nil {
			t.Errorf("%s: err = %v, want nil", reason, err)
		}
	}
}

// The refusal a caller meets when a win claims nothing must name BOTH ways
// forward. A refusal that says only "no" leaves them guessing which of the two
// the product wanted, and guessing produces the fabricated contract this rule
// exists to prevent.
func TestTheRefusalNamesBothWaysForward(t *testing.T) {
	message := (&WinEvidenceMissingError{}).Error()

	if !strings.Contains(message, "signed contract") {
		t.Errorf("the refusal does not mention attaching a contract: %q", message)
	}
	if !strings.Contains(message, "reason") {
		t.Errorf("the refusal does not mention stating a reason: %q", message)
	}
	for _, reason := range WonWithoutContractReasons {
		if !strings.Contains(message, reason) {
			t.Errorf("the refusal does not name the %q option: %q", reason, message)
		}
	}
}

// A stated reason is accepted without looking for paper: somebody who has told
// the product there is none should not then be told there is none.
func TestAStatedReasonNeedsNoContractLookup(t *testing.T) {
	reason := "purchase_order"
	in := AdvanceDealInput{WonWithoutContractReason: &reason}

	// A nil transaction proves the point: reaching the database here would
	// panic, so passing means the contract lookup was never attempted.
	if err := ensureWinEvidence(t.Context(), nil, ids.New[ids.DealKind](), in); err != nil {
		t.Fatalf("a stated reason was refused: %v", err)
	}
}

// A detail made of invisible characters explains exactly nothing, which is the
// state the reason vocabulary exists to refuse. A zero-width space is not
// whitespace to Go's TrimSpace and not whitespace to Postgres's btrim either,
// so it would otherwise satisfy both the Go check and the column's CHECK.
func TestADetailOfInvisibleCharactersIsRefused(t *testing.T) {
	for name, detail := range map[string]string{
		"zero-width space":   "\u200b",
		"non-breaking space": " ",
		"tab":                "\t",
		"newline":            "\n",
		"soft hyphen":        "\u00ad",
	} {
		t.Run(name, func(t *testing.T) {
			var needsDetail *WonReasonDetailRequiredError
			if !errors.As(validateWonReason("other", &detail), &needsDetail) {
				t.Errorf("%q was accepted as an explanation", detail)
			}
		})
	}
}

func TestADetailWithRealWordsIsAccepted(t *testing.T) {
	detail := "closed on a framework call-off"
	if err := validateWonReason("other", &detail); err != nil {
		t.Errorf("a real explanation was refused: %v", err)
	}
}

// A draft contract has asserted nothing — it is the state an agreement is born
// in — so paper stapled to one is the unsigned template the gate exists to
// refuse. The query must say so, or the gate's hardest case passes.
func TestTheEvidenceQueryRefusesADraftContract(t *testing.T) {
	if !strings.Contains(evidenceQuery, "c.status <> 'draft'") {
		t.Error("the evidence query admits a draft contract; an unsigned template would satisfy the gate")
	}
	if !strings.Contains(evidenceQuery, "c.signed_on IS NOT NULL") {
		t.Error("the evidence query does not require a signed date")
	}
	if !strings.Contains(evidenceQuery, "a.archived_at IS NULL") {
		t.Error("the evidence query admits an archived attachment; archive leaves the row in place")
	}
	if !strings.Contains(evidenceQuery, "doc_state IN ('current', 'final')") {
		t.Error("the evidence query admits a draft document")
	}
}

// The contract's maxLength on this field is a promise the SCHEMA makes and the
// generated server does not keep: it validates no string length, and the column
// is plain `text` whose only CHECK is the "other needs a detail" rule. So a
// documented bound nobody applied was worse than no bound at all — a client
// trusts it, stops truncating, and the value lands anyway.
func TestADetailPastTheContractsBoundIsRefused(t *testing.T) {
	over := strings.Repeat("x", maxWonReasonDetail+1)
	var tooLong *WonReasonDetailTooLongError
	if !errors.As(validateWonReason("other", &over), &tooLong) {
		t.Fatalf("a %d-character detail was accepted against a %d bound",
			len(over), maxWonReasonDetail)
	}
	if tooLong.Length != maxWonReasonDetail+1 {
		t.Errorf("the refusal reports %d characters, want %d — a caller shortening it needs the real number",
			tooLong.Length, maxWonReasonDetail+1)
	}
	field, code, _ := tooLong.FieldFault()
	if field != "won_without_contract_detail" || code != "too_long" {
		t.Errorf("field fault = (%q, %q), want (won_without_contract_detail, too_long)", field, code)
	}

	// Exactly at the bound goes: an off-by-one here refuses a value the
	// contract advertises as legal.
	at := strings.Repeat("x", maxWonReasonDetail)
	if err := validateWonReason("other", &at); err != nil {
		t.Errorf("a detail of exactly %d characters was refused: %v", maxWonReasonDetail, err)
	}
}

// Counted in RUNES, because the contract's maxLength is. A caller who wrote 500
// characters of German must not be refused for the bytes their umlauts cost.
func TestTheDetailsBoundCountsCharactersRatherThanBytes(t *testing.T) {
	umlauts := strings.Repeat("ü", maxWonReasonDetail)
	if len(umlauts) <= maxWonReasonDetail {
		t.Fatal("the fixture is not multi-byte, so it proves nothing about counting")
	}
	if err := validateWonReason("other", &umlauts); err != nil {
		t.Errorf("%d characters of German were refused for their byte length: %v", maxWonReasonDetail, err)
	}
}

// A detail supplied alongside ANY reason is bounded: a caller may send one with
// a reason that does not require it, and the column takes whatever arrives.
func TestTheBoundHoldsForAReasonThatNeedsNoDetail(t *testing.T) {
	over := strings.Repeat("x", maxWonReasonDetail+1)
	var tooLong *WonReasonDetailTooLongError
	if !errors.As(validateWonReason("purchase_order", &over), &tooLong) {
		t.Error("an over-long detail rode in on a reason that requires none")
	}
}

// The bound belongs to the COLUMN, so it is asked before the branch that looks
// for a reason.
//
// Every test above drives validateWonReason, which is only reached when the
// caller states a reason — and that is exactly how the gap survived them. A win
// that sends a detail and NO reason takes the contract-lookup branch, and
// deal_advance.go writes both fields on every won landing, so the detail landed
// unmeasured however long it was.
//
// The transaction is nil on purpose. If the bound is asked before the branch,
// the refusal returns without a database ever being reached; if it moves back
// behind the reason arm, this panics rather than quietly passing.
func TestAnOverlongDetailIsRefusedEvenWhenNoReasonIsStated(t *testing.T) {
	over := strings.Repeat("x", maxWonReasonDetail+1)

	var tooLong *WonReasonDetailTooLongError
	err := ensureWinEvidence(context.Background(), nil, ids.DealID{UUID: ids.NewV7()},
		AdvanceDealInput{WonWithoutContractDetail: &over})
	if !errors.As(err, &tooLong) {
		t.Fatalf("a %d-character detail with no stated reason was admitted (%v) — a win with a signed contract would have written it, past the bound the schema advertises",
			maxWonReasonDetail+1, err)
	}

	// A detail within the bound and no reason falls through to the contract
	// lookup, which is the ordinary path. Asserting the refusal alone would
	// pass against a gate that refused every such win.
	within := strings.Repeat("x", maxWonReasonDetail)
	if err := ensureDetailWithinBound(&within); err != nil {
		t.Errorf("a detail of exactly %d characters was refused: %v", maxWonReasonDetail, err)
	}
}
