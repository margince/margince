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
// Under the correspondence and the rep's typed purpose come the rep's own app
// language and then the installation's: a contact who has never written gets
// the language the sender reads, before the one the team configured.
func draftEnvelope(pool *pgxpool.Pool, log *slog.Logger) *draftfloor.Resolver {
	seats := identity.NewService(pool)
	return draftfloor.NewResolver().
		WithSender(seats).
		WithUserLanguage(func(ctx context.Context) string {
			return actorLocale(ctx, seats, log)
		}).
		WithBaseLanguage(func(ctx context.Context) string {
			return identity.BaseLanguageForPrompt(ctx, pool)
		}).
		WithLogger(log)
}

// actorLocale is the acting rep's chosen app language, or "" when they never
// chose one. A failed read degrades to the next tier rather than failing the
// draft, and says so in the log.
func actorLocale(ctx context.Context, seats *identity.Service, log *slog.Logger) string {
	profile, err := seats.ActorProfile(ctx)
	if err != nil {
		if log == nil {
			log = slog.Default()
		}
		log.WarnContext(ctx, "the rep's app language could not be read; the installation's is used instead",
			"reason", err)
		return ""
	}
	return profile.Locale
}
