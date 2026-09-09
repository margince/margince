// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The captured-company auto-enrich lane end to end (ADR-0072/A118): a
// system-requested deep read APPLIES its findings directly (fill-empty, no
// confirm-first proposal) and records the sweep cursor terminal outcome; and
// the AutoEnrichStore's eligibility read + atomic daily cap behave over a real
// migrated Postgres.

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAutoEnrichLaneAppliesDirectlyInsteadOfStaging(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	store := capture.NewAutoEnrichStore(e.DB())
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())

	// The dossier is created system-requested (as the sweep does), and its
	// cursor armed (MarkQueued) so the worker's terminal MarkResolved has a row.
	adminCtx := e.As(e.Rep1, nil, integration.AdminPerms)
	read, _, err := e.People.StartSiteRead(adminCtx, companyIDOf(company), seedURL, systemAutoEnrichActor)
	if err != nil {
		t.Fatalf("StartSiteRead: %v", err)
	}
	if err := store.MarkQueued(adminCtx, companyIDOf(company), 7*24*time.Hour); err != nil {
		t.Fatalf("MarkQueued: %v", err)
	}
	args := SiteDeepReadArgs{
		Workspace: e.WS, CompanyID: company, SiteReadID: read.ID,
		RequestedBy: read.RequestedBy,
	}

	if err := worker.run(context.Background(), args); err != nil {
		t.Fatalf("run: %v", err)
	}

	// The company fields + facts were applied directly — NOT staged as a deepread
	// proposal a human must accept.
	if n := deepReadApprovals(t, e); n != 0 {
		t.Fatalf("%d deepread proposals staged, want 0 — the auto lane applies directly", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_profile_field WHERE company_id = $1`, company); n == 0 {
		t.Fatal("the auto lane applied no profile fields")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_fact WHERE company_id = $1`, company); n == 0 {
		t.Fatal("the auto lane applied no category facts")
	}
	// The sweep cursor is terminal: outcome 'applied', never re-enqueued.
	var outcome string
	var nextAttempt *time.Time
	if err := database.WithWorkspaceTx(adminCtx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT last_outcome, next_attempt_at FROM capture_auto_enrich_state WHERE company_id = $1`,
			company).Scan(&outcome, &nextAttempt)
	}); err != nil {
		t.Fatalf("reading the cursor: %v", err)
	}
	if outcome != "applied" || nextAttempt != nil {
		t.Fatalf("cursor = (%q, %v), want (applied, <nil>)", outcome, nextAttempt)
	}
}

// insertDomainCompany seeds a captured, domain-named company (name_source='domain') with
// a live primary domain — the shape the sweep's ListDueCompanies considers.
func insertDomainCompany(t *testing.T, e *integration.Env, domain string) ids.CompanyID {
	t.Helper()
	companyID := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO company (id, owner_id, display_name, name_source, source, captured_by)
			VALUES ($1, $2, $3, 'domain', 'connector:gmail', 'connector:gmail')`,
			companyID, e.Rep1, domain); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'connector:gmail', 'connector:gmail')`, companyID, domain)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.CompanyKind](companyID)
}

func TestAutoEnrichStoreEligibilityAndCap(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewAutoEnrichStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	// How the company was NAMED no longer decides: a person creating one is
	// usually the moment they want the dossier, and the old name_source='domain'
	// rule had made this lane inert anyway (every company in the demo workspace is
	// human-named). What still excludes a company is having a dossier already.
	due1 := insertDomainCompany(t, e, "gitex.com")
	insertDomainCompany(t, e, "acme.example")
	insertCompany(t, e, e.Rep1, "human.example", "") // name_source='human' — now DUE
	// Give due1 a completed site read so it is excluded (already enriched).
	if _, _, err := e.People.StartSiteRead(ctx, due1, "https://gitex.com", "human:"+e.Rep1.String()); err != nil {
		t.Fatalf("seed dossier: %v", err)
	}

	dueList, err := store.ListDueCompanies(ctx, 10)
	if err != nil {
		t.Fatalf("ListDueCompanies: %v", err)
	}
	gotDomains := []string{}
	for _, d := range dueList {
		gotDomains = append(gotDomains, d.Domain)
	}
	sort.Strings(gotDomains)
	if !slices.Equal(gotDomains, []string{"acme.example", "human.example"}) {
		t.Fatalf("due = %v, want acme.example and human.example — a human-named company "+
			"is enriched too, and only the one holding a dossier is excluded", gotDomains)
	}

	// The daily cap is atomic: with a cap of 2, the first two reservations
	// succeed and the third is refused.
	got := []bool{}
	for range 3 {
		slot, err := store.ReserveBudget(ctx, 2)
		if err != nil {
			t.Fatalf("ReserveBudget: %v", err)
		}
		got = append(got, slot.Reserved)
	}
	if got[0] != true || got[1] != true || got[2] != false {
		t.Fatalf("reservations = %v, want [true true false] at cap 2", got)
	}
}

// runReadTo takes one company's read to a terminal status the way the
// worker does — start, claim, report — so the sweep sees the dossier state a
// real read leaves behind rather than a hand-written row.
func runReadTo(t *testing.T, e *integration.Env, companyID ids.CompanyID, seedURL, status string) {
	t.Helper()
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	read, _, err := e.People.StartSiteRead(ctx, companyID, seedURL, systemAutoEnrichActor)
	if err != nil {
		t.Fatalf("start the read: %v", err)
	}
	if _, err := e.People.BeginSiteRead(ctx, read.ID, time.Minute); err != nil {
		t.Fatalf("claim the read: %v", err)
	}
	if err := e.People.FinishSiteRead(ctx, read.ID, people.FinishSiteReadInput{Status: status}); err != nil {
		t.Fatalf("report the read as %s: %v", status, err)
	}
}

func TestTheInstallationsOwnCompanyIsNeverSwept(t *testing.T) {
	// This test carries more weight than it used to. The anchor was excluded as
	// a SIDE EFFECT of being human-named, and the sweep no longer selects on
	// that — so `NOT o.is_anchor` is now the only thing keeping the company the
	// installation IS out of a lane that applies machine values directly. Its
	// own refresh compares each proposal against confirmed truth and asks the
	// human to resolve conflicts (ADR-0065 §8), and cold start has already read
	// it, so a sweep here would be both redundant and unreviewed.
	e := integration.Setup(t)
	store := capture.NewAutoEnrichStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	website := "https://anchor.example"
	if _, err := e.People.SaveCompany(ctx, people.SaveCompanyInput{
		DisplayName: "Anchor", Website: &website,
	}); err != nil {
		t.Fatalf("describe the installation's own company: %v", err)
	}
	insertDomainCompany(t, e, "captured.example")

	due, err := store.ListDueCompanies(ctx, 10)
	if err != nil {
		t.Fatalf("ListDueCompanies: %v", err)
	}
	if len(due) != 1 || due[0].Domain != "captured.example" {
		t.Fatalf("due = %+v, want only the captured company — the anchor has a live primary domain too", due)
	}
}

func TestACancelledReadDoesNotRetireACompanyForever(t *testing.T) {
	// A read cancelled because the operator had auto-enrich off when the worker
	// claimed it produced no dossier at all. Turning the setting back on has to
	// reach that company, or the sweep's self-healing stops at exactly the companies
	// the setting stopped.
	e := integration.Setup(t)
	store := capture.NewAutoEnrichStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	cancelled := insertDomainCompany(t, e, "offagain.example")
	enriched := insertDomainCompany(t, e, "enriched.example")
	runReadTo(t, e, cancelled, "https://offagain.example", "cancelled")
	runReadTo(t, e, enriched, "https://enriched.example", "done")

	due, err := store.ListDueCompanies(ctx, 10)
	if err != nil {
		t.Fatalf("ListDueCompanies: %v", err)
	}
	if len(due) != 1 || due[0].Domain != "offagain.example" {
		t.Fatalf("due = %+v, want the cancelled company offered again and the one holding a dossier left alone", due)
	}
}

func TestAutoEnrichExpireExhausted(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewAutoEnrichStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	company := insertDomainCompany(t, e, "fail.example")

	// Two attempts used (backoff 0 so the cursor stays due, not future-armed):
	// at the attempt bound it is no longer a candidate...
	for range 2 {
		if err := store.MarkQueued(ctx, company, 0); err != nil {
			t.Fatalf("MarkQueued: %v", err)
		}
	}
	due, err := store.ListDueCompanies(ctx, 10)
	if err != nil {
		t.Fatalf("ListDueCompanies: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("due = %+v, want none — the company used every attempt", due)
	}

	// ...and the per-pass expiry retires it: outcome 'exhausted', cursor cleared
	// so it leaves the due index.
	if err := store.ExpireExhausted(ctx); err != nil {
		t.Fatalf("ExpireExhausted: %v", err)
	}
	var outcome string
	var nextAttempt *time.Time
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT last_outcome, next_attempt_at FROM capture_auto_enrich_state WHERE company_id = $1`,
			company).Scan(&outcome, &nextAttempt)
	}); err != nil {
		t.Fatalf("reading the cursor: %v", err)
	}
	if outcome != "exhausted" || nextAttempt != nil {
		t.Fatalf("cursor = (%q, %v), want (exhausted, <nil>)", outcome, nextAttempt)
	}
}

