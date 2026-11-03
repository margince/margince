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
	"github.com/margince/margince/backend/internal/platform/settings"
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

// PreviewBinding binds switching enforcement on to the preview an admin saw:
// Seen is its digest, and Preview recomputes it. The zero value binds nothing.
type PreviewBinding struct {
	Seen    *string
	Preview PreviewFunc
}

// SetQualifiedForecastEnforced switches enforcement, as an admin's act. Turning
// it on waits for a check that judged deals. A run from before receipts existed,
// or one that failed, leaves every deal unknown and nothing to count.

// Switching on recomputes the bound preview and saves the setting in one
// transaction, under the setting's write lock. So the counts the admin saw are
// the counts the switch was made against.
func (s *Store) SetQualifiedForecastEnforced(ctx context.Context, cfg *config.Config, on bool, binding PreviewBinding) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	// The lock first, so a setting the environment fixes says so rather than
	// asking for a preview or a check that would not unlock it either.
	if _, err := config.LockedChange(cfg, QualifiedForecastEnforced, func(boot bool) bool { return boot == on }); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, QualifiedForecastEnforced.Key()); err != nil {
			return err
		}
		if on {
			if err := s.admitSwitchOn(ctx, tx, cfg, binding); err != nil {
				return err
			}
		}
		return config.SaveTx(ctx, cfg, tx, QualifiedForecastEnforced, on)
	})
}

// admitSwitchOn refuses switching on against a preview that is no longer true,
// and before any check has judged a deal. Switching on again while enforcing
// changes nothing, so it needs no preview.
func (s *Store) admitSwitchOn(ctx context.Context, tx pgx.Tx, cfg *config.Config, binding PreviewBinding) error {
	if binding.Preview != nil {
		enforcing, err := config.StoredValTx(ctx, tx, cfg, QualifiedForecastEnforced)
		if err != nil {
			return err
		}
		if !enforcing {
			current, err := binding.Preview(ctx, tx)
			if err != nil {
				return err
			}
			if binding.Seen == nil || *binding.Seen != current.Digest {
				return &httperr.DetailedError{
					Status: http.StatusConflict, Code: "preview_stale",
					Detail: "The deals this would leave out changed since you looked. " +
						"Check the new numbers, then switch it on again.",
				}
			}
		}
	}
	var judged bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assurance_deal_receipt)`).Scan(&judged); err != nil {
		return fmt.Errorf("assurance: reading whether any deal was judged: %w", err)
	}
	if !judged {
		return &httperr.DetailedError{
			Status: http.StatusConflict, Code: "input_check_not_started",
			Detail: "Start the input check first. Until it has run once, no deal has been judged, " +
				"so the qualified forecast would count nothing.",
		}
	}
	return nil
}

// ReadPreview builds the preview in a transaction of its own, for a reader
// that only shows it. The preview counts every open deal, so only an admin,
// who may switch enforcement, reads it.
func (s *Store) ReadPreview(ctx context.Context, preview PreviewFunc) (EnforcementPreview, error) {
	var out EnforcementPreview
	if err := auth.RequireAdmin(ctx); err != nil {
		return out, err
	}
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		var err error
		out, err = preview(ctx, tx)
		return err
	})
	return out, err
}

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{QualifiedForecastEnforced}
}
