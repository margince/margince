// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The total beside a list page, over a real Postgres. The number labels what
// the reader is looking at, so the ways it can lie are the test:
//
//   - it counts the whole matching set, not the page, so a list longer than
//     one page reports its length rather than the page size;
//   - it ignores the keyset cursor, so it does not shrink as the reader pages
//     forward and report the list emptying under them;
//   - it carries the caller's row scope, so it says how many rows this reader
//     may see rather than how many exist — a count is a disclosure too;
//   - it carries the request's filters, so a narrowed list is counted narrowed.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedContacts writes n workspace-visible contacts owned by the given user.
// Hand-inserted for the same reason the capture-privacy seeds are: what is
// under test is the READ, and these rows only have to exist and be scoped.
func seedContacts(t *testing.T, e *privacyEnv, owner ids.UUID, n int) {
	t.Helper()
	ctx := e.as(owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for i := range n {
			if _, err := tx.Exec(ctx, `
				INSERT INTO contact (id, full_name, owner_id, source, captured_by, visibility)
				VALUES ($1, $2, $3, 'seed', 'human:seed', 'workspace')`,
				ids.New[ids.ContactKind](), "Contact "+string(rune('A'+i%26))+ids.NewV7().String(), owner); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %d contacts: %v", n, err)
	}
}

// seedLeads writes n leads owned by the given user.
func seedLeads(t *testing.T, e *privacyEnv, owner ids.UUID, n int) {
	t.Helper()
	ctx := e.as(owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for range n {
			if _, err := tx.Exec(ctx, `
				INSERT INTO lead (id, full_name, owner_id, source, captured_by, status, score)
				VALUES ($1, $2, $3, 'seed', 'human:seed', 'new', 50)`,
				ids.New[ids.LeadKind](), "Lead "+ids.NewV7().String(), owner); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %d leads: %v", n, err)
	}
}

// totalOf runs one list read and returns its page total, failing when the
// read answers no total at all — an absent total is the defect this whole
// change exists to remove, so a test that tolerated nil would pass on it.
func totalOf(ctx context.Context, t *testing.T, e *privacyEnv, in ListContactsInput) int {
	t.Helper()
	_, page, err := e.store.ListContacts(ctx, in)
	if err != nil {
		t.Fatalf("listing contacts: %v", err)
	}
	if page.Total == nil {
		t.Fatal("the contact list answered no total: the count line has nothing to say but how many rows it loaded")
	}
	return *page.Total
}

func TestAListTotalCountsTheWholeSetRatherThanThePage(t *testing.T) {
	e := setupCapturePrivacy(t)
	const seeded = 30
	seedContacts(t, e, e.owner, seeded)

	limit := 10
	ctx := e.as(e.owner, principal.RowScopeAll)
	rows, page, err := e.store.ListContacts(ctx, ListContactsInput{Limit: &limit})
	if err != nil {
		t.Fatalf("listing contacts: %v", err)
	}
	if len(rows) != limit {
		t.Fatalf("page holds %d rows, want the requested %d", len(rows), limit)
	}
	if !page.HasMore {
		t.Fatal("has_more is false over a set three pages long")
	}
	if page.Total == nil {
		t.Fatal("no total beside a page that says there is more")
	}
	if *page.Total != seeded {
		t.Fatalf("total is %d, want %d — the count is reporting the page, not the set", *page.Total, seeded)
	}
}

func TestAListTotalDoesNotShrinkAsTheReaderPagesForward(t *testing.T) {
	e := setupCapturePrivacy(t)
	const seeded = 30
	seedContacts(t, e, e.owner, seeded)

	limit := 10
	ctx := e.as(e.owner, principal.RowScopeAll)
	_, first, err := e.store.ListContacts(ctx, ListContactsInput{Limit: &limit})
	if err != nil {
		t.Fatalf("listing the first page: %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("no cursor after the first of three pages")
	}

	// The second page carries the cursor. Its total must still describe the
	// whole set: a count that inherited the keyset clause would answer 20
	// here and 10 on the last page, so the list would appear to empty as the
	// reader walked it.
	second := totalOf(ctx, t, e, ListContactsInput{Limit: &limit, Cursor: &first.NextCursor})
	if second != seeded {
		t.Fatalf("total on the second page is %d, want %d — the count inherited the cursor", second, seeded)
	}
}

func TestAListTotalCountsOnlyTheRowsTheReaderMaySee(t *testing.T) {
	e := setupCapturePrivacy(t)
	seedContacts(t, e, e.owner, 5)
	// One capture-private row: connector-created, visibility='owner', and so
	// the owner's alone until a human promotes it. Everything else in this
	// workspace is visibility='workspace', which every seat reads — the only
	// row-scope difference a contact list has to show is this one.
	e.captureContact(t, "owner")

	owner := totalOf(ctx(e, e.owner, principal.RowScopeAll), t, e, ListContactsInput{})
	if owner != 6 {
		t.Fatalf("the capturing user's total is %d, want their 5 shared rows plus their own capture", owner)
	}

	// The admin reads every shared row and not the capture — the founder
	// decision is the importing user ONLY, at any row scope. A count that
	// skipped the scope clause would answer 6 here and disclose, as a number,
	// that a colleague has captured a contact this reader may not open.
	admin := totalOf(ctx(e, e.admin, principal.RowScopeAll), t, e, ListContactsInput{})
	if admin != 5 {
		t.Fatalf("a non-capturing reader's total is %d, want the 5 shared rows — the count is not row-scoped", admin)
	}
}

func TestAListTotalCountsTheFilteredSet(t *testing.T) {
	e := setupCapturePrivacy(t)
	seedContacts(t, e, e.owner, 5)
	seedContacts(t, e, e.admin, 7)

	// Narrowed to one owner, the total narrows with it. A count that dropped
	// the request's filters would label a filtered page with the unfiltered
	// number, which is the same lie in the other direction.
	owner := ids.From[ids.UserKind](e.admin)
	narrowed := totalOf(ctx(e, e.admin, principal.RowScopeAll), t, e, ListContactsInput{OwnerID: &owner})
	if narrowed != 7 {
		t.Fatalf("total for one owner's rows is %d, want 7 — the count dropped the filter", narrowed)
	}
}

// The leads list has TWO page paths: a sorted one through listPage, and the
// default work queue, which orders by SLA band and is its own read. The queue
// is what /leads answers with no sort — the screen's own default — so a total
// wired only into listPage would leave the list most readers actually open
// with no count at all. That is exactly how this shipped the first time.
func TestTheLeadWorkQueueCountsItsWholeSetToo(t *testing.T) {
	e := setupCapturePrivacy(t)
	const seeded = 12
	seedLeads(t, e, e.owner, seeded)

	limit := 5
	ctx := e.as(e.owner, principal.RowScopeAll)
	// No Sort: this is the work-queue path, not listPage.
	rows, page, err := e.store.ListLeads(ctx, ListLeadsInput{Limit: &limit})
	if err != nil {
		t.Fatalf("listing leads: %v", err)
	}
	if len(rows) != limit {
		t.Fatalf("queue page holds %d rows, want %d", len(rows), limit)
	}
	if page.Total == nil {
		t.Fatal("the lead work queue answered no total: the default leads list has nothing to count with")
	}
	if *page.Total != seeded {
		t.Fatalf("queue total is %d, want %d", *page.Total, seeded)
	}

	// And it holds still on the second page, for the same reason the record
	// lists must: the cursor selects the page, never the size of the set.
	second := ListLeadsInput{Limit: &limit, Cursor: &page.NextCursor}
	_, next, err := e.store.ListLeads(ctx, second)
	if err != nil {
		t.Fatalf("listing the second queue page: %v", err)
	}
	if next.Total == nil {
		t.Fatal("no total on the second queue page")
	}
	if *next.Total != seeded {
		t.Fatalf("queue total on page two is %d, want %d — the count took the cursor", *next.Total, seeded)
	}
}

// ctx is a short spelling of the env's principal binding, so the assertions
// above read as the questions they ask rather than as principal plumbing.
func ctx(e *privacyEnv, user ids.UUID, scope principal.RowScope) context.Context {
	return e.as(user, scope)
}
