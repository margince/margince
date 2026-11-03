// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// edgeBetween finds the case's pair joining two members.
func edgeBetween(t *testing.T, read crmcontracts.ContactCase, a, b ids.UUID) crmcontracts.ContactCaseEdge {
	t.Helper()
	for _, e := range read.Edges {
		left, right := ids.UUID(e.LeftId), ids.UUID(e.RightId)
		if (left == a && right == b) || (left == b && right == a) {
			return e
		}
	}
	t.Fatalf("the case holds no pair between %s and %s", a, b)
	return crmcontracts.ContactCaseEdge{}
}

// caseHolds says whether the case read from one contact names another. A
// contact left in no open pair reads as no case.
func (c *caseEnv) caseHolds(ctx context.Context, t *testing.T, from, member ids.UUID) bool {
	t.Helper()
	got, err := c.store.GetContactCase(ctx, from)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	return slices.Contains(uuidsOf(memberIDs(got)), member)
}

func TestAnOwnerOfOneEndDismissesAColleaguesPairOnlyUnderThePolicy(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	rep, stranger := c.rep(c.e.Rep1), c.rep(c.e.Rep3)
	pair := edgeBetween(t, c.read(rep, t, members[0]), members[0], members[1])

	if pair.CanDismiss {
		t.Error("with the policy off the pair reads can_dismiss for an owner of one end")
	}
	if _, err := c.store.DisposeDedupeCandidate(rep, ids.UUID(pair.PairId), "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a dismissal with the policy off = %v, want 403", err)
	}

	c.setPolicy(t, true)
	before := c.read(rep, t, members[0])
	if !edgeBetween(t, before, members[0], members[1]).CanDismiss {
		t.Error("under the policy the pair does not read can_dismiss for an owner of one end")
	}
	if edgeBetween(t, c.read(stranger, t, members[2]), members[0], members[1]).CanDismiss {
		t.Error("under the policy the pair reads can_dismiss for a rep who owns neither end")
	}
	if _, err := c.store.DisposeDedupeCandidate(stranger, ids.UUID(pair.PairId), "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a dismissal by a rep who owns neither end = %v, want 403", err)
	}
	if _, err := c.store.DisposeDedupeCandidate(rep, ids.UUID(pair.PairId), "not_a_duplicate", nil); err != nil {
		t.Fatalf("an owner of one end dismissing under the policy: %v", err)
	}
	if c.caseHolds(rep, t, members[0], members[1]) {
		after := c.read(rep, t, members[0])
		if after.Revision == before.Revision || edgeBetween(t, after, members[0], members[1]).Status != crmcontracts.ContactCaseEdgeStatusNotADuplicate {
			t.Error("after the dismissal the case still shows the pair open")
		}
	}

	c.setPolicy(t, false)
	if _, err := c.store.UndoDedupeDisposition(rep, ids.UUID(pair.PairId)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an undo with the policy off = %v, want 403", err)
	}
	c.setPolicy(t, true)
	if _, err := c.store.UndoDedupeDisposition(rep, ids.UUID(pair.PairId)); err != nil {
		t.Fatalf("an owner of one end undoing under the policy: %v", err)
	}
	if !c.caseHolds(rep, t, members[0], members[1]) {
		t.Error("after the undo the case does not hold both contacts again")
	}
}

func TestAReadSeatOwnerMayNotDismissAColleaguesPair(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE app_user SET seat_type = 'read' WHERE id = $1`, c.e.Rep2); err != nil {
		t.Fatal(err)
	}
	c.setPolicy(t, true)
	ctx := c.rep(c.e.Rep2)
	p, _ := principal.Actor(ctx)
	p.SeatType = principal.SeatRead
	reader := principal.WithActor(ctx, p)

	pair := edgeBetween(t, c.read(reader, t, members[1]), members[0], members[1])
	if pair.CanDismiss {
		t.Error("a read seat reads can_dismiss on a colleague's pair")
	}
	if _, err := c.store.DisposeDedupeCandidate(reader, ids.UUID(pair.PairId), "not_a_duplicate", nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a read seat's dismissal = %v, want 403", err)
	}
}
