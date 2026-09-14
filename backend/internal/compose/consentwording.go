// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Publishing the wording this installation stands behind.
//
// Two callers and two reasons. Workspace seeding publishes so the very first
// grant has a row to name; every boot republishes so an UPGRADED installation
// — which skips seeding entirely, because its workspace already exists — is not
// left pinning versions nothing has published.
//
// Both are safe to repeat: PublishTextVersionTx is idempotent on
// key + version + locale, and refuses when the same version's words have
// changed. So a boot that finds the wording edited without a version bump fails
// loudly here rather than quietly serving one thing while past proofs name
// another.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// publishConsentWording republishes every wording this build ships, on each
// boot. See the call site for why it cannot live in workspace seeding alone.
//
// UNDER A SYSTEM PRINCIPAL, which is not decoration: publishing a wording
// writes an audit row, and storekit refuses one with no actor bound to the
// context. Boot starts from context.Background(), so passing it straight
// through made the publish fail — on exactly the upgraded installations this
// exists for, where the rows really are missing, and the API then exits on
// every restart. A fresh installation hid it, because seeding supplies an
// actor and publishes everything before this runs.
//
// The same shape workspace seeding uses (identity's bootstrap): a system
// principal and a fresh correlation id, because boot IS the originating
// operation here and the audit rows want something to trace to.
func publishConsentWording(ctx context.Context, pool *pgxpool.Pool, wsID ids.UUID) error {
	bootCtx := principal.WithActor(principal.WithWorkspaceID(ctx, wsID), principal.Principal{
		Type: principal.PrincipalSystem, ID: systemActor,
	})
	bootCtx = principal.WithCorrelationID(bootCtx, ids.NewV7())
	return InstallationDB(pool).Tx(bootCtx, func(tx pgx.Tx) error {
		now := time.Now()
		if err := consent.PublishControllerTemplatesTx(bootCtx, tx, now); err != nil {
			return err
		}
		return consent.PublishMarketingQuestionTx(bootCtx, tx, now)
	})
}

// seedConsentText publishes the installation's own wording beside the purpose
// catalog and the retention defaults, in the same transaction.
//
// Here rather than at a later door because a proof row may name a version from
// the first grant onwards: wording published after the fact would leave the
// earliest proofs pointing at nothing, and those are exactly the rows nobody
// can reconstruct later.
func seedConsentText(ctx context.Context, tx pgx.Tx) error {
	now := time.Now()
	if err := consent.PublishControllerTemplatesTx(ctx, tx, now); err != nil {
		return err
	}
	// The question the confirm PAGE asks, which is what a subscription consent
	// is actually given to. Published here beside the mail wording and for the
	// same reason this whole function runs at boot: a grant names the row, so
	// the row has to exist before the first grant can.
	if err := consent.PublishMarketingQuestionTx(ctx, tx, now); err != nil {
		return err
	}
	return consent.SeedDefaultRetentionTx(ctx, tx)
}
