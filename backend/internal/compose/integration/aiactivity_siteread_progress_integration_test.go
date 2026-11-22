// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A running website read puts the pages it has committed on the AI activity
// read, through the real writer (UpdateSiteReadProgress) and the real
// projection. The crawl has no known length, so the count never comes with a
// total and the rail's ring stays unmeasured.

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (f *websiteReadFixture) liveItem(t *testing.T) aiactivity.Item {
	t.Helper()
	feed, err := aiactivity.NewStore(f.env.DB()).
		Mine(f.env.As(f.env.Rep1, nil, principal.Permissions{}), time.Time{}, nil)
	if err != nil {
		t.Fatalf("reading the AI activity feed: %v", err)
	}
	if len(feed.Live) != 1 {
		t.Fatalf("live items = %d, want the one running read", len(feed.Live))
	}
	return feed.Live[0]
}

func TestARunningWebsiteReadSurfacesItsPageCountWithoutATotal(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	f.drain(t)
	if item := f.liveItem(t); item.Processed != nil || item.Total != nil {
		t.Fatalf("a claimed read that has committed nothing reports %v of %v, want both absent", item.Processed, item.Total)
	}

	pages := []contacts.SiteReadPage{{URL: "https://acme.example", Kind: "home"}, {URL: "https://acme.example/about", Kind: "about"}}
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, claim.ClaimedAt, "crawling", pages, siteReadLease); err != nil {
		t.Fatalf("UpdateSiteReadProgress: %v", err)
	}
	f.drain(t)
	item := f.liveItem(t)
	if item.Processed == nil || *item.Processed != 2 {
		t.Fatalf("processed = %v, want the 2 committed pages", item.Processed)
	}
	if item.Total != nil {
		t.Fatalf("total = %d, want absent: the crawl's length is not known", *item.Total)
	}
	if got := f.projection(t); got.StaleAfter == nil {
		t.Fatal("a progress report dropped the read's lease, so a dead worker would render as live forever")
	}

	more := append(slices.Clone(pages), contacts.SiteReadPage{URL: "https://acme.example/team", Kind: "team"})
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, claim.ClaimedAt, "crawling", more, siteReadLease); err != nil {
		t.Fatalf("UpdateSiteReadProgress: %v", err)
	}
	f.drain(t)
	if item := f.liveItem(t); item.Processed == nil || *item.Processed != 3 {
		t.Fatalf("processed = %v after a third page, want 3: a progress report at the same attempt and state must land", item.Processed)
	}
}

func TestASettledWebsiteReadCarriesNoCount(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	pages := []contacts.SiteReadPage{{URL: "https://acme.example", Kind: "home"}}
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, claim.ClaimedAt, "crawling", pages, siteReadLease); err != nil {
		t.Fatalf("UpdateSiteReadProgress: %v", err)
	}
	f.drain(t)
	if err := f.env.Contacts.FinishSiteRead(f.worker, f.readID, contacts.FinishSiteReadInput{
		Status: "done", ClaimedAt: &claim.ClaimedAt, Pages: pages,
	}); err != nil {
		t.Fatalf("FinishSiteRead: %v", err)
	}
	f.drain(t)

	var processed *int
	if err := f.env.Pool.QueryRow(t.Context(),
		`SELECT progress_processed FROM ai_task_run WHERE source = $1 AND occurrence_key = $2`,
		contacts.SiteReadActivitySource, f.readID.String()).Scan(&processed); err != nil {
		t.Fatalf("reading the projected count: %v", err)
	}
	if processed != nil {
		t.Fatalf("a settled read still holds processed = %d; a settled run reports its outcome, not a position", *processed)
	}
}

// siteReadFacts is what the dossier row and the staged events say about one read,
// read straight from the tables so a refused write is seen to change nothing.
type siteReadFacts struct {
	pagesRead, outbox, ledger int
	phase                     *string
}

func (f *websiteReadFixture) facts(t *testing.T) siteReadFacts {
	t.Helper()
	var got siteReadFacts
	err := f.env.Pool.QueryRow(t.Context(), `
		SELECT pages_read, phase,
		       (SELECT count(*) FROM event_outbox
		         WHERE envelope->>'type' = 'ai_task.state_changed'
		           AND envelope->'payload'->>'occurrence_key' = $1),
		       (SELECT count(*) FROM system_log
		         WHERE action = 'ai_task.state_changed' AND detail->>'occurrence_key' = $1)
		  FROM site_read WHERE id = $2`, f.readID.String(), f.readID).
		Scan(&got.pagesRead, &got.phase, &got.outbox, &got.ledger)
	if err != nil {
		t.Fatalf("reading the dossier and its staged events: %v", err)
	}
	return got
}

func TestProgressFromAReclaimedAttemptIsRefusedAndChangesNothing(t *testing.T) {
	f := newWebsiteReadFixture(t)
	first, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	if _, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, time.Microsecond); err != nil {
		t.Fatalf("the reclaim: %v", err)
	}
	before := f.facts(t)

	pages := []contacts.SiteReadPage{{URL: "https://acme.example", Kind: "home"}}
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, first.ClaimedAt, "crawling", pages, siteReadLease); err != nil {
		t.Fatalf("a stale claim is a no-op, not an error: %v", err)
	}
	if after := f.facts(t); after != before {
		t.Fatalf("the abandoned worker's progress changed the dossier or staged an event: before %+v, after %+v", before, after)
	}
}

func TestAnInvalidLeaseMutatesNeitherTheDossierNorTheProjection(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	before := f.facts(t)
	pages := []contacts.SiteReadPage{{URL: "https://acme.example", Kind: "home"}}
	for _, lease := range []time.Duration{0, -time.Minute} {
		if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, claim.ClaimedAt, "crawling", pages, lease); err == nil {
			t.Fatalf("lease %v was accepted; it would store no stale_after and a dead read would never stall", lease)
		}
	}
	if after := f.facts(t); after != before {
		t.Fatalf("a refused progress write changed state: before %+v, after %+v", before, after)
	}
}

func TestACrawlStartCommitsItsDossierLedgerAndOutboxRowsTogether(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	before := f.facts(t)
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, claim.ClaimedAt, "crawling", nil, siteReadLease); err != nil {
		t.Fatalf("UpdateSiteReadProgress: %v", err)
	}
	after := f.facts(t)
	if after.phase == nil || *after.phase != "crawling" {
		t.Fatalf("phase = %v, want crawling", after.phase)
	}
	if after.outbox != before.outbox+1 || after.ledger != before.ledger+1 {
		t.Fatalf("the phase change staged outbox %d->%d and ledger %d->%d rows, want one of each",
			before.outbox, after.outbox, before.ledger, after.ledger)
	}
}
