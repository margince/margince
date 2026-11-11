// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func relatedClauseFor(t *testing.T, objects map[string]principal.ObjectGrant, anchor *ids.ContactID) string {
	t.Helper()
	next := 0
	clause, err := contactRelatedClause(edgeGrantCtx(objects), anchor, func(any) int { next++; return next })
	if err != nil {
		t.Fatalf("contactRelatedClause: %v", err)
	}
	return clause
}

func TestTheRelatedFilterAddsNothingWhenNoContactIsNamed(t *testing.T) {
	if clause := relatedClauseFor(t, map[string]principal.ObjectGrant{"contact": {Read: true}}, nil); clause != "" {
		t.Errorf("an unfiltered list got a related clause: %q", clause)
	}
}

func TestTheRelatedFilterMatchesNobodyWithoutEdgeRead(t *testing.T) {
	anchor := ids.New[ids.ContactKind]()
	clause := relatedClauseFor(t, map[string]principal.ObjectGrant{"contact": {Read: true}, "deal": {Read: true}}, &anchor)
	if clause != scopeNoRows {
		t.Errorf("a reader without relationship read got %q, want %q", clause, scopeNoRows)
	}
}

func TestTheRelatedFilterReachesDealSeatsOnlyWithDealRead(t *testing.T) {
	anchor := ids.New[ids.ContactKind]()
	edgesOnly := map[string]principal.ObjectGrant{"contact": {Read: true}, "relationship": {Read: true}}
	if clause := relatedClauseFor(t, edgesOnly, &anchor); strings.Contains(clause, "JOIN deal") {
		t.Errorf("a reader without deal read was matched through deals:\n%s", clause)
	}
	withDeals := map[string]principal.ObjectGrant{"contact": {Read: true}, "relationship": {Read: true}, "deal": {Read: true}}
	if clause := relatedClauseFor(t, withDeals, &anchor); !strings.Contains(clause, "JOIN deal") {
		t.Errorf("a reader with deal read was not matched through deals:\n%s", clause)
	}
}

// A caller with no actor bound is a wiring fault, so every part refuses rather
// than handing back a clause that would read as "all rows".
func TestTheRelatedFilterRefusesACallerWithNoActor(t *testing.T) {
	anchor := ids.New[ids.ContactKind]()
	arg := func(any) int { return 1 }
	if clause, err := contactRelatedClause(context.Background(), &anchor, arg); err == nil {
		t.Errorf("contactRelatedClause(no actor) = %q, want an error", clause)
	}
	if clause, err := anchorColleaguesSQL(context.Background(), 1, arg); err == nil {
		t.Errorf("anchorColleaguesSQL(no actor) = %q, want an error", clause)
	}
	if clause, err := anchorDealPeersSQL(context.Background(), 1, arg); err == nil {
		t.Errorf("anchorDealPeersSQL(no actor) = %q, want an error", clause)
	}
}

func TestAnEmptyEdgeScopeReadsAsEveryRow(t *testing.T) {
	if got := allRowsIfEmpty(""); got != scopeAllRows {
		t.Errorf("allRowsIfEmpty(\"\") = %q, want %q", got, scopeAllRows)
	}
	if got := allRowsIfEmpty("mine.owner_id = $1"); got != "mine.owner_id = $1" {
		t.Errorf("allRowsIfEmpty kept %q, want the scope itself", got)
	}
}
