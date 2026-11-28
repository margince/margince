// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UncertainDelivery identifies a shared post that cannot safely be repeated automatically.
type UncertainDelivery struct {
	ID      ids.UUID
	State   string
	Created time.Time
	Updates int
	Card    bool
}

// DeliveryReview collects unresolved shared posts for reviewed operator review.
type DeliveryReview struct {
	Copies     ProviderCopies
	Link       ids.UUID
	CardPaused bool
	Deliveries []UncertainDelivery
}

// DeliveryReviewInput pins recovery choices to the reviewed delivery versions.
type DeliveryReviewInput struct {
	Link         ids.UUID
	Deliveries   []ids.UUID
	Action       string
	Acknowledged bool
}

const uncertainDeliverySQL = `(state='unknown' OR (state='sending' AND updated_at<now()-interval '60 seconds'))`

// DeliveryReview collects unresolved shared posts for reviewed operator review.
func (s *Store) DeliveryReview(ctx context.Context, kind string, id ids.UUID) (DeliveryReview, error) {
	if _, err := memberActor(ctx); err != nil {
		return DeliveryReview{}, err
	}
	var out DeliveryReview
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		live, err := s.readMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordRead(live, kind); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(live, tx, kind, id); err != nil {
			return err
		}
		link, err := liveLinkOf(live, tx, kind, id)
		if err != nil {
			return err
		}
		out, err = readDeliveryReview(live, tx, link)
		return err
	})
	return out, err
}

func readDeliveryReview(ctx context.Context, tx pgx.Tx, link Link) (DeliveryReview, error) {
	out := DeliveryReview{Link: link.ID, CardPaused: link.CardPaused, Deliveries: []UncertainDelivery{}}
	rows, err := tx.Query(ctx, `SELECT id,state,created_at,cardinality(carried_item_ids),request_key LIKE 'card:%' FROM chat_bot_delivery WHERE link_id=@link AND `+uncertainDeliverySQL+` ORDER BY created_at,id`, pgx.StrictNamedArgs{linkKey: link.ID})
	if err != nil {
		return out, err
	}
	out.Deliveries, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (UncertainDelivery, error) {
		var delivery UncertainDelivery
		err := row.Scan(&delivery.ID, &delivery.State, &delivery.Created, &delivery.Updates, &delivery.Card)
		return delivery, err
	})
	if err != nil {
		return out, err
	}
	out.Copies, err = readProviderCopies(ctx, tx, link.ID)
	return out, err
}

// ReviewDeliveries rechecks delivery versions before applying recovery choices.
func (s *Store) ReviewDeliveries(ctx context.Context, kind string, id ids.UUID, in DeliveryReviewInput) (DeliveryReview, error) {
	if _, err := memberMutationActor(ctx); err != nil {
		return DeliveryReview{}, err
	}
	if s.jobs == nil {
		return DeliveryReview{}, ErrEventsUnavailable
	}
	if !validDeliveryReview(in) {
		return DeliveryReview{}, apperrors.ErrInvalidArgument
	}
	var out DeliveryReview
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		live, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordUpdate(live, kind); err != nil {
			return err
		}
		if err := requireRecordRead(live, kind); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(live, tx, kind, id); err != nil {
			return err
		}
		if err := auth.HoldWritableLive(live, tx, kind, id); err != nil {
			return err
		}
		link, err := heldLiveLinkOf(live, tx, kind, id)
		if err != nil {
			return err
		}
		if link.ID != in.Link {
			return apperrors.ErrConflict
		}
		if _, err := tx.Exec(live, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "chat-bot-delivery:" + link.ID.String()}); err != nil {
			return err
		}
		if err := s.applyDeliveryReview(live, tx, link, in); err != nil {
			return err
		}
		current, err := liveLinkOf(live, tx, kind, id)
		if err != nil {
			return err
		}
		out, err = readDeliveryReview(live, tx, current)
		return err
	})
	return out, err
}

func validDeliveryReview(in DeliveryReviewInput) bool {
	if !in.Acknowledged || in.Link.IsZero() || in.Action != "skip_unknown" && in.Action != restartCardAction {
		return false
	}
	if in.Action == "skip_unknown" && (len(in.Deliveries) == 0 || len(in.Deliveries) > 100) || in.Action == "restart_card" && len(in.Deliveries) != 0 {
		return false
	}
	return true
}
