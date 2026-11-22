// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A running website read puts the pages it has committed on the AI activity
// read, through the real writer (UpdateSiteReadProgress) and the real
// projection. The crawl has no honest length, so the count never comes with a
// total and the rail's ring stays unmeasured.

import (
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
	if _, err := f.env.Contacts.BeginSiteRead(f.worker, f.readID, siteReadLease); err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	f.drain(t)
	if item := f.liveItem(t); item.Processed != nil || item.Total != nil {
		t.Fatalf("a claimed read that has committed nothing reports %v of %v, want both absent", item.Processed, item.Total)
	}

	pages := []contacts.SiteReadPage{{URL: "https://acme.example", Kind: "home"}, {URL: "https://acme.example/about", Kind: "about"}}
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, "crawling", pages, siteReadLease); err != nil {
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

	more := append(pages, contacts.SiteReadPage{URL: "https://acme.example/team", Kind: "team"})
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, "crawling", more, siteReadLease); err != nil {
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
	if err := f.env.Contacts.UpdateSiteReadProgress(f.worker, f.readID, "crawling", pages, siteReadLease); err != nil {
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
