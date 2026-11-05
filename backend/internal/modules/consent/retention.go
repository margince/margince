// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The retention-policy defaults a new installation is planted with, editable
// per installation. One action per row; a ladder is separate rows at
// increasing retain_days.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SeedDefaultRetentionTx plants the default policies inside the
// workspace-bootstrap transaction, beside the purpose catalog.
//
// Each row is a default an admin may edit, and each lands `enabled`. A number
// changed here reaches only installations bootstrapped later; a running one
// needs a migration.
//
// Margince does not destroy a record just because it is old. A seeded row
// archives, which keeps the record restorable. Or it erases a second copy or
// derived data: the raw provider original, AI call payloads, deal-risk days.
// So `activity/transcript` and `contact/no_consent_no_deal` stay unseeded, and
// TestEveryActionableRetentionScopeShipsADefaultOrSaysWhyNot records why.
// An unconverted lead is archived too; an installation that must not keep its
// identity authors `anonymize`.
//
// `raw_capture` ages on received_at, the import time. The extracted activity
// and its `(source_system, source_id)` tombstone survive the erase, so only the
// second copy goes. Its selector keeps the correspondence floor and legal hold.
//
// `ai_call_payload` / `content` is the only bound on payloads an Art. 17
// erasure cannot find, because they cite no record and name no address.
// The doc of `ai.capture_payloads` names this policy.
//
// `deal_risk_day` keeps the widest window GET /worklist/response reads, and
// TestTheVerdictWindowIsTheFiguresWidestWindow keeps the two numbers one.
func SeedDefaultRetentionTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
		SELECT v.object_type, v.category, v.retain_days, v.action, 'storage_limitation'
		FROM (VALUES
		  ('lead',     'unconverted',        365,  'archive'),
		  ('activity', NULL,                 1095, 'archive'),
		  ('deal',     'lost',               1825, 'archive'),
		  ('ai_call_payload', 'content',     365,  'erase'),
		  ('raw_capture', NULL,              90,   'erase'),
		  ('deal_risk_day', NULL,            90,   'erase')
		) AS v(object_type, category, retain_days, action)`)
	return err
}
