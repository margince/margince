// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Binding the correspondence envelope every drafting surface is handed.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// draftEnvelope builds the resolver that answers what language a draft is
// written in, what time it is, and who is writing it.
//
// One constructor for every drafting surface, which is the point: the contact
// composer, the account composer and the endpoints that run with no model lane
// all resolve the sender the same way, so a draft cannot be written as the
// right contact on one screen and as nobody on another.
//
// The sender and the installation's base language are identity's, and they are
// the only things here that reach the database. Everything else the resolver
// does is pure.
//
// The base language is the tier under the correspondence: a thread too short to
// detect, and a FIRST message which has no correspondence at all, both land on
// what the team said they work in rather than on English by default.
func draftEnvelope(pool *pgxpool.Pool, log *slog.Logger) *draftfloor.Resolver {
	return draftfloor.NewResolver().
		WithSender(identity.NewService(pool)).
		WithBaseLanguage(func(ctx context.Context) string {
			return identity.BaseLanguageForPrompt(ctx, pool)
		}).
		WithLogger(log)
}
