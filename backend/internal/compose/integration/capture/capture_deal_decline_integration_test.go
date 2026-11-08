// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A colleague's "not about this deal" and "none of them". The decline goes
// through the HTTP handler the API serves; mail arrives through the production
// sink, and the repair runs as its data task does.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// decline posts body to the decline route as ctx's principal.
func decline(ctx context.Context, t *testing.T, e *integration.SearchEnv, mail ids.UUID, body string) (int, crmcontracts.ActivityDealDecline) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/activities/"+mail.String()+"/deal-decline", strings.NewReader(body))
	activities.NewHandlers(e.DB()).DeclineActivityDeal(rec, req.WithContext(ctx), crmcontracts.Id(mail))
	var out crmcontracts.ActivityDealDecline
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("reading the decline's answer: %v", err)
		}
	}
	return rec.Code, out
}

func dealBody(deal ids.UUID) string { return `{"deal_id":"` + deal.String() + `"}` }

// asColleague is the deal world's colleague with other grants, or as another
// kind of principal.
func asColleague(w dealWorld, kind principal.PrincipalType, objects map[string]principal.ObjectGrant) context.Context {
	actor, _ := principal.Actor(w.ctx)
	actor.Type = kind
	if kind == principal.PrincipalAgent {
		actor.ID = "agent:decline-test"
	}
	if objects != nil {
		actor.Permissions.Objects = objects
	}
	return principal.WithActor(w.ctx, actor)
}

// The sole open deal files the mail, and a later capture copies the deal's
// project onto it. The colleague declines the deal: both links come off, and
// a live capture and the repair leave the mail off that deal.
func TestADeclinedDealStaysOffForCaptureAndTheRepair(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	rollout := w.project(t, "Rollout")
	deal := w.dealOnProject(t, "Acme rollout", rollout.ID)
	raw := mailFrom("alice@acme.example", "dec1@acme.example", "", "invoice")
	env.sync(t, raw)
	env.sync(t, raw)
	wantDeals(t, e, "dec1@acme.example", deal)
	if got := linkedProject(t, e, "dec1@acme.example"); got != rollout.ID {
		t.Fatalf("the mail is on project %s, want the deal's project %s", got, rollout.ID)
	}

	mail := activityOf(t, e, "dec1@acme.example")
	status, out := decline(w.ctx, t, e, mail, dealBody(deal))
	if status != http.StatusOK || !out.Unlinked || !out.ProjectUnfiled || out.ProjectKept {
		t.Fatalf("the decline answered %d %+v, want the deal and its project taken off", status, out)
	}
	if len(out.DeclinedDealIds) != 1 || ids.UUID(out.DeclinedDealIds[0]) != deal {
		t.Fatalf("the decline names %v, want %s", out.DeclinedDealIds, deal)
	}
	wantDeals(t, e, "dec1@acme.example")
	if got := linkedProject(t, e, "dec1@acme.example"); !got.IsZero() {
		t.Fatalf("the project %s that came from the declined deal stayed", got)
	}

	env.sync(t, raw)
	wantDeals(t, e, "dec1@acme.example")
	if got := linkedProject(t, e, "dec1@acme.example"); !got.IsZero() {
		t.Fatalf("a live capture filed the mail under %s again", got)
	}
	if changed := newRepairer(e, w).run(t); changed != 0 {
		t.Fatalf("the repair changed %d mails, want the declined one left off its deal", changed)
	}
	wantDeals(t, e, "dec1@acme.example")
	relinks := `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = '` + mail.String() + `'`
	before := countRows(t, e, relinks)
	if again, _ := decline(w.ctx, t, e, mail, dealBody(deal)); again != http.StatusOK {
		t.Fatalf("declining the same deal again answered %d, want the same success", again)
	}
	if after := countRows(t, e, relinks); after != before {
		t.Fatalf("declining the same deal again wrote %d audit rows, want none", after-before)
	}
}

// A decline holds for its one deal. Once a second deal fits the mail better,
// the refile pass files it there, and the declined deal stays off.
func TestADeclineLeavesTheMailOpenToAnotherDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	first := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "dec2@acme.example", "", "status"))
	wantDeals(t, e, "dec2@acme.example", first)

	if status, _ := decline(w.ctx, t, e, activityOf(t, e, "dec2@acme.example"), dealBody(first)); status != http.StatusOK {
		t.Fatalf("the decline answered %d", status)
	}
	second := w.deal(t, "Acme reporting")
	w.stakeholder(t, alice, second)
	refile(t, refiler(env, w), ids.From[ids.ActivityKind](activityOf(t, e, "dec2@acme.example")))
	wantDeals(t, e, "dec2@acme.example", second)
}

