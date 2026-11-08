// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The migrations that add the key columns and the indexes the re-key scans.
const (
	keyedIdentifiersMigration      = "core/1794125373"
	keyedIdentifiersIndexMigration = "core/1794125374"
)

// keyedIdentifierAudit is why these writes add no audit row: each re-encodes
// a stored hash or link under the installation's key, the form the runtime
// writers (storekit.SuppressionHash, the preference link minter) already
// write without one.
const keyedIdentifierAudit = "re-encodes a stored hash or link into the keyed form its runtime writer writes, without an audit row, as that writer does"

// keyedIdentifiersDataTask re-keys every identifier hashed before the
// installation had a root key, and hashes the stored preference links. With
// no root key every step inspects nothing.
func keyedIdentifiersDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794125373_keyed_privacy_identifiers", Version: 1,
		Title:    "Protecting stored erasure hashes and preference links with the installation key",
		Requires: []string{keyedIdentifiersMigration, keyedIdentifiersIndexMigration},
		Steps: []datatask.Step{
			suppressionListStep(),
			keyedUUIDStep("anonymized_addresses", privacy.RekeyAnonymizedAddressesPageTx, privacy.CountUnkeyedAnonymizedContacts),
			keyedUUIDStep("kept_stops", consent.RekeyKeptStopsPageTx, consent.CountUnkeyedStops),
			keyedUUIDStep("login_attempts", storekit.RekeyLoginAttemptsPageTx, storekit.CountUnkeyedLoginAttempts),
			keyedUUIDStep("preference_links", consent.HashPreferenceLinksPageTx, consent.CountStoredPreferenceLinks),
		},
	}
}

type keyedPage func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, error)

type keyedCount func(context.Context, storekit.RowQuerier, int64) (int64, error)

func keyedEstimate(count keyedCount) func(context.Context, datatask.Querier, int64) (int64, bool, error) {
	return func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
		if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
			return 0, false, err
		}
		found, err := count(ctx, q, maximum)
		return min(found, maximum), found > maximum, err
	}
}

func keyedUUIDStep(name string, correct keyedPage, count keyedCount) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 500, MinLimit: 1, EmptyWhenDone: true,
		Audit:    datatask.AuditPolicy{Reason: keyedIdentifierAudit},
		Estimate: keyedEstimate(count),
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			scanned, err := correct(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, scanned, len(scanned))
		},
	}
}

func suppressionListStep() datatask.Step {
	return datatask.Step{
		Name: "suppression_list", Limit: 500, MinLimit: 1, EmptyWhenDone: true,
		Audit:    datatask.AuditPolicy{Reason: keyedIdentifierAudit},
		Estimate: keyedEstimate(privacy.CountUnkeyedSuppressions),
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			var after privacy.SuppressionCursor
			if len(in.Cursor) > 0 {
				if err := json.Unmarshal(in.Cursor, &after); err != nil {
					return datatask.BatchOut{}, &storekit.MalformedCursorError{}
				}
			}
			page, err := privacy.RekeySuppressionListPageTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			out := datatask.BatchOut{Cursor: in.Cursor, Scanned: len(page), Changed: len(page), Done: len(page) < in.Limit}
			if len(page) > 0 {
				out.Cursor, err = json.Marshal(page[len(page)-1])
			}
			return out, err
		},
	}
}
