// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seams between the ranked queue and the contact and company pages'
// last-touch readers: a row and the record it opens answer "who wrote last"
// from one statement.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionContactTouch reads when a contact last wrote and when we did through
// the contact page's own set reader.
type attentionContactTouch struct{ pool *pgxpool.Pool }

func (r attentionContactTouch) LastTouch(
	ctx context.Context, contactIDs []ids.UUID,
) (map[ids.UUID]attention.TouchMoments, error) {
	return touchMomentsThrough(ctx, r.pool, contactIDs,
		func(ctx context.Context, tx pgx.Tx, wanted []ids.ContactID) (map[ids.ContactID]contact360.LastTouch, error) {
			return contact360.LastTouchFor(ctx, tx, wanted, contact360.AssembleOptions{})
		},
		func(touch contact360.LastTouch) attention.TouchMoments {
			return attention.TouchMoments{LastInbound: touch.InboundAt, LastOutbound: touch.OutboundAt}
		})
}

// attentionCompanyTouch is the same seam for a row about an account, through
// the company page's set reader.
type attentionCompanyTouch struct{ pool *pgxpool.Pool }

func (r attentionCompanyTouch) LastTouch(
	ctx context.Context, companyIDs []ids.UUID,
) (map[ids.UUID]attention.TouchMoments, error) {
	return touchMomentsThrough(ctx, r.pool, companyIDs,
		func(ctx context.Context, tx pgx.Tx, wanted []ids.CompanyID) (map[ids.CompanyID]company360.LastTouch, error) {
			return company360.LastTouchFor(ctx, tx, wanted, company360.AssembleOptions{})
		},
		func(touch company360.LastTouch) attention.TouchMoments {
			return attention.TouchMoments{LastInbound: touch.InboundAt, LastOutbound: touch.OutboundAt}
		})
}

// touchMomentsThrough asks a page's set reader, typed by record kind, about the
// bare ids the queue holds, inside the caller's workspace transaction.
func touchMomentsThrough[K interface {
	ids.EntityKind
	comparable
}, T any](
	ctx context.Context, pool *pgxpool.Pool, recordIDs []ids.UUID,
	read func(context.Context, pgx.Tx, []ids.ID[K]) (map[ids.ID[K]]T, error),
	moments func(T) attention.TouchMoments,
) (map[ids.UUID]attention.TouchMoments, error) {
	wanted := make([]ids.ID[K], 0, len(recordIDs))
	for _, id := range recordIDs {
		wanted = append(wanted, ids.From[K](id))
	}
	out := make(map[ids.UUID]attention.TouchMoments, len(recordIDs))
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		touched, err := read(ctx, tx, wanted)
		if err != nil {
			return err
		}
		for id, touch := range touched {
			out[id.UUID] = moments(touch)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
