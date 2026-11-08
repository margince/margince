// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Copies of a record's content kept for delivery: the published event in the
// outbox and the body a webhook rendered from it. Each is kept for a window and
// then cleared, unless a hold or floor keeps the record it was made from.
// Undelivered work is never cleared for its age: an unpublished event and a
// pending or retrying delivery are not past their window.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// How long a delivered copy stays. These are operational defaults, not legal
// retention periods.
const (
	outboxRetainDays      = 30
	webhookBodyRetainDays = 7
)

// WithOutboxReaders returns a service that keeps every published event a
// reader still needs. Without them the pass leaves the outbox alone rather
// than remove input a reader has not used.
func (s *RetentionService) WithOutboxReaders(readers storekit.OutboxReaders) *RetentionService {
	out := *s
	out.outboxReaders = &readers
	return &out
}

// copyStore is one place copies are kept: how to page the ones past a cutoff
// and how to clear them.
type copyStore struct {
	name  string
	days  int
	past  func(ctx context.Context, tx pgx.Tx, cutoff time.Time, after ids.UUID, limit int) ([]RecordCopy, error)
	clear func(ctx context.Context, tx pgx.Tx, copies []ids.UUID) error
}

// pruneRecordCopies clears the copies past their window in each store. The
// retain-only posture keeps them all.
func (s *RetentionService) pruneRecordCopies(ctx context.Context, retainOnly bool) error {
	if retainOnly {
		s.log.Info("retention: retain-only posture keeps published events and webhook bodies")
		return nil
	}
	stores := []copyStore{{
		name: "webhook bodies", days: webhookBodyRetainDays,
		past: s.purgeRawCaptures.DeliveryCopiesPastTx, clear: s.purgeRawCaptures.ClearDeliveryCopiesTx,
	}}
	if s.outboxReaders == nil {
		s.log.Warn("retention: published events are kept, because the outbox's readers were not supplied")
	} else {
		stores = append(stores, copyStore{
			name: "published events", days: outboxRetainDays,
			past: s.publishedEventsPast, clear: storekit.DeleteOutboxCopies,
		})
	}
	for _, store := range stores {
		if err := s.pruneCopies(ctx, store); err != nil {
			return fmt.Errorf("retention %s: %w", store.name, err)
		}
	}
	return nil
}

// pruneCopies walks one store's copies past their window in id order, a page
// per transaction, and clears each one no hold or floor keeps. A kept copy is
// stepped over, so it never stalls the walk.
func (s *RetentionService) pruneCopies(ctx context.Context, store copyStore) error {
	cutoff := time.Now().AddDate(0, 0, -store.days)
	var after ids.UUID
	for {
		var page []RecordCopy
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			floor, err := ResolveFloorPolicy(ctx, tx, s.eraser.country)
			if err != nil {
				return err
			}
			if page, err = store.past(ctx, tx, cutoff, after, retentionBatch); err != nil {
				return err
			}
			held, err := heldCopies(ctx, tx, page, floor)
			if err != nil {
				return err
			}
			gone := make([]ids.UUID, 0, len(page))
			for _, c := range page {
				if !held[c.ID] {
					gone = append(gone, c.ID)
				}
			}
			return store.clear(ctx, tx, gone)
		})
		if err != nil || len(page) < retentionBatch {
			return err
		}
		after = page[len(page)-1].ID
	}
}

// publishedEventsPast is storekit's page of published events, with the record
// each envelope names. An envelope naming no readable record id is held by
// nothing.
func (s *RetentionService) publishedEventsPast(ctx context.Context, tx pgx.Tx, cutoff time.Time, after ids.UUID, limit int) ([]RecordCopy, error) {
	events, err := storekit.PublishedOutboxCopies(ctx, tx, cutoff, after, limit, *s.outboxReaders)
	if err != nil {
		return nil, err
	}
	page := make([]RecordCopy, len(events))
	for i, event := range events {
		page[i] = RecordCopy{ID: event.ID, EntityType: event.EntityType}
		if record, err := ids.Parse(event.EntityID); err == nil {
			page[i].EntityID = record
		}
	}
	return page, nil
}

// scrubDeliveryCopies clears the webhook bodies rendered from the erased
// subject and their leads, and cancels those still to be sent.
func (e *Eraser) scrubDeliveryCopies(ctx context.Context, tx pgx.Tx, subject ids.ContactID, leads []ids.UUID) error {
	if e.purgeRawCaptures == nil {
		// An eraser built without the seam may still erase a subject no
		// webhook carried, which is every subject on an installation without one.
		var carried bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webhook_delivery
			WHERE (entity_type = 'contact' AND entity_id = @subject) OR (entity_type = 'lead' AND entity_id = ANY(@leads)))`,
			pgx.StrictNamedArgs{"subject": subject.UUID, "leads": leads}).Scan(&carried); err != nil || !carried {
			return err
		}
		return errors.New("privacy: a webhook carried this subject, and this eraser was built without the seam that clears its bodies")
	}
	if err := e.purgeRawCaptures.ScrubDeliveryCopiesTx(ctx, tx, contactObject, []ids.UUID{subject.UUID}); err != nil {
		return err
	}
	return e.purgeRawCaptures.ScrubDeliveryCopiesTx(ctx, tx, "lead", leads)
}
