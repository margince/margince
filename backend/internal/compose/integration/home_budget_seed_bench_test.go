// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Home profile is the daily corpus at one small fixed scale. On top come
// three shapes that make Home slow, all on the rep who owns the most.
const (
	homeBudgetScale = 0.01
	// homeAgentUpdates is the busy automation log: the owner's agent changing
	// their open deals, each change one audit row the receipt reads.
	homeAgentUpdates = 500
	// homeAwaitedMails are mails the owner sent that nobody answered yet.
	homeAwaitedMails = 60
	// homeAwaitedAge puts every awaited mail past the follow-up setting and
	// inside the lane's lookback.
	homeAwaitedAge = 7 * 24 * time.Hour
)

// HomeProfile is what the Home check opens: the heaviest owner's seat and the
// counts the seeder wrote for them.
type HomeProfile struct {
	Owner         Seat
	AgentUpdates  int
	AwaitedMails  int
	OwnedContacts int
}

// seedHomeProfile seeds the daily corpus, picks its heaviest owner, gives them
// a brief, a busy agent log and unanswered mail, and checks every count.
// Every shape goes through the product's own writers, so no audit row or
// participant is written behind the product's back.
func seedHomeProfile(t *testing.T, e *apptest.AppEnv, seats Seats) HomeProfile {
	t.Helper()
	loadDailyCorpus(t, e, seats, homeBudgetScale).dropOrdinals()
	owner := heaviestOwner(t, e, seats)
	// The receipt's window opens at the owner's last brief, so the brief comes
	// before the agent's changes it must show.
	makeDailyBriefs(t, e, Seats{Reps: []Seat{owner}})
	profile := HomeProfile{Owner: owner}
	profile.AgentUpdates = seedAgentLog(t, e, owner)
	profile.AwaitedMails = seedAwaitedMail(t, e, owner)
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM contact WHERE owner_id = $1`, owner.UserID).Scan(&profile.OwnedContacts); err != nil {
		t.Fatalf("counting the heaviest owner's contacts: %v", err)
	}
	assertHomeProfile(t, e, profile)
	return profile
}

// heaviestOwner is the rep who owns the most contacts: Home's worst seat,
// where the median rep is the daily bench's typical one.
func heaviestOwner(t *testing.T, e *apptest.AppEnv, seats Seats) Seat {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(context.Background(), `SELECT owner_id::text FROM contact
	      GROUP BY owner_id ORDER BY count(*) DESC, owner_id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("finding the heaviest owner: %v", err)
	}
	for _, rep := range seats.Reps {
		if rep.UserID == id {
			return rep
		}
	}
	t.Fatalf("the heaviest owner %s holds no rep seat; the check would time a seat that owns nothing", id)
	return Seat{}
}

// seedAgentLog has the owner's agent change their open deals' amounts, round
// after round, through the deals store: one domain row, audit row and event each.
func seedAgentLog(t *testing.T, e *apptest.AppEnv, owner Seat) int {
	t.Helper()
	ctx := context.Background()
	ws := apptest.InstallationWorkspaceUUID(ctx, t, e.Pool)
	repID := ids.MustParse(owner.UserID)
	agent := dailyRepAgent(t, identity.NewServiceFor(e.DB()), ws, repID)
	open := dailyOpenDeals(t, e, repID, homeAgentUpdates)
	if len(open) == 0 {
		t.Fatalf("the heaviest owner %s has no open deal for the agent to change", owner.Name)
	}
	store := deals.NewStore(e.DB(), installseam.Deals())
	start := time.Now()
	for i := range homeAgentUpdates {
		amount := int64(100_000 + i)
		if _, err := store.UpdateDeal(agent, ids.From[ids.DealKind](open[i%len(open)]), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
			t.Fatalf("the agent's change %d to deal %s: %v", i+1, open[i%len(open)], err)
		}
	}
	t.Logf("seed: %d agent changes over %d open deals in %s", homeAgentUpdates, len(open), time.Since(start).Round(time.Millisecond))
	return homeAgentUpdates
}

