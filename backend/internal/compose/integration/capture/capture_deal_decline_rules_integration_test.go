// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The rules a decline keeps. It answers only a question the caller can read,
// and declines the deals that question offered. The latest decision about a
// deal counts, and only a project the deal filed comes off.

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ambiguousThread captures our mail and an authenticated reply at a company
// with two open deals, so the colleague who wrote it is asked once. It
// answers the reply's source id.
func ambiguousThread(t *testing.T, env captureEnv, key string) string {
	t.Helper()
	ours, reply := key+"1@myco.example", key+"2@acme.example"
	env.syncSent(t, map[string]bool{ours: true}, email(captureOwner, "", "alice@acme.example", ours, ""))
	env.sync(t, authenticated(mailFrom("alice@acme.example", reply, ours, "Re: project"), "acme.example"))
	if n := openQuestions(t, env.e); n != 1 {
		t.Fatalf("%d open deal questions, want 1", n)
	}
	return reply
}

// The colleague asked limits their question to themselves. Another colleague
// may change the shared mail but cannot read that question, so "none of them"
// is refused and declines nothing.
func TestNoneOfThemAnswersOnlyAQuestionTheCallerCanRead(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	shareMailbox(t, env)
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")
	reply := ambiguousThread(t, env, "priv")
	var question ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_system = 'which_deal'`).Scan(&question)
	}); err != nil {
		t.Fatalf("reading the deal question: %v", err)
	}
	if _, err := activities.NewStore(e.DB()).SetAudience(w.ctx, ids.From[ids.ActivityKind](question), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	}); err != nil {
		t.Fatalf("limiting the question to the colleague asked: %v", err)
	}

	actor, _ := principal.Actor(w.ctx)
	actor.ID, actor.UserID = "human:"+e.Rep3.String(), e.Rep3
	status, out := decline(principal.WithActor(w.ctx, actor), t, e, activityOf(t, e, reply), `{}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("none of them over a question the caller cannot read answered %d %+v, want 422", status, out)
	}
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE after->>'declined' = 'true'`); n != 0 {
		t.Fatalf("%d declines written for a question the caller cannot read, want none", n)
	}
	if n := openQuestions(t, e); n != 1 {
		t.Fatalf("%d open deal questions, want the question left open", n)
	}
}

// "None of them" declines the deals the question offered when it was asked.
// A deal archived since is still declined; one opened since is not.
func TestNoneOfThemDeclinesTheDealsTheQuestionOffered(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	operations := w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	reply := ambiguousThread(t, env, "offer")

	archiver := asColleague(w, principal.PrincipalHuman, map[string]principal.ObjectGrant{"deal": {Read: true, Update: true, Delete: true}})
	if _, err := w.store.ArchiveDeal(archiver, ids.From[ids.DealKind](operations), nil); err != nil {
		t.Fatalf("archiving an offered deal: %v", err)
	}
	opened := w.deal(t, "Acme rollout")

	status, out := decline(w.ctx, t, e, activityOf(t, e, reply), `{}`)
	if status != http.StatusOK {
		t.Fatalf("none of them answered %d", status)
	}
	got := make([]ids.UUID, 0, len(out.DeclinedDealIds))
	for _, deal := range out.DeclinedDealIds {
		got = append(got, ids.UUID(deal))
	}
	want := []ids.UUID{operations, reporting}
	slices.SortFunc(want, compareIDs)
	slices.SortFunc(got, compareIDs)
	if !slices.Equal(got, want) {
		t.Fatalf("none of them declined %v, want the offered %v and not %s", got, want, opened)
	}
}

func compareIDs(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }

// The latest decision about a deal counts. After a decline, a relink and a
// second decline, the mail is declined on that deal and kept by nobody. The
// refile pass and the repair both file it on the deal its project now names.
func TestTheLatestDecisionAboutADealHolds(t *testing.T) {
	for _, pass := range []struct {
		name string
		run  func(*testing.T, captureEnv, dealWorld, ids.ActivityID)
	}{
		{"the refile pass", func(t *testing.T, env captureEnv, w dealWorld, mail ids.ActivityID) {
			refile(t, refiler(env, w), mail)
		}},
		{"the repair", func(t *testing.T, env captureEnv, w dealWorld, _ ids.ActivityID) {
			newRepairer(env.e, w).run(t)
		}},
	} {
		t.Run(pass.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			first := w.deal(t, "Acme rollout")
			env.sync(t, mailFrom("alice@acme.example", "late1@acme.example", "", "status"))
			wantDeals(t, e, "late1@acme.example", first)
			mail := activityOf(t, e, "late1@acme.example")

			mustDecline(w.ctx, t, e, mail, first)
			relinkAsColleague(t, e, w, mail, "deal", first)
			if again := mustDecline(w.ctx, t, e, mail, first); len(again.DeclinedDealIds) != 1 {
				t.Fatalf("the decline after a relink answered %+v, want the deal declined anew", again)
			}
			reporting := w.project(t, "Reporting")
			second := w.dealOnProject(t, "Acme reporting", reporting.ID)
			relinkAsColleague(t, e, w, mail, "project", reporting.ID)

			pass.run(t, env, w, ids.From[ids.ActivityKind](mail))
			wantDeals(t, e, "late1@acme.example", second)
		})
	}
}

func relinkAsColleague(t *testing.T, e *integration.SearchEnv, w dealWorld, mail ids.UUID, entityType string, target ids.UUID) {
	t.Helper()
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, ids.From[ids.ActivityKind](mail),
		activities.RelinkActivityInput{EntityType: entityType, EntityID: target}); err != nil {
		t.Fatalf("relinking the mail to a %s as a colleague: %v", entityType, err)
	}
}

func mustDecline(ctx context.Context, t *testing.T, e *integration.SearchEnv, mail, deal ids.UUID) crmcontracts.ActivityDealDecline {
	t.Helper()
	status, out := decline(ctx, t, e, mail, dealBody(deal))
	if status != http.StatusOK {
		t.Fatalf("declining %s answered %d", deal, status)
	}
	return out
}

// refile runs the refile pass's per-mail step as its worker does.
func refile(t *testing.T, r repairer, mail ids.ActivityID) {
	t.Helper()
	if _, err := r.repair.Refile(r.system, r.e.DB(), mail, captureNow); err != nil {
		t.Fatalf("running the refile pass: %v", err)
	}
}

// A project the reply took from its thread stays when its deal is declined.
// That holds even when it is the deal's project and came after the deal link.
func TestADeclineKeepsAProjectTheThreadFiled(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	rollout := w.project(t, "Rollout")
	deal := w.dealOnProject(t, "Acme rollout", rollout.ID)
	env.syncSent(t, map[string]bool{"tp1@myco.example": true}, email(captureOwner, "", "alice@acme.example", "tp1@myco.example", ""))
	raw := authenticated(mailFrom("alice@acme.example", "tp2@acme.example", "tp1@myco.example", "Re: project"), "acme.example")
	env.sync(t, raw)
	wantDeals(t, e, "tp2@acme.example", deal)

	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, ids.From[ids.ActivityKind](activityOf(t, e, "tp1@myco.example")),
		activities.RelinkActivityInput{EntityType: "project", EntityID: rollout.ID}); err != nil {
		t.Fatalf("filing our mail under the project as a human: %v", err)
	}
	env.sync(t, raw)
	if got := linkedProject(t, e, "tp2@acme.example"); got != rollout.ID {
		t.Fatalf("the reply is on project %s, want the thread's %s", got, rollout.ID)
	}

	status, out := decline(w.ctx, t, e, activityOf(t, e, "tp2@acme.example"), dealBody(deal))
	if status != http.StatusOK || !out.Unlinked || out.ProjectUnfiled {
		t.Fatalf("the decline answered %d %+v, want the deal off and the thread's project kept", status, out)
	}
	if got := linkedProject(t, e, "tp2@acme.example"); got != rollout.ID {
		t.Fatalf("the decline took off the project the thread filed; the reply is on %s", got)
	}
}
