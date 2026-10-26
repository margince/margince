// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A credential relinks activities as its colleague could in the app, at once.
// It moves the exact set it names: a thread key is refused, and a mail in the
// conversation it did not name is not moved. A project filing is made the same
// way, and only a member can undo it.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// relinkLender gives Rep1 a real role holding what staging, releasing and the
// move read and write; the gate re-derives it on every call.
func relinkLender(t *testing.T, e *Env) {
	t.Helper()
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ('relink-lender', 'Relink lender', $1::jsonb)`,
		`{"objects":{"activity":{"read":true,"update":true},"project":{"read":true},"company":{"read":true,"update":true},"contact":{"read":true}},"row_scope":"all"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'relink-lender'`, e.Rep1)
}

func relinkedCount(t *testing.T, out json.RawMessage) int {
	t.Helper()
	var envelope struct {
		Data struct {
			Relinked int `json:"relinked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	return envelope.Data.Relinked
}

func companyLinks(t *testing.T, e *Env, activity, company ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND company_id = $2`, activity, company)
}

func TestACredentialRelinksExactlyTheNamedActivitiesToACompanyAtOnce(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink company"))
	company := e.SeedCompany(t, "Right GmbH", nil)

	// The key form is refused for every destination: a key names a conversation
	// that may grow.
	_, err := registry.Invoke(agent, "relink_thread", json.RawMessage(
		`{"thread_key":"`+f.key+`","entity_type":"company","entity_id":"`+company.String()+`"}`))
	var bad *agents.BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Guidance, "relink_activities") {
		t.Fatalf("relink_thread onto a company → %v, want a refusal naming relink_activities", err)
	}

	// A mail in the conversation the call does not name.
	late := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, thread_key)
		VALUES ($1, 'email', 'Re: milestone (late)', 'body', now(), 'manual', 'human:x', $2)`, f.key)

	out, err := registry.Invoke(agent, "relink_activities", json.RawMessage(`{"activity_ids":["`+
		f.mine[0].String()+`","`+f.mine[1].String()+`"],"entity_type":"company","entity_id":"`+company.String()+`"}`))
	if err != nil {
		t.Fatalf("relink_activities onto a company on a passport → %v, want it run at once", err)
	}
	if got := relinkedCount(t, out); got != 2 {
		t.Errorf("relinked = %d, want the two named activities", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Errorf("the relink left %d staged approval(s), want none", n)
	}
	for _, id := range f.mine {
		if companyLinks(t, e, id, company) != 1 {
			t.Errorf("named activity %s is not filed under the company", id)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = $1`, id); n != 1 {
			t.Errorf("audit rows for %s = %d, want 1", id, n)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1::text`, id); n != 1 {
			t.Errorf("activity.updated events for %s = %d, want 1", id, n)
		}
	}
	if companyLinks(t, e, late, company) != 0 {
		t.Error("a mail the call did not name was moved")
	}
}

// A credential files an activity under a project at once, and stamps it. The
// undo is a named member's decision the credential cannot make.
func TestACredentialFilesUnderAProjectAndOnlyAMemberUndoesTheFiling(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink project"))

	out, err := registry.Invoke(agent, "relink_activities", json.RawMessage(
		`{"activity_ids":["`+f.mine[0].String()+`"],"entity_type":"project","entity_id":"`+f.project.String()+`"}`))
	if err != nil {
		t.Fatalf("relink_activities onto a project on a passport → %v, want it filed at once", err)
	}
	if got := relinkedCount(t, out); got != 1 {
		t.Errorf("relinked = %d, want the one named activity", got)
	}
	if projectLinks(t, e, f.mine[0], f.project) != 1 || projectLinks(t, e, f.mine[1], f.project) != 0 {
		t.Error("the relink did not file exactly the named activity")
	}
	filed := ids.ActivityID{UUID: f.mine[0]}
	if readProjectStamp(t, e, filed.UUID).class == nil {
		t.Fatal("the filing was not stamped")
	}

	if _, err := e.Activities.UndoProjectFiling(agent, filed, undoReason); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent undoing the filing → %v, want a permission denial", err)
	}
	member := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	if _, err := e.Activities.UndoProjectFiling(member, filed, undoReason); err != nil {
		t.Fatalf("the member undoing the filing → %v", err)
	}
	if readProjectStamp(t, e, filed.UUID).class != nil || projectLinks(t, e, filed.UUID, f.project) != 0 {
		t.Error("the member's undo left the class or the project link behind")
	}
}

// A set that names an activity outside the lender's scope is refused as a
// whole, so none of it moves.
func TestARelinkReachingOutsideTheLendersScopeMovesNothing(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink bounds"))
	company := e.SeedCompany(t, "Right GmbH", nil)

	_, err := registry.Invoke(agent, "relink_activities", json.RawMessage(`{"activity_ids":["`+
		f.mine[0].String()+`","`+f.theirs.String()+`"],"entity_type":"company","entity_id":"`+company.String()+`"}`))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("relink_activities over a hidden activity → %v, want not found", err)
	}
	for _, id := range []ids.UUID{f.mine[0], f.theirs} {
		if companyLinks(t, e, id, company) != 0 {
			t.Errorf("activity %s was filed although the set was refused as a whole", id)
		}
	}
}

// A thread approval staged before thread moves stopped staging names a key the
// conversation has since outgrown. The credential cannot release it, and even a
// contact's release cannot make the retry run.
func TestAThreadApprovalStagedBeforeThreadMovesStoppedStagingNeverRuns(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink legacy"))
	company := e.SeedCompany(t, "Right GmbH", nil)

	args := `{"thread_key":"` + f.key + `","entity_type":"company","entity_id":"` + company.String() + `"}`
	canonical, hash, err := diffhash.Canonical(json.RawMessage(args))
	if err != nil {
		t.Fatalf("hashing the call: %v", err)
	}
	svc := compose.NewApprovalsService(e.DB())
	id, _, err := svc.StageAgentCall(agent, approvals.StageInput{
		Kind: "relink_thread", ProposedChange: canonical, DiffHash: hash,
		TargetType: "company", TargetID: company, Summary: "Re-associate a conversation",
	})
	if err != nil {
		t.Fatalf("planting the pending approval: %v", err)
	}

	var incomplete *approvals.PreviewContextError
	if _, err := registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+id.String()+`","decision":"approve"}`)); !errors.As(err, &incomplete) {
		t.Fatalf("the credential releasing a thread approval: %v, want non-decidable preview", err)
	}

	contact := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"custom"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true},
			"company":  {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	if _, err := svc.Decide(contact, id, true, nil); !errors.As(err, &incomplete) {
		t.Fatalf("a human releasing the retired operation: %v, want non-decidable preview", err)
	}
	_, err = registry.Invoke(agent, "relink_thread", json.RawMessage(
		args[:len(args)-1]+`,"approval_id":"`+id.String()+`"}`))
	if !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("the undecidable thread retry: %v, want approval refusal", err)
	}
	for _, m := range f.mine {
		if companyLinks(t, e, m, company) != 0 {
			t.Errorf("activity %s was moved by a released thread approval", m)
		}
	}
}
