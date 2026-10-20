// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestMalformedCommercialPayloadHasAnExplicitIncompletePreview(t *testing.T) {
	for _, payload := range []string{`{"operation":"updateCompany","body":"not fields"}`, `{"fields":["owner_id"]}`} {
		var args map[string]any
		if err := json.Unmarshal([]byte(payload), &args); err != nil {
			t.Fatal(err)
		}
		preview, err := (approvalPreviewReader{}).read(context.Background(), crmcontracts.Approval{Status: "pending", Kind: "update_record", ProposedChange: &args})
		if err != nil {
			t.Fatal(err)
		}
		if preview == nil || preview.State != "incomplete" || preview.Changes == nil || len(preview.Changes) != 0 {
			t.Fatalf("preview=%+v", preview)
		}
	}
}

func TestCommercialPreviewNormalizesTheStagedEffect(t *testing.T) {
	cases := []struct{ name, kind, payload, field, want string }{
		{"tool owner", "update_record", `{"id":"record","fields":{"owner_id":"next"}}`, "owner_id", `"next"`},
		{"REST owner", "update_record", `{"operation":"updateDeal","path":"/v1/deals/record","body":{"owner_id":"next"}}`, "owner_id", `"next"`},
		{"automation owner", "assign_owner", `{"owner_id":"next"}`, "owner_id", `"next"`},
		{"tool stage", "advance_deal", `{"deal_id":"record","to_stage_id":"next"}`, "stage_id", `"next"`},
		{"REST stage", "advance_deal", `{"operation":"advanceDeal","path":"/v1/deals/record/advance","body":{"to_stage_id":"next"}}`, "stage_id", `"next"`},
		{"lifecycle", "lifecycle_change", `{"company_id":"record","current_lifecycle":"customer","proposed_lifecycle":"former_customer"}`, "lifecycle", `"former_customer"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.payload), &args); err != nil {
				t.Fatal(err)
			}
			changes, covered, err := commercialChanges(crmcontracts.Approval{Kind: tc.kind, ProposedChange: &args})
			if err != nil {
				t.Fatal(err)
			}
			if !covered || len(changes) != 1 || string(changes[tc.field]) != tc.want {
				t.Fatalf("changes=%s, covered=%v", changes, covered)
			}
		})
	}
}

func TestCommercialPreviewKeepsAdditionalEffects(t *testing.T) {
	var args map[string]any
	if err := json.Unmarshal([]byte(`{"deal_id":"record","to_stage_id":"next","note":"Call the buyer","lost_reason":"No budget"}`), &args); err != nil {
		t.Fatal(err)
	}
	changes, covered, err := commercialChanges(crmcontracts.Approval{Kind: "progress_deal", ProposedChange: &args})
	if err != nil {
		t.Fatal(err)
	}
	if !covered || len(changes) != 3 || string(changes["note"]) != `"Call the buyer"` || string(changes["lost_reason"]) != `"No budget"` {
		t.Fatalf("changes=%s, covered=%v", changes, covered)
	}
}

func TestCommercialPreviewPreservesScalarValues(t *testing.T) {
	for _, tc := range []struct{ raw, kind, text string }{
		{`12345678901234567890`, "number", "12345678901234567890"},
		{`false`, "boolean", "false"},
		{`null`, "empty", ""},
		{`"Budget approved"`, "text", "Budget approved"},
		{`{"city": "Berlin"}`, "structured", `{"city":"Berlin"}`},
		{`["example.test"]`, "structured", `["example.test"]`},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			value, err := (approvalPreviewReader{}).value(context.Background(), "description", json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if value.Kind != tc.kind || value.Text != tc.text {
				t.Fatalf("value=%+v", value)
			}
		})
	}
}

func TestInvalidTransitionIsIncompleteRatherThanAnInboxFailure(t *testing.T) {
	for _, err := range []error{&deals.LostReasonRequiredError{}, &deals.StagePipelineMismatchError{}, &deals.MissingFxRateError{From: "EUR", To: "USD"}} {
		if !previewUnavailable(err) {
			t.Fatalf("validation became an inbox error: %v", err)
		}
	}
	if previewUnavailable(errors.New("database disconnected")) {
		t.Fatal("infrastructure failure was hidden")
	}
}

func TestDecidedApprovalsDoNotAskForAnotherDecision(t *testing.T) {
	for _, status := range []crmcontracts.ApprovalStatus{"approved", "rejected", "expired"} {
		preview, err := (approvalPreviewReader{}).read(context.Background(), crmcontracts.Approval{Kind: "advance_deal", Status: status})
		if err != nil || preview != nil {
			t.Fatalf("decided preview: %+v, %v", preview, err)
		}
	}
}

func TestUnsupportedSignalCitationsCannotDisappearFromAReadyPreview(t *testing.T) {
	for _, source := range []string{"document", "unknown", "activity"} {
		t.Run(source, func(t *testing.T) {
			citation := crmcontracts.SignalEvidence{SourceType: new(crmcontracts.SignalEvidenceSourceType(source))}
			if source != "activity" {
				citation.SourceId = new("01900000-0000-7000-8000-000000000001")
			}
			evidence, err := (approvalPreviewReader{}).signalActivityEvidence(context.Background(), crmcontracts.Signal{Evidence: []crmcontracts.SignalEvidence{citation}})
			if !errors.Is(err, apperrors.ErrInvalidArgument) || len(evidence) != 0 {
				t.Fatalf("incomplete citation: %+v, %v", evidence, err)
			}
		})
	}
}

func TestPreviewDatesAreTypedAndValidated(t *testing.T) {
	for _, field := range []string{"closed_at", "fx_rate_date", "expected_close_date"} {
		value, err := (approvalPreviewReader{}).value(context.Background(), field, json.RawMessage(`"2026-09-15"`))
		if err != nil || value.Kind != "date" || value.Text != "2026-09-15" {
			t.Fatalf("%s: %+v, %v", field, value, err)
		}
		if _, err := (approvalPreviewReader{}).value(context.Background(), field, json.RawMessage(`"not a date"`)); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid %s: %v", field, err)
		}
	}
}
