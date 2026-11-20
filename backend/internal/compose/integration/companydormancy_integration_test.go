// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The company clock the dormancy ladder reads counts no personal mail, moves
// with new activity and with an archive, and the company page reads the ladder
// and its follow-up from it.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companyclockrefold"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// companyClock reads the stored clock of the company the world's deal is on.
func (w dealEvidenceWorld) companyClock(t *testing.T) *time.Time {
	t.Helper()
	var got *time.Time
	if err := database.WithWorkspaceTx(w.e.Admin(), w.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(w.e.Admin(), `SELECT c.last_activity_at FROM deal d JOIN company c ON c.id = d.company_id
			WHERE d.id = $1`, w.deal.UUID).Scan(&got)
	}); err != nil {
		t.Fatalf("reading the company clock: %v", err)
	}
	return got
}

// A held thread captured into a classified mailbox reads as participants mail,
// which no company clock counts. Should such a row still reach the workspace,
// the hold alone keeps it off the company clock, and ending the hold gives the
// company its date back. Only the verdict's trigger can move the clock then.
func TestAPersonalHoldKeepsAWorkspaceMailOffTheCompanyClock(t *testing.T) {
	w := newDealEvidenceWorld(t)
	thread, at := "thread-"+ids.NewV7().String(), w.now.AddDate(0, 0, -2)
	dinner := w.mail(t, thread, at)
	w.judgeThread(t, thread, threadverdict.KindPersonal)
	w.e.WsExec(t, `UPDATE activity SET audience = 'workspace', audience_reason = NULL WHERE id = $1`, dinner)
	if clock := w.companyClock(t); clock != nil {
		t.Fatalf("company last_activity_at = %v while the hold stands, want none", clock)
	}
	w.e.WsExec(t, `DELETE FROM capture_thread_verdict WHERE thread_key = $1`, thread)
	if clock := w.companyClock(t); clock == nil || !clock.Equal(at) {
		t.Errorf("company last_activity_at = %v after the hold ended, want the mail at %v", clock, at)
	}
}

// A mail filed only on a deal reaches the company through it. Judged not about
// the deal, it reaches the company no more; unjudged, it still does.
func TestAMailJudgedNotAboutTheDealDoesNotWarmItsCompany(t *testing.T) {
	w := newDealEvidenceWorld(t)
	at := w.now.AddDate(0, 0, -3).Truncate(time.Second)
	direction, subject := "inbound", "Lunch?"
	logged, _, err := w.e.Activities.LogActivity(w.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: w.deal.UUID}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	if clock := w.companyClock(t); clock == nil || !clock.Equal(at) {
		t.Fatalf("company last_activity_at = %v, want the unjudged mail at %v", clock, at)
	}
	w.e.WsExec(t, `UPDATE activity_link SET deal_relevance = 'unrelated', relevance_source = 'human'
		WHERE activity_id = $1 AND deal_id = $2`, ids.UUID(logged.Id), w.deal.UUID)
	if clock := w.companyClock(t); clock != nil {
		t.Errorf("company last_activity_at = %v after the link was judged unrelated, want none", clock)
	}
}

