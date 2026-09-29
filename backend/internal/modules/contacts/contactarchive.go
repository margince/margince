// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Retiring a contact: the refusal that can be answered before a human is asked,
// the archive itself, and the child rows that go with it.
//
// Split from contact.go on the concept rather than the line count: these three
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

// RefuseArchiveContact answers every authority refusal ArchiveContact would
// answer with, and writes nothing — the stage-time half of the archive, so a
// staged approval is never spent on a call the store was always going to
// refuse. No version probe: a version that is right at staging can be wrong by
// the time the human answers, so the pin is the write's business.
func (s *Store) RefuseArchiveContact(ctx context.Context, id ids.ContactID) error {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritable(ctx, tx, "contact", id.UUID)
	})
}

// ArchiveContact soft-deletes the contact and cascades to its owned child
// rows and referencing edges in the same transaction (data-model §1.10).
//
// ArchiveContact retires one contact and their satellites, conditioned on
// ifVersion wherever the caller's authority named a version.
func (s *Store) ArchiveContact(
	ctx context.Context, id ids.ContactID, ifVersion *int64, opts ...WriteOption,
) (crmcontracts.Contact, error) {
	options := collectWriteOptions(opts)
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return crmcontracts.Contact{}, err
	}
	active, err := s.activeColumns(ctx, "contact")
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	var out crmcontracts.Contact
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := archiveContactInTx(ctx, tx, id, ifVersion, options); err != nil {
			return err
		}
		out, err = readContact(ctx, tx, id, storekit.IncludeArchived, active)
		return err
	})
	return out, err
}

// ArchiveContactTx is ArchiveContact on the caller's transaction, for a bulk
// change that archives many contacts in one commit. It asks every gate
// ArchiveContact asks and answers nothing: the caller already knows which row
// it archived.
func (s *Store) ArchiveContactTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, ifVersion *int64) error {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return err
	}
	return archiveContactInTx(ctx, tx, id, ifVersion, writeOptions{})
}

// archiveContactInTx is the archive itself, behind the object gate its callers
// ask: the row's write check, the caller's precondition, and the cascade.
func archiveContactInTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, ifVersion *int64, options writeOptions) error {
	if err := auth.EnsureWritable(ctx, tx, "contact", id.UUID); err != nil {
		return err
	}
	// The precondition, under the row lock the write takes: a caller that
	// asked for this write only while nobody had touched the record gets
	// that answered HERE rather than in a read that already committed.
	if err := refuseIfHumanTouched(ctx, tx, "contact", id.UUID, options); err != nil {
		return err
	}
	// A liveness probe, not a wire read — no custom columns needed.
	if _, err := readContact(ctx, tx, id, storekit.LiveOnly, nil); err != nil {
		return err
	}
	return archiveContactRows(ctx, tx, id, time.Now().UTC(), ifVersion)
}

// archiveContactRows retires a contact and its satellites and lands the write
// shape for it — the archive audit row and contact.archived. It is the one
// spelling of "archive a contact" inside a transaction, shared by the archive
// verb and by a lead demotion that unwinds the contact a promotion created.
//
// ifVersion pins the CONTACT row where the caller's authority named a version;
// the satellites below take no pin because they are a cascade off that row
// rather than second decisions — the guard on the contact is what serializes
// all of them.
//
// A caller with nothing to pin passes nil and takes the row lock instead,
// which costs the lead demotion nothing: it already holds FOR UPDATE on this
// contact (demote.go), and LockRow re-takes an owned lock idempotently rather
// than queueing behind itself.
func archiveContactRows(ctx context.Context, tx pgx.Tx, id ids.ContactID, now time.Time, ifVersion *int64) error {
	p := storekit.NewPatch()
	p.Set("archived_at", nil, now)
	if err := p.ApplyGuarded(ctx, tx, "contact", id.UUID, ifVersion); err != nil {
		return err
	}
	// Every row the cascade retires or deletes is recorded on the archive's
	// audit row, which is what an un-archive reads to put it back.
	var cascade storekit.ArchiveCascade
	for _, retire := range []struct{ table, statement string }{
		{tableContactEmail, `UPDATE contact_email SET archived_at = $2 WHERE contact_id = $1 AND archived_at IS NULL RETURNING id`},
		{tableContactPhone, `UPDATE contact_phone SET archived_at = $2 WHERE contact_id = $1 AND archived_at IS NULL RETURNING id`},
		// A live channel identity under an archived Contact would keep
		// resolving inbound messages onto a record that has been
		// soft-deleted; archived, the next message starts a fresh one.
		{"contact_channel_identity", `UPDATE contact_channel_identity SET archived_at = $2 WHERE contact_id = $1 AND archived_at IS NULL RETURNING id`},
		{tableRelationship, `UPDATE relationship SET archived_at = $2 WHERE (contact_id = $1 OR counterparty_contact_id = $1) AND archived_at IS NULL RETURNING id`},
	} {
		if err := cascade.Retire(ctx, tx, retire.table, retire.statement, id, now); err != nil {
			return err
		}
	}
	// Polymorphic membership/tag rows have no archived_at; the §1.10
	// cleanup rule removes them with the entity.
	if err := cascade.DropMemberships(ctx, tx,
		`WITH gone AS (
			DELETE FROM list_member WHERE entity_type = 'contact' AND entity_id = @record
			RETURNING list_id, entity_type, entity_id, added_by, created_at, note),
		logged AS (
			INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor)
			SELECT list_id, entity_type, entity_id, 'removed', 'record_archived', @actor FROM gone)
		SELECT list_id, added_by, created_at, note FROM gone`, id.UUID); err != nil {
		return err
	}
	if err := cascade.DropTags(ctx, tx,
		`DELETE FROM taggable WHERE entity_type = 'contact' AND entity_id = $1 RETURNING tag_id, assigned_by, assigned_by_kind, assigned_at`, id.UUID); err != nil {
		return err
	}

	auditID, err := storekit.AuditWithEvidence(ctx, tx, "archive", "contact", id.UUID, nil, nil, cascade.Evidence())
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactArchived{})
}

// EnsureContactByEmail resolves the live contact who owns email, or
// creates one through the normal governed write path — the idempotent-
// on-email contract of the public capture surfaces (feedback/14): a
// returning booker never becomes a duplicate contact.
