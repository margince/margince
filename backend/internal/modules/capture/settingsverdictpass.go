// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// How much one confidentiality pass may judge. A setting rather than process
// configuration, because the right number follows the mail an installation
// gets. A burst from one sender wants more per pass; a quiet mailbox never
// reaches the default.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The default and the ceiling. The ceiling catches a typo and is not a computed
// capacity. A pass the job timeout cuts short loses nothing, because each answer
// commits on its own and the rest stays claimable for the next pass.
const (
	DefaultConfidentialityPassCap = 40
	MaxConfidentialityPassCap     = 200
)

// ConfidentialityPassCap is how many held threads one pass asks the model about
// per workspace, answered or not. The rest stay claimable for the next pass, so
// the cap sets how fast a backlog drains and never whether a thread is judged.
//
// It is MachineryApplied because the engine runs as a system principal that
// holds no `capture_settings` grant.
var ConfidentialityPassCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.confidentiality_pass_cap",
	Env:     "MARGINCE_CAPTURE_CONFIDENTIALITY_PASS_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultConfidentialityPassCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxConfidentialityPassCap),
	Doc:     "Held mail threads one review pass may judge; the rest wait for the next pass.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, Changed: emitSettingChanged("capture.confidentiality_pass_cap"),
})

// PassCap is the cap in force for the pass about to start. It is read per pass
// and never held on the engine, so an admin's change is the next pass's cap.
func (s *ThreadVerdictStore) PassCap(ctx context.Context) (int, error) {
	return readPassCap(ctx, s.db, ConfidentialityPassCap.Key(), func(tx pgx.Tx) (int, error) {
		return config.StoredApplyTx(ctx, tx, config.Bound(ctx), ConfidentialityPassCap)
	})
}
