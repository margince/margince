// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestEveryWorklistSourceDeclaresWhoItsWorkIsFor(t *testing.T) {
	t.Parallel()
	known := map[crmcontracts.WorklistItemSource]bool{}
	for _, source := range everyReachSource(t) {
		known[source] = true
		if _, ok := sourceOwnerAnswers[source]; !ok {
			t.Errorf("%q has no admission policy, so none of its rows can reach a page", source)
		}
	}
	for source := range sourceOwnerAnswers {
		if !known[source] {
			t.Errorf("%q has a policy but the contract no longer offers it", source)
		}
	}
}

func TestEveryProducedRowGivesAnOwnerItsSourceAllows(t *testing.T) {
	t.Parallel()
	rows := classifyDay(dayOfEveryLane(), rankInstant, dayMoney{})
	if len(rows) == 0 {
		t.Fatal("the census classified nothing")
	}
	for _, row := range rows {
		if allowed := sourceOwnerAnswers[row.item.Source]; !allowed[row.ownerRef.kind] {
			t.Errorf("%q produced owner kind %d, which its policy does not allow", row.item.Source, row.ownerRef.kind)
		}
	}
}

func TestARowFromAnUnknownSourceCreatesNoWorkAndFailsItsLane(t *testing.T) {
	t.Parallel()
	known := classifyTask(crmcontracts.AttentionItem{Id: "task-1", Source: "task"}, rankInstant)
	unknown := known
	unknown.item.Source = "new_producer"
	unknown.lane = crmcontracts.WorklistItemSourceTask
	unknown.ownerRef = ownedBy(ids.NewV7())

	kept, refused := admitBySource(context.Background(), []ranked{unknown, known})
	if len(kept) != 1 || kept[0].item.Source != "task" {
		t.Fatalf("admitted = %+v, want only the row whose source has a policy", kept)
	}
	if len(refused) != 1 || refused[0].Source != "task" || refused[0].Reason != "failed" {
		t.Fatalf("refused = %+v, want the task lane reported failed", refused)
	}
}

func TestARowClaimingAnOwnerItsSourceMayNotGiveIsNobodysWork(t *testing.T) {
	t.Parallel()
	row := classifyDecay(lapsedItem(QuietRelationship{ContactID: ids.NewV7(), Name: "Ines"}), rankInstant)
	row.lane = crmcontracts.WorklistItemSourceRelationshipDecay
	row.ownerRef = ownedByWhoeverIsReading()

	kept, refused := admitBySource(context.Background(), []ranked{row})
	if len(kept) != 0 {
		t.Fatalf("admitted = %+v, want the row excluded rather than shared or unassigned", kept)
	}
	if len(refused) != 1 || refused[0].Source != string(crmcontracts.WorklistItemSourceRelationshipDecay) {
		t.Fatalf("refused = %+v, want the decay lane reported failed", refused)
	}
}

// The production projection: a lane-bound task lane that carries a row with no
// admission reaches no scope, and the page reports the lane as incomplete.
func TestARefusedRowReachesNoScopeAndThePageSaysTheLaneFailed(t *testing.T) {
	t.Parallel()
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	assignee := openapi_types.UUID(reader)
	planned := []crmcontracts.AttentionItem{{Id: "unregistered", Source: "new_producer", AssigneeId: &assignee}}
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: planned}

	for _, scope := range []string{scopeMine, scopeUnassigned, scopeAll} {
		out := (&Service{}).worklistFrom(ctx, day, scope, "", 25, waitingRead{}, worklistCursor{}, nil)
		if len(out.Queue) != 0 {
			t.Errorf("%s: the queue holds %+v, want no row from an unregistered producer", scope, out.Queue)
		}
		failed := false
		for _, missing := range out.SourcesUnavailable {
			failed = failed || (missing.Source == "task" && missing.Reason == "failed")
		}
		if !failed {
			t.Errorf("%s: sources unavailable = %+v, want the task lane reported failed", scope, out.SourcesUnavailable)
		}
	}
}
