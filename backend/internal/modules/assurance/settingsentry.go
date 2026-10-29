// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// QualifiedForecastEnforced is whether the qualified forecast applies the
// viability reviews. While on, a deal under an open review adds nothing to the
// qualified readings, and the close-date sweep stops lowering categories.
//
// DEFAULT FALSE, unlike most switches here. Turning it on changes forecast
// figures somebody may already report. An enrolled installation sees the
// reviews and tasks first, reads the preview, and an admin then turns it on.
// While it is off the qualified readings are absent, never the baseline under
// another name.
var QualifiedForecastEnforced = config.DefineStored(config.Spec[bool]{
	Key:     "assurance.qualified_forecast_enforced",
	Env:     "MARGINCE_ASSURANCE_QUALIFIED_FORECAST_ENFORCED",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Leave a deal whose customer stopped answering out of the forecast line of deals the customer supports, and stop the close-date sweep lowering forecast categories.",
}, config.Store{
	RBACObject: "installation_settings", AuditVerb: "update",
	MachineryApplied: true, // the forecast read and the sweep read it for every seat
})

// SetQualifiedForecastEnforced switches enforcement, as an admin's act. Turning
// it on waits for a check that judged deals. A run from before receipts existed,
// or one that failed, leaves every deal unknown and nothing to count.
func (s *Store) SetQualifiedForecastEnforced(ctx context.Context, cfg *config.Config, on bool) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	if on {
		judged, err := s.hasReceipts(ctx)
		if err != nil {
			return err
		}
		if !judged {
			return &httperr.DetailedError{
				Status: http.StatusConflict, Code: "input_check_not_started",
				Detail: "Start the input check first. Until it has run once, no deal has been judged, " +
					"so the qualified forecast would count nothing.",
			}
		}
	}
	return config.Save(ctx, cfg, QualifiedForecastEnforced, on)
}

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{QualifiedForecastEnforced}
}

// hasReceipts answers whether any check has judged a deal yet.
func (s *Store) hasReceipts(ctx context.Context) (bool, error) {
	var judged bool
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assurance_deal_receipt)`).Scan(&judged)
	})
	if err != nil {
		return false, fmt.Errorf("assurance: reading whether any deal was judged: %w", err)
	}
	return judged, nil
}
