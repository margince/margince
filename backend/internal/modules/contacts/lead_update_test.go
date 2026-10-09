// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// leadWithOverride is a lead whose score 90 is a human override; the
// machine value 23 is retained in score_computed.
func leadWithOverride() crmcontracts.Lead {
	return crmcontracts.Lead{Score: 90, ScoreOverrideReason: new("board-level sponsor"), ScoreComputed: new(23)}
}

func TestScoreOverrideNullClearsAndResumesRecompute(t *testing.T) {
	p := storekit.NewPatch()
	resume, err := applyScoreOverride(p, leadWithOverride(), UpdateLeadInput{ClearScoreOverride: true})
	if err != nil {
		t.Fatalf("clearing via null: %v", err)
	}
	if !resume {
		t.Fatal("clearing the override must resume recompute")
	}
	after := p.After()
	if after["score_override_reason"] != nil {
		t.Fatalf("reason not cleared: %v", after["score_override_reason"])
	}
	if after["score"] != 23 {
		t.Fatalf("score must track the retained machine value: %v, want 23", after["score"])
	}
	if after["score_computed"] != nil {
		t.Fatalf("score_computed must clear on resume: %v", after["score_computed"])
	}
}

func TestScoreOverrideNullWithoutOverrideIsANoOp(t *testing.T) {
	p := storekit.NewPatch()
	resume, err := applyScoreOverride(p, crmcontracts.Lead{Score: 23}, UpdateLeadInput{ClearScoreOverride: true})
	if err != nil {
		t.Fatalf("clearing with nothing in force: %v", err)
	}
	if resume {
		t.Fatal("nothing was cleared, so recompute must not be forced")
	}
	if !p.Empty() {
		t.Fatalf("no-op clear must patch nothing: %v", p.After())
	}
}

func TestScoreOverrideSetDemandsAWrittenReason(t *testing.T) {
	for name, in := range map[string]UpdateLeadInput{
		"reason absent":        {Score: new(90)},
		"reason explicit null": {Score: new(90), ClearScoreOverride: true},
		"reason empty":         {Score: new(90), ScoreOverrideReason: new("")},
		"reason blank":         {Score: new(90), ScoreOverrideReason: new("   ")},
	} {
		p := storekit.NewPatch()
		_, err := applyScoreOverride(p, crmcontracts.Lead{Score: 23}, in)
		if _, ok := errors.AsType[*ScoreOverrideReasonRequiredError](err); !ok {
			t.Errorf("%s: got %v, want ScoreOverrideReasonRequiredError", name, err)
		}
	}
}

func TestScoreOverrideEmptyStringReasonIsInvalidNotAClear(t *testing.T) {
	p := storekit.NewPatch()
	_, err := applyScoreOverride(p, leadWithOverride(), UpdateLeadInput{ScoreOverrideReason: new("")})
	if _, ok := errors.AsType[*ScoreOverrideReasonEmptyError](err); !ok {
		t.Fatalf("empty-string reason: got %v, want ScoreOverrideReasonEmptyError", err)
	}
	if !p.Empty() {
		t.Fatalf("a rejected input must patch nothing: %v", p.After())
	}
}

func TestScoreOverrideNullScoreWithWrittenReasonIsContradictory(t *testing.T) {
	p := storekit.NewPatch()
	_, err := applyScoreOverride(p, leadWithOverride(),
		UpdateLeadInput{ClearScoreOverride: true, ScoreOverrideReason: new("still strategic")})
	if _, ok := errors.AsType[*ScoreOverrideClearConflictError](err); !ok {
		t.Fatalf("null score + written reason: got %v, want ScoreOverrideClearConflictError", err)
	}
}

func TestScoreOverrideSetRetainsMachineValueOnce(t *testing.T) {
	// First override: the machine value moves into score_computed.
	p := storekit.NewPatch()
	resume, err := applyScoreOverride(p, crmcontracts.Lead{Score: 23},
		UpdateLeadInput{Score: new(90), ScoreOverrideReason: new("board-level sponsor")})
	if err != nil || resume {
		t.Fatalf("first override: err=%v resume=%v", err, resume)
	}
	if after := p.After(); after["score"] != 90 || after["score_computed"] != 23 {
		t.Fatalf("first override must retain the machine value: %v", after)
	}

	// Refreshing an override in force must NOT clobber the retained value.
	p = storekit.NewPatch()
	if _, err := applyScoreOverride(p, leadWithOverride(),
		UpdateLeadInput{Score: new(95), ScoreOverrideReason: new("expanded scope")}); err != nil {
		t.Fatalf("refreshing the override: %v", err)
	}
	if _, touched := p.After()["score_computed"]; touched {
		t.Fatalf("refresh must not clobber score_computed: %v", p.After())
	}
}

func TestScoreOverrideAmendsReasonOnlyWhenOneIsInForce(t *testing.T) {
	p := storekit.NewPatch()
	if _, err := applyScoreOverride(p, leadWithOverride(), UpdateLeadInput{ScoreOverrideReason: new("updated note")}); err != nil {
		t.Fatalf("amending the note: %v", err)
	}
	if p.After()["score_override_reason"] != "updated note" {
		t.Fatalf("note not amended: %v", p.After())
	}

	// A reason with no score to attach it to: the MISSING input is the score,
	// so the refusal must name the score. Telling the caller to supply a
	// score_override_reason it already sent is guidance it cannot act on.
	p = storekit.NewPatch()
	_, err := applyScoreOverride(p, crmcontracts.Lead{Score: 23}, UpdateLeadInput{ScoreOverrideReason: new("a reason for nothing")})
	var want *ScoreOverrideWithoutScoreError
	if !errors.As(err, &want) {
		t.Fatalf("reason without a score or override: got %v, want ScoreOverrideWithoutScoreError", err)
	}
	if field, _, _ := want.FieldFault(); field != leadScoreField {
		t.Errorf("FieldFault names %q, want the score the caller must supply", field)
	}
}

// The wire gesture: JSON null is CLEAR, omission is leave-alone, and the
// generated contract struct cannot say which — LeadUpdateRequest must.
func TestLeadUpdateRequestKeepsNullDistinctFromAbsent(t *testing.T) {
	cases := map[string]struct {
		body      string
		wantClear bool
	}{
		"both omitted": {`{"full_name":"Vera"}`, false},
		"reason null":  {`{"score_override_reason":null}`, true},
		"score null":   {`{"score":null}`, true},
		"both null":    {`{"score":null,"score_override_reason":null}`, true},
		"override set": {`{"score":90,"score_override_reason":"sponsor"}`, false},
		"reason amend": {`{"score_override_reason":"sponsor"}`, false},
	}
	for name, tc := range cases {
		var req LeadUpdateRequest
		if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		in := leadUpdateInput(req, new(int64(1)))
		if in.ClearScoreOverride != tc.wantClear {
			t.Errorf("%s: ClearScoreOverride = %v, want %v", name, in.ClearScoreOverride, tc.wantClear)
		}
	}

	// The embedded contract fields still decode through the wrapper.
	var req LeadUpdateRequest
	if err := json.Unmarshal([]byte(`{"score":90,"score_override_reason":"sponsor","full_name":"Vera"}`), &req); err != nil {
		t.Fatal(err)
	}
	in := leadUpdateInput(req, nil)
	if in.Score == nil || *in.Score != 90 || in.ScoreOverrideReason == nil || *in.ScoreOverrideReason != "sponsor" ||
		in.FullName == nil || *in.FullName != "Vera" {
		t.Fatalf("wrapper dropped embedded fields: %+v", in)
	}
}