// "None of them" declines every deal the question offered, on the mail and
// on the rest of its conversation, and closes the question. When one of
// those deals later becomes the company's only open deal, nothing files the
// conversation on it.
func TestNoneOfThemDeclinesEveryOfferedDealAndClosesTheQuestion(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	operations := w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")
	env.syncSent(t, map[string]bool{"none1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "none1@myco.example", ""))
	env.sync(t, authenticated(mailFrom("alice@acme.example", "none2@acme.example", "none1@myco.example", "Re: project"), "acme.example"))
	if n := openQuestions(t, e); n != 1 {
		t.Fatalf("%d open deal questions, want 1", n)
	}

	status, out := decline(w.ctx, t, e, activityOf(t, e, "none2@acme.example"), `{}`)
	if status != http.StatusOK || out.QuestionsClosed != 1 || len(out.DeclinedDealIds) != 2 {
		t.Fatalf("none of them answered %d %+v, want both deals declined and the question closed", status, out)
	}
	if n := openQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions still open, want 0", n)
	}
	if status, _ := decline(w.ctx, t, e, activityOf(t, e, "none2@acme.example"), `{}`); status != http.StatusUnprocessableEntity {
		t.Fatalf("none of them with no open question answered %d, want 422", status)
	}

	archiver := asColleague(w, principal.PrincipalHuman, map[string]principal.ObjectGrant{"deal": {Read: true, Update: true, Delete: true}})
	if _, err := w.store.ArchiveDeal(archiver, ids.From[ids.DealKind](operations), nil); err != nil {
		t.Fatalf("archiving one of the offered deals: %v", err)
	}
	newRepairer(e, w).run(t)
	wantDeals(t, e, "none1@myco.example")
	wantDeals(t, e, "none2@acme.example")
}

// The decline answers by the caller's access: 403 without the grants it
// needs, 404 for a message or a deal the caller cannot reach. An agent's
// decline takes the deal off and keeps the project: only a member takes a
// project filing back.
func TestADeclineAnswersByTheCallersAccess(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	rollout := w.project(t, "Rollout")
	deal := w.dealOnProject(t, "Acme rollout", rollout.ID)
	raw := mailFrom("alice@acme.example", "dec3@acme.example", "", "status")
	env.sync(t, raw)
	env.sync(t, raw)
	mail := activityOf(t, e, "dec3@acme.example")

	readOnly := map[string]principal.ObjectGrant{"activity": {Read: true}, "deal": {Read: true}}
	noDeals := map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}}
	for _, c := range []struct {
		name string
		ctx  context.Context
		mail ids.UUID
		body string
		want int
	}{
		{"without activity update", asColleague(w, principal.PrincipalHuman, readOnly), mail, dealBody(deal), http.StatusForbidden},
		{"without the deal read", asColleague(w, principal.PrincipalHuman, noDeals), mail, dealBody(deal), http.StatusForbidden},
		{"a message nobody has", w.ctx, ids.NewV7(), dealBody(deal), http.StatusNotFound},
		{"a deal nobody has", w.ctx, mail, dealBody(ids.NewV7()), http.StatusNotFound},
	} {
		if status, _ := decline(c.ctx, t, e, c.mail, c.body); status != c.want {
			t.Errorf("%s: the decline answered %d, want %d", c.name, status, c.want)
		}
	}
	wantDeals(t, e, "dec3@acme.example", deal)

	status, out := decline(asColleague(w, principal.PrincipalAgent, nil), t, e, mail, dealBody(deal))
	if status != http.StatusOK || !out.Unlinked || out.ProjectUnfiled || !out.ProjectKept {
		t.Fatalf("the agent's decline answered %d %+v, want the deal off and the project kept", status, out)
	}
	wantDeals(t, e, "dec3@acme.example")
	if got := linkedProject(t, e, "dec3@acme.example"); got != rollout.ID {
		t.Fatalf("the agent's decline left project %s, want %s", got, rollout.ID)
	}
}
