// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal a colleague closed less than 30 days ago stays closed whatever
// arrives. One Margince closed may be reopened at once, and the colleague's
// close may be reopened once it is older.
func TestADealAColleagueClosedRecentlyStaysClosed(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	byColleague := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(3))
	globex := e.SeedCompany(t, "Globex", &e.AdminUser)
	byMargince := e.lostDeal(t, globex, e.AdminUser, e.daysAgo(3))
	e.WsExec(t, `UPDATE deal_stage_history SET changed_by = $2 WHERE id = (SELECT id FROM deal_stage_history
		 WHERE deal_id = $1 ORDER BY changed_at DESC, id DESC LIMIT 1)`, byMargince, marginceActorID)
	e.buyerPair(t, acme, e.daysAgo(1))
	e.buyerPair(t, globex, e.daysAgo(1))

	if done := e.openScouted(t); done != 1 {
		t.Fatalf("acted on %d suggestions, want only the deal Margince closed", done)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'lost'`, byColleague); n != 1 {
		t.Fatal("a deal a colleague closed 3 days ago was reopened")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open'`, byMargince); n != 1 {
		t.Fatal("a deal Margince closed 3 days ago was not reopened")
	}

	e.WsExec(t, `UPDATE deal SET closed_at = $2 WHERE id = $1`, byColleague, e.daysAgo(31))
	if done := e.openScouted(t); done != 1 {
		t.Errorf("a colleague's close 31 days ago acted %d times, want the reopen", done)
	}
}

// Undo of a reopened deal the team worked on since asks first. It asks again
// when more work arrives after the warning, and closes it once confirmed.
func TestUndoOfAReopenTheTeamWorkedOnNeedsConfirming(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	deal := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	e.buyerPair(t, acme, e.daysAgo(10))
	e.openScouted(t)
	correction := e.revivedCorrection(t, deal)
	file := func(kind, subject string) {
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: kind, Subject: &subject, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	file("note", "Call notes")

	_, err := e.Deals.RevertCorrection(e.Admin(), correction.ID, nil, nil)
	var asks *deals.DealOpenUndoNeedsConfirmError
	if !errors.As(err, &asks) || asks.Notes != 1 {
		t.Fatalf("an unconfirmed Undo answered %v, want the question naming one note", err)
	}
	file("task", "Send the offer")
	_, err = e.Deals.RevertCorrectionConfirmed(e.Admin(), correction.ID, nil, nil, asks.Work)
	var again *deals.DealOpenUndoNeedsConfirmError
	if !errors.As(err, &again) || again.Tasks != 1 || again.Work == asks.Work {
		t.Fatalf("a confirmation over stale work answered %v, want the question with the new task", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open'`, deal); n != 1 {
		t.Fatal("an unconfirmed Undo closed the deal")
	}
	if _, err := e.Deals.RevertCorrectionConfirmed(e.Admin(), correction.ID, nil, nil, again.Work); err != nil {
		t.Fatalf("the confirmed Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'lost'`, deal); n != 1 {
		t.Error("the confirmed Undo left the deal open")
	}
}

// Evidence that changes while the reopen waits for its lock reopens nothing:
// the times are read after the evidence is held.
func TestEvidenceChangedWhileTheReopenWaitsReopensNothing(t *testing.T) {
	e := setupScout(t)
	e.GrantRole(t, e.AdminUser, "admin")
	acme := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
	deal := e.lostDeal(t, acme, e.AdminUser, e.daysAgo(40))
	mail, buyer := e.buyingEmail(t, acme, e.daysAgo(10))
	e.signal(t, "new_opportunity", acme, mail, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", acme, mail, e.now, ids.Nil)
	e.pass(t)

	updated, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		writerDone <- database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(e.Admin(), `UPDATE activity SET occurred_at = occurred_at + interval '1 hour' WHERE id = $1`, mail); err != nil {
				return err
			}
			close(updated)
			<-release
			return nil
		})
	}()
	<-updated
	type outcome struct {
		done int
		err  error
	}
	reopen := make(chan outcome, 1)
	go func() {
		done, err := newScoutDealOpener(e.Pool, slog.New(slog.DiscardHandler)).OpenWorkspace(e.system())
		reopen <- outcome{done, err}
	}()
	e.waitForALockWaiter(t)
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatalf("changing the cited mail: %v", err)
	}
	got := <-reopen
	if got.err != nil || got.done != 0 {
		t.Fatalf("acted %d times (%v) on evidence that changed under the reopen, want none", got.done, got.err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'lost'`, deal); n != 1 {
		t.Error("the deal was reopened on evidence that changed under the reopen")
	}
}

// waitForALockWaiter returns once a session of this test's database waits on
// a lock, so the next step runs while the reopen is blocked.
func (e *scoutEnv) waitForALockWaiter(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := e.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
			 WHERE NOT l.granted AND a.datname = current_database()`).Scan(&waiting); err != nil {
			t.Fatalf("no session waited on a lock: %v", err)
		}
		if waiting > 0 {
			return
		}
	}
}
