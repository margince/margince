// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The historical recheck: a deal link the rule wrote before the topic check
// existed is asked about once, within the window.

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
		_, _, err := compose.ActivityDealFiler{}.FileOnDealIfUnfiled(connector, tx, mail, deal, nil)
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

// A link the model confidently says the mail is not about comes off, audited
// as the recheck. The answer is remembered, so a second pass asks nothing.
func TestTheRecheckTakesBackALinkTheMailIsNotAbout(t *testing.T) {
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
	wantDeals(t, e, "rc1@acme.example")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		 AND evidence->>'reason' = 'captured_activity_deal_recheck' AND before->>'entity_type' = 'deal'`); n != 1 {
		t.Fatalf("%d recheck audit rows, want the one retraction", n)
	}
	recheck(t, r, mail)
	if asked := env.topic.asked() - before; asked != 1 {
		t.Fatalf("the model was asked %d times over two rechecks, want once", asked)
	}
	if got := rechecking(t, r, captureNow); slices.Contains(got, mail.UUID) {
		t.Fatal("a rechecked mail is listed again")
	}
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
