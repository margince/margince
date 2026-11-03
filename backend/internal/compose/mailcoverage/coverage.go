// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailcoverage keeps an unavailable observation from hiding recorded evidence.
package mailcoverage

import (
	"context"
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Reader keeps capture authority outside record-page transactions.
type Reader func(context.Context, *time.Time, time.Time) (crmcontracts.MailCaptureCoverage, error)

// An optional connection read cannot spend the record page's response window.
const observationBudget = time.Second

// Read preserves record access when the optional capture observation is unavailable.
func Read(ctx context.Context, read Reader, from *time.Time, at time.Time) crmcontracts.MailCaptureCoverage {
	out := crmcontracts.MailCaptureCoverage{
		Channel:      crmcontracts.MailCaptureCoverageChannelEmail,
		State:        crmcontracts.MailCaptureCoverageStateUnknown,
		IntervalFrom: from, IntervalTo: at, EvaluatedAt: at,
	}
	if read == nil {
		return out
	}
	var coverage crmcontracts.MailCaptureCoverage
	err := database.WithDegradableRead(ctx, observationBudget, func(ctx context.Context) error {
		var err error
		coverage, err = read(ctx, from, at)
		return err
	})
	if err == nil {
		return coverage
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		out.State = crmcontracts.MailCaptureCoverageStateWithheld
	} else {
		slog.WarnContext(ctx, "mail capture coverage could not be observed", "reason", err)
	}
	return out
}
