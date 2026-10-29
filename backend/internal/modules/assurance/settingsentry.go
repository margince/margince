// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
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
// it on waits for the first input check. Before that every deal is unknown, and
// the qualified forecast would have nothing to count.
func (s *Store) SetQualifiedForecastEnforced(ctx context.Context, cfg *config.Config, on bool) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	if on {
		started, err := s.EverRun(ctx)
		if err != nil {
			return err
		}
		if !started {
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
