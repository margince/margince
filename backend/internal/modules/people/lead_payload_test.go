// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The lead family: drives leadPromotedPayload — the exact function
// FinalizeLeadPromotion's promotion path calls to build its lead.promoted
// emit (promote.go) — then round-trips the result through JSON exactly as
// storekit.EmitEvent marshals it into the outbox envelope's payload column.
// It also proves the OPEN lead.updated envelope's changed_fields map
// preserves a runtime cf_* custom-field key verbatim, since that is exactly
// why lead.updated is modeled as an open map rather than a strictly typed
// struct. There is no non-integration harness in this repo that drives a
// Store method against a real Postgres (every such test lives under
// compose/integration, gated `//go:build integration`, needing db-up);
// testing the production payload-construction functions directly — the one
// place a schema/code mismatch would show up — is the honest substitute.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadPromotedPayload_WithEvidence(t *testing.T) {
	personID := ids.From[ids.PersonKind](ids.NewV7())
	evidenceID := ids.From[ids.ActivityKind](ids.NewV7())

	payload := leadPromotedPayload(personID, "created", "inbound_reply", &evidenceID, nil)

	if payload.EventType() != "lead.promoted" {
		t.Errorf("got %v, want %v", payload.EventType(), "lead.promoted")
	}
	if payload.EntityType() != "lead" {
		t.Errorf("got %v, want %v", payload.EntityType(), "lead")
	}
	if payload.PromotedPersonId != openapi_types.UUID(personID.UUID) {
		t.Errorf("got %v, want %v", payload.PromotedPersonId, openapi_types.UUID(personID.UUID))
	}
	if payload.DedupeOutcome != "created" {
		t.Errorf("got %v, want %v", payload.DedupeOutcome, "created")
	}
	if payload.Trigger != "inbound_reply" {
		t.Errorf("got %v, want %v", payload.Trigger, "inbound_reply")
	}
	if payload.EvidenceRef == nil {
		t.Fatalf("expected non-nil value")
	}
	if *payload.EvidenceRef != openapi_types.UUID(evidenceID.UUID) {
		t.Errorf("got %v, want %v", *payload.EvidenceRef, openapi_types.UUID(evidenceID.UUID))
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded crmcontracts.PublicEventLeadPromoted
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatalf("unexpected error: %v", json.Unmarshal(raw, &decoded))
	}
	if !reflect.DeepEqual(decoded, payload) {
		t.Errorf("got %v, want %v", decoded, payload)
	}
}

// The carried ids ride the payload, and an empty carry is OMITTED rather than
// sent as []. A lead with no timeline and a build that does not report one are
// different facts, and a consumer reading [] as "nothing to do" would take the
// second for the first — which is the fallback this field exists beside.
func TestLeadPromotedPayload_CarriesTheActivitiesItMoved(t *testing.T) {
	personID := ids.From[ids.PersonKind](ids.NewV7())
	moved := []ids.UUID{ids.NewV7(), ids.NewV7()}

	payload := leadPromotedPayload(personID, "merged", "human_qualify", nil, moved)

	if payload.CarriedActivityIds == nil {
		t.Fatal("a promotion that moved two activities named none of them, so a " +
			"consumer has only the person id — which on a merge is the wrong set")
	}
	if got := len(*payload.CarriedActivityIds); got != len(moved) {
		t.Fatalf("named %d carried activities, want %d", got, len(moved))
	}
	for i, want := range moved {
		if (*payload.CarriedActivityIds)[i] != openapi_types.UUID(want) {
			t.Errorf("carried id %d is %v, want %v",
				i, (*payload.CarriedActivityIds)[i], openapi_types.UUID(want))
		}
	}
}

func TestLeadPromotedPayload_OmitsAnEmptyCarry(t *testing.T) {
	personID := ids.From[ids.PersonKind](ids.NewV7())

	payload := leadPromotedPayload(personID, "created", "human_qualify", nil, nil)

	if payload.CarriedActivityIds != nil {
		t.Errorf("a promotion that carried nothing sent %v — absent and empty say "+
			"different things here", *payload.CarriedActivityIds)
	}
}

func TestLeadPromotedPayload_MergedNoEvidence(t *testing.T) {
	personID := ids.From[ids.PersonKind](ids.NewV7())

	payload := leadPromotedPayload(personID, "merged", "human_qualify", nil, nil)

	if payload.DedupeOutcome != "merged" {
		t.Errorf("got %v, want %v", payload.DedupeOutcome, "merged")
	}
	if payload.EvidenceRef != nil {
		t.Errorf("expected nil, got %v", payload.EvidenceRef)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(raw), `"evidence_ref"`) {
		t.Errorf("an absent evidence_ref must be omitted from the wire body, not marshaled as null: should not contain %v", `"evidence_ref"`)
	}
}

// TestLeadUpdatedChangedFieldsPreservesCustomField proves the OPEN
// lead.updated envelope's changed_fields map round-trips a runtime cf_*
// custom-field key verbatim — the honest reason lead.updated is an open
// map rather than a strictly typed struct.
func TestLeadUpdatedChangedFieldsPreservesCustomField(t *testing.T) {
	payload := crmcontracts.PublicEventLeadUpdated{
		ChangedFields: map[string]any{
			"score":              float64(72),
			"cf_lead_source_ref": "partner-9f2",
		},
	}

	if payload.EventType() != "lead.updated" {
		t.Errorf("got %v, want %v", payload.EventType(), "lead.updated")
	}
	if payload.EntityType() != "lead" {
		t.Errorf("got %v, want %v", payload.EntityType(), "lead")
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded crmcontracts.PublicEventLeadUpdated
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatalf("unexpected error: %v", json.Unmarshal(raw, &decoded))
	}
	if decoded.ChangedFields["cf_lead_source_ref"] != "partner-9f2" {
		t.Errorf("the open changed_fields map must preserve a cf_* custom-field key untouched: got %v, want %v", decoded.ChangedFields["cf_lead_source_ref"], "partner-9f2")
	}
	if !reflect.DeepEqual(decoded, payload) {
		t.Errorf("got %v, want %v", decoded, payload)
	}
}
