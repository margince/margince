// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which quiet records the clock scan is allowed to remind about, proven
// against a real migrated Postgres. The scan's candidate query
// (activities/lasttouch.go's LastTouchBefore) answers two questions at
// once — has this record gone quiet, and is anyone actually working it —
// and each suite below pins one arm of the second question. Quietness
// itself, the occurrence key, and the re-arm on a fresh touch are proven
// next door (timescan_integration_test.go).
//
// The clock is pinned (NewTimeScannerWithClock) so "no activity for N
// days" is evaluated against seeded timestamps, never the wall clock; no
// sleep, no real-time flakiness. Rows created through the module stores
// carry a real now() creation stamp, so every fixture that must predate
// the cutoff is shifted explicitly through the owner connection —
// otherwise the creation grace (a record younger than the cutoff is not
// stale, however old its imported activities are) would hide the very
// behaviour under test.

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// eligibilityScanNow is the instant every suite in this file evaluates
// against. With the seeded automation carrying no params, the threshold is
// the 7-day default, so the cutoff is eligibilityScanNow-7d.
var eligibilityScanNow = time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)

// quietSince is comfortably past that cutoff — the last genuine touch of
// every fixture below.
var quietSince = eligibilityScanNow.AddDate(0, 0, -30)

// longEstablished is a creation stamp well before the cutoff, so the
// creation grace never decides a test that is about something else.
var longEstablished = eligibilityScanNow.AddDate(0, 0, -60)

func TestQuietOrganizationWithNoOpenDealIsNotRemindedAbout(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	org := e.SeedOrg(t, "Dormant Account", nil)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	linkQuietTouch(t, owner, e.WS, "organization", org)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "organization", org); got != 0 {
		t.Fatalf("reminder tasks on an account with no open deal = %d, want 0 — nobody is working this company", got)
	}
	if got := runCountForHandler(t, e, "no_activity_reminder"); got != 0 {
		t.Fatalf("workflow_run rows = %d, want 0 — an ineligible record must never reach the batch at all", got)
	}
}

func TestQuietOrganizationWithAnOpenDealFiresOnceAndStaysClaimed(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Live Account", nil)
	deal := e.SeedDeal(t, "Live Account Renewal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)
	// Linked to the ACCOUNT only, so this suite is about the organization
	// arm of the eligibility rule and not about the deal's own arm.
	linkQuietTouch(t, owner, e.WS, "organization", org)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)
	if got := taskCountOn(t, e, "organization", org); got != 1 {
		t.Fatalf("reminder tasks after the first pass = %d, want exactly 1 — an account with an open deal is worth a reminder", got)
	}

	runEligibilityScan(t, e)
	if got := taskCountOn(t, e, "organization", org); got != 1 {
		t.Fatalf("reminder tasks after the second pass = %d, want still exactly 1 — the unchanged anchor must hold its claim", got)
	}
}

// A row a CALLER dressed as the system's still counts as engagement. source
// arrives verbatim on the create wire, so anyone can write source='system' —
// only the pair (source AND captured_by, which is stamped from the principal)
// marks the engine's own output. Were source alone the exclusion, a caller
// could hide real engagement from this scan, or plant an old-dated row to
// drag a worked account into the reminder draw forever.
func TestAPlantedSystemSourceRowStillCountsAsEngagement(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Planted Account", nil)
	deal := e.SeedDeal(t, "Planted Account Renewal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)
	linkQuietTouch(t, owner, e.WS, "organization", org)

	// The planted row: recent, source claims the system, captured_by names
	// the human who actually wrote it. It IS this account's latest touch.
	planted := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'note', 'Dressed as the system', $2, 'system', 'human:x')`,
		planted, eligibilityScanNow.AddDate(0, 0, -1)); err != nil {
		t.Fatalf("seeding the planted row: %v", err)
	}
	linkTouch(t, owner, e.WS, planted, "organization", org)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)
	if got := taskCountOn(t, e, "organization", org); got != 0 {
		t.Fatalf("reminder tasks = %d, want 0 — the planted row is a real recent touch, and excluding it on its source alone would let a caller steer the scan", got)
	}
}

// The engine's OWN recent output still does not count: both halves of the
// pair are the system's, and a reminder task must not reset the very clock
// it fires off.
func TestTheEnginesOwnRowStillDoesNotCountAsEngagement(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Reminded Account", nil)
	deal := e.SeedDeal(t, "Reminded Account Renewal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)
	linkQuietTouch(t, owner, e.WS, "organization", org)

	// captured_by is the NAMESPACED principal the time scan actually binds,
	// not the bare "system" the bus path uses. A case planting the bare form
	// passes against an equality on it and says nothing about the rows this
	// engine writes — which is how a reminder that reset its own clock
	// reached main behind a green gate.
	engineRow := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'task', 'Follow up (engine-minted)', $2, 'system', 'system:time-scan')`,
		engineRow, eligibilityScanNow.AddDate(0, 0, -1)); err != nil {
		t.Fatalf("seeding the engine row: %v", err)
	}
	linkTouch(t, owner, e.WS, engineRow, "organization", org)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)
	if got := taskCountOn(t, e, "organization", org); got == 0 {
		t.Fatal("no reminder fired — the engine's own output counted as engagement and reset the clock it fires off")
	}
}

