// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Rep1 replies throughout to a client who wrote to Rep2. Each case varies only
// where Rep1 stands: on the thread, on the deal's team, or on neither.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The harness seeds Rep1, Rep2 and Rep3 with these logins, in that order.
const (
	replierAddress   = "a@authz.test"
	colleagueAddress = "b@authz.test"
	clientAddress    = "client@buyer.test"
)

type standingEnv struct {
	*Env
	pipeline ids.PipelineID
	stage    ids.StageID
}

func setupStanding(t *testing.T) *standingEnv {
	t.Helper()
	e := Setup(t)
	pipeline, err := e.Deals.CreatePipeline(e.Admin(), deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	s := &standingEnv{Env: e, pipeline: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))}
	stage, err := e.Deals.CreateStage(e.Admin(), deals.CreateStageInput{PipelineID: s.pipeline, Name: "Qualified", Position: 1, Semantic: "open"})
	if err != nil {
		t.Fatal(err)
	}
	s.stage = ids.From[ids.StageKind](ids.UUID(stage.Id))
	return s
}

type threadParty struct {
	role, address string
	seat          *ids.UUID
}

// message writes one email of a thread as capture leaves it, with its parties.
func (s *standingEnv) message(t *testing.T, threadKey, audience string, parties ...threadParty) ids.UUID {
	t.Helper()
	id := SeedIDRow(t, OwnerConn(t), `
		INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, thread_key, audience)
		VALUES ($1, 'email', 'inbound', 'Pricing', now(), 'gmail', 'human:seed', $2, $3)`, threadKey, audience)
	for _, p := range parties {
		s.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id) VALUES ($1, $2, NULLIF($3, ''), $4)`,
			id, p.role, p.address, p.seat)
	}
	return id
}

// clientToColleague is the anchor every case answers: the client wrote to Rep2 alone.
func (s *standingEnv) clientToColleague(t *testing.T, threadKey string) ids.UUID {
	t.Helper()
	return s.message(t, threadKey, "workspace",
		threadParty{role: "from", address: clientAddress}, threadParty{role: "to", address: colleagueAddress})
}

func (s *standingEnv) dealOwnedBy(t *testing.T, owner ids.UUID, filed ...ids.UUID) ids.UUID {
	t.Helper()
	deal := s.SeedDeal(t, "Fleet renewal", s.pipeline, s.stage, &owner)
	for _, activity := range filed {
		LinkActivity(t, OwnerConn(t), activity, "deal", deal)
	}
	return deal
}

// assign puts a user or a team on the deal through the assignment store.
func (s *standingEnv) assign(t *testing.T, deal ids.UUID, kind crmcontracts.AssignmentSubjectKind, subject ids.UUID) {
	t.Helper()
	key := map[crmcontracts.AssignmentSubjectKind]string{
		crmcontracts.AssignmentSubjectKindUser: "sales_engineer", crmcontracts.AssignmentSubjectKindTeam: "technical_contact",
	}[kind]
	role := s.WsScalar(t, `SELECT id::text FROM record_role WHERE key = $1`, key)
	roleID, err := ids.Parse(role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assignments.NewStore(s.DB()).CreateAssignment(s.Admin(), assignments.CreateAssignmentInput{
		RecordType: crmcontracts.AssignmentRecordTypeDeal, RecordID: deal,
		SubjectKind: kind, SubjectID: subject, RoleID: roleID,
	}); err != nil {
		t.Fatalf("assigning %s to the deal: %v", subject, err)
	}
}

// replier is Rep1 reading activities and contacts, plus the deal grant when dealRead.
func (s *standingEnv) replier(dealRead bool) context.Context {
	objects := map[string]principal.ObjectGrant{objActivity: {Read: true}, objContact: {Read: true}}
	if dealRead {
		objects[objDeal] = principal.ObjectGrant{Read: true}
	}
	return s.As(s.Rep1, []ids.UUID{s.Team1}, principal.Permissions{RoleKeys: []string{roleRep}, Objects: objects, RowScope: principal.RowScopeAll})
}

func (s *standingEnv) standing(ctx context.Context, t *testing.T, anchor ids.UUID) crmcontracts.ReplyRecipient {
	t.Helper()
	rec := httptest.NewRecorder()
	activities.NewHandlers(s.DB()).GetReplyRecipient(rec,
		httptest.NewRequest(http.MethodGet, "/v1/activities/x/reply-recipient", nil).WithContext(ctx),
		crmcontracts.Id(anchor))
	if rec.Code != http.StatusOK {
		t.Fatalf("reply-recipient answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.ReplyRecipient
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the recipient: %v", err)
	}
	return out
}

func wantStanding(t *testing.T, got crmcontracts.ReplyRecipient, on bool, owner *ids.UUID) {
	t.Helper()
	if got.ViewerOnConversation != on {
		t.Errorf("viewer_on_conversation = %v, want %v", got.ViewerOnConversation, on)
	}
	switch {
	case owner == nil && got.DealOwnerUserId != nil:
		t.Errorf("deal_owner_user_id = %s, want null", *got.DealOwnerUserId)
	case owner != nil && got.DealOwnerUserId == nil:
		t.Errorf("deal_owner_user_id = null, want %s", *owner)
	case owner != nil && ids.UUID(*got.DealOwnerUserId) != *owner:
		t.Errorf("deal_owner_user_id = %s, want %s", *got.DealOwnerUserId, *owner)
	}
}

func TestAReplierNamedOnTheThreadIsOnTheConversation(t *testing.T) {
	s := setupStanding(t)
	ctx := s.replier(true)
	rep1 := s.Rep1

	cases := []struct {
		name string
		seed func(anchor ids.UUID, thread string)
		on   bool
	}{
		{"copied on the message answered", func(anchor ids.UUID, _ string) {
			s.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', $2)`, anchor, replierAddress)
		}, true},
		{"stamped by seat on the message answered", func(anchor ids.UUID, _ string) {
			s.WsExec(t, `INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'to', $2)`, anchor, rep1)
		}, true},
		{"addressed on an earlier message of the thread", func(_ ids.UUID, thread string) {
			s.message(t, thread, "workspace", threadParty{role: "to", address: replierAddress})
		}, true},
		{"blind-copied only", func(anchor ids.UUID, _ string) {
			s.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'bcc', $2)`, anchor, replierAddress)
		}, false},
		{"addressed on an archived message of the thread", func(_ ids.UUID, thread string) {
			earlier := s.message(t, thread, "workspace", threadParty{role: "to", address: replierAddress})
			s.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, earlier)
		}, false},
		{"addressed on another thread", func(_ ids.UUID, thread string) {
			s.message(t, thread+"-other", "workspace", threadParty{role: "to", address: replierAddress})
		}, false},
		{"addressed on a thread message they may not read", func(_ ids.UUID, thread string) {
			s.message(t, thread, "participants", threadParty{role: "to", address: replierAddress})
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := "thread-" + tc.name
			anchor := s.clientToColleague(t, thread)
			tc.seed(anchor, thread)
			wantStanding(t, s.standing(ctx, t, anchor), tc.on, nil)
		})
	}
}

func TestADealTeamMemberIsOnTheConversation(t *testing.T) {
	s := setupStanding(t)
	ctx := s.replier(true)

	t.Run("assigned to the deal", func(t *testing.T) {
		anchor := s.clientToColleague(t, "thread-seat")
		deal := s.dealOwnedBy(t, s.Rep3, anchor)
		s.assign(t, deal, crmcontracts.AssignmentSubjectKindUser, s.Rep1)
		wantStanding(t, s.standing(ctx, t, anchor), true, &s.Rep3)
	})
	t.Run("a member of a team assigned to the deal", func(t *testing.T) {
		anchor := s.clientToColleague(t, "thread-team")
		deal := s.dealOwnedBy(t, s.Rep3, anchor)
		s.assign(t, deal, crmcontracts.AssignmentSubjectKindTeam, s.Team1)
		wantStanding(t, s.standing(ctx, t, anchor), true, &s.Rep3)
	})
	t.Run("on the team of the first of two deals", func(t *testing.T) {
		anchor := s.clientToColleague(t, "thread-two-deals")
		deal := s.dealOwnedBy(t, s.Rep3, anchor)
		s.assign(t, deal, crmcontracts.AssignmentSubjectKindUser, s.Rep1)
		s.SeedWonDealLinkedTo(t, anchor)
		wantStanding(t, s.standing(ctx, t, anchor), true, &s.Rep3)
	})
	t.Run("the deal's owner", func(t *testing.T) {
		anchor := s.clientToColleague(t, "thread-owner")
		s.dealOwnedBy(t, s.Rep1, anchor)
		wantStanding(t, s.standing(ctx, t, anchor), true, &s.Rep1)
	})
}

func TestAReplierOnNeitherIsToldWhoOwnsTheDeal(t *testing.T) {
	s := setupStanding(t)
	anchor := s.clientToColleague(t, "thread-1")
	deal := s.dealOwnedBy(t, s.Rep3, anchor)
	s.assign(t, deal, crmcontracts.AssignmentSubjectKindTeam, s.Team2)
	wantStanding(t, s.standing(s.replier(true), t, anchor), false, &s.Rep3)
}

func TestTheOwnerNamedIsOfTheOpenDealThenTheLatestFiling(t *testing.T) {
	s := setupStanding(t)
	ctx := s.replier(true)
	anchor := s.clientToColleague(t, "thread-1")
	s.dealOwnedBy(t, s.Rep3, anchor)
	s.SeedWonDealLinkedTo(t, anchor)
	wantStanding(t, s.standing(ctx, t, anchor), false, &s.Rep3)

	s.dealOwnedBy(t, s.AdminUser, anchor)
	wantStanding(t, s.standing(ctx, t, anchor), false, &s.AdminUser)
}

// Every seat with the deal grant reads every live deal, so the grant is what
// hides one. A hidden deal answers as no deal, even to its own team.
func TestADealTheReplierCannotReadNamesNoOwnerAndNoTeam(t *testing.T) {
	s := setupStanding(t)
	anchor := s.clientToColleague(t, "thread-1")
	deal := s.dealOwnedBy(t, s.Rep3, anchor)
	s.assign(t, deal, crmcontracts.AssignmentSubjectKindUser, s.Rep1)
	wantStanding(t, s.standing(s.replier(false), t, anchor), false, nil)
}

func TestAThreadUnderNoDealNamesNoOwner(t *testing.T) {
	s := setupStanding(t)
	anchor := s.clientToColleague(t, "thread-1")
	wantStanding(t, s.standing(s.replier(true), t, anchor), false, nil)
}
