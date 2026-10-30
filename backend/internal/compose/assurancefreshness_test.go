// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// envelopeOf is an event as its writer stages it: the generated payload, under
// the entity the event is named for.
func envelopeOf(t *testing.T, payload events.Payload, entity ids.UUID) events.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding %s: %v", payload.EventType(), err)
	}
	return events.Envelope{
		Type: payload.EventType(), Payload: raw, Entity: events.EntityRef{Type: payload.EntityType(), ID: entity},
	}
}

// A merge is named for the deal that went, and the deal that was kept now
// carries its record, so both are marked.
func TestADealMergeMarksBothDeals(t *testing.T) {
	from, into := ids.New[ids.DealKind](), ids.New[ids.DealKind]()
	merged := envelopeOf(t, crmcontracts.PublicEventDealMerged{
		MergedFromId: openapi_types.UUID(from.UUID), MergedIntoId: openapi_types.UUID(into.UUID),
	}, from.UUID)
	changes, err := inputChangesOf([]events.Envelope{merged})
	if err != nil {
		t.Fatalf("sorting the merge: %v", err)
	}
	for _, want := range []assurance.ChangedSubject{assurance.DealChanged(from.UUID), assurance.DealChanged(into.UUID)} {
		if !slices.Contains(changes.subjects, want) {
			t.Errorf("the merge marks %v, missing %v", changes.subjects, want)
		}
	}
}

// Each record that bears on a buying signal is followed to it: a signal by its
// own id, a company and the one a merge kept by lookup, and a suggestion,
// whose event names no entity, by the audit row it links to.
func TestEventsThatBearOnSignalsAreFollowedToThem(t *testing.T) {
	signal, company, kept, audit := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	suggestion := envelopeOf(t, crmcontracts.InternalEventDealSuggestionCreated{EvidenceCount: 1}, ids.Nil)
	suggestion.Entity = events.EntityRef{}
	suggestion.Trace.AuditLogID = audit
	changes, err := inputChangesOf([]events.Envelope{
		envelopeOf(t, crmcontracts.PublicEventSignalUpdated{}, signal),
		envelopeOf(t, crmcontracts.PublicEventCompanyMerged{
			MergedFromId: openapi_types.UUID(company), MergedIntoId: openapi_types.UUID(kept),
		}, company),
		suggestion,
	})
	if err != nil {
		t.Fatalf("sorting the events: %v", err)
	}
	if !slices.Equal(changes.subjects, []assurance.ChangedSubject{assurance.SignalChanged(signal)}) {
		t.Errorf("subjects = %v, want the signal alone", changes.subjects)
	}
	if !slices.Equal(changes.companies, []ids.UUID{company, kept}) {
		t.Errorf("companies = %v, want the merged company and the one kept", changes.companies)
	}
	if !slices.Equal(changes.suggestionAudits, []ids.UUID{audit}) {
		t.Errorf("suggestion audits = %v, want the event's audit row", changes.suggestionAudits)
	}
}
