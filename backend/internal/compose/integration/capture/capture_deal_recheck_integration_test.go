// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The historical recheck: a deal link the rule wrote before the topic check
// existed is asked about once, within the window. The answer marks the link.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unrecheckedLink captures a mail with no model, so it carries no topic memo.
// It then files it on deal as the connector's automatic filing did before the
// check existed.
func unrecheckedLink(t *testing.T, env captureEnv, sourceID string, deal ids.UUID) ids.ActivityID {
	t.Helper()
	e := env.e
	env.topic.unconfigured = true
	env.sync(t, mailFrom("alice@acme.example", sourceID, "", "status"))
	env.topic.unconfigured = false
	mail := ids.From[ids.ActivityKind](activityOf(t, e, sourceID))
	connector := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		SeatType: principal.SeatFull, Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}, "deal": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	connector = principal.WithCorrelationID(connector, ids.NewV7())
	if err := database.WithWorkspaceTx(connector, e.Pool, func(tx pgx.Tx) error {
		_, _, err := compose.ActivityDealFiler{}.FileOnDealIfUnfiled(connector, tx, mail, deal, nil, "")
		return err
	}); err != nil {
		t.Fatalf("filing the mail on its deal as the connector: %v", err)
	}
	wantDeals(t, e, sourceID, deal)
	return mail
}

// recheck runs the recheck's per-mail step as the refile worker does.
func recheck(t *testing.T, r repairer, mail ids.ActivityID) {
	t.Helper()
	if _, err := r.repair.Recheck(r.system, r.e.DB(), mail); err != nil {
		t.Fatalf("running the recheck: %v", err)
	}
}

// rechecking lists the recheck's candidates as of asOf.
func rechecking(t *testing.T, r repairer, asOf time.Time) []ids.UUID {
	t.Helper()
	var found []ids.UUID
	if err := database.WithWorkspaceTx(r.system, r.e.Pool, func(tx pgx.Tx) error {
		var err error
		found, err = r.repair.RecheckCandidatesTx(r.system, tx, asOf, ids.Nil, maxUUID, 100)
		return err
	}); err != nil {
		t.Fatalf("listing the recheck's candidates: %v", err)
	}
	return found
}

var maxUUID = ids.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

