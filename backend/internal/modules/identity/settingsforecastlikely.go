// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// ForecastLikelyThreshold is the win probability, in percent, from which an
// open deal with a confirmed close date counts in the Likely forecast.
var ForecastLikelyThreshold = config.DefineStored(config.Spec[int]{
	Key:     "installation.forecast_likely_threshold",
	Env:     "MARGINCE_INSTALLATION_FORECAST_LIKELY_THRESHOLD",
	Type:    config.Int,
	Default: config.Some(90),
	Validate: func(percent int) error {
		if percent < 1 || percent > 100 {
			return fmt.Errorf("the Likely threshold is a percent from 1 to 100, not %d", percent)
		}
		return nil
	},
	Doc: "The win probability, in percent, from which an open deal with a confirmed close date counts in the Likely forecast.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true,
	MachineryApplied: true, // every forecast reader applies it, whatever their grants
})

// ForecastLikelyThresholdTx reads the threshold inside a transaction the caller
// already holds, ungated: every forecast obeys it, whatever the reader's grants.
func ForecastLikelyThresholdTx(ctx context.Context, tx pgx.Tx) (int, error) {
	return config.StoredApplyTx(ctx, tx, config.Bound(), ForecastLikelyThreshold)
}

func (s *InstallationSettingsStore) readForecastLikelyThreshold(ctx context.Context, out *InstallationSettings) error {
	var err error
	out.ForecastLikelyThreshold, err = config.StoredVal(ctx, s.config, ForecastLikelyThreshold)
	return err
}

func (p InstallationPatch) forecastLikelyThresholdWrites() ([]pendingWrite, error) {
	write, err := encodeStoredField(ForecastLikelyThreshold, p.ForecastLikelyThreshold)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{write}, nil
}
