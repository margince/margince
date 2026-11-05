// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The thresholds the Account coverage section uses when the framework names
// none. An admin publishes others with the framework.
const (
	defaultMaxEngagedSeats = 1
	defaultUntouchedDays   = 21
	defaultSilentDays      = 30
	defaultWonWithinDays   = 180
)

// AccountCoverageThresholds answers the thresholds in force, every field
// filled, and the framework revision they came from. It needs no framework
// grant: a threshold discloses no figure.
func AccountCoverageThresholds(ctx context.Context, tx pgx.Tx) (crmcontracts.AccountCoverageThresholds, int64, error) {
	framework, _, err := currentFramework(ctx, tx)
	if err != nil {
		return crmcontracts.AccountCoverageThresholds{}, 0, err
	}
	return filledThresholds(framework.Definition.AccountCoverage), framework.Revision, nil
}

func filledThresholds(in *crmcontracts.AccountCoverageThresholds) crmcontracts.AccountCoverageThresholds {
	seats, untouched, silent, won := defaultMaxEngagedSeats, defaultUntouchedDays, defaultSilentDays, defaultWonWithinDays
	if in != nil {
		if in.MaxEngagedSeats != nil {
			seats = *in.MaxEngagedSeats
		}
		if in.UntouchedDays != nil {
			untouched = *in.UntouchedDays
		}
		if in.SilentDays != nil {
			silent = *in.SilentDays
		}
		if in.WonWithinDays != nil {
			won = *in.WonWithinDays
		}
	}
	return crmcontracts.AccountCoverageThresholds{MaxEngagedSeats: &seats, UntouchedDays: &untouched, SilentDays: &silent, WonWithinDays: &won}
}
