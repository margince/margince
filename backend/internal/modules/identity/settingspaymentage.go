// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/config"
)

// PaymentLongOverdueDays only groups Home invoices; it never writes off debt.
var PaymentLongOverdueDays = config.DefineStored(config.Spec[int]{
	Key:  "installation.payment_long_overdue_days",
	Env:  "MARGINCE_INSTALLATION_PAYMENT_LONG_OVERDUE_DAYS",
	Type: config.Int, Default: config.Some(365),
	Validate: func(days int) error {
		if days < 1 || days > 3650 {
			return fmt.Errorf("long overdue age must be 1..3650 days")
		}
		return nil
	},
	Doc: "Days overdue after which Home folds invoices into Long overdue. Company totals stay the same.",
}, config.Store{Cacheable: true, RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true, MachineryApplied: true})

// PaymentAgeForFinance applies the grouping preference without granting settings access.
func PaymentAgeForFinance(ctx context.Context, tx pgx.Tx) (int, error) {
	return config.StoredApplyTx(ctx, tx, config.Bound(), PaymentLongOverdueDays)
}
