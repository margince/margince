// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func recipientMailCopy(ctx context.Context, pool *pgxpool.Pool) mailcopy.Copy {
	profile, err := identity.NewService(pool).ActorProfile(ctx)
	if err != nil {
		slog.WarnContext(ctx, "recipient language unavailable; using installation fallback", "cause", err)
		return mailcopy.For(identity.BaseLanguageForPrompt(ctx, pool))
	}
	language := profile.Locale
	if !mailcopy.Known(language) {
		language = identity.BaseLanguageForPrompt(ctx, pool)
	}
	return mailcopy.For(language)
}

func roomInviteIdentity(ctx context.Context, pool *pgxpool.Pool) (string, string, error) {
	name, _, err := identity.NewService(pool).ActorIdentity(ctx)
	if err != nil {
		return "", "", err
	}
	var company string
	err = InstallationDB(pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		company, err = config.StoredApplyTx(ctx, tx, loadedConfig(), identity.Name)
		return err
	})
	return name, company, err
}

func recipientDeadline(ctx context.Context, pool *pgxpool.Pool, at time.Time) time.Time {
	hours, _, err := identity.NewService(pool).MyWorkingHours(ctx)
	if err == nil {
		var zone *time.Location
		zone, err = hours.Location()
		if err == nil {
			return at.In(zone)
		}
	}
	slog.WarnContext(ctx, "recipient timezone unavailable; displaying UTC", "cause", err)
	return at.UTC()
}
