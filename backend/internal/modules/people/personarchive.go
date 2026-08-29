// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Retiring a person: the refusal that can be answered before a human is asked,
// the archive itself, and the child rows that go with it.
//
// Split from person.go on the concept rather than the line count: these three
// are one story (a record leaves day-to-day work without being deleted) and none
// of them is on the create or update path beside them.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RefuseArchivePerson answers every authority refusal ArchivePerson would
// answer with, and writes nothing — the stage-time half of the archive, so a
// staged approval is never spent on a call the store was always going to
// refuse. No version probe: a version that is right at staging can be wrong by
// the time the human answers, so the pin is the write's business.
func (s *Store) RefuseArchivePerson(ctx context.Context, id ids.PersonID) error {
	if err := auth.Require(ctx, "person", principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritable(ctx, tx, "person", id.UUID)
	})
}

// ArchivePerson soft-deletes the person and cascades to its owned child
// rows and referencing edges in the same transaction (data-model §1.10).
//
// ArchivePerson retires one person and their satellites, conditioned on
// ifVersion wherever the caller's authority named a version.
func (s *Store) ArchivePerson(ctx context.Context, id ids.PersonID, ifVersion *int64) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionDelete); err != nil {
		return crmcontracts.Person{}, err
	}
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return crmcontracts.Person{}, err
	}
	var out crmcontracts.Person
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "person", id.UUID); err != nil {
			return err
		}
		// A liveness probe, not a wire read — no custom columns needed.
		if _, err := readPerson(ctx, tx, id, storekit.LiveOnly, nil); err != nil {
			return err
		}

		if err := archivePersonRows(ctx, tx, id, time.Now().UTC(), ifVersion); err != nil {
			return err
		}
		out, err = readPerson(ctx, tx, id, storekit.IncludeArchived, active)
		return err
	})
	return out, err
}

// archivePersonRows retires a person and its satellites and lands the write
// shape for it — the archive audit row and person.archived. It is the one
// spelling of "archive a person" inside a transaction, shared by the archive
// verb and by a lead demotion that unwinds the person a promotion created.
//
// ifVersion pins the PERSON row where the caller's authority named a version;
// the satellites below take no pin because they are a cascade off that row
// rather than second decisions — the guard on the person is what serializes
// all of them.
//
// A caller with nothing to pin passes nil and takes the row lock instead,
// which costs the lead demotion nothing: it already holds FOR UPDATE on this
// person (demote.go), and LockRow re-takes an owned lock idempotently rather
// than queueing behind itself.
func archivePersonRows(ctx context.Context, tx pgx.Tx, id ids.PersonID, now time.Time, ifVersion *int64) error {
	p := storekit.NewPatch()
	p.Set("archived_at", nil, now)
	if err := p.ApplyGuarded(ctx, tx, "person", id.UUID, ifVersion); err != nil {
		return err
	}
	for _, stmt := range []string{
		`UPDATE person_email SET archived_at = $2 WHERE person_id = $1 AND archived_at IS NULL`,
		`UPDATE person_phone SET archived_at = $2 WHERE person_id = $1 AND archived_at IS NULL`,
		// A live channel identity under an archived Person would keep
		// resolving inbound messages onto a record that has been
		// soft-deleted; archived, the next message starts a fresh one.
		`UPDATE person_channel_identity SET archived_at = $2 WHERE person_id = $1 AND archived_at IS NULL`,
		`UPDATE relationship SET archived_at = $2 WHERE (person_id = $1 OR counterparty_person_id = $1) AND archived_at IS NULL`,
	} {
		if _, err := tx.Exec(ctx, stmt, id, now); err != nil {
			return err
		}
	}
	// Polymorphic membership/tag rows have no archived_at; the §1.10
	// cleanup rule removes them with the entity.
	if _, err := tx.Exec(ctx,
		`DELETE FROM list_member WHERE entity_type = 'person' AND entity_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM taggable WHERE entity_type = 'person' AND entity_id = $1`, id); err != nil {
		return err
	}

	auditID, err := storekit.Audit(ctx, tx, "archive", "person", id.UUID, nil, nil)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventPersonArchived{})
}

// EnsurePersonByEmail resolves the live person who owns email, or
// creates one through the normal governed write path — the idempotent-
// on-email contract of the public capture surfaces (feedback/14): a
// returning booker never becomes a duplicate person.
