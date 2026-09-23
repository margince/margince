// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// One walk of the contact list ranks against one instant.
//
// The order here is derived in Go from values SQL cannot sort by, and both of
// them move with the clock: strength decays and engagement flips. A page that
// recomputed `now` at its own arrival time therefore sorted against a different
// order than the page before it, and a contact whose standing crossed the
// boundary in between was served twice or not at all. The page looks complete
// either way — which is the same failure the 25-row truncation had, moved from
// the first page to the boundary between pages.
//
// The cursor carried the instant the whole time and nothing read it.
//
// ASSERTED ON `computed_at`, which is the instant on the wire, rather than on a
// reordering. Making two contacts provably swap places would mean a fixture
// tuned to the half-life constants, which reads as a test of those constants
// and goes red the day they are retuned for reasons that have nothing to do
// with paging. The instant is what the fix establishes and what a client can
// see; the ordering follows from it.

import (
	"errors"
	"testing"
	"time"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movableClock is the read's clock with a hand the test can turn, so a page can
// arrive at a different moment from the one before it.
type movableClock struct{ at time.Time }

func (c *movableClock) now() time.Time { return c.at }

func company360ServiceAt(e *integration.Env, clock *movableClock) *company360svc.Service {
	return company360svc.NewService(e.Pool, contacts.NewStore(e.DB()), e.Deals, e.Projects,
		approvals.NewService(e.DB()), clock.now)
}

func TestAResumedContactPageRanksAgainstTheWalksOwnInstant(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	owner := integration.OwnerConn(t)
	clock := &movableClock{at: company360Clock}
	svc := company360ServiceAt(e, clock)

	company := e.SeedCompany(t, "Brandt GmbH", nil)
	for i := range 5 {
		contact := e.SeedContact(t, string(rune('A'+i))+" Contact", nil)
		employ(t, e, contact, company, "Fleet")
		mail := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: your proposal",
			"inbound", company360Clock.AddDate(0, 0, -(i+1)))
		integration.LinkActivity(t, owner, mail, "contact", contact)
	}

	limit := 2
	first, err := svc.ContactPage(ctx, ids.CompanyID{UUID: company},
		company360svc.ContactListQuery{Limit: &limit})
	if err != nil {
		t.Fatalf("the first page: %v", err)
	}
	if first.Page.NextCursor == nil {
		t.Fatal("the first page carries no cursor, so there is no walk to resume and this proves nothing")
	}
	walkStarted := computedAtOf(t, first.Data)

	// Long enough that every decay in the ranking has moved.
	clock.at = company360Clock.AddDate(0, 0, 200)

	// The control: a walk STARTED now reports now. Without this the assertion
	// below would also pass against a clock that never moved.
	fresh, err := svc.ContactPage(ctx, ids.CompanyID{UUID: company},
		company360svc.ContactListQuery{Limit: &limit})
	if err != nil {
		t.Fatalf("a fresh first page: %v", err)
	}
	if got := computedAtOf(t, fresh.Data); !got.Equal(clock.at) {
		t.Fatalf("a page started at %v reports %v — the clock this test turns is not the one the read "+
			"asks, so nothing below is measuring what it claims", clock.at, got)
	}

	resumed, err := svc.ContactPage(ctx, ids.CompanyID{UUID: company},
		company360svc.ContactListQuery{Limit: &limit, Cursor: first.Page.NextCursor})
	if err != nil {
		t.Fatalf("resuming the walk: %v", err)
	}
	if got := computedAtOf(t, resumed.Data); !got.Equal(walkStarted) {
		t.Errorf("the resumed page ranks as of %v and the walk began at %v — the two pages are slices "+
			"of two different orderings, so a contact whose standing moved between them is served "+
			"twice or not at all", got, walkStarted)
	}

	// And the whole walk still covers the account exactly once, which is what
	// the instant is for.
	seen := map[ids.UUID]bool{}
	cursor := first.Page.NextCursor
	for _, row := range first.Data {
		seen[ids.UUID(row.ContactId)] = true
	}
	for page := 0; page < 5 && cursor != nil; page++ {
		got, err := svc.ContactPage(ctx, ids.CompanyID{UUID: company},
			company360svc.ContactListQuery{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("walking page %d: %v", page+2, err)
		}
		for _, row := range got.Data {
			id := ids.UUID(row.ContactId)
			if seen[id] {
				t.Fatalf("the walk serves %s twice", id)
			}
			seen[id] = true
		}
		cursor = got.Page.NextCursor
	}
	if len(seen) != 5 {
		t.Errorf("the walk covered %d of 5 contacts — a rep paging this account misses one and the "+
			"page looks complete", len(seen))
	}
}

// computedAtOf reads the instant a page ranked against, off the wire.
func computedAtOf(t *testing.T, rows []crmcontracts.CompanyContact) time.Time {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("an empty page carries no instant to read")
	}
	at := rows[0].Strength.ComputedAt
	if at == nil {
		t.Fatal("the row reports no computed_at, so the walk's instant is not on the wire")
	}
	return at.UTC()
}

// A cursor naming an instant no page was served from is refused.
//
// THE TOKEN IS UNSIGNED. storekit.EncodeOpaque is base64 over JSON, and that
// package's own note says a well-formed token is not yet a valid position — the
// caller checks the fields before trusting them. Reading `as_of` therefore means
// reading a client-controlled instant, so the two readings that name no walk are
// refused rather than honoured: zero, because a token this service minted always
// carries one, and the future, because no page has been served from there.
//
// Written because the guard was claimed and not held. The code says it and the
// pull request says it; neither is a test, and a refusal nothing exercises is a
// sentence rather than a control.
func TestAForgedWalkInstantIsRefused(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	clock := &movableClock{at: company360Clock}
	svc := company360ServiceAt(e, clock)

	company := e.SeedCompany(t, "Brandt GmbH", nil)
	contact := e.SeedContact(t, "Ute Sommer", nil)
	employ(t, e, contact, company, "Fleet")

	limit := 1
	for name, forged := range map[string]contactCursorForTest{
		"an instant in the future": {Sort: "recommended", ID: contact, AsOf: company360Clock.AddDate(0, 0, 1)},
		"no instant at all":        {Sort: "recommended", ID: contact},
	} {
		t.Run(name, func(t *testing.T) {
			token := forgeCursor(t, forged)
			_, err := svc.ContactPage(ctx, ids.CompanyID{UUID: company},
				company360svc.ContactListQuery{Limit: &limit, Cursor: &token})
			var malformed *storekit.MalformedCursorError
			if !errors.As(err, &malformed) {
				t.Errorf("a cursor carrying %s answered %v, want a malformed-cursor refusal — the "+
					"token is unsigned, so an instant nobody was served from is a client's "+
					"invention rather than a position to resume from", name, err)
			}
		})
	}
}

// contactCursorForTest is the token's shape, spelled here because the writer's
// own type is unexported. The JSON keys are what bind the two: a rename on
// either side makes this forge a token the reader does not recognise, which
// answers malformed for the wrong reason — so the keys are asserted by the
// round trip in the case above rather than assumed.
type contactCursorForTest struct {
	Sort string    `json:"s"`
	ID   ids.UUID  `json:"i"`
	AsOf time.Time `json:"a"`
}

func forgeCursor(t *testing.T, pos contactCursorForTest) string {
	t.Helper()
	token, err := storekit.EncodeOpaque(pos)
	if err != nil {
		t.Fatalf("forging a cursor: %v", err)
	}
	return token
}
