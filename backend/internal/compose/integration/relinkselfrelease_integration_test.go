// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A conversation filed under the wrong company is moved back by relinking, so a
// credential may release the move it staged for its own human. What it releases
// is the exact set of activities it named: a thread key is refused, and a mail
// that joins the conversation afterwards is not moved under an approval that
// never described it. A project is the opposite case — its retention mark is
// write-once — and stays the contact's to release.

import (
	"context"
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
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
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

func TestACredentialReleasesTheCompanyRelinkItStagedAndMovesExactlyTheNamedActivities(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink self-release"))
	company := e.SeedCompany(t, "Right GmbH", nil)

	// The key form is refused for every destination: it is re-read at the retry.
	_, err := registry.Invoke(agent, "relink_thread", json.RawMessage(
		`{"thread_key":"`+f.key+`","entity_type":"company","entity_id":"`+company.String()+`"}`))
	var bad *agents.BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Guidance, "relink_activities") {
		t.Fatalf("relink_thread onto a company → %v, want a refusal naming relink_activities", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Fatalf("relink_thread left %d staged approval(s); a thread key cannot be approved", n)
	}

	args := `{"activity_ids":["` + f.mine[0].String() + `","` + f.mine[1].String() +
		`"],"entity_type":"company","entity_id":"` + company.String() + `"}`
	_, err = registry.Invoke(agent, "relink_activities", json.RawMessage(args))
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("relink_activities onto a company → %v, want a staged approval", err)
	}
	if !staged.ReleasableByCaller {
		t.Fatal("the staged answer does not say this credential can release the move; it is undoable")
	}

	// A mail joins the conversation after the call was staged.
	late := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, thread_key)
		VALUES ($1, 'email', 'Re: milestone (late)', 'body', now(), 'manual', 'human:x', $2)`, f.key)

	if _, err := registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+staged.ApprovalID.String()+`","decision":"approve"}`)); err != nil {
		t.Fatalf("the credential could not release the company relink it staged: %v", err)
	}
	var decidedByLender bool
	var actor string
	if err := owner.QueryRow(context.Background(), `
		SELECT a.decided_by = p.on_behalf_of,
		       (SELECT l.actor_type FROM audit_log l WHERE l.entity_id = a.id AND l.action = 'approve')
		  FROM approval a JOIN passport p ON p.id = a.passport_id WHERE a.id = $1`,
		staged.ApprovalID.UUID).Scan(&decidedByLender, &actor); err != nil {
		t.Fatalf("reading the decision back: %v", err)
	}
	if !decidedByLender || actor != "agent" {
		t.Errorf("decision recorded as lender=%v through actor %q, want the lender through the agent", decidedByLender, actor)
	}

	out, err := registry.Invoke(agent, "relink_activities", json.RawMessage(
		args[:len(args)-1]+`,"approval_id":"`+staged.ApprovalID.String()+`"}`))
	if err != nil {
		t.Fatalf("the released retry → %v", err)
	}
	if got := relinkedCount(t, out); got != 2 {
		t.Errorf("relinked = %d, want the two named activities", got)
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
		t.Error("a mail that joined the thread after the approval was moved under an approval that never described it")
	}
}

// A project relink writes a retention mark nothing removes, so the credential
// that staged it is refused with the reason, and the staged answer says it is
// not this credential's to release.
func TestACredentialDoesNotReleaseAProjectRelinkItStaged(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink project"))

	_, err := registry.Invoke(agent, "relink_activities", json.RawMessage(
		`{"activity_ids":["`+f.mine[0].String()+`"],"entity_type":"project","entity_id":"`+f.project.String()+`"}`))
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("relink_activities onto a project → %v, want a staged approval", err)
	}
	if staged.ReleasableByCaller {
		t.Error("the staged answer says this credential can release a project relink, which it would be refused")
	}

	_, err = registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+staged.ApprovalID.String()+`","decision":"approve"}`))
	if !errors.Is(err, apperrors.ErrPermissionDenied) || !strings.Contains(err.Error(), "retention mark is write-once") {
		t.Fatalf("self-release of a project relink → %v, want a denial naming the write-once retention mark", err)
	}
	if projectLinks(t, e, f.mine[0], f.project) != 0 {
		t.Error("the refused release still filed the activity under the project")
	}
}

// A mail on another team's private contact is outside the lender's row scope:
// even a released approval moves nothing, and the whole set rolls back.
func TestAReleasedRelinkNamingAnActivityOutsideTheLendersScopeMovesNothing(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink bounds"))
	company := e.SeedCompany(t, "Right GmbH", nil)

	args := `{"activity_ids":["` + f.mine[0].String() + `","` + f.theirs.String() +
		`"],"entity_type":"company","entity_id":"` + company.String() + `"}`
	_, err := registry.Invoke(agent, "relink_activities", json.RawMessage(args))
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("relink_activities → %v, want a staged approval", err)
	}
	if _, err := registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+staged.ApprovalID.String()+`","decision":"approve"}`)); err != nil {
		t.Fatalf("releasing: %v", err)
	}
	_, err = registry.Invoke(agent, "relink_activities", json.RawMessage(
		args[:len(args)-1]+`,"approval_id":"`+staged.ApprovalID.String()+`"}`))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the released retry over a row outside the lender's scope → %v, want ErrNotFound", err)
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
	svc := approvals.NewService(e.DB())
	id, _, err := svc.StageAgentCall(agent, approvals.StageInput{
		Kind: "relink_thread", ProposedChange: canonical, DiffHash: hash,
		TargetType: "company", TargetID: company, Summary: "Re-associate a conversation",
	})
	if err != nil {
		t.Fatalf("planting the pending approval: %v", err)
	}

	if _, err := registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+id.String()+`","decision":"approve"}`)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the credential releasing a thread approval → %v, want ErrPermissionDenied", err)
	}

	contact := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"custom"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true},
			"company":  {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	if _, err := svc.Decide(contact, id, true, nil); err != nil {
		t.Fatalf("the contact releasing it: %v", err)
	}
	_, err = registry.Invoke(agent, "relink_thread", json.RawMessage(
		args[:len(args)-1]+`,"approval_id":"`+id.String()+`"}`))
	var bad *agents.BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(bad.Error(), "thread key cannot be confirmed") {
		t.Fatalf("the released thread retry → %v, want the thread-key refusal", err)
	}
	for _, m := range f.mine {
		if companyLinks(t, e, m, company) != 0 {
			t.Errorf("activity %s was moved by a released thread approval", m)
		}
	}
}
