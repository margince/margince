// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A colleague's agent reads and answers the deal question Margince asked that
// colleague, by the colleague's own rules. Another colleague's agent neither
// sees it nor answers it.
func TestAnAgentReadsAndAnswersOnlyItsOwnColleaguesDealQuestion(t *testing.T) {
	d := newTwoDoors(t, "agent-which-deal", "read", "write")
	agentOf := func(email string) (*apptest.MCPClient, string) {
		t.Helper()
		const password = "agent-which-deal-password"
		member := createdID(t, d.e, "/v1/users", AnyMap{"email": email, "display_name": strings.Split(email, "@")[0], "role": "admin"})
		apptest.SetPasswordByLink(t, d.e, member, password)
		seat := d.e.AsSecondSeat(t, email, password)
		bearer, _ := apptest.PassportWithID(t, seat, "which deal agent", "read", "write")
		return apptest.NewMCPClient(seat, strings.TrimPrefix(bearer["Authorization"], "Bearer ")), member
	}
	stem := ids.NewV7().String()[24:]
	asked, askedID := agentOf("asked-" + stem + "@agent-which-deal.test")
	other, _ := agentOf("other-" + stem + "@agent-which-deal.test")

	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := createdID(t, d.e, "/v1/deals", AnyMap{
		"name": "Which deal rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	})
	mail := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Which rollout?", "source": "manual",
	})
	ws := apptest.InstallationWorkspaceUUID(t.Context(), t, d.e.Owner)
	system := principal.SystemActing(principal.WithWorkspaceID(t.Context(), ws), "system:capture-which-deal")
	if err := database.WithWorkspaceTx(system, d.e.Pool, func(tx pgx.Tx) error {
		return activities.AskWhichDealTx(system, tx, activities.WhichDealQuestion{
			Mail: ids.From[ids.ActivityKind](ids.MustParse(mail)), Seat: ids.MustParse(askedID),
			Deals: []ids.UUID{ids.MustParse(deal)}, Reason: "Which deal is this about?",
		})
	}); err != nil {
		t.Fatalf("asking the deal question: %v", err)
	}

	type queue struct {
		Data []struct {
			ID       string  `json:"id"`
			EntityID string  `json:"entity_id"`
			Unit     *string `json:"unit"`
		} `json:"data"`
	}
	var mine, theirs queue
	asked.CallOK(t, "read_workspace", map[string]any{"action": "link_suggestions"}).JSON(t, &mine)
	if len(mine.Data) != 1 || mine.Data[0].EntityID != deal || mine.Data[0].Unit != nil {
		t.Fatalf("the asked colleague's agent reads %+v, want Margince's question about the deal", mine.Data)
	}
	other.CallOK(t, "read_workspace", map[string]any{"action": "link_suggestions"}).JSON(t, &theirs)
	if len(theirs.Data) != 0 {
		t.Fatalf("another colleague's agent reads %+v, want nothing", theirs.Data)
	}
	question := mine.Data[0].ID
	if refused := other.Call(t, "change_activities", map[string]any{"action": "reject_link_suggestion", "id": question}); !refused.IsError {
		t.Fatalf("another colleague's agent answered the question: %s", refused.Text)
	}

	asked.CallOK(t, "change_activities", map[string]any{"action": "reject_link_suggestion", "id": question})
	var status string
	var declines int
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT s.status, (SELECT count(*) FROM audit_log WHERE entity_id = s.activity_id AND after->>'declined' = 'true')
		  FROM activity_link_suggestion s WHERE s.id = $1`, question).Scan(&status, &declines); err != nil {
		t.Fatalf("reading the answered question: %v", err)
	}
	if status != "rejected" || declines != 1 {
		t.Fatalf("the agent's reject left the question %s with %d declines, want rejected and the deal declined as the colleague's reject does", status, declines)
	}
}