func TestTwoRecordsSharingOneLastTouchInstantEachGetTheirOwnReminder(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Shared Anchor Account", nil)
	deal := e.SeedDeal(t, "Shared Anchor Deal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	person := e.SeedPerson(t, "Champion", nil)
	seedStakeholderSeat(t, owner, person, deal)
	for _, row := range []struct {
		table string
		id    ids.UUID
	}{{"organization", org}, {"deal", deal}, {"person", person}} {
		backdateCreatedAt(t, owner, row.table, row.id, longEstablished)
	}

	// ONE captured mail on both timelines — the account's and its
	// champion's — which is exactly how two records come to share a single
	// last-touch instant.
	touch := seedQuietTouch(t, owner, e.WS)
	linkTouch(t, owner, e.WS, touch, "organization", org)
	linkTouch(t, owner, e.WS, touch, "person", person)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "organization", org); got != 1 {
		t.Errorf("reminder tasks on the account = %d, want exactly 1", got)
	}
	if got := taskCountOn(t, e, "person", person); got != 1 {
		t.Errorf("reminder tasks on the champion = %d, want exactly 1 — one record's claim must not absorb the other's", got)
	}
	if got := runCountForHandler(t, e, "no_activity_reminder"); got != 2 {
		t.Errorf("workflow_run rows = %d, want 2 — one claim per record, not one per anchor instant", got)
	}
}

func TestARecordYoungerThanTheCutoffIsNotStale(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Imported Yesterday", nil)
	deal := e.SeedDeal(t, "Imported Yesterday Deal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)
	// The account itself arrived one day before the scan; the mail history
	// imported with it is weeks old. Age of the CONTENT is not neglect.
	backdateCreatedAt(t, owner, "organization", org, eligibilityScanNow.AddDate(0, 0, -1))
	linkQuietTouch(t, owner, e.WS, "organization", org)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "organization", org); got != 0 {
		t.Fatalf("reminder tasks on an account created yesterday = %d, want 0 — backfilled history is not a quiet spell", got)
	}
}

func TestOnlyAStakeholderSeatMakesAPersonACandidate(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Busy Account", nil)
	deal := e.SeedDeal(t, "Busy Account Deal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	stakeholder := e.SeedPerson(t, "Champion", nil)
	seedStakeholderSeat(t, owner, stakeholder, deal)
	// An employee of the same busy account with no seat on the deal. If
	// employment alone made a candidate, every colleague would earn a
	// reminder duplicating the account's own.
	colleague := e.SeedPerson(t, "Colleague", nil)
	seedEmployment(t, owner, colleague, org)
	for _, row := range []struct {
		table string
		id    ids.UUID
	}{{"organization", org}, {"deal", deal}, {"person", stakeholder}, {"person", colleague}} {
		backdateCreatedAt(t, owner, row.table, row.id, longEstablished)
	}
	linkQuietTouch(t, owner, e.WS, "person", stakeholder)
	linkQuietTouch(t, owner, e.WS, "person", colleague)
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "person", stakeholder); got != 1 {
		t.Errorf("reminder tasks on the deal's champion = %d, want exactly 1", got)
	}
	if got := taskCountOn(t, e, "person", colleague); got != 0 {
		t.Errorf("reminder tasks on a colleague with no seat on the deal = %d, want 0", got)
	}
}

