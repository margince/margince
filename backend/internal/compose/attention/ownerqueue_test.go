// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A manager asking for a rep's queue gets the REP's work.
//
// The narrowing happens twice on this path and both halves have to agree: the
// lanes are read for the named person, and the assembled rows are then kept for
// them. A projection that consulted the wrong service saw no named owner, fell
// through to "mine", and handed back the MANAGER's own day under the rep's
// name — a page that is wrong in the one way its reader cannot detect, because
// it looks exactly like the answer they asked for.

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var (
	theManager = ids.MustParse("01a05500-0000-7000-8000-0000000000a1")
	theRep     = ids.MustParse("01a05500-0000-7000-8000-0000000000a2")
)

// managerReading is a lead who reaches every row, so the tier admits the ask
// and what the test then measures is the NARROWING rather than the refusal.
func managerReading() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      theManager,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// waitingOwnedBy is the who-is-waiting lane over rows attributed to two people.
type waitingOwnedBy []WaitingCustomer

func (w waitingOwnedBy) Unanswered(context.Context, time.Time) ([]WaitingCustomer, bool, error) {
	return []WaitingCustomer(w), false, nil
}

// Opening a named person's queue keeps THEIR waiting customers and drops the
// reader's own.
//
// Both rows qualify, both are visible to this reader, and the only thing
// separating them is who owes the reply — so a projection that narrows against
// the wrong person returns exactly the wrong row while still returning one.
func TestANamedOwnersQueueCarriesTheirWaitingCustomersAndNotTheReadersOwn(t *testing.T) {
	t.Parallel()

	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	svc.waiting = waitingOwnedBy{
		{
			ActivityID: ids.NewV7(), Subject: "the rep's customer",
			Since: readInstant.Add(-2 * 24 * time.Hour), OwnerID: theRep,
		},
		{
			ActivityID: ids.NewV7(), Subject: "the manager's own customer",
			Since: readInstant.Add(-2 * 24 * time.Hour), OwnerID: theManager,
		},
	}
	svc.teammates = teammatesSaying(true)

	day, err := svc.Worklist(managerReading(), "", "", theRep, 25)
	if err != nil {
		t.Fatalf("opening the named owner's queue: %v", err)
	}

	titles := map[string]bool{}
	for _, row := range day.Queue {
		if row.Title != nil {
			titles[*row.Title] = true
		}
	}
	if !titles["the rep's customer"] {
		t.Error("the rep's own waiting customer is absent from the queue asked for by their name")
	}
	if titles["the manager's own customer"] {
		t.Error("the reader's OWN waiting customer arrived on a page headed with " +
			"somebody else's name — the projection narrowed against the wrong person")
	}
}

// The two narrowing passes keep the SAME rows.
//
// A waiting row is narrowed twice: once before its crowding is decided, once
// with the whole assembled page. Between them dropDealsAlreadyWaiting absorbs a
// drifting deal INTO the waiting row and attaches that deal's facts to it — so
// the evidence the filter reads changes under it.
//
// While the deal was asked first, the two passes could answer differently. A
// deal reassigned between the at-risk read and the waiting read gave the row a
// lane owner of Bob and an attached deal owned by Alice: Bob's queue kept it on
// the first pass and dropped it on the second, Alice's dropped it on the first,
// and the waiting customer appeared on nobody's queue with nothing to say so.
func TestBothNarrowingPassesKeepTheSameWaitingRow(t *testing.T) {
	t.Parallel()

	alice := openapi_types.UUID(theManager)
	// The row as it stands AFTER the absorption: the lane resolved theRep, and
	// the deal it absorbed is owned by somebody else.
	absorbed := ranked{
		item: crmcontracts.WorklistItem{
			Id:     "wait-1",
			Source: sourceWaiting,
			Deal:   &crmcontracts.WorklistDealFacts{OwnerId: &alice},
		},
		owner: theRep,
	}

	reader := principal.Principal{Type: principal.PrincipalHuman, UserID: theRep}
	if !ownedByReader(absorbed, reader) {
		t.Fatal("the second pass judged the wait against the absorbed deal's owner rather " +
			"than the owner its own lane resolved — the row is kept once and dropped once")
	}
	if got := keepOwnedBy([]ranked{absorbed}, theRep); len(got) != 1 {
		t.Fatal("a named owner's queue dropped their own waiting customer because the row " +
			"had absorbed a deal belonging to somebody else")
	}
}
