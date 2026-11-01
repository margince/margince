// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Reads exactly POC_READS Worklist first pages for the member POC_MEMBER_INDEX,
// so pg_stat_statements can be read around it.
func TestWorklistStatementCensusPoC(t *testing.T) {
	owner, app := pocPools(t)
	reads, _ := strconv.Atoi(os.Getenv("POC_READS"))
	index, _ := strconv.Atoi(os.Getenv("POC_MEMBER_INDEX"))
	asOf := time.Now().UTC()
	service := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })
	m := pocMembers(t, owner, app)[index]
	for range reads {
		ctx := principal.WithCorrelationID(m.ctx, ids.UUID(uuid.Must(uuid.NewV7())))
		if _, err := service.WorklistView(ctx, "today", "mine", "", ids.Nil, 25, ""); err != nil {
			t.Fatal(err)
		}
	}
}
