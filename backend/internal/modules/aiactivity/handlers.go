// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aiactivity

// The personal read (GET /me/ai-activity), shadowing the generated stub over
// the projection. No RBAC object gates it: the feed is the caller's own by
// construction, so there is no wider set to withhold and the caller's identity
// is the whole of the authorization.

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Reader is the one read this transport needs. Stated as an interface so the
// refusal and the wire mapping — the two rules that live in the transport
// rather than in SQL — can be pinned without a database. *Store is the
// production implementation.
type Reader interface {
	Mine(ctx context.Context, startOfToday time.Time, kinds []string) (Feed, error)
}

// Handlers serves one contact's view of the AI's work.
type Handlers struct {
	store Reader
	// now stamps as_of and bounds "today" — injected so a test states the
	// instant it means rather than racing the wall clock across midnight.
	now func() time.Time
}

// NewHandlers binds the transport to a ready reader; compose constructs it once
// per process role.
func NewHandlers(store Reader, now func() time.Time) Handlers {
	return Handlers{store: store, now: now}
}

// GetMyAiActivity answers with what is live for the caller now and what settled
// for them today.
//
// A caller with no user identity is REFUSED rather than served empty arrays: an
// empty feed is the real answer for an AI at rest, so handing one to an
// unidentified caller would report "nothing is running" about a contact the
// server never resolved.
func (h Handlers) GetMyAiActivity(w http.ResponseWriter, r *http.Request, params crmcontracts.GetMyAiActivityParams) {
	// Refused HERE as well as in the store, and the duplication is deliberate:
	// this one turns an unidentified caller into a 401 the client understands,
	// where the store's refusal maps to a 403. The store's is the one that
	// makes it impossible; this one makes it legible.
	p, ok := principal.Actor(r.Context())
	if !ok || p.UserID.IsZero() {
		httperr.Unauthorized(w, r, "reading your AI activity needs an authenticated caller")
		return
	}
	kinds := requestedKinds(params)
	now := h.now()
	feed, err := h.store.Mine(r.Context(), startOfDay(now), kinds)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AiActivity{
		AsOf:    now,
		Running: toWire(feed.Live),
		Recent:  toWire(feed.Settled),
		// What went wrong today, carried beside what settled rather than found
		// inside it. `recent` is the newest ten occurrences of any outcome, so
		// ten later successes push a fault off it — and the rail holds a fault
		// until somebody acknowledges it, which is precisely the case where
		// nobody has looked yet.
		Faults:    toWire(feed.Faults),
		LiveTotal: &feed.LiveTotal,
	})
}

// requestedKinds is the caller's filter as the store takes it: nil for every
// kind, or the kinds a client draws. The vocabulary is enforced before the
// handler runs, from the contract's own enum, so an empty or unknown kind never
// reaches here.
func requestedKinds(params crmcontracts.GetMyAiActivityParams) []string {
	if params.Kinds == nil {
		return nil
	}
	out := make([]string, 0, len(*params.Kinds))
	for _, kind := range *params.Kinds {
		out = append(out, string(kind))
	}
	return out
}

// startOfDay is midnight in the clock's own location, which is what "today"
// means to the reader reading the rail.
func startOfDay(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// toWire maps the projection's facts onto the contract's shape.
//
// The result is always an allocated slice, so an empty feed serializes as `[]`
// and never as `null`: the contract declares both fields as arrays, and a
// client that iterates what it was promised crashes on a null.
//
// kind and state are passed through rather than re-mapped. Both vocabularies
// are already the contract's — kind because the emitter writes the catalog name
// the copy is keyed on, state because the projection's CHECK and the contract's
// enum are held equal by a fitness test — so a translation layer here would be
// a second place for them to disagree.
func toWire(items []Item) []crmcontracts.AiActivityItem {
	wire := make([]crmcontracts.AiActivityItem, 0, len(items))
	for _, item := range items {
		wire = append(wire, crmcontracts.AiActivityItem{
			Id:            openapi_types.UUID(item.ID),
			Kind:          crmcontracts.AiActivityKind(item.Kind),
			State:         crmcontracts.AiActivityItemState(item.State),
			StartedAt:     item.StartedAt,
			FinishedAt:    item.FinishedAt,
			DegradeReason: item.DegradeReason,
			Summary:       item.Summary,
			SubjectLabel:  item.SubjectLabel,
			SubjectType:   item.SubjectType,
			SubjectId:     contractUUID(item.SubjectID),
		})
	}
	return wire
}

// contractUUID is the projection's optional id in the contract's spelling —
// the inverse of handler.go's derefContractID. nil stays nil, so an occurrence
// about no record carries no subject on the wire.
func contractUUID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	out := openapi_types.UUID(*id)
	return &out
}
