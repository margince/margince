// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// quietCandidates reads the candidates as one reader, through the given read.
func quietCandidates(t *testing.T, e *integration.Env, read search.QuietRead) []ids.UUID {
	t.Helper()
	reader := e.As(e.Rep1, nil, integration.AdminPerms)
	var got []ids.UUID
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		quiet, err := search.QuietContacts(reader, tx, read)
		for _, c := range quiet {
			got = append(got, c.Edge.ContactID)
		}
		return err
	}); err != nil {
		t.Fatalf("reading the quiet candidates: %v", err)
	}
	return got
}

// The limit cuts the strongest quiet relationships, and nothing else decides
// which: an excluded contact and a recently touched one spend no slot however
// strong, and a tie is broken by the older silence. The owner's read and the
// unnarrowed read answer the same rows for that owner.
func TestTheQuietLimitKeepsTheStrongestRelationshipsThatAreReallyQuiet(t *testing.T) {
	e := integration.Setup(t)
	excluded := e.SeedContact(t, "Strongest but set aside", &e.Rep1)
	touched := e.SeedContact(t, "Strong but written to last week", &e.Rep1)
	strong := e.SeedContact(t, "Strong and quiet", &e.Rep1)
	newer := e.SeedContact(t, "Weaker, quiet eighty days", &e.Rep1)
	older := e.SeedContact(t, "Weaker, quiet ninety days", &e.Rep1)
	for _, days := range []int{60, 61, 62} {
		seedQuietExchange(t, e, e.Rep1, ids.UUID{}, excluded, days)
	}
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, touched, 70)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, touched, 7)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, strong, 70)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, strong, 71)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, newer, 80)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, older, 90)

	read := search.QuietRead{
		Owner:        e.Rep1,
		QuietBefore:  quietLaneNow.AddDate(0, 0, -(relstrength.QuietDays - 1)),
		TouchedAfter: quietCeilingFrom(quietLaneNow),
		AsOf:         quietLaneNow,
		Limit:        2,
		Exclude: func(arg func(any) int) (string, error) {
			return fmt.Sprintf("e.contact_id <> $%d", arg(excluded)), nil
		},
	}
	if got, want := quietCandidates(t, e, read), []ids.UUID{strong, older}; !slices.Equal(got, want) {
		t.Errorf("the owner's two strongest quiet relationships = %v, want %v (strong, then the older of the tie)", got, want)
	}
	read.Owner, read.Limit = ids.UUID{}, 10
	if got, want := quietCandidates(t, e, read), []ids.UUID{strong, older, newer}; !slices.Equal(got, want) {
		t.Errorf("the unnarrowed read = %v, want %v, the owner's read in the same order", got, want)
	}
}
