// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers serves the card.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to the service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetDealStatus returns the card, writing it when the cached one is stale. It
// is a read: the only row it writes is its own cache entry, which is derived
// content and carries no audit or outbox row.
func (h Handlers) GetDealStatus(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetDealStatusParams,
) {
	checking := params.IfRecommendation != nil
	if checking != (params.IfRule != nil) || (checking && (params.FactsOnly == nil || !*params.FactsOnly)) {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	refresh := params.Refresh != nil && *params.Refresh
	dealID := ids.From[ids.DealKind](ids.UUID(id))
	var out crmcontracts.DealStatusCard
	var err error
	if params.FactsOnly != nil && *params.FactsOnly {
		out, err = h.svc.ReadFacts(r.Context(), dealID)
	} else {
		out, err = h.svc.Get(r.Context(), dealID, refresh)
	}
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	if checking {
		if err := checkExpectedRecommendation(out.Next, *params.IfRecommendation, *params.IfRule); err != nil {
			httperr.Write(w, r, err)
			return
		}
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func checkExpectedRecommendation(move *crmcontracts.DealStatusCardMove, fingerprint, rule string) error {
	if move == nil || move.Recommendation == nil || move.Recommendation.Rule != rule || move.Recommendation.BasisFingerprint != fingerprint {
		return fmt.Errorf("the suggested step changed; refresh it before opening: %w", apperrors.ErrVersionSkew)
	}
	return nil
}
