// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var errCompanyFactsRefused = errors.New("the database refused the statement")

// refusingCompanyFactsTx refuses every statement.
type refusingCompanyFactsTx struct{ pgx.Tx }

func (refusingCompanyFactsTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errCompanyFactsRefused
}

// A refused delete of the subject's company facts fails the erasure rather
// than leaving the facts behind it unreported.
func TestARefusedCompanyFactPurgeFailsTheErasure(t *testing.T) {
	err := purgeSubjectCompanyEvidence(context.Background(), refusingCompanyFactsTx{}, ids.NewV7())
	if !errors.Is(err, errCompanyFactsRefused) {
		t.Errorf("err = %v, want the refusal", err)
	}
}