// seedAwaitedMail logs mail the owner sent to their own contacts on open
// deals, through the activities endpoint, with no reply after it.
func seedAwaitedMail(t *testing.T, e *apptest.AppEnv, owner Seat) int {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `SELECT DISTINCT ON (d.id) d.id::text, r.contact_id::text
	      FROM deal d JOIN relationship r ON r.deal_id = d.id
	      JOIN contact c ON c.id = r.contact_id AND c.archived_at IS NULL AND (c.owner_id = d.owner_id OR c.visibility <> 'owner')
	      WHERE d.owner_id = $1 AND d.status = 'open' AND d.archived_at IS NULL
	      ORDER BY d.id, r.contact_id LIMIT $2`, owner.UserID, homeAwaitedMails)
	if err != nil {
		t.Fatalf("picking the owner's deal contacts: %v", err)
	}
	type recipient struct{ deal, contact string }
	var recipients []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.deal, &r.contact); err != nil {
			t.Fatalf("picking the owner's deal contacts: %v", err)
		}
		recipients = append(recipients, r)
	}
	if err := rows.Err(); err != nil || len(recipients) == 0 {
		t.Fatalf("picking the owner's deal contacts: %d found, err %v", len(recipients), err)
	}
	sentAt := time.Now().Add(-homeAwaitedAge).UTC()
	for i := range homeAwaitedMails {
		to := recipients[i%len(recipients)]
		var answer AnyMap
		if status := owner.env(e).Call(t, "POST", "/v1/activities", AnyMap{
			"kind": "email", "direction": "outbound", "subject": fmt.Sprintf("Proposal %d", i+1),
			"occurred_at": sentAt.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			"links": []AnyMap{
				{"entity_type": "contact", "entity_id": to.contact},
				{"entity_type": "deal", "entity_id": to.deal},
			},
		}, nil, &answer); status != http.StatusCreated {
			t.Fatalf("logging the owner's mail %d answered %d: %v", i+1, status, answer)
		}
	}
	return homeAwaitedMails
}

// assertHomeProfile fails a short profile before any timing: every count must
// be in the database, and the reads Home sends must see each shape.
func assertHomeProfile(t *testing.T, e *apptest.AppEnv, p HomeProfile) {
	t.Helper()
	var audited, mailed int
	if err := e.Owner.QueryRow(context.Background(), `SELECT
	        (SELECT count(*) FROM audit_log a JOIN deal d ON d.id = a.entity_id
	          WHERE a.entity_type = 'deal' AND a.action = 'update' AND a.actor_type = 'agent' AND d.owner_id = $1),
	        (SELECT count(*) FROM activity a JOIN activity_participant me ON me.activity_id = a.id AND me.role = 'from'
	          WHERE a.kind = 'email' AND a.direction = 'outbound' AND me.user_id = $1 AND a.subject LIKE 'Proposal %')`,
		p.Owner.UserID).Scan(&audited, &mailed); err != nil {
		t.Fatalf("counting the Home profile: %v", err)
	}
	if audited != p.AgentUpdates || mailed != p.AwaitedMails {
		t.Fatalf("the Home profile is short: %d of %d agent changes audited, %d of %d awaited mails sent by the owner",
			audited, p.AgentUpdates, mailed, p.AwaitedMails)
	}
	var magic struct {
		Done []struct{} `json:"done"`
	}
	mustCall(t, p.Owner.env(e), "GET", "/v1/magic?limit=100", nil, http.StatusOK, &magic)
	var worklist struct {
		Queue []struct {
			Source string `json:"source"`
		} `json:"queue"`
	}
	mustCall(t, p.Owner.env(e), "GET", worklistDay+"mine", nil, http.StatusOK, &worklist)
	awaited := 0
	for _, row := range worklist.Queue {
		if row.Source == "awaiting_reply" {
			awaited++
		}
	}
	if len(magic.Done) == 0 || awaited == 0 {
		t.Fatalf("Home cannot see the profile: the receipt shows %d done lines and the day %d awaited replies", len(magic.Done), awaited)
	}
	t.Logf("seed: Home profile for %s: %d contacts, %d agent changes, %d awaited mails; Home sees %d done lines and %d awaited replies",
		p.Owner.Name, p.OwnedContacts, p.AgentUpdates, p.AwaitedMails, len(magic.Done), awaited)
}
