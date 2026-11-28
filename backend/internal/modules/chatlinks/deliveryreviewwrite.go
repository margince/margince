// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Store) applyDeliveryReview(ctx context.Context, tx pgx.Tx, link Link, in DeliveryReviewInput) error {
	after := link
	if in.Action == restartCardAction {
		var err error
		after, err = restartedCardTx(ctx, tx, link)
		if err != nil {
			return err
		}
	} else {
		seen := map[ids.UUID]bool{}
		for _, id := range in.Deliveries {
			if seen[id] {
				return apperrors.ErrInvalidArgument
			}
			seen[id] = true
			card, err := acknowledgeDeliveryTx(ctx, tx, link.ID, id)
			if err != nil {
				return err
			}
			after.CardPaused = after.CardPaused || card
		}
	}
	if err := writeReviewedCard(ctx, tx, link, after); err != nil {
		return err
	}
	return s.jobs.EnqueueFlushTx(ctx, tx, link.ID)
}

func acknowledgeDeliveryTx(ctx context.Context, tx pgx.Tx, link, id ids.UUID) (bool, error) {
	var before BotDelivery
	var card bool
	err := tx.QueryRow(ctx, `SELECT id,state,coalesce(message_id,''),carried_item_ids,request_key LIKE 'card:%' FROM chat_bot_delivery WHERE id=@id AND link_id=@link AND `+uncertainDeliverySQL+` FOR UPDATE`, pgx.StrictNamedArgs{"id": id, linkKey: link}).Scan(&before.ID, &before.State, &before.Message, &before.Carried, &card)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.ErrConflict
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_bot_delivery SET state='reviewed_unknown',updated_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": id}); err != nil {
		return false, err
	}
	after := before
	after.State = "reviewed_unknown"
	if _, err := storekit.Audit(ctx, tx, "update", "chat_bot_delivery", id, botDeliveryImage(before), botDeliveryImage(after)); err != nil {
		return false, err
	}
	if err := withdrawUncertainItems(ctx, tx, link, before.Carried); err != nil {
		return false, err
	}
	return card, nil
}

func withdrawUncertainItems(ctx context.Context, tx pgx.Tx, link ids.UUID, items []ids.UUID) error {
	rows, err := tx.Query(ctx, `UPDATE chat_post_item SET withdrawn_at=now() WHERE link_id=@link AND id=ANY(@items) AND posted_at IS NULL AND withdrawn_at IS NULL RETURNING id,withdrawn_at`, pgx.StrictNamedArgs{linkKey: link, "items": items})
	if err != nil {
		return err
	}
	type withdrawal struct {
		ID ids.UUID
		At time.Time
	}
	affected, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (withdrawal, error) {
		var item withdrawal
		err := row.Scan(&item.ID, &item.At)
		return item, err
	})
	if err != nil {
		return err
	}
	for _, item := range affected {
		if _, err := storekit.Audit(ctx, tx, "update", "chat_post_item", item.ID, map[string]any{"withdrawn_at": nil}, map[string]any{"withdrawn_at": item.At}); err != nil {
			return err
		}
	}
	return nil
}

func writeReviewedCard(ctx context.Context, tx pgx.Tx, before, after Link) error {
	if before.CardPaused == after.CardPaused && before.CardGeneration == after.CardGeneration {
		return nil
	}
	patch := storekit.NewPatch()
	patch.Set("card_paused", before.CardPaused, after.CardPaused)
	if after.CardGeneration != "" && before.CardGeneration != after.CardGeneration {
		patch.Set("card_generation", before.CardGeneration, after.CardGeneration)
		patch.Set("card_message_id", before.CardMessageID, nil)
		patch.Set("card_fingerprint", before.CardFingerprint, nil)
	}
	if err := patch.ApplyWithVersion(ctx, tx, "chat_link", before.ID, before.Version); err != nil {
		return err
	}
	_, err := storekit.Audit(ctx, tx, "update", chatLinkEntity, before.ID, auditImage(before), auditImage(after))
	return err
}

func restartedCardTx(ctx context.Context, tx pgx.Tx, link Link) (Link, error) {
	after := link
	if !link.CardPaused || link.RootID != "" || !link.PostSupported {
		return Link{}, apperrors.ErrConflict
	}
	var blocked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_bot_delivery WHERE link_id=@link AND state IN ('sending','unknown'))`, pgx.StrictNamedArgs{linkKey: link.ID}).Scan(&blocked); err != nil {
		return Link{}, err
	}
	if blocked {
		return Link{}, apperrors.ErrConflict
	}
	after.CardPaused = false
	after.CardGeneration = ids.NewV7().String()
	after.CardMessageID, after.CardFingerprint = "", ""
	return after, nil
}
