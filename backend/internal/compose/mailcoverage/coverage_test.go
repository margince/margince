// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcoverage

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestUnavailableObservationPreservesRecordedEvidenceWithoutConnectionDetails(t *testing.T) {
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		err   error
		state crmcontracts.MailCaptureCoverageState
	}{
		{"denied", apperrors.ErrPermissionDenied, crmcontracts.MailCaptureCoverageStateWithheld},
		{"failed", errors.New("fixture boundary failure"), crmcontracts.MailCaptureCoverageStateUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(context.Context, *time.Time, time.Time) (crmcontracts.MailCaptureCoverage, error) {
				return crmcontracts.MailCaptureCoverage{LastSuccessAt: &at}, tc.err
			}
			got := Read(context.Background(), read, &at, at)
			if got.State != tc.state || got.LastSuccessAt != nil || got.EvaluatedAt != at || got.IntervalFrom == nil || *got.IntervalFrom != at {
				t.Fatalf("observation fallback = %+v", got)
			}
		})
	}
	got := Read(context.Background(), nil, nil, at)
	if got.State != crmcontracts.MailCaptureCoverageStateUnknown || got.EvaluatedAt != at {
		t.Fatalf("unbound observation = %+v", got)
	}
}
