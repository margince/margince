// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Body, raw and meeting notes share the supplied content decision.
// The reply channel uses the audience of its source.
func TestDirectCaseGatesContentAndTheReplyChannelSource(t *testing.T) {
	const opaqueDecision = "coalesce(a.audience = 'workspace', false)"
	for _, decision := range []string{"TRUE", "FALSE", "NULL::boolean", opaqueDecision} {
		t.Run(decision, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem})
			calls := 0
			arg := func(any) int { calls++; return calls }
			share, err := auth.ActivityInternalMailShareClause(ctx, "a", arg)
			if err != nil {
				t.Fatal(err)
			}
			sourceAudience, err := auth.ActivitySourceAudienceArm(ctx, "s", arg)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := ActivityProjection(ctx, decision, arg)
			if err != nil {
				t.Fatal(err)
			}
			columns := gatekit.SplitSQLList(projection)
			if len(columns) != len(activityProjection) {
				t.Fatal("projection width changed")
			}
			if len(activityScanTargets(&activityScan{})) != len(columns) {
				t.Fatal("scan target count changed")
			}
			gated := 0
			for i, column := range activityProjection {
				want := column.sql
				switch {
				case column.sql == meetingoutcome.ReadNotesSQL("a"):
					want = "CASE WHEN (" + decision + ") THEN (" + column.sql + ") END AS meeting_notes"
					gated++
				case column.sql == "a.body" || column.sql == "a.raw":
					want = "CASE WHEN (" + decision + ") THEN " + column.sql + " END AS " + strings.TrimPrefix(column.sql, "a.")
					gated++
				case column.sql == storekit.ReplyChannelSQL("a", ""):
					want = storekit.ReplyChannelSQL("a", sourceAudience)
					gated++
				case column.sql == mailboxImportedColumn || column.sql == threadAudienceYoursColumn:
					want = importFactColumns(ctx, "a", arg)[column.sql]
				case i == len(columns)-2:
					want = "CASE WHEN a.internal_mail_authorization IS NOT NULL THEN (" + share + ") ELSE NULL END AS can_share_internal"
				case i == len(columns)-1:
					want = "(" + decision + ") AS content_available"
				}
				if strings.TrimSpace(columns[i]) != want {
					t.Errorf("column %d = %q, want %q", i, columns[i], want)
				}
			}
			if gated != 4 {
				t.Fatalf("gated %d columns, want body, raw, meeting notes and the reply channel", gated)
			}
			if decision == opaqueDecision && strings.Count(projection, decision) != 4 {
				t.Fatal("supplied decision was not reused for every content value and the final boolean")
			}
			if calls != 0 {
				t.Fatal("rendering a supplied decision registered new audience arguments")
			}
		})
	}
}

func TestDirectCaseKeepsTheMissingActorError(t *testing.T) {
	projection, err := ActivityProjection(context.Background(), "TRUE", func(any) int { return 1 })
	if err == nil || projection != "" {
		t.Fatal("projection rendered without its required actor")
	}
}