func TestOnlyALeadStillInPlayIsACandidate(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	working := seedLeadInStatus(t, owner, "contacted")
	disqualified := seedLeadInStatus(t, owner, "disqualified")
	for _, id := range []ids.UUID{working, disqualified} {
		backdateCreatedAt(t, owner, "lead", id, longEstablished)
		linkQuietTouch(t, owner, e.WS, "lead", id)
	}
	seedNoActivityReminder(t, owner, e.WS)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "lead", working); got != 1 {
		t.Errorf("reminder tasks on a lead still being worked = %d, want exactly 1", got)
	}
	if got := taskCountOn(t, e, "lead", disqualified); got != 0 {
		t.Errorf("reminder tasks on a disqualified lead = %d, want 0 — that lead is finished business", got)
	}
}

// runEligibilityScan drives one full time-scan pass at the pinned instant.
func runEligibilityScan(t *testing.T, e *Env) {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	scanner := compose.NewTimeScannerWithClock(e.DB(), func() time.Time { return eligibilityScanNow }, quiet)
	if err := scanner.ScanWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), e.WS); err != nil {
		t.Fatalf("time-scan pass: %v", err)
	}
}

// seedQuietTouch inserts one human-logged mail at quietSince and returns
// its id, leaving the caller to decide which timelines it lands on.
func seedQuietTouch(t *testing.T, owner *pgx.Conn, ws ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'email', 'Last genuine engagement', $2, 'manual', 'human:x')`,
		id, quietSince); err != nil {
		t.Fatalf("seeding the quiet touch: %v", err)
	}
	return id
}

// linkQuietTouch is the one-record shorthand: a fresh quiet mail on
// exactly one entity's timeline.
func linkQuietTouch(t *testing.T, owner *pgx.Conn, ws ids.UUID, entityType string, entity ids.UUID) {
	t.Helper()
	linkTouch(t, owner, ws, seedQuietTouch(t, owner, ws), entityType, entity)
}

// linkTouch attaches an activity to any of the record types the candidate
// query knows — the harness's own LinkActivity only spans person and deal.
func linkTouch(t *testing.T, owner *pgx.Conn, ws, activity ids.UUID, entityType string, entity ids.UUID) {
	t.Helper()
	column, ok := map[string]string{
		"person": "person_id", "organization": "organization_id",
		"deal": "deal_id", "lead": "lead_id",
	}[entityType]
	if !ok {
		t.Fatalf("no activity_link column for entity type %q", entityType)
	}
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, `+column+`) VALUES ($1, $2, $3)`, activity, entityType, entity); err != nil {
		t.Fatalf("linking the touch to %s %s: %v", entityType, entity, err)
	}
}

// taskCountOn counts the reminder tasks standing on one record's timeline.
func taskCountOn(t *testing.T, e *Env, entityType string, entity ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `
		SELECT count(*) FROM activity a
		JOIN activity_link al ON al.activity_id = a.id
		WHERE al.entity_type = $1
		  AND coalesce(al.person_id, al.organization_id, al.deal_id, al.lead_id) = $2
		  AND a.kind = 'task' AND a.archived_at IS NULL`, entityType, entity)
}

// backdateCreatedAt shifts a record's creation stamp through the owner
// connection: rows seeded by the module stores are created "now", and the
// creation grace reads created_at directly.
func backdateCreatedAt(t *testing.T, owner *pgx.Conn, table string, id ids.UUID, at time.Time) {
	t.Helper()
	if _, ok := map[string]struct{}{
		"person": {}, "organization": {}, "deal": {}, "lead": {},
	}[table]; !ok {
		t.Fatalf("backdating %q is not part of this fixture's vocabulary", table)
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE `+table+` SET created_at = $1 WHERE id = $2`, at, id); err != nil {
		t.Fatalf("backdating %s %s: %v", table, id, err)
	}
}

// attachDealToOrg puts the deal on the account, which is what makes the
// account itself worth reminding about.
func attachDealToOrg(t *testing.T, owner *pgx.Conn, deal, org ids.UUID) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal); err != nil {
		t.Fatalf("attaching deal %s to organization %s: %v", deal, org, err)
	}
}

// seedStakeholderSeat gives a person a live seat on a deal.
func seedStakeholderSeat(t *testing.T, owner *pgx.Conn, person, deal ids.UUID) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		 VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, person, deal); err != nil {
		t.Fatalf("seeding the stakeholder seat: %v", err)
	}
}

// seedEmployment employs a person at an organization — a relationship the
// candidate query deliberately does NOT treat as live work.
func seedEmployment(t *testing.T, owner *pgx.Conn, person, org ids.UUID) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO relationship (kind, person_id, organization_id, source, captured_by)
		 VALUES ('employment', $1, $2, 'manual', 'human:x')`,
		person, org); err != nil {
		t.Fatalf("seeding the employment edge: %v", err)
	}
}

