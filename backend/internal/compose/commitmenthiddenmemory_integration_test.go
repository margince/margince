// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHiddenReassignedTaskIsNotRecreatedWhenAttributionIsLost(t *testing.T) {
	for _, visibility := range []string{"private", "restricted"} {
		t.Run(visibility, func(t *testing.T) {
			e := setupTranscript(t)
			e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
			e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)})
			task := ids.MustParse(e.wsString(t, `SELECT id::text FROM activity WHERE kind='task'`))
			source, err := e.Activities.GetActivity(e.Admin(), e.activity, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			owner := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
			if _, err := e.Activities.UpdateActivity(owner, ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), OccurredAt: &source.OccurredAt}); err != nil {
				t.Fatal(err)
			}
			if visibility == "private" {
				if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](task), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
					t.Fatal(err)
				}
			} else {
				integration.HoldUnderStatutoryFloor(t, e.Env, task)
			}

			quote := strings.Split(transcriptBody, "\n")[2]
			span := "3:" + quote
			c := Commitment{
				AsOf: source.OccurredAt, SourceActivityID: e.activity.UUID, Extractor: transcriptProposalActor,
				Summary: "Send revised pricing", Quote: quote, Span: span, Confidence: 0.95,
				Locator:  commitmentLocator(e.activity.UUID, claimKindOurs, unnamedPromiserKey(""), span),
				Evidence: approvals.Evidence{SourceType: "activity", SourceID: e.activity.UUID, Snippet: quote},
				parties:  []commitmentParty{{Ref: "seat:" + e.Rep2.String(), Seat: &e.Rep2, Role: "attendee"}},
			}
			dispatcher := NewCommitmentDispatcher(e.Pool, e.svc)
			for _, ctx := range []context.Context{e.Admin(), principal.SystemActing(e.Admin(), "commitment-replay")} {
				if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					_, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if e.taskCount(t) != 1 || e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='commitment_task'`) != 0 {
				t.Fatal("hidden task was recreated under a new attribution")
			}
		})
	}
}
