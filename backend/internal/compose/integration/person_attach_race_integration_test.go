// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An attach cannot outrun the archive that is retiring the person it hangs off.
//
// archivePersonRows archives the person AND sweeps their relationships, in one
// transaction. A writer that inserted a relationship without holding the person
// row could commit between the archive's own probe and its sweep, leaving a
// LIVE relationship pointing at an ARCHIVED person — the orphan the "block
// rather than orphan" rule exists to prevent (issue #1625).
//
// Forced rather than hoped for, the same shape as the erasure race: the archive
// is held open on its own transaction, the attach is started and must PARK
// behind it, the archive commits, and the attach must then be refused because
// there is no live person left to attach to.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAttachCannotOutrunTheArchive(t *testing.T) {
	e := Setup(t)
	person := ids.From[ids.PersonKind](e.SeedPerson(t, "Retiring Stakeholder", nil))
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Attaching Company", nil))

	// The archive, held open. It IS the holder — the real interleaving, and the
	// transaction the attach actually races.
	holderCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	holder, err := e.Pool.Begin(holderCtx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(holderCtx,
		`SELECT set_config('app.workspace_id', $1::text, true)`, e.WS); err != nil {
		t.Fatal(err)
	}
	var holderPID int
	if err := holder.QueryRow(holderCtx,
		`UPDATE person SET archived_at = now() WHERE id = $1 RETURNING pg_backend_pid()`,
		person.UUID).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}

	attached := make(chan error, 1)
	go func() {
		_, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
			Kind:           "employment",
			PersonID:       &person,
			OrganizationID: &org,
		})
		attached <- err
	}()

	// It has to actually PARK behind the archive, asked of Postgres rather than
	// assumed after a sleep — otherwise the archive could commit before the
	// attach opened its transaction, and the test would pass against a writer
	// that takes no lock at all.
	waitForBlockedOn(holderCtx, t, holder, holderPID)

	if err := holder.Commit(holderCtx); err != nil {
		t.Fatalf("committing the archive: %v", err)
	}

	// The attach is refused, because the person it named is no longer live.
	// That is the right answer, and the alternative is the orphan: a live
	// relationship on an archived person, which the archive's sweep has already
	// run past and will never come back for.
	err = <-attached
	if err == nil {
		t.Fatal("the attach succeeded against a person archived while it waited — that is a live " +
			"relationship on an archived person, and the sweep that would have caught it has run")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the attach failed with %v, want a not-found: the person is gone, which is a "+
			"fact about the target rather than a fault of the request", err)
	}
	assertNoLiveRelationship(t, e, person)
}

func assertNoLiveRelationship(t *testing.T, e *Env, personID ids.PersonID) {
	t.Helper()
	var live int
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM relationship WHERE person_id = $1 AND archived_at IS NULL`,
			personID.UUID).Scan(&live)
	}); err != nil {
		t.Fatalf("counting the person's live relationships: %v", err)
	}
	if live != 0 {
		t.Errorf("%d live relationship(s) point at an archived person", live)
	}
}