// seedLeadInStatus plants a lead in one named lifecycle status, so a suite can
// put two leads either side of the in-play boundary and assert that only the
// one still in play is a candidate. The status is the whole variable — every
// other column is held constant so a difference in the result can only come
// from it.
func seedLeadInStatus(t *testing.T, owner *pgx.Conn, status string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, status, source, captured_by)
		 VALUES ($1, 'Inbound Lead', $2, 'manual', 'human:x')`,
		id, status); err != nil {
		t.Fatalf("seeding a %s lead: %v", status, err)
	}
	return id
}

// recentlyWorked is INSIDE the cutoff: a touch at this instant means somebody
// is working the relationship right now.
var recentlyWorked = eligibilityScanNow.AddDate(0, 0, -1)

// seedTouchAt is seedQuietTouch with the instant named, so a suite can put one
// touch either side of the cutoff and assert which one decided the outcome.
func seedTouchAt(t *testing.T, owner *pgx.Conn, at time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, 'email', 'Genuine engagement', $2, 'manual', 'human:x')`,
		id, at); err != nil {
		t.Fatalf("seeding a touch at %s: %v", at, err)
	}
	return id
}

// An account is reached by mail filed against its CONTACT, which is how
// capture files it — the message names the person it was with, never the
// company. Counting only the account's own links, this account looks untouched
// since the old direct mail and earns a reminder about a relationship a rep
// worked yesterday.
func TestAnAccountWorkedThroughItsContactIsNotRemindedAbout(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Worked Through Its People", nil)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)

	// The account's own last direct mail is old.
	linkTouch(t, owner, e.WS, seedTouchAt(t, owner, quietSince), "organization", org)

	// Yesterday a rep mailed the contact. Capture files that against the
	// PERSON, so it carries no organization link of its own.
	contact := e.SeedPerson(t, "Ingrid Sattler", nil)
	seedEmployment(t, owner, contact, org)
	linkTouch(t, owner, e.WS, seedTouchAt(t, owner, recentlyWorked), "person", contact)

	seedNoActivityReminder(t, owner, e.WS)
	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "organization", org); got != 0 {
		t.Fatalf("reminder tasks on an account mailed through its contact yesterday = %d, want 0 — "+
			"the account is being worked, and the mail reaches it through the person it was with", got)
	}
}

// The mirror case, and the reason this is not just a false-positive fix: an
// account whose correspondence NEVER carried a direct link was not merely
// mis-dated, it was invisible. Counting only its own links it has no last
// touch at all, so it never entered the draw and could never be reminded about
// however long it went quiet.
func TestAnAccountWhoseOnlyMailIsItsContactsIsStillDrawnWhenItGoesQuiet(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	org := e.SeedOrg(t, "Quiet Through Its People", nil)
	deal := e.SeedDeal(t, "Renewal", pipeline, open, nil)
	attachDealToOrg(t, owner, deal, org)
	backdateCreatedAt(t, owner, "organization", org, longEstablished)
	backdateCreatedAt(t, owner, "deal", deal, longEstablished)

	// Every message this account ever had is filed against its contact, and
	// the last of them is long past the cutoff. No direct link, ever.
	contact := e.SeedPerson(t, "Ingrid Sattler", nil)
	seedEmployment(t, owner, contact, org)
	linkTouch(t, owner, e.WS, seedTouchAt(t, owner, quietSince), "person", contact)

	seedNoActivityReminder(t, owner, e.WS)
	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "organization", org); got != 1 {
		t.Fatalf("reminder tasks on a quiet account reached only through its contact = %d, want 1 — "+
			"an account with no direct link is still an account somebody stopped talking to", got)
	}
}
