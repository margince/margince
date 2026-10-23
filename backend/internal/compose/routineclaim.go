// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var errRoutineClaimSpent = errors.New("routine edition already claimed")

type routineClaim struct {
	morning briefs.MailAttempt
	week    weekly.MailAttempt
	digest  bool
}

func routineBriefEngine(pool *pgxpool.Pool) *briefs.BriefEngine {
	return briefs.NewBriefEngine(pool, contacts.NewStore(InstallationDB(pool)))
}

func claimRoutine(ctx context.Context, pool *pgxpool.Pool, runID, reviewID ids.UUID, part morningDigest, now time.Time, holds ...func(pgx.Tx) error) (routineClaim, bool, error) {
	var claim routineClaim
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var won bool
		var err error
		claim.morning, won, err = routineBriefEngine(pool).ClaimMailAttemptTx(ctx, tx, runID, now)
		if err != nil {
			return err
		}
		if !won {
			return errRoutineClaimSpent
		}
		if claim.morning.Email == "" {
			return errRoutineClaimSpent
		}
		if !reviewID.IsZero() {
			claim.week, won, err = newWeeklyEngine(pool).ClaimMailAttemptTx(ctx, tx, reviewID, now)
			if err != nil {
				return err
			}
			if !won || claim.week.Email == "" {
				return errRoutineClaimSpent
			}
		}
		if part.lines.total > 0 {
			claim.digest, err = notices.NewStore(InstallationDB(pool)).ClaimDigestRunTx(ctx, tx, ids.From[ids.UserKind](mailUser(ctx)), part.window.Day)
		}
		for _, hold := range holds {
			if err == nil {
				err = hold(tx)
			}
		}
		return err
	})
	if errors.Is(err, errRoutineClaimSpent) {
		return routineClaim{}, false, nil
	}
	return claim, err == nil, err
}

func recordRoutineFailure(ctx context.Context, pool *pgxpool.Pool, claim routineClaim, part morningDigest, cause string, log *slog.Logger) {
	if err := routineBriefEngine(pool).MailFailed(ctx, claim.morning.Run.ID, cause); err != nil {
		log.WarnContext(ctx, "combined morning failure could not be recorded", "cause", err)
	}
	if !claim.week.Review.ID.IsZero() {
		if err := newWeeklyEngine(pool).MailFailed(ctx, claim.week.Review.ID, cause); err != nil {
			log.WarnContext(ctx, "combined weekly failure could not be recorded", "cause", err)
		}
	}
	if claim.digest {
		if err := notices.NewStore(InstallationDB(pool)).DigestFailed(ctx, ids.From[ids.UserKind](mailUser(ctx)), part.window.Day, cause); err != nil {
			log.WarnContext(ctx, "combined digest failure could not be recorded", "cause", err)
		}
	}
}
