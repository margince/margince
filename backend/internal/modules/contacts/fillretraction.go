// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Taking back one machine fill of a contact's profile fields, from its history.
//
// A signature or a site read fills blanks: a title, a phone number, the company
// a contact says they work for. Each fill is one audit row naming the fields it
// landed and the message or page it read. Undoing that row clears those fields
// again, and this file is the only place that knows what "clear" means for each
// of them — a sidecar evidence row, the title column it mirrors, the phone
// number it added, the LinkedIn slot it claimed.
//
// Only a FILL is taken back here. Where the pass replaced a value somebody had
// set, putting that value back is RestoreProfileField's job, and where somebody
// has changed the field since, their answer stands. Both are refused rather than
// cleared, because clearing either would undo a human's answer to undo the
// machine's.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FillRetraction is one machine fill of one contact: which pass wrote it, what
// it read, which fields landed, and when.
type FillRetraction struct {
	Source    string
	SourceRef string
	Fields    []string
	FilledAt  time.Time
	// MirroredTitle is the title a site read wrote straight onto the column
	// with no title evidence row of its own (fillSiteContactFields). Empty for
	// a signature, whose title has its own row.
	MirroredTitle string
}

// retractableFillSources are the passes that only fill blanks and record each
// field they landed as a sidecar row carrying their source and source_ref.
var retractableFillSources = map[string]bool{enrichSource: true, siteFieldSource: true}

// profileFields are the fields those passes can land.
var profileFields = map[string]bool{
	fieldTitle: true, fieldPhone: true, fieldRole: true, fieldCompanyName: true,
	fieldAddress: true, fieldLinkedin: true, fieldWebsite: true,
}

// fillRetractSource marks the audit row of a retraction, so a later reader can
// tell it from the fill it took back.
const fillRetractSource = "fill_retracted"

// FillOf reads whether one contact audit row is a fill this module can take
// back. The row qualifies when a retractable pass wrote it, it names what it
// read, and every field it names was empty before and is a profile field.
func FillOf(before, after, evidence json.RawMessage, occurredAt time.Time) (FillRetraction, bool) {
	var was, now map[string]json.RawMessage
	var said struct {
		Source    string `json:"source"`
		SourceRef string `json:"source_ref"`
	}
	if json.Unmarshal(before, &was) != nil || json.Unmarshal(after, &now) != nil ||
		json.Unmarshal(evidence, &said) != nil {
		return FillRetraction{}, false
	}
	if !retractableFillSources[said.Source] || said.SourceRef == "" || len(now) == 0 || len(was) != len(now) {
		return FillRetraction{}, false
	}
	fields := make([]string, 0, len(now))
	for field := range now {
		prior, stated := was[field]
		if !stated || string(prior) != "null" || !profileFields[field] {
			return FillRetraction{}, false
		}
		fields = append(fields, field)
	}
	slices.Sort(fields)
	fill := FillRetraction{Source: said.Source, SourceRef: said.SourceRef, Fields: fields, FilledAt: occurredAt}
	var title string
	if json.Unmarshal(now[fieldTitle], &title) == nil && title != signatureFieldFilled {
		fill.MirroredTitle = title
	}
	return fill, true
}

// FillRetractionRefusal says why a fill cannot be taken back. Moved names the
// fields somebody has changed since; Replaced names the fields where the pass
// took the place of an earlier value rather than filling a blank.
type FillRetractionRefusal struct {
	Moved    []string
	Replaced []string
}

func (e *FillRetractionRefusal) Error() string {
	return fmt.Sprintf("contacts: the fill cannot be taken back (changed since: %v, replaced a value: %v)",
		e.Moved, e.Replaced)
}