// A link the model confidently says the mail is not about stays, marked
// unrelated by the classifier and audited as the recheck. The answer is
// remembered, so a second pass asks nothing.
func TestTheRecheckMarksALinkTheMailIsNotAbout(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	mail := unrecheckedLink(t, env, "rc1@acme.example", deal)
	r := refiler(env, w)
	if got := rechecking(t, r, captureNow); !slices.Contains(got, mail.UUID) {
		t.Fatalf("the recheck lists %v, want the unchecked link", got)
	}
	env.topic.answer = func(string) (string, float64) { return "not_about", 0.95 }
	before := env.topic.asked()
	recheck(t, r, mail)
	wantDeals(t, e, "rc1@acme.example", deal)
	wantMark(t, e, "rc1@acme.example", deal, "unrelated/classifier")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		 AND evidence->>'reason' = 'captured_activity_deal_recheck' AND after->>'deal_relevance' = 'unrelated'`); n != 1 {
		t.Fatalf("%d recheck audit rows, want the one mark", n)
	}
	recheck(t, r, mail)
	if asked := env.topic.asked() - before; asked != 1 {
		t.Fatalf("the model was asked %d times over two rechecks, want once", asked)
	}
	if got := rechecking(t, r, captureNow); slices.Contains(got, mail.UUID) {
		t.Fatal("a rechecked mail is listed again")
	}
}

// A colleague's mark stands. The recheck asks the model nothing about a link
// a colleague judged, and the classifier's own writer refuses to change it.
func TestTheClassifierNeverChangesAColleaguesMark(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	mail := unrecheckedLink(t, env, "rc6@acme.example", deal)
	if _, err := activities.NewStore(e.DB()).SetDealRelevance(w.ctx, mail, deal, "relevant"); err != nil {
		t.Fatalf("marking the link as a colleague: %v", err)
	}
	r := refiler(env, w)
	env.topic.forbid = t
	recheck(t, r, mail)
	wantMark(t, e, "rc6@acme.example", deal, "relevant/human")

	var changed bool
	if err := database.WithWorkspaceTx(r.system, e.Pool, func(tx pgx.Tx) error {
		var err error
		changed, err = compose.ActivityDealFiler{}.MarkDealLink(r.system, tx, mail, deal, "unrelated", nil)
		return err
	}); err != nil {
		t.Fatalf("marking the link as the classifier: %v", err)
	}
	if changed {
		t.Fatal("the classifier's writer changed a colleague's mark")
	}
	wantMark(t, e, "rc6@acme.example", deal, "relevant/human")
}

// With no answer the link stays, and a later pass asks again. A colleague's
// own filing is never asked about.
func TestTheRecheckKeepsALinkWithoutAnAnswerAndLeavesColleaguesAlone(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	kept := unrecheckedLink(t, env, "rc2@acme.example", deal)
	human := unrecheckedLink(t, env, "rc3@acme.example", deal)
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, human, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: deal, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("filing the mail on its deal again as a colleague: %v", err)
	}
	r := refiler(env, w)

	env.topic.unconfigured = true
	recheck(t, r, kept)
	wantDeals(t, e, "rc2@acme.example", deal)
	if got := rechecking(t, r, captureNow); !slices.Contains(got, kept.UUID) {
		t.Fatal("a link kept for want of an answer is not offered again")
	}
	env.topic.unconfigured, env.topic.forbid = false, t
	recheck(t, r, human)
}

// The recheck reaches back 90 days from its pass and no further.
func TestTheRecheckWindowIsNinetyDays(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	mail := unrecheckedLink(t, env, "rc4@acme.example", w.deal(t, "Acme rollout"))
	r := refiler(env, w)
	window := 90 * 24 * time.Hour
	if got := rechecking(t, r, mailDay.Add(window)); !slices.Contains(got, mail.UUID) {
		t.Fatal("a mail exactly at the window's edge is not rechecked")
	}
	if got := rechecking(t, r, mailDay.Add(window+time.Hour)); slices.Contains(got, mail.UUID) {
		t.Fatal("a mail older than the window is rechecked")
	}
}

// Every answer marks the link and keeps it: a fit relevant, doubt uncertain,
// a confident "not about it" unrelated. The answer is remembered, so the
// recheck does not ask about it again.
func TestTheRecheckMarksALinkByItsAnswer(t *testing.T) {
	for _, c := range []struct {
		name       string
		verdict    string
		confidence float64
		mark       string
	}{
		{"unsure", "unsure", 0.5, "uncertain/classifier"},
		{"not about it, below the floor", "not_about", 0.6, "uncertain/classifier"},
		{"not about it", "not_about", 0.9, "unrelated/classifier"},
		{"about it", "about", 0.9, "relevant/classifier"},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			deal := w.deal(t, "Acme rollout")
			mail := unrecheckedLink(t, env, "rc5@acme.example", deal)
			r := refiler(env, w)
			env.topic.answer = func(string) (string, float64) { return c.verdict, c.confidence }
			recheck(t, r, mail)
			wantDeals(t, e, "rc5@acme.example", deal)
			wantMark(t, e, "rc5@acme.example", deal, c.mark)
			if got := rechecking(t, r, captureNow); slices.Contains(got, mail.UUID) {
				t.Fatal("an answered recheck is offered again")
			}
		})
	}
}

// A reply the thread filed on a deal stands on our mail's link. Once a
// colleague marks our mail not about that deal, the reply's link rests on
// nothing but a suggestion, so the recheck asks about it.
func TestTheRecheckAsksAboutAReplyWhoseThreadWasMarkedNotAboutTheDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	env.syncSent(t, map[string]bool{"rt1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "rt1@myco.example", ""))
	ours := ids.From[ids.ActivityKind](activityOf(t, e, "rt1@myco.example"))
	store := activities.NewStore(e.DB())
	if _, err := store.RelinkActivity(w.ctx, ours, activities.RelinkActivityInput{EntityType: "deal", EntityID: reporting}); err != nil {
		t.Fatalf("filing our mail on a deal as a colleague: %v", err)
	}
	env.sync(t, authenticated(mailFrom("alice@acme.example", "rt2@acme.example", "rt1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "rt2@acme.example", reporting)
	if _, err := store.SetDealRelevance(w.ctx, ours, reporting, "unrelated"); err != nil {
		t.Fatalf("marking our mail not about the deal: %v", err)
	}
	env.topic.answer = func(string) (string, float64) { return "not_about", 0.95 }
	recheck(t, refiler(env, w), ids.From[ids.ActivityKind](activityOf(t, e, "rt2@acme.example")))
	wantMark(t, e, "rt2@acme.example", reporting, "unrelated/classifier")
}

// A recheck every model declines is remembered like a doubtful answer: the
// link stands, and a second recheck under the same prompt asks nothing.
func TestTheRecheckAsksADeclinedMailOnce(t *testing.T) {
	env := newCaptureEnv(t)
	w := newDealWorld(t, env.e)
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")
	mail := unrecheckedLink(t, env, "rcdecl@acme.example", deal)
	r := refiler(env, w)
	env.topic.withheld = true
	before := env.topic.asked()
	recheck(t, r, mail)
	recheck(t, r, mail)
	if asked := env.topic.asked() - before; asked != 1 {
		t.Fatalf("the model was asked %d times over two rechecks of a declined mail, want once", asked)
	}
	wantDeals(t, env.e, "rcdecl@acme.example", deal)
}

// The refile pass remembers a decline too: a mail the models declined is not
// asked about again on the next pass.
func TestTheRefilePassAsksADeclinedMailOnce(t *testing.T) {
	env := newCaptureEnv(t)
	w := newDealWorld(t, env.e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	env.topic.unconfigured = true
	env.sync(t, mailFrom("alice@acme.example", "rfdecl@acme.example", "", "status"))
	env.topic.unconfigured, env.topic.withheld = false, true
	mail := ids.From[ids.ActivityKind](activityOf(t, env.e, "rfdecl@acme.example"))
	r := refiler(env, w)
	before := env.topic.asked()
	refile(t, r, mail)
	refile(t, r, mail)
	if asked := env.topic.asked() - before; asked != 1 {
		t.Fatalf("the model was asked %d times over two refile passes of a declined mail, want once", asked)
	}
	wantDeals(t, env.e, "rfdecl@acme.example")
}
