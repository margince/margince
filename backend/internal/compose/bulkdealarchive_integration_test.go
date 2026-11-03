// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABulkArchiveTakesTheRepsOwnDealsAndSkipsATeammates(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	item := func(owner ids.UUID) crmcontracts.BulkItem {
		id := ids.From[ids.DealKind](e.SeedDeal(t, "Deal "+ids.NewV7().String()[:8], pipeline, open, &owner))
		deal, err := e.Deals.GetDeal(e.Admin(), id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return crmcontracts.BulkItem{Id: deal.Id, Version: *deal.Version}
	}
	mine, theirs := item(e.Rep1), item(e.Rep2)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	out, err := bulkEngineFor(e).Execute(rep, bulkChange{
		recordType: crmcontracts.BulkRecordTypeDeal, verb: crmcontracts.BulkVerbArchive,
		items: []crmcontracts.BulkItem{mine, theirs},
	})
	if err != nil {
		t.Fatalf("a rep's bulk archive was refused whole: %v", err)
	}
	if out.Changed != 1 || skipReasons(out.Skipped)[theirs.Id] != crmcontracts.BulkSkipReasonNotWritable {
		t.Fatalf("changed %d, skipped %v; want my deal archived and the teammate's skipped", out.Changed, out.Skipped)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = ANY($1) AND archived_at IS NOT NULL`,
		[]openapi_types.UUID{mine.Id, theirs.Id}); n != 1 {
		t.Errorf("%d deals archived, want only the rep's own", n)
	}
}
