// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An owner deciding, for themselves, whether a thread is the team's to read.
//
// The classifier answers first and is usually right; this is what happens when
// it is not. A founder whose customer thread was held as `legal` shares it; a
// rep whose ordinary-looking thread turned personal keeps it private. Either
// way the decision is the owner's OWN contribution to the message's audience —
// it never overrules a colleague, because a message reaching two mailboxes
// ends at the strictest of what the two of them ask for.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ThreadAudienceOutcome reports what an owner's decision did, and — when it did
// less than they asked — why.
type ThreadAudienceOutcome struct {
	// Messages is how many of the thread's messages this seat imported and the
	// decision therefore reached.
	Messages int `json:"messages"`
	// Shared reports whether the messages are now readable by the workspace.
	// False after a share means somebody else still holds them.
	Shared bool `json:"shared"`
	// HeldByOthers names how many OTHER seats still ask for the thread to be
	// held. Reported by count and never by name or reason: whose mail a person
	// keeps private is itself private, and "your colleague is holding this"
	// already says more than a held message should.
	HeldByOthers int `json:"held_by_others"`
}

// ThreadAudienceSetter applies an owner's decision to their own view of a
// thread.
type ThreadAudienceSetter struct {
	pool    *pgxpool.Pool
	threads *capture.ThreadVerdictStore
}

// NewThreadAudienceSetter builds the setter over one database.
func NewThreadAudienceSetter(pool *pgxpool.Pool) *ThreadAudienceSetter {
	return &ThreadAudienceSetter{
		pool:    pool,
		threads: capture.NewThreadVerdictStore(InstallationDB(pool)),
	}
}

// Decide records what this owner concluded and re-derives every message of the
// thread they imported.
//
// One transaction, so a reader never falls between the ledger row and the
// audience of the messages it governs — a thread that said one thing and showed
// another for even a moment is a thread somebody screenshotted.
func (s *ThreadAudienceSetter) Decide(ctx context.Context, threadKey string, share bool) (ThreadAudienceOutcome, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return ThreadAudienceOutcome{}, apperrors.ErrPermissionDenied
	}
	// A read seat is licensed to look, not to change what colleagues can read.
	// The same pair the purge takes, and for the same reason: being a person is
	// not a grant, and RequireHuman inside the store reads none.
	if !actor.SeatType.CanMutate() {
		return ThreadAudienceOutcome{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return ThreadAudienceOutcome{}, err
	}
	if threadKey == "" {
		return ThreadAudienceOutcome{}, apperrors.ErrNotFound
	}
	var outcome ThreadAudienceOutcome
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// The seat's OWN messages on the thread. A caller who imported none has
		// nothing to decide about, and answering not-found rather than zero
		// keeps a thread key from confirming that somebody else's thread exists.
		messages, err := capture.ThreadActivityIDsTx(ctx, tx, threadKey, actor.UserID)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			return apperrors.ErrNotFound
		}
		if err := s.threads.DecideAsOwner(ctx, tx, threadKey, share); err != nil {
			return err
		}
		for _, id := range messages {
			if err := activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](id)); err != nil {
				return err
			}
		}
		outcome.Messages = len(messages)
		held, err := othersHoldingTx(ctx, tx, messages, actor.UserID)
		if err != nil {
			return err
		}
		outcome.HeldByOthers = held
		outcome.Shared = share && held == 0
		return nil
	})
	if err != nil {
		return ThreadAudienceOutcome{}, err
	}
	return outcome, nil
}

// othersHoldingTx counts the OTHER seats whose contribution still holds the
// caller's OWN messages.
//
// It is what makes a share honest: an owner who shares a thread a colleague is
// holding gets a 200 and a message that stayed private, and being told the
// count is the difference between "the product ignored me" and "somebody else
// has a say here too".
//
// Over the activity ids the caller imported, NEVER over the thread key. A
// thread key is the RFC822 References root taken verbatim from a sender's
// header, so it is both guessable and forgeable, and the workspace shares one
// namespace of them. Counting by thread key would walk messages the caller
// never received — and a capture_import row is itself an arm of the audience
// gate (platform/auth ActivityContentClause), so the count would read exactly
// the membership a held message hides. A seat could then map which colleagues
// are on which private conversations, one thread key at a time, without ever
// reading a word of them.
//
// It is also the honest number: the caller is owed how many colleagues co-hold
// THEIR messages, not how many hold a stranger's message that happens to carry
// the same header value.
func othersHoldingTx(ctx context.Context, tx pgx.Tx, messages []ids.UUID, user ids.UUID) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	var held int
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT i.user_id)
		  FROM capture_import i
		 WHERE i.activity_id = ANY($1) AND i.user_id <> $2
		   AND (coalesce(i.posture_at_import, 'shared') <> 'shared'
		        OR i.verdict_status IN ('held', 'unsure', 'held_by_owner', 'pending'))`,
		messages, user).Scan(&held); err != nil {
		return 0, fmt.Errorf("compose: counting the seats still holding a thread: %w", err)
	}
	return held, nil
}
