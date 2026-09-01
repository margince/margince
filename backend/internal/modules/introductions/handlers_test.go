// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The wire shape carries what the two surfaces read. A field that maps to the
// wrong place, or silently to nothing, reads on screen as a feature that does
// not work — and the store tests cannot see it, because they never cross the
// wire.
func TestTheWireCarriesEachPieceOfCopySeparately(t *testing.T) {
	through := ids.NewV7()
	suggested := ids.NewV7()
	activity := ids.NewV7()
	decided := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	r := &Request{
		ID:               ids.NewV7(),
		PersonID:         ids.NewV7(),
		RequesterUserID:  ids.NewV7(),
		IntroducerUser:   ids.NewV7(),
		RouteType:        "through_contact",
		ThroughPersonID:  &through,
		InternalReason:   "she ran the migration we are pitching",
		ValueForTarget:   "a shorter path to the same answer",
		ForwardableNote:  "Hallo Dana, darf ich vorstellen",
		NoteGeneratedBy:  "model",
		NoteAIGenerated:  true,
		FallbackPolicy:   "name_drop",
		NameDropAllowed:  true,
		Status:           StatusSuggestOther,
		SuggestedUserID:  &suggested,
		SourceActivityID: &activity,
		DecidedAt:        &decided,
		Version:          3,
	}

	got := wire(r)

	// The three pieces of copy are three different messages. Collapsing any two
	// would put the internal case for the ask in front of the contact.
	if got.InternalReason != r.InternalReason {
		t.Errorf("internal_reason went out as %q", got.InternalReason)
	}
	if got.ValueForTarget == nil || *got.ValueForTarget != r.ValueForTarget {
		t.Errorf("value_for_target went out as %v", got.ValueForTarget)
	}
	if got.ForwardableNote == nil || *got.ForwardableNote != r.ForwardableNote {
		t.Errorf("forwardable_note went out as %v", got.ForwardableNote)
	}
	// Provenance has to survive the wire or the reader cannot see that a model
	// wrote the words they are about to send under their own name.
	if got.NoteGeneratedBy != crmcontracts.IntroNoteOriginModel || !got.NoteAiGenerated {
		t.Errorf("provenance went out as %q / %v", got.NoteGeneratedBy, got.NoteAiGenerated)
	}
	if got.ThroughPersonId == nil || ids.UUID(*got.ThroughPersonId) != through {
		t.Error("the intermediary did not reach the wire")
	}
	if got.SuggestedUserId == nil || ids.UUID(*got.SuggestedUserId) != suggested {
		t.Error("the suggested colleague did not reach the wire")
	}
	if got.SourceActivityId == nil || ids.UUID(*got.SourceActivityId) != activity {
		t.Error("the evidence did not reach the wire")
	}
	if got.Version != 3 {
		t.Errorf("version went out as %d — a client that cannot echo it cannot write", got.Version)
	}
}

// An absent value and an empty one are different answers. A note nobody wrote
// must not go out as an empty string a client renders as an empty plate.
func TestUnsetCopyIsAbsentRatherThanEmpty(t *testing.T) {
	got := wire(&Request{
		RouteType:      "direct",
		InternalReason: "worth asking",
		Status:         StatusRequested,
	})
	if got.ValueForTarget != nil {
		t.Errorf("an unwritten value_for_target went out as %q", *got.ValueForTarget)
	}
	if got.ForwardableNote != nil {
		t.Errorf("an unwritten forwardable_note went out as %q", *got.ForwardableNote)
	}
	if got.ThroughPersonId != nil || got.SuggestedUserId != nil || got.SourceActivityId != nil {
		t.Error("an unset id went out as a value")
	}
}

// A name-drop and an introduction are different events, and the wire keeps them
// apart: no ask ever carries both timestamps, so a client cannot render lent
// permission as a handshake that happened.
func TestANameDroppedAskCarriesNoIntroducedAt(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	got := wire(&Request{
		RouteType:      "direct",
		InternalReason: "worth asking",
		Status:         StatusNameDropped,
		NameDroppedAt:  &at,
	})
	if got.Status != crmcontracts.IntroRequestStatusNameDropped {
		t.Errorf("status went out as %q", got.Status)
	}
	if got.IntroducedAt != nil {
		t.Error("a name-drop went out carrying introduced_at")
	}
	if got.NameDroppedAt == nil {
		t.Error("a name-drop went out with no time of its own")
	}
}

// A client that sends no provenance has a person typing. Defaulting the other
// way would mark honest copy as machine-authored, which is the same lie in
// reverse and just as visible to whoever reads the disclosure.
func TestUnstatedProvenanceIsHuman(t *testing.T) {
	if got := noteOriginOf(nil); got != "human" {
		t.Errorf("an unstated origin defaulted to %q", got)
	}
	model := crmcontracts.IntroNoteOriginModel
	if got := noteOriginOf(&model); got != "model" {
		t.Errorf("a stated origin became %q", got)
	}
}

// An unstated fallback is "none" and never a policy the requester did not pick:
// a default of name_drop would lend the colleague's name on their behalf.
func TestUnstatedFallbackIsNone(t *testing.T) {
	if got := fallbackOf(nil); got != "none" {
		t.Errorf("an unstated fallback defaulted to %q", got)
	}
	drop := crmcontracts.IntroFallbackPolicyNameDrop
	if got := fallbackOf(&drop); got != "name_drop" {
		t.Errorf("a stated fallback became %q", got)
	}
}

// The route's two halves have to agree. This is the one refusal the transport
// owns, because the caller is told WHICH half is wrong — the table's CHECK can
// only say that the row is impossible.
func TestARouteNamesItsIntermediaryOrIsDirect(t *testing.T) {
	id := openapi_types.UUID(ids.NewV7())
	cases := []struct {
		name    string
		route   crmcontracts.PersonGraphRouteType
		through *openapi_types.UUID
		refused bool
	}{
		{"direct with nobody named", crmcontracts.PersonGraphRouteTypeDirect, nil, false},
		{"through a named contact", crmcontracts.PersonGraphRouteTypeThroughContact, &id, false},
		{"direct that names somebody", crmcontracts.PersonGraphRouteTypeDirect, &id, true},
		{"through nobody", crmcontracts.PersonGraphRouteTypeThroughContact, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := json.Marshal(crmcontracts.IntroRequestInput{
				IntroducerUserId: openapi_types.UUID(ids.NewV7()),
				RouteType:        c.route,
				ThroughPersonId:  c.through,
				InternalReason:   "worth asking",
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/people/x/intro-requests",
				bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			// No store: a body this check refuses never needs one. A body it
			// admits goes on to the store, and an unauthenticated request is
			// turned away there — so 422 means "this check refused it" and any
			// other status means it got past.
			h := Handlers{now: func() time.Time { return testClock }}
			h.CreateIntroRequest(rec, req, openapi_types.UUID(ids.NewV7()))

			refused := rec.Code == http.StatusUnprocessableEntity
			if refused != c.refused {
				t.Errorf("refused=%v (status %d), wanted %v", refused, rec.Code, c.refused)
			}
		})
	}
}

var testClock = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