// JudgeFillRetraction answers whether the fill still stands as the pass left
// it, on the caller's transaction: nil when it can be taken back, a
// *FillRetractionRefusal when it cannot, any other error when the read failed.
func JudgeFillRetraction(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction) error {
	var refusal FillRetractionRefusal
	for _, field := range fill.Fields {
		stands, replaced, err := fieldStands(ctx, tx, contactID, fill, field)
		if err != nil {
			return err
		}
		switch {
		case !stands:
			refusal.Moved = append(refusal.Moved, field)
		case replaced:
			refusal.Replaced = append(refusal.Replaced, field)
		}
	}
	if len(refusal.Moved) == 0 && len(refusal.Replaced) == 0 {
		return nil
	}
	return &refusal
}

// fieldStands reads one field the fill named. A site read's title has no
// evidence row of its own, so it stands while the column still holds what the
// read wrote there.
func fieldStands(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction, field string) (stands, replaced bool, err error) {
	rows, stands, replaced, err := fillStands(ctx, tx, contactID, fill, field)
	if err != nil || rows > 0 || field != fieldTitle || fill.MirroredTitle == "" {
		return stands, replaced, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, false, err
	}
	err = tx.QueryRow(ctx, `SELECT title IS NOT DISTINCT FROM $2 FROM contact WHERE id = $1`,
		contactID, fill.MirroredTitle).Scan(&stands)
	if err != nil {
		return false, false, fmt.Errorf("contacts: reading whether the filled title still stands: %w", err)
	}
	return stands, false, nil
}

// fillStands reads one field's evidence rows: how many the pass wrote that are
// still there, whether every one is uncorrected and still what the record
// shows, and whether any took the place of an earlier value. A phone number the
// pass did not insert — it only confirmed one already on the record — counts as
// taking a place, because clearing it would remove somebody else's number.
func fillStands(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction, field string) (rows int, stands, replaced bool, err error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return 0, false, false, err
	}
	err = tx.QueryRow(ctx, `
		SELECT count(*),
		       count(*) > 0
		       AND bool_and(NOT EXISTS (
		         SELECT 1 FROM ai_feedback
		          WHERE subject_type = 'contact' AND subject_id = f.contact_id
		            AND claim_kind = 'profile_field' AND verdict = 'corrected'
		            AND claim_key = encode(sha256(('profile_field:' || f.field)::bytea), 'hex')))
		       AND bool_and(f.field <> $5 OR EXISTS (
		         SELECT 1 FROM contact p WHERE p.id = f.contact_id AND p.title IS NOT DISTINCT FROM f.value))
		       AND bool_and(f.field <> $6 OR EXISTS (
		         SELECT 1 FROM contact_phone n
		          WHERE n.contact_id = f.contact_id AND n.phone = f.value AND n.source = f.source
		            AND n.archived_at IS NULL)),
		       coalesce(bool_or(f.superseded_value IS NOT NULL OR (f.field = $6 AND NOT EXISTS (
		         SELECT 1 FROM contact_phone n
		          WHERE n.contact_id = f.contact_id AND n.phone = f.value AND n.created_at = $7))), false)
		  FROM contact_profile_field f
		 WHERE f.contact_id = $1 AND f.field = $2 AND f.source = $3 AND f.source_ref = $4`,
		contactID, field, fill.Source, fill.SourceRef, fieldTitle, fieldPhone, fill.FilledAt).Scan(&rows, &stands, &replaced)
	if err != nil {
		return 0, false, false, fmt.Errorf("contacts: reading whether the %s fill still stands: %w", field, err)
	}
	return rows, stands, replaced, nil
}

