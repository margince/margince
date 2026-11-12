// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The cap bounds the messages before any link is joined. A busy mailbox's scan
// then stops at WaitingScanCap instead of judging every message in the horizon.
// Every rule, the live-contact one included, sits inside that bound.
func TestTheWaitingScanCapsTheMessagesBeforeJoiningTheirLinks(t *testing.T) {
	sql := waitingRepliesSQL
	candidates := strings.Index(sql, "a.id = ANY (ARRAY(SELECT capped.id FROM (")
	limit := strings.Index(sql, "LIMIT %[4]s))")
	group := strings.Index(sql, "GROUP BY a.id")
	if candidates < 0 || limit < candidates || group < limit {
		t.Fatalf("the cap is not taken over the messages ahead of the grouping: candidates at %d, cap at %d, grouping at %d",
			candidates, limit, group)
	}
	if arms := strings.Count(sql[candidates:limit], filedUnderALiveContactSQL); arms != 2 {
		t.Errorf("the live-contact rule is in %d of the two capped arms, so a page can come back short of WaitingScanCap while eligible messages remain", arms)
	}
	if strings.Contains(sql, "HAVING") {
		t.Error("a HAVING over grouped links runs after every message in the horizon is joined, which is the scan the cap exists to stop")
	}
}

// Outside a generic-plan snapshot, a waiting scan keeps its bound values
// visible so the plan can stop at the cap.
func TestTheWaitingScanOutsideAReusedSnapshotPlansAtEveryRun(t *testing.T) {
	args := planEachBind(context.Background(), []any{"asOf"})
	if len(args) != 2 || args[0] != pgx.QueryExecModeDescribeExec || args[1] != "asOf" {
		t.Fatalf("planEachBind(asOf) = %v, want the unnamed exec mode ahead of the arguments", args)
	}
	if plain := waitingListArgs(context.Background(), ListActivitiesInput{}, []any{"x"}); len(plain) != 1 {
		t.Errorf("a list without the waiting filter is planned specially: %v", plain)
	}
}
