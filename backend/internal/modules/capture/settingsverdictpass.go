// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// How much one confidentiality pass may judge. A setting rather than process
// configuration because the right answer follows the mail an installation
// gets: a quiet one never reaches the default, a first backfill or a burst from
// one sender wants more per pass, and an admin who sees the oldest held thread
// waiting longer than the pass interval should be able to raise it without a
// redeploy.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// The default and ceiling, exported for the engine that sizes its pass against
// the default.
//
// The ceiling is what the pass can finish inside its job timeout: every thread
// is one model call, so 200 of them leave a pass of the declared twenty minutes
// six seconds a thread. A higher number would only be a pass the timeout cuts
// short.
const (
	DefaultConfidentialityPassCap = 40
	MaxConfidentialityPassCap     = 200
)

// ConfidentialityPassCap is how many held threads one pass judges per
// workspace. The remainder stays claimable and is judged by the next pass, so
// the cap paces how fast a backlog drains and never decides whether a thread is
// judged.
//
// Read at the start of every pass, which is why it is MachineryApplied: the
// engine runs as a system principal that holds no `capture_settings` grant.
var ConfidentialityPassCap = config.DefineStored(config.Spec[int]{
	Key:     "capture.confidentiality_pass_cap",
	Env:     "MARGINCE_CAPTURE_CONFIDENTIALITY_PASS_CAP",
	Type:    config.Int,
	Default: config.Some(DefaultConfidentialityPassCap),
	Min:     config.Some(1),
	Max:     config.Some(MaxConfidentialityPassCap),
	Doc:     "Held mail threads one confidentiality pass may judge; the rest wait for the next pass.",
}, config.Store{
	RBACObject: captureSettingsObject, AuditVerb: captureAuditVerb,
	MachineryApplied: true, // the engine reads it at the start of every pass
})

// PassCap is the cap in force for the pass that is about to start. It is read
// per pass, never held on the engine: an admin's change is then the next pass's
// cap, and a boot source that sets the key wins here as it does in the app.
func (s *ThreadVerdictStore) PassCap(ctx context.Context) (int, error) {
	var passCap int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		passCap, err = config.StoredApplyTx(ctx, tx, config.Bound(), ConfidentialityPassCap)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("capture: reading the confidentiality pass cap: %w", err)
	}
	return passCap, nil
}