// RetractFill takes one fill back: the evidence rows it wrote, and what each
// put on the record. The caller marks the context with the entry it reverses
// (storekit.WithReversal), which is what the history links the two by.
func (s *Store) RetractFill(ctx context.Context, contactID ids.ContactID, fill FillRetraction, ifVersion *int64) error {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, entityContact, contactID.UUID); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRow(ctx, `SELECT version FROM contact WHERE id = $1`, contactID).Scan(&version); err != nil {
			return fmt.Errorf("contacts: reading the contact a fill is taken back from: %w", err)
		}
		if err := refuseIfVersionMoved(entityContact, &version, writeOptions{atVersion: ifVersion}); err != nil {
			return err
		}
		if err := JudgeFillRetraction(ctx, tx, contactID, fill); err != nil {
			return err
		}
		for _, field := range fill.Fields {
			if err := retractField(ctx, tx, contactID, fill, field); err != nil {
				return err
			}
		}
		if err := touchContact(ctx, tx, contactID.UUID); err != nil {
			return err
		}
		// Named, not quoted, like the fill it takes back: the values came out of
		// somebody else's message, and audit_log outlives the erasure of the
		// record they came from.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, entityContact, contactID.UUID,
			map[string]any{}, map[string]any{},
			map[string]any{auditKeySource: fillRetractSource, auditKeyFields: fill.Fields, auditKeySourceRef: fill.SourceRef})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, contactID.UUID, crmcontracts.PublicEventContactUpdated{
			ChangedFields: map[string]any{auditKeyFields: fill.Fields, auditKeySource: fillRetractSource},
		})
	})
}

// retractField clears what the pass put on the record for one field, then
// removes its evidence rows. The column and number writes compare against the
// value the pass wrote, so nothing somebody set since is touched.
func retractField(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction, field string) error {
	rows, err := tx.Query(ctx, `
		DELETE FROM contact_profile_field
		 WHERE contact_id = $1 AND field = $2 AND source = $3 AND source_ref = $4
		RETURNING value`, contactID, field, fill.Source, fill.SourceRef)
	if err != nil {
		return fmt.Errorf("contacts: removing the %s the fill wrote: %w", field, err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("contacts: removing the %s the fill wrote: %w", field, err)
	}
	if len(values) == 0 && field == fieldTitle && fill.MirroredTitle != "" {
		values = []string{fill.MirroredTitle}
	}
	for _, value := range values {
		if err := clearFilledValue(ctx, tx, contactID, fill, field, value); err != nil {
			return err
		}
	}
	return nil
}

// clearFilledValue takes one value off the record where the field has a home
// beyond its evidence row.
func clearFilledValue(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction, field, value string) error {
	// Re-taken here, where the record is written: RetractFill already holds the
	// row, and a lock this transaction owns is taken again without waiting.
	if _, err := storekit.LockRow(ctx, tx, entityContact, contactID.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, entityContact, contactID.UUID); err != nil {
		return err
	}
	var err error
	switch field {
	case fieldTitle:
		_, err = tx.Exec(ctx, `UPDATE contact SET title = NULL WHERE id = $1 AND title = $2 AND archived_at IS NULL`, contactID, value)
	case fieldPhone:
		_, err = tx.Exec(ctx, `
			UPDATE contact_phone SET archived_at = now()
			 WHERE contact_id = $1 AND phone = $2 AND source = $3 AND created_at = $4 AND archived_at IS NULL`,
			contactID, value, fill.Source, fill.FilledAt)
	case fieldLinkedin:
		err = clearClaimedLinkedinSlot(ctx, tx, contactID, fill, value)
	}
	if err != nil {
		return fmt.Errorf("contacts: clearing the %s the fill wrote: %w", field, err)
	}
	return nil
}

// clearClaimedLinkedinSlot empties the LinkedIn slot only when this fill is
// what filled it. fillEmptyLinkedinSlot records that on its own audit row, in
// the fill's transaction and therefore at the fill's instant; a slot somebody
// set before or after has no such row and is left alone.
func clearClaimedLinkedinSlot(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fill FillRetraction, value string) error {
	handle, err := NormalizeLinkedInURL(value)
	if err != nil {
		//nolint:nilerr // a value the normalizer refuses never reached the slot, so there is nothing to clear
		return nil
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM contact_social
		 WHERE contact_id = $1 AND platform = $2 AND handle = $3
		   AND EXISTS (
		     SELECT 1 FROM audit_log a
		      WHERE a.entity_type = $4 AND a.entity_id = $1 AND a.occurred_at = $5
		        AND a.after ? $6)`,
		contactID, socialLinkedIn, handle, entityContact, fill.FilledAt, auditKeySocial)
	return err
}
