// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// enqueueRow is one subscription's delivery of an event: the body rendered
// for it, or the reason it could not be.
type enqueueRow struct {
	subID ids.UUID
	// ownerID is the owner the visibility check authorized this row for.
	ownerID     ids.UUID
	payload     []byte
	contentType *string
	renderError string
}

// renderFor builds each visible subscription's body from the public
// envelope. A subscription without a template gets the envelope itself.
func renderFor(cands []subCandidate, envelope []byte) []enqueueRow {
	rows := make([]enqueueRow, 0, len(cands))
	for _, c := range cands {
		row := enqueueRow{subID: c.id, ownerID: c.ownerID, payload: envelope}
		if c.bodyTemplate != nil {
			row.contentType = c.bodyContentType
			tpl, err := parseBodyTemplate(*c.bodyTemplate, deref(c.bodyContentType))
			if err == nil {
				row.payload, err = tpl.render(envelope)
			}
			if err != nil {
				row.payload, row.renderError = nil, "the body template could not be rendered: "+err.Error()
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// enqueueForSubscriptions records one delivery per row, idempotently: the
// (subscription, event) unique key makes a redelivered bus event a no-op,
// so it never double-POSTs. A row whose body could not be rendered is
// dead-lettered at once with its reason. Only freshly created pending rows
// come back to be attempted.
func (s *Store) enqueueForSubscriptions(ctx context.Context, rows []enqueueRow, eventType string, eventID ids.UUID, entityType string, entityID ids.UUID) ([]attemptTarget, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	subIDs := make([]ids.UUID, len(rows))
	owners := make([]ids.UUID, len(rows))
	payloads := make([]string, len(rows))
	contentTypes := make([]*string, len(rows))
	renderErrors := make([]*string, len(rows))
	bodies := make(map[ids.UUID][]byte, len(rows))
	ownersBySub := make(map[ids.UUID]ids.UUID, len(rows))
	for i, r := range rows {
		subIDs[i], owners[i], payloads[i], contentTypes[i] = r.subID, r.ownerID, string(r.payload), r.contentType
		if r.renderError != "" {
			renderErrors[i] = &r.renderError
		}
		bodies[r.subID] = r.payload
		ownersBySub[r.subID] = r.ownerID
	}
	var targets []attemptTarget
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Query(ctx, `
			WITH wanted AS (
				SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $9::uuid[])
				  AS w(subscription_id, payload, content_type, render_error, owner_id)
			), matched AS (
				SELECT s.id, s.target_url, s.signing_secret_ref, s.custom_headers,
				       w.payload, w.content_type, w.render_error
				FROM webhook_subscription s JOIN wanted w ON w.subscription_id = s.id
				WHERE s.state = 'active' AND s.archived_at IS NULL AND s.owner_id = w.owner_id
			), created AS (
				INSERT INTO webhook_delivery
				  (subscription_id, event_id, event_type, payload, content_type, status,
				   last_error, dead_lettered_at, entity_type, entity_id)
				SELECT m.id, $6, $5, m.payload, m.content_type,
				       CASE WHEN m.render_error IS NULL THEN 'pending' ELSE 'dead_lettered' END,
				       m.render_error, CASE WHEN m.render_error IS NULL THEN NULL ELSE now() END,
				       $7, $8
				FROM matched m
				ON CONFLICT (subscription_id, event_id) DO NOTHING
				RETURNING id, subscription_id, status, content_type
			)
			SELECT c.id, c.subscription_id, m.target_url, m.signing_secret_ref, m.custom_headers, c.content_type
			FROM created c JOIN matched m ON m.id = c.subscription_id
			WHERE c.status = 'pending'`,
			subIDs, payloads, contentTypes, renderErrors, eventType, eventID, entityType, entityID, owners)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			t := attemptTarget{eventType: eventType, eventID: eventID}
			var contentType *string
			if err := res.Scan(&t.deliveryID, &t.subID, &t.targetURL, &t.sealedSecret, &t.headers, &contentType); err != nil {
				return err
			}
			t.payload, t.contentType = bodies[t.subID], deref(contentType)
			t.entityType, t.entityID, t.ownerID = entityType, entityID, ownersBySub[t.subID]
			targets = append(targets, t)
		}
		return res.Err()
	})
	return targets, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
