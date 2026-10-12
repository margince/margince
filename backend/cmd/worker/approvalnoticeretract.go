// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Wiring the consumer that takes back a settled card's pending-decision lines.
//
// A staged proposal is announced to every seat that could decide it, because
// any of them may. When one does, the others are left holding a line asking for
// a decision that has been made — a badge that never clears and a card that
// refuses when opened.
//
// A consumer rather than a step on the decide path, for the reason the outcome
// ledger beside it gives: expiry is a verdict with no decider, written by the
// sweep, and it reaches the same approval.decided event. Clearing only what a
// human answered would leave every expired card's lines standing, which is the
// case where nobody is ever coming to clear them by hand.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
)

// startApprovalNoticeRetract subscribes the announcement's closing half.
//
// Deterministic like the projections beside it, so it runs on every worker: an
// installation without it accumulates undismissable lines in every seat that
// was ever asked to decide something somebody else decided.
func startApprovalNoticeRetract(
	ctx context.Context,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	background *sync.WaitGroup,
	logger *slog.Logger,
	stdout io.Writer,
) {
	retract := compose.NewApprovalNoticeRetract(
		pool, notices.NewStore(compose.InstallationDB(pool)), identity.NewService(pool), logger)
	_, _ = fmt.Fprintln(stdout,
		"worker taking back settled cards' decision notices (cg:approval-notice-retract)")
	background.Go(func() {
		runSubscriber(ctx, rdb, "cg:approval-notice-retract", retract.HandleEvent, logger, 0)
	})
}
