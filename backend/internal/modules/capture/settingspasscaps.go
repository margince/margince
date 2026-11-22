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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
var VerdictPassCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.verdict_pass_cap",
	Env:     "MARGINCE_CAPTURE_VERDICT_PASS_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultVerdictPassCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxVerdictPassCap),
	Doc:     "Unresolved senders one counterparty review run may judge; the rest wait for the next run.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, Changed: emitSettingChanged("capture.verdict_pass_cap"),
})

// ClassifyPassCap is how many captured messages one label run settles per
// workspace. An unlabeled message stays in the backlog for the next run.
var ClassifyPassCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.classify_pass_cap",
	Env:     "MARGINCE_CAPTURE_CLASSIFY_PASS_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultClassifyPassCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxClassifyPassCap),
	Doc:     "Captured messages one labeling run may handle; the rest wait for the next run.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, Changed: emitSettingChanged("capture.classify_pass_cap"),
})

// OwedClassifyPassCap is how many new messages one owed-reply run judges per
// workspace. The re-judge of older mail has its own budget.
var OwedClassifyPassCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.owed_classify_pass_cap",
	Env:     "MARGINCE_CAPTURE_OWED_CLASSIFY_PASS_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultOwedClassifyPassCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxOwedClassifyPassCap),
	Doc:     "New messages one owed-reply run may judge; the rest wait for the next run.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, Changed: emitSettingChanged("capture.owed_classify_pass_cap"),
})

// emitSettingChanged is the capture_settings.updated event for one key, in the
// transaction of the write that changed it.
func emitSettingChanged(key string) func(context.Context, pgx.Tx, ids.UUID) error {
	return func(ctx context.Context, tx pgx.Tx, auditID ids.UUID) error {
		return storekit.EmitEvent(ctx, tx, auditID, storekit.MustWorkspace(ctx),
			crmcontracts.InternalEventCaptureSettingsUpdated{ChangedKeys: []string{key}})
	}
}

// PassCaps reads the caps in force for the pass about to start.
type PassCaps struct{ db *database.DB }

// NewPassCaps builds the reader over the installation's workspace handle.
func NewPassCaps(db *database.DB) *PassCaps { return &PassCaps{db: db} }

// Verdict is the cap of the counterparty verdict run.
func (p *PassCaps) Verdict(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, VerdictPassCap.Key(), func(tx pgx.Tx) (int, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(), VerdictPassCap)
	})
}

// Classify is the cap of the capture label run.
func (p *PassCaps) Classify(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, ClassifyPassCap.Key(), func(tx pgx.Tx) (int, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(), ClassifyPassCap)
	})
}

// OwedClassify is the cap of the owed-reply run.
func (p *PassCaps) OwedClassify(ctx context.Context) (int, error) {
	return readPassCap(ctx, p.db, OwedClassifyPassCap.Key(), func(tx pgx.Tx) (int, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(), OwedClassifyPassCap)
	})
}

// readPassCap runs one cap read in its own transaction. A boot source that sets
// the key wins here as it does in the app.
func readPassCap(ctx context.Context, db *database.DB, key string, read func(tx pgx.Tx) (int, error)) (int, error) {
	var passCap int
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		passCap, err = read(tx)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("capture: reading the %s setting: %w", key, err)
	}
	return passCap, nil
}
