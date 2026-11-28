// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestChannelReconfirmationIsOneReviewedUndatedFollowUpAndKeepsAcceptedDeadline(t *testing.T) {
	for _, change := range []string{"unchanged", "declined", "corrected", "removed", "record access lost"} {
		t.Run(change, func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			company := e.SeedCompany(t, "Pilot account", &e.AdminUser)
			ctx := e.Admin()
			links := []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}}
			due := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
			assignee := ids.From[ids.UserKind](e.AdminUser)
			accepted, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: new("Accepted policy delivery"), DueAt: &due, AssigneeID: &assignee, Source: "manual", Links: links})
			if err != nil {
				t.Fatal(err)
			}
			question := chatsummary.Reconfirmation{Question: "Can Jules confirm the requested earlier date?", Quote: "Security requests 12 June. Jules has not accepted the earlier date."}
			raw := map[string]any{"provider": "slack", "channel_name": "Pilot team", "reconfirmation": question, "generated_fingerprint": activities.ChatSummaryFingerprint("Earlier date needs confirmation", "Jules accepted 14 June.")}
			source := provenance.ChatSummarySource
			key := "synthetic-channel-root"
			note, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Earlier date needs confirmation"), Body: new("Jules accepted 14 June."), Source: provenance.ChatSummaryOrigin, SourceSystem: &source, SourceID: &key, Raw: &raw, Origin: activities.OriginAgent, Links: links})
			if err != nil {
				t.Fatal(err)
			}
			writer := channelReconfirmation(e.Pool)
			for range 2 {
				if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
					return writer(ctx, tx, ids.UUID(note.Id), ids.Ref{Type: "company", ID: company}, question)
				}); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewApprovalsServiceWithEffects(e.Pool)
			status := "pending"
			rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, RequestedForReader: true, Limit: 20})
			if err != nil || len(rows) != 1 {
				t.Fatalf("owner Worklist proposals=%d error=%v", len(rows), err)
			}
			var proposal CommitmentTaskProposal
			encoded, err := json.Marshal(rows[0].ProposedChange)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &proposal); err != nil {
				t.Fatal(err)
			}
			if proposal.DueDate != "" || proposal.ExistingTaskID != nil || proposal.Quote != question.Quote {
				t.Fatalf("request became a new accepted date: %+v", proposal)
			}
			if got := e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE kind='task'`); got != "1" {
				t.Fatalf("staging created work without approval: %s", got)
			}

			if change == "declined" {
				if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), false, new("The owner reviewed this request.")); err != nil {
					t.Fatal(err)
				}
				if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
					return writer(ctx, tx, ids.UUID(note.Id), ids.Ref{Type: "company", ID: company}, question)
				}); err != nil {
					t.Fatal(err)
				}
				count, err := svc.CountPending(ctx, approvals.ListInput{RequestedForReader: true})
				if err != nil || count.Count != 0 {
					t.Fatalf("declined question returned: %+v %v", count, err)
				}
				return
			}
			switch change {
			case "corrected", "removed":
				metadata := map[string]any{"provider": "slack", "channel_name": "Pilot team"}
				revision := activities.ChatSummaryRevision{Subject: "Request withdrawn", Body: "The earlier request no longer applies.", Metadata: metadata, Deleted: change == "removed"}
				if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
					_, err := e.Activities.ReviseChatSummaryTx(principal.SystemActing(ctx, "chat_links"), tx, ids.From[ids.ActivityKind](ids.UUID(note.Id)), revision)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			case "record access lost":
				ctx = e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
			}
			_, err = svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), true, nil)
			if change == "unchanged" && err != nil {
				t.Fatalf("reviewed follow-up refused: %v", err)
			}
			if change != "unchanged" && err == nil {
				t.Fatal("accepted a stale or unavailable reconfirmation")
			}
			want := "1"
			if change == "unchanged" {
				want = "2"
			}
			if got := e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE kind='task'`); got != want {
				t.Fatalf("tasks=%s want %s", got, want)
			}
			if got := e.WsScalar(t, `SELECT due_at::date::text FROM activity WHERE id=$1`, accepted.Id); got != "2026-06-14" {
				t.Fatalf("changed accepted deadline: %s", got)
			}
			if change == "unchanged" {
				if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
					return writer(ctx, tx, ids.UUID(note.Id), ids.Ref{Type: "company", ID: company}, question)
				}); err != nil {
					t.Fatal(err)
				}
				count, err := svc.CountPending(ctx, approvals.ListInput{RequestedForReader: true})
				if err != nil || count.Count != 0 {
					t.Fatalf("accepted question returned: %+v %v", count, err)
				}
				if got := e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE kind='task' AND id<>$1 AND due_at IS NULL`, accepted.Id); got != "1" {
					t.Fatal("invented a follow-up deadline")
				}
			}
		})
	}
}
