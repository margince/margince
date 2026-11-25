// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// RegenerateOffer shadows the method deals.Handlers promotes into Server: a
// method declared on Server wins over a promoted one, and only compose can see
// offerDrafter, which needs the model lane and the retrieval seam a module may
// not import. Without WithOfferDraft wired it writes exactly the mechanical
// response deals would have written.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RegenerateOffer runs the mechanical mint FIRST, and only then asks
// offerDrafter to stage evidence-grounded lines on the fresh draft. Drafting
// is advisory over the committed mint: when it fails, the plain copy is the
// answer (ai_generated=false), which the rail records as degraded, not failed.
func (s Server) RegenerateOffer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RegenerateOfferParams) {
	ctx := r.Context()
	priorID := ids.From[ids.OfferKind](ids.UUID(id))

	offer, err := s.dealsStore.RegenerateOffer(ctx, priorID)
	if err != nil {
		deals.WriteOfferError(w, r, err)
		return
	}

	if s.offerDrafter != nil {
		// offer.Id is the FRESH draft revision RegenerateOffer just minted,
		// never the path's priorID — that offer is now superseded, and
		// AddStagedOfferLines refuses to stage onto anything but a draft.
		newRevisionID := ids.From[ids.OfferKind](ids.UUID(offer.Id))
		drafted, draftErr := s.offerDrafter.DraftOfferLines(ai.WithFloor(ctx), newRevisionID)
		if draftErr != nil {
			s.log.WarnContext(ctx, "compose: offer draft unavailable after regenerate — serving the mechanical draft",
				"offer_id", newRevisionID, "err", draftErr)
		} else {
			offer = drafted.Offer
		}
	}

	w.Header().Set("Location", "/v1/offers/"+offer.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, offer)
}
