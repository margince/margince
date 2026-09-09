// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mode guard in front of "this is not a company".
//
// Rejecting a company is two native writes in one commit: the company row
// and the standing domain refusal capture consults before it mints another.
// Neither has an incumbent analogue, and in overlay mode the native
// company table holds none of the workspace's records — so the unguarded
// verb would answer "not found" about a company the reader is looking at, and
// the domain refusal would be recorded for a capture path that is not the one
// creating the records.
//
// ADR-0018's bounded-capability rule takes the other answer: a capability that
// is not served says so with the declared sentinel, because "this is not
// available here" is visibly wrong and "no such company" is not.
//
// The write-admission guard next door cannot cover this one. It keys off the
// generated agent-policy table's tool classification, and this operation is
// human-only and names no tool — so it is let through as a governance write,
// which is the right default for the rest of the capture posture and wrong for
// the half of this verb that archives a record.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// RejectCompany shadows the people transport so the mode guard runs
// before the native store sees the request.
func (s Server) RejectCompany(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.RejectCompanyParams) {
	if refuseInOverlayMode(w, r, s.sorDispatch) {
		return
	}
	s.peopleHandlers.RejectCompany(w, r, id, params)
}
