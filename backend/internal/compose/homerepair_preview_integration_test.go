// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRepairPreviewDoesNotLockSourceOrPendingDecisions(t *testing.T) {
	for _, age := range []int{0, 90} {
		t.Run(strconv.Itoa(age), func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
			commitment, proposal := stageRepairProposal(t, e, now.AddDate(0, 0, -age))
			operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
			err := e.DB().Tx(operator, func(preview pgx.Tx) error {
				review, err := previewHomeRepairSourceTx(operator, preview, "approval", ids.Nil, 10, now)
				if err != nil {
					return err
				}
				if review.Limit != 1 {
					t.Fatalf("review count=%d, want one", review.Limit)
				}
				return e.DB().Tx(operator, func(writer pgx.Tx) error {
					if _, err := writer.Exec(operator, `SELECT id FROM activity WHERE id=@id FOR UPDATE NOWAIT`, pgx.NamedArgs{"id": commitment.SourceActivityID}); err != nil {
						return err
					}
					_, err := writer.Exec(operator, `SELECT id FROM approval WHERE id=@id FOR UPDATE NOWAIT`, pgx.NamedArgs{"id": proposal})
					return err
				})
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