// A company clock stored before the rule still names the personal mail. The
// data task refolds it once, and a second pass finds nothing.
func TestTheDataTaskTakesAPersonalMailOffAStoredCompanyClock(t *testing.T) {
	w := newDealEvidenceWorld(t)
	thread, at := "thread-"+ids.NewV7().String(), w.now.AddDate(0, 0, -2)
	w.mail(t, thread, at)
	w.judgeThread(t, thread, threadverdict.KindPersonal)
	w.e.WsExec(t, `UPDATE company SET last_activity_at = $2 WHERE id = (SELECT company_id FROM deal WHERE id = $1)`, w.deal.UUID, at)

	step := companyclockrefold.PersonalMailTask().Steps[0]
	ctx := principal.SystemActing(w.e.Admin(), "system:datatask-test")
	for pass, want := range []int{1, 0} {
		if err := database.WithWorkspaceTx(ctx, w.e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 200, RunID: ids.NewV7()})
			if err == nil && out.Changed != want {
				t.Errorf("pass %d refolded %d company clocks, want %d", pass+1, out.Changed, want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if clock := w.companyClock(t); clock != nil {
		t.Errorf("company last_activity_at = %v after the refold, want none", clock)
	}
}

// dormantAccount is a company the first rep owns, with one mail filed on it
// 70 days ago.
type dormantAccount struct {
	e       *Env
	now     time.Time
	company ids.CompanyID
	mail    ids.ActivityID
	at      time.Time
}

func newDormantAccount(t *testing.T) dormantAccount {
	t.Helper()
	e := Setup(t)
	a := dormantAccount{e: e, now: clocktest.Now(t)}
	a.company = ids.From[ids.CompanyKind](e.SeedCompany(t, "Quiet GmbH", &e.Rep1))
	a.at = a.now.AddDate(0, 0, -70).Truncate(time.Second)
	direction, subject := "outbound", "Checking in"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &a.at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: a.company.UUID}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	a.mail = ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	return a
}

func (a dormantAccount) page(t *testing.T) crmcontracts.Company360 {
	t.Helper()
	svc := company360.NewService(a.e.Pool, a.e.Contacts, a.e.Deals, a.e.Projects, approvals.NewService(a.e.DB()),
		func() time.Time { return a.now })
	page, err := svc.Assemble(a.e.Admin(), a.company)
	if err != nil {
		t.Fatalf("assembling the company page: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Dormancy == nil {
		t.Fatalf("the page carries no dormancy reading: %+v", page.StateStrip)
	}
	return page
}

func goneDormant(page crmcontracts.Company360) *crmcontracts.Company360Suggestion {
	if page.Suggestions == nil {
		return nil
	}
	for _, s := range *page.Suggestions {
		if s.Kind == crmcontracts.Company360SuggestionKindGoneDormant {
			return &s
		}
	}
	return nil
}

// The page reads the stored clock on the ladder and offers its owner a
// follow-up. Archiving the only activity leaves an account with no activity:
// stale, with the marker, and no follow-up.
func TestTheCompanyPageReadsTheLadderFromTheStoredClock(t *testing.T) {
	a := newDormantAccount(t)
	page := a.page(t)
	reading := page.StateStrip.Dormancy
	if reading.Rung != crmcontracts.DormancyRungDormant || reading.LastActivityAt == nil || !reading.LastActivityAt.Equal(a.at) {
		t.Fatalf("the strip reads %+v, want dormant from the mail at %v", reading, a.at)
	}
	follow := goneDormant(page)
	if follow == nil || follow.Action == nil || follow.Action.Task == nil || follow.Action.Task.AssigneeId == nil ||
		ids.UUID(*follow.Action.Task.AssigneeId) != a.e.Rep1 {
		t.Fatalf("the page offers %+v, want a follow-up prepared for the owner", follow)
	}

	if _, err := a.e.Activities.ArchiveActivity(a.e.Admin(), a.mail, nil); err != nil {
		t.Fatalf("archiving the mail: %v", err)
	}
	page = a.page(t)
	if got := page.StateStrip.Dormancy; got.Rung != crmcontracts.DormancyRungStale || !got.NoActivity || got.LastActivityAt != nil {
		t.Errorf("after the archive the strip reads %+v, want stale with no activity", got)
	}
	if follow := goneDormant(page); follow != nil {
		t.Errorf("an account with no activity offers %+v, want no follow-up", follow)
	}
}

// New activity moves the clock forward, and the account reads active again.
func TestNewActivityWarmsADormantAccount(t *testing.T) {
	a := newDormantAccount(t)
	recent := a.now.AddDate(0, 0, -1).Truncate(time.Second)
	direction, subject := "inbound", "Back from leave"
	if _, _, err := a.e.Activities.LogActivity(a.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &recent, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: a.company.UUID}},
	}); err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	page := a.page(t)
	if got := page.StateStrip.Dormancy; got.Rung != crmcontracts.DormancyRungActive || got.LastActivityAt == nil || !got.LastActivityAt.Equal(recent) {
		t.Errorf("after new activity the strip reads %+v, want active from %v", got, recent)
	}
	if follow := goneDormant(page); follow != nil {
		t.Errorf("an active account offers %+v, want no follow-up", follow)
	}
}

// The installation switch drops the follow-up and keeps the ladder.
func TestTheFollowUpSwitchKeepsTheLadder(t *testing.T) {
	a := newDormantAccount(t)
	a.e.WsExec(t, `INSERT INTO setting (key, value) VALUES ('contacts.dormancy_follow_up_enabled', 'false'::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`)
	page := a.page(t)
	if page.StateStrip.Dormancy.Rung != crmcontracts.DormancyRungDormant {
		t.Errorf("switched off, the strip reads %+v, want still dormant", page.StateStrip.Dormancy)
	}
	if follow := goneDormant(page); follow != nil {
		t.Errorf("switched off, the page offers %+v, want no follow-up", follow)
	}
}

// An account nobody owns raises no follow-up: automatic work never falls to
// somebody else.
func TestAnUnownedDormantAccountRaisesNoFollowUp(t *testing.T) {
	a := newDormantAccount(t)
	a.e.WsExec(t, `UPDATE company SET owner_id = NULL WHERE id = $1`, a.company.UUID)
	if follow := goneDormant(a.page(t)); follow != nil {
		t.Errorf("an unowned account offers %+v, want no follow-up", follow)
	}
}
