// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// ForecastSelfCorrecting is whether the forecast moves stage chances toward the
// installation's recorded outcomes and applies learned risk effects. Off, every
// surface weights an open deal by its stage's typed chance alone.
var ForecastSelfCorrecting = config.DefineStored(config.Spec[bool]{
	Key:     "installation.forecast_self_correcting",
	Env:     "MARGINCE_INSTALLATION_FORECAST_SELF_CORRECTING",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let the forecast correct itself: each stage's rate moves with the outcome of the deals recorded from it, and a risk signal counts once frozen forecasts show its effect. Off, the forecast uses the rate set on each stage.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true,
	MachineryApplied: true, // every forecast reader applies it, whatever their grants
})

// ForecastSelfCorrectingTx reads the switch inside a transaction the caller
// already holds, ungated: every forecast obeys it, whatever the reader's grants.
func ForecastSelfCorrectingTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	return config.StoredApplyTx(ctx, tx, config.Bound(), ForecastSelfCorrecting)
}

func (s *InstallationSettingsStore) readForecastSelfCorrecting(ctx context.Context, out *InstallationSettings) error {
	var err error
	out.ForecastSelfCorrecting, err = config.StoredVal(ctx, s.config, ForecastSelfCorrecting)
	return err
}

func (p InstallationPatch) forecastSelfCorrectingWrites() ([]pendingWrite, error) {
	write, err := encodeStoredField(ForecastSelfCorrecting, p.ForecastSelfCorrecting)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{write}, nil
}
