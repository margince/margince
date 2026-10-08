// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The web surface of SeatNames.
//
// Its own file rather than a paragraph in handlers_roster.go, which owns the two
// PAGED lists: this one pages nothing and admits on membership alone, the same
// split seatnames.go already draws against users.go.

import (
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxNamedSeats bounds one naming request, and must agree with the `maxItems`
// on the contract's `id` parameter. The contract's number enforces nothing
// (nothing in internal/contracts checks it), so this is the check that holds,
// the same arrangement company_list.go's maxCompanyIDFilter has.
const maxNamedSeats = 100

// NameSeats serves GET /users/names.
func (h Handlers) NameSeats(w http.ResponseWriter, r *http.Request, params crmcontracts.NameSeatsParams) {
	if len(params.Id) > maxNamedSeats {
		httperr.Write(w, r, httperr.Validation("id", "too_many",
			fmt.Sprintf("name at most %d colleagues per request", maxNamedSeats)))
		return
	}
	asked := distinctSeats(params.Id)
	seats := make([]ids.UserID, 0, len(asked))
	for _, id := range asked {
		seats = append(seats, ids.From[ids.UserKind](ids.UUID(id)))
	}
	named, err := h.svc.SeatNames(r.Context(), seats)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Walked in the order asked rather than ranged over the map: Go randomises
	// map order, so ranging would answer one request two ways.
	data := make([]crmcontracts.SeatName, 0, len(named))
	for _, id := range asked {
		name, resolved := named[ids.UUID(id)]
		if !resolved {
			continue
		}
		data = append(data, crmcontracts.SeatName{Id: id, DisplayName: name})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SeatNameListResponse{Data: data})
}

// distinctSeats keeps the caller's order and drops repeats, so a page that names
// one colleague on forty rows asks about them a single time and gets one name back.
func distinctSeats(in []openapi_types.UUID) []openapi_types.UUID {
	out := make([]openapi_types.UUID, 0, len(in))
	seen := make(map[openapi_types.UUID]struct{}, len(in))
	for _, id := range in {
		if _, repeated := seen[id]; repeated {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
