// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The cap bounds the messages before any link is joined. A busy mailbox's scan
// then stops at WaitingScanCap instead of judging every message in the horizon.
// Every rule, the live-contact one included, sits inside that bound.
func TestTheWaitingScanCapsTheMessagesBeforeJoiningTheirLinks(t *testing.T) {
	sql := waitingRepliesSQL
	candidates := strings.Index(sql, "a.id = ANY (ARRAY(SELECT a.id FROM activity a")
	limit := strings.Index(sql, "LIMIT %[4]d))")
	group := strings.Index(sql, "GROUP BY a.id")
	if candidates < 0 || limit < candidates || group < limit {
		t.Fatalf("the cap is not taken over the messages ahead of the grouping: candidates at %d, cap at %d, grouping at %d",
			candidates, limit, group)
	}
	if !strings.Contains(sql[candidates:limit], filedUnderALiveContactSQL) {
		t.Error("the live-contact rule is applied after the cap, so a page can come back short of WaitingScanCap while eligible messages remain")
	}
	if strings.Contains(sql, "HAVING") {
		t.Error("a HAVING over grouped links runs after every message in the horizon is joined, which is the scan the cap exists to stop")
	}
}

// A statement carrying the scan is planned with its values at every run. A
// cached generic plan gives up the walk that stops at the cap.
func TestTheWaitingScanIsPlannedAtEveryRun(t *testing.T) {
	args := planEachBind([]any{"asOf"})
	if len(args) != 2 || args[0] != pgx.QueryExecModeDescribeExec || args[1] != "asOf" {
		t.Fatalf("planEachBind(asOf) = %v, want the unnamed exec mode ahead of the arguments", args)
	}
	if plain := waitingListArgs(ListActivitiesInput{}, []any{"x"}); len(plain) != 1 {
		t.Errorf("a list without the waiting filter is planned specially: %v", plain)
	}
}
