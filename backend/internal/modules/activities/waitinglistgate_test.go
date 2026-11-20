// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A waiting list adds no content gate of its own to the waiting set. The reply
// channel projection gates the source row it reads, a different row than the
// set's, so the statement carries that one besides.
func TestAWaitingListCarriesOnlyTheGatesOfItsWaitingSet(t *testing.T) {
	asOf := clocktest.Now(t)
	in := ListActivitiesInput{WaitingReplyAsOf: &asOf}
	set, err := appendWaitingReplyClause(teamScopedCtx(), in, func(any) int { return 1 }, nil)
	if err != nil {
		t.Fatalf("rendering the waiting set: %v", err)
	}
	if gateCopies(set) == 0 {
		t.Fatal("the waiting set carries no content gate, so the list would admit unreadable mail")
	}
	sourceAudience, err := auth.ActivityAudienceArm(teamScopedCtx(), "s", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("rendering the source audience: %v", err)
	}
	replyChannel := auth.ContentGateCopies(storekit.ReplyChannelSQL("a", sourceAudience))
	if replyChannel == 0 {
		t.Fatal("the reply channel holds no source-audience gate, so it would leak a held source's kind")
	}
	page, err := timelinePageSQL(teamScopedCtx(), in, 50)
	if err != nil {
		t.Fatalf("rendering the waiting list: %v", err)
	}
	if got, want := gateCopies([]string{page.sql}), gateCopies(set)+replyChannel; got != want {
		t.Fatalf("the waiting list carries %d content gates, want %d: the set's and the reply channel's", got, want)
	}
	if !strings.Contains(page.sql, "waiting_thread") {
		t.Fatal("the list holds no waiting set, so this test reads the wrong statement")
	}
}
