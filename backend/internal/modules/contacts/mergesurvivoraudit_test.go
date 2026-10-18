// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAMergeWhoseSurvivorRowCannotBeWrittenFailsTheMerge(t *testing.T) {
	var tx pgx.Tx // never reached: an audit with no actor is refused before any SQL
	err := auditMergeSurvivor(context.Background(), tx, "contact", ids.NewV7(), ids.NewV7(), nil)
	if err == nil || !strings.Contains(err.Error(), "audit contact merge on the survivor") {
		t.Fatalf("auditMergeSurvivor = %v, want the failure named for the survivor's row", err)
	}
}
