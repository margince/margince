// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestActivityContentSourceProjectsOneDecision(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem})
	decision := "a.audience = 'workspace'"
	from, available := ActivityContentFrom("activity a", decision)
	columns, err := activityColumns(ctx, available, func(any) int { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	query := "SELECT " + columns + " FROM " + from
	if strings.Count(query, decision) != 1 || !strings.Contains(from, "AS available OFFSET 0) content_decision") {
		t.Fatal("content decision was duplicated or lost its evaluation boundary")
	}
	for _, payload := range []string{"a.body", "a.raw"} {
		if !strings.Contains(columns, "CASE WHEN (content_decision.available) THEN "+payload+" END") {
			t.Fatalf("%s is not controlled by the shared decision", payload)
		}
	}
	if !strings.HasSuffix(columns, "(content_decision.available) AS content_available") {
		t.Fatal("the scanner's nullable-decision behavior changed")
	}
}

func TestActivityContentSourceReusesAlreadyComputedDecisions(t *testing.T) {
	for _, decision := range []string{sqlTrue, "TRUE", SeatCopyReadable} {
		from, available := ActivityContentFrom("activity a", decision)
		if from != "activity a" || available != decision {
			t.Fatalf("already computed decision %s gained another source", decision)
		}
	}
}

func TestActivityContentSourceFollowsTheOrdinaryPageLimit(t *testing.T) {
	page, err := timelinePageSQL(teamScopedCtx(), ListActivitiesInput{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.sql, "(SELECT a.id FROM activity a WHERE ") ||
		!strings.Contains(page.sql, " LIMIT 51) bounded JOIN activity a ON a.id = bounded.id CROSS JOIN LATERAL (SELECT (") ||
		strings.Contains(page.sql, "a.*") ||
		strings.Count(page.sql, "AS available OFFSET 0) content_decision") != 1 {
		t.Fatal("ordinary page did not bound its rows before deciding content")
	}
}

func TestActivityContentSourceKeepsCollapsedDecisionsInTheirExistingSource(t *testing.T) {
	page, err := timelinePageSQL(teamScopedCtx(), ListActivitiesInput{CollapseSeatCopies: true}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page.sql, "content_decision") || !strings.Contains(page.sql, "CASE WHEN ("+SeatCopyReadable+") THEN a.body END") {
		t.Fatal("collapsed page stopped reusing its copy decision")
	}
}

func TestActivityCountColumnsCarryTheDecisionOnce(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem})
	decision := "a.audience = 'workspace'"
	columns, err := activityCountColumns(ctx, decision, func(any) int { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(columns, decision) != 1 || strings.Contains(columns, "THEN a.body") || strings.Contains(columns, "THEN a.raw") {
		t.Fatal("the count projection repeats the audience test in columns it never reads")
	}
	if !strings.HasSuffix(columns, "("+decision+") AS content_available") {
		t.Fatal("the count projection lost the decision column that binds its arguments")
	}
}
