// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RefuseArchiveOffer is ArchiveOffer's authority probe without the write: the
// stage-time half a governed archive runs before it asks a human.
func (s *Store) RefuseArchiveOffer(ctx context.Context, id ids.OfferID) error {
	if err := auth.Require(ctx, "offer", principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		offer, err := writableOffer(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		return refuseAcceptedOfferArchive(offer)
	})
}

func (s *Store) ArchiveOffer(ctx context.Context, id ids.OfferID) (crmcontracts.Offer, error) {
	return s.ArchiveOfferAt(ctx, id, nil)
}

func (s *Store) ArchiveOfferAt(ctx context.Context, id ids.OfferID, ifVersion *int64) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionDelete); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		// An accepted offer is the source of its deal's price, so retiring it
		// would leave the deal priced from a document nobody can open.
		if err := refuseAcceptedOfferArchive(current); err != nil {
			return err
		}
		patch := storekit.NewPatch()
		patch.Set("archived_at", nil, time.Now().UTC())
		if err := patch.ApplyGuarded(ctx, tx, "offer", id.UUID, ifVersion); err != nil {
			return fmt.Errorf("archive offer: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "archive", "offer", id.UUID, nil, nil); err != nil {
			return fmt.Errorf("audit offer archive: %w", err)
		}
		if out, err = readOfferWithLines(ctx, tx, id, storekit.IncludeArchived); err != nil {
			return fmt.Errorf("read archived offer: %w", err)
		}
		return nil
	})
	return out, err
}

func refuseAcceptedOfferArchive(offer crmcontracts.Offer) error {
	if offer.Status != crmcontracts.OfferStatusAccepted {
		return nil
	}
	return &httperr.DetailedError{Status: http.StatusConflict, Code: "offer_accepted", Detail: "an accepted offer prices its deal and cannot be archived; the deal's price has to change first"}
}
