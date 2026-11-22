// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// How much each model pass of the capture pipeline may judge in one run. They
// are settings for the reason ConfidentialityPassCap is: the right number
// follows the mail an installation gets. Each pass reads its cap at the start
// of a run, so an admin's change applies to the next run without a restart.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
)

// The defaults and the ceilings. A ceiling is five times the default, which
// catches a typo and is not a computed capacity. A run the job timeout cuts
// short loses nothing, because each answer commits on its own.
const (
	DefaultVerdictPassCap      = 200
	MaxVerdictPassCap          = 1000
	DefaultClassifyPassCap     = 500
	MaxClassifyPassCap         = 2500
	DefaultOwedClassifyPassCap = 500
	MaxOwedClassifyPassCap     = 2500
)

// VerdictPassCap is how many unresolved senders one counterparty verdict run
// asks the model about per workspace. The rest stay claimable for the next run.
var VerdictPassCap = definePassCap("capture.verdict_pass_cap",
	"MARGINCE_CAPTURE_VERDICT_PASS_CAP", DefaultVerdictPassCap, MaxVerdictPassCap,
	"Unresolved senders one counterparty review run may judge; the rest wait for the next run.")

// ClassifyPassCap is how many captured messages one label run settles per
// workspace. An unlabelled message stays in the backlog for the next run.
var ClassifyPassCap = definePassCap("capture.classify_pass_cap",
	"MARGINCE_CAPTURE_CLASSIFY_PASS_CAP", DefaultClassifyPassCap, MaxClassifyPassCap,
	"Captured messages one labelling run may settle; the rest wait for the next run.")

// OwedClassifyPassCap is how many new messages one owed-reply run judges per
// workspace. The re-judge of older mail has its own budget.
var OwedClassifyPassCap = definePassCap("capture.owed_classify_pass_cap",
	"MARGINCE_CAPTURE_OWED_CLASSIFY_PASS_CAP", DefaultOwedClassifyPassCap, MaxOwedClassifyPassCap,
	"New messages one owed-reply run may judge; the rest wait for the next run.")

// definePassCap declares one pass cap. MachineryApplied because the passes run
// as system principals that hold no `capture_settings` grant.
func definePassCap(key, env string, def, maxCap int, doc string) *config.Setting[int] {
	return config.DefineStored(config.Spec[int]{
		Key: key, Env: env, Type: config.Int,
		Default: config.Some(def), Min: config.Some(1), Max: config.Some(maxCap),
		Doc: doc,
	}, config.Store{
		RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
		MachineryApplied: true,
	})
}

// PassCaps reads the caps in force for the pass about to start.
type PassCaps struct{ db *database.DB }

// NewPassCaps builds the reader over the installation's workspace handle.
func NewPassCaps(db *database.DB) *PassCaps { return &PassCaps{db: db} }

// Verdict is the cap of the counterparty verdict run.
func (p *PassCaps) Verdict(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, VerdictPassCap)
}

// Classify is the cap of the capture label run.
func (p *PassCaps) Classify(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, ClassifyPassCap)
}

// OwedClassify is the cap of the owed-reply run.
func (p *PassCaps) OwedClassify(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, OwedClassifyPassCap)
}

// readPassCap reads one cap inside its own transaction. A boot source that sets
// the key wins here as it does in the app.
func readPassCap(ctx context.Context, db *database.DB, entry *config.Setting[int]) (int, error) {
	var passCap int
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		passCap, err = config.StoredApplyTx(ctx, tx, config.Bound(), entry)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("capture: reading the %s setting: %w", entry.Key(), err)
	}
	return passCap, nil
}
