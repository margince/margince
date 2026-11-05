// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// IndividualPerformance is whether anyone may measure a colleague's work one by
// one. Off, every reader keeps their own figures and the team's totals. No
// surface shows a number keyed to another colleague. A works council can rule
// out assessing individual employees without taking grants away from roles.
var IndividualPerformance = config.DefineStored(config.Spec[bool]{
	Key:     "installation.individual_performance",
	Env:     "MARGINCE_INSTALLATION_INDIVIDUAL_PERFORMANCE",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Show each colleague's figures in reports, forecasts and team reviews. Off, each member sees their own figures and team totals only.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true,
	MachineryApplied: true, // every reader's analytics and the weekly job apply it, whatever their grants
})

// IndividualPerformanceTx reads the switch inside a transaction the caller
// already holds, ungated: every reader's figures obey it, whatever their grants.
func IndividualPerformanceTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	return config.StoredApplyTx(ctx, tx, config.Bound(), IndividualPerformance)
}

// individualPerformance reads the switch for a door that holds no transaction.
func (s *Service) individualPerformance(ctx context.Context) (bool, error) {
	var allowed bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		allowed, err = IndividualPerformanceTx(ctx, tx)
		return err
	})
	return allowed, err
}

func (s *InstallationSettingsStore) readIndividualPerformance(ctx context.Context, out *InstallationSettings) error {
	var err error
	out.IndividualPerformance, err = config.StoredVal(ctx, s.config, IndividualPerformance)
	return err
}

func (p InstallationPatch) individualPerformanceWrites() ([]pendingWrite, error) {
	write, err := encodeStoredField(IndividualPerformance, p.IndividualPerformance)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{write}, nil
}
