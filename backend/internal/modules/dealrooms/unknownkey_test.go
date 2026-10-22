// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// A deal room request declares every key it reads. A key it does not know, such
// as `expires` for `expires_at`, is refused rather than dropped with a success.
// Each refusal lands before the store, which a zero Handlers has none of.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestADealRoomRequestRefusesAKeyItDoesNotKnow(t *testing.T) {
	t.Parallel()
	room, other := crmcontracts.Id(ids.NewV7()), crmcontracts.Id(ids.NewV7())
	h := Handlers{}
	for _, tc := range []struct {
		route string
		serve func(http.ResponseWriter, *http.Request)
	}{
		{"create room", func(w http.ResponseWriter, r *http.Request) {
			h.CreateDealRoom(w, r, crmcontracts.CreateDealRoomParams{})
		}},
		{"update room", func(w http.ResponseWriter, r *http.Request) {
			h.UpdateDealRoom(w, r, room, crmcontracts.UpdateDealRoomParams{})
		}},
		{"invite participant", func(w http.ResponseWriter, r *http.Request) {
			h.InviteDealRoomParticipant(w, r, room)
		}},
		{"update participant", func(w http.ResponseWriter, r *http.Request) {
			h.UpdateDealRoomParticipant(w, r, room, other)
		}},
		{"add document", func(w http.ResponseWriter, r *http.Request) {
			h.AddDealRoomDocument(w, r, room)
		}},
		{"update document", func(w http.ResponseWriter, r *http.Request) {
			h.UpdateDealRoomDocument(w, r, room, other, crmcontracts.UpdateDealRoomDocumentParams{})
		}},
	} {
		t.Run(tc.route, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/v1/deal-rooms", strings.NewReader(`{"expires":"2027-01-01T00:00:00Z"}`))
			w := httptest.NewRecorder()

			tc.serve(w, r)

			body := w.Body.String()
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "unknown_field") || !strings.Contains(body, `expires`) {
				t.Errorf("got %d %s, want a 422 unknown_field naming expires", w.Code, body)
			}
		})
	}
}

func TestASeamRoomCreateRefusesAKeyItDoesNotKnow(t *testing.T) {
	t.Parallel()
	_, err := offlineProvider().Create(ungrantedSeller(), datasource.CreateInput{
		EntityType: datasource.EntityDealRoom,
		Fields:     map[string]any{"deal_id": ids.NewV7().String(), "title": "Acme rollout", "expires": "2027-01-01T00:00:00Z"},
		Source:     "agent",
	})

	var unknown *datasource.UnknownFieldError
	if !errors.As(err, &unknown) || !slices.Equal(unknown.Fields, []string{"expires"}) {
		t.Errorf("got %v, want a refusal naming the unknown key expires", err)
	}
}