// A company waiting behind a day's worth of newer arrivals is still reached.
//
// The pass takes ONE page, bounded by the daily cap, and queues what it takes —
// which writes a cursor row that drops that company out of the due set until its
// next attempt falls due. So the set shrinks by exactly what was worked, and
// which END the page comes from decides whether it drains.
//
// Taking the newest end starves: in a workspace gaining more eligible companies
// between passes than the cap, arrivals keep landing in front of a company that
// has never been reached, and it waits forever — not retried, not exhausted, not
// visible as skipped, so nothing anywhere says it was missed.
//
// A page of one against three eligible companies is the same shape as 500
// against a day's arrivals, and it is the shape the assertion can actually read.
func TestTheOldestCompanyIsSweptFirstSoNoneWaitsForever(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewAutoEnrichStore(e.DB())
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	// Inserted oldest first. The id is a uuidv7, so this is also id order —
	// which is what the query sorts on, and why it does.
	oldest := insertDomainCompany(t, e, "waiting.example")
	insertDomainCompany(t, e, "newer.example")
	insertDomainCompany(t, e, "newest.example")

	page, err := store.ListDueCompanies(ctx, 1)
	if err != nil {
		t.Fatalf("ListDueCompanies: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("a page of 1 returned %d companies", len(page))
	}
	if page[0].Domain != "waiting.example" {
		t.Errorf("the pass takes %q, want waiting.example — the company that has been due "+
			"longest. Taking the newest end leaves it behind every arrival, and nothing "+
			"retries it or reports it skipped", page[0].Domain)
	}
	if page[0].CompanyID != oldest {
		t.Errorf("the pass takes company %s, want %s", page[0].CompanyID, oldest)
	}

	// And the set really does shrink by what was worked: once the oldest holds a
	// dossier it leaves, and the next pass takes the one behind it rather than
	// the same row again. Without this the ordering above would be a queue that
	// never advances.
	if _, _, err := e.People.StartSiteRead(ctx, oldest, "https://waiting.example",
		"human:"+e.Rep1.String()); err != nil {
		t.Fatalf("seed the oldest company's dossier: %v", err)
	}
	next, err := store.ListDueCompanies(ctx, 1)
	if err != nil {
		t.Fatalf("ListDueCompanies after the first company was worked: %v", err)
	}
	if len(next) != 1 || next[0].Domain != "newer.example" {
		t.Errorf("the next pass takes %v, want newer.example — the queue has to advance, "+
			"or oldest-first is a company that blocks every other one", next)
	}
}
