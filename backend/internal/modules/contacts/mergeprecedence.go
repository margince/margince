// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The one rule for combining two contacts' values. Every contact merge runs it:
// the pair merge, the duplicate queue's merge, and each member of a case merge.
//
//  1. Every value either side holds stays on the survivor: each address,
//     number and employment. A number both hold is one number, so the
//     survivor's row stays and the merged-away row is archived onto it.
//  2. One value fills each single slot: the name, the title, the primary
//     address, the primary number and the current employer. A case reviewer
//     may prefer a member for all but the number, and that member wins.
//     Otherwise the survivor keeps what it holds, and an empty slot takes the
//     merged-away contact's value.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeField is a single slot a reviewer may fill from a chosen member.
type MergeField string

// The slots a case review offers, spelled as the contract's `prefer` keys.
const (
	MergeFieldName       MergeField = "name"
	MergeFieldTitle      MergeField = "title"
	MergeFieldEmail      MergeField = "email"
	MergeFieldEmployment MergeField = "employment"
)

// mergeFields lists every slot in the order the audit row names them.
var mergeFields = []MergeField{MergeFieldName, MergeFieldTitle, MergeFieldEmail, MergeFieldEmployment}

// MergePreference names, per slot, the member whose value the survivor takes.
// A slot it leaves out follows rule 2's default. The zero value prefers nobody.
type MergePreference map[MergeField]ids.UUID

// MergePreferenceOf reads the contract's `prefer` object.
func MergePreferenceOf(in *crmcontracts.ContactCasePreference) MergePreference {
	if in == nil {
		return nil
	}
	prefer := MergePreference{}
	for field, member := range map[MergeField]*openapi_types.UUID{
		MergeFieldName: in.Name, MergeFieldTitle: in.Title, MergeFieldEmail: in.Email, MergeFieldEmployment: in.Employment,
	} {
		if member != nil {
			prefer[field] = ids.UUID(*member)
		}
	}
	return prefer
}

// sourceWins reports whether the merged-away contact's value takes the slot.
func (p MergePreference) sourceWins(field MergeField, source ids.ContactID) bool {
	member, ok := p[field]
	return ok && member == source.UUID
}

// wonBy names the slots the merged-away contact won, for the survivor's audit row.
func (p MergePreference) wonBy(source ids.ContactID) []string {
	won := []string{}
	for _, field := range mergeFields {
		if p.sourceWins(field, source) {
			won = append(won, string(field))
		}
	}
	return won
}

// mergePhoneHeld is true when the survivor already holds number a.phone live.
// Enrichment keeps rule 1 in confirmKnownNumber, which dates a held number
// instead of adding it again.
const mergePhoneHeld = `EXISTS (SELECT 1 FROM contact_phone b
 WHERE b.contact_id = @target AND b.phone = a.phone AND b.archived_at IS NULL)`

// mergeHeldPhonePrimary is true when the survivor's own row of number a.phone is
// its primary, so the number stays primary after the merge.
const mergeHeldPhonePrimary = `EXISTS (SELECT 1 FROM contact_phone b
 WHERE b.contact_id = @target AND b.phone = a.phone AND b.is_primary AND b.archived_at IS NULL)`

// buildSurvivorshipPatch folds the merged-away contact's name and title onto
// the survivor by rule 2, and fills the survivor's empty address and lead link.
func buildSurvivorshipPatch(target, source crmcontracts.Contact, sourceID ids.ContactID, prefer MergePreference) *storekit.Patch {
	p := storekit.NewPatch()
	// A chosen name is never mixed with the parts of another member's.
	_, nameChosen := prefer[MergeFieldName]
	switch {
	case prefer.sourceWins(MergeFieldName, sourceID) && source.FullName != target.FullName:
		takeName(p, target, source)
	case !nameChosen:
		takeString(p, columnFirstName, target.FirstName, source.FirstName, false)
		takeString(p, columnLastName, target.LastName, source.LastName, false)
	}
	takeString(p, "title", target.Title, source.Title, prefer.sourceWins(MergeFieldTitle, sourceID))
	if target.ConvertedFromLeadId == nil && source.ConvertedFromLeadId != nil {
		p.Set("converted_from_lead_id", nil, ids.UUID(*source.ConvertedFromLeadId))
	}
	fillMergeAddress(p, target.Address, source.Address)
	return p
}

// takeName copies the source's whole name. The display name and its parts are
// one name, so mixing parts of two names would invent a third.
func takeName(p *storekit.Patch, target, source crmcontracts.Contact) {
	p.Set(contactNameColumn, target.FullName, source.FullName)
	for _, part := range []struct {
		column         string
		target, source *string
	}{{columnFirstName, target.FirstName, source.FirstName}, {columnLastName, target.LastName, source.LastName}} {
		if derefString(part.target) == derefString(part.source) {
			continue
		}
		var before, after any
		if part.target != nil {
			before = *part.target
		}
		if part.source != nil {
			after = *part.source
		}
		p.Set(part.column, before, after)
	}
}

// takeString sets a text column from the source when the survivor has none, or
// when the source wins the slot and holds a different value.
func takeString(p *storekit.Patch, column string, target, source *string, sourceWins bool) {
	if source == nil || (target != nil && (!sourceWins || *target == *source)) {
		return
	}
	if target == nil {
		p.Set(column, nil, *source)
		return
	}
	p.Set(column, *target, *source)
}

// yieldPreferredSlots frees the survivor's primary address and current
// employer when the merged-away contact won them and holds one to give. The
// relink then carries the source's primary flag across, because the slot is
// empty. A preferred employer always lands. The relink archives the source's
// current edge only as a twin of an equally current one. That twin then
// inherits the slot (archiveDuplicatesKeepingPrimarySlot).
func yieldPreferredSlots(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID, prefer MergePreference) error {
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID}
	if prefer.sourceWins(MergeFieldEmail, sourceID) {
		// The address that wins is the one the review showed for the member: its
		// reachable-first address, which is its primary when it has one.
		for _, stmt := range []string{
			`UPDATE contact_email SET is_primary = false
 WHERE contact_id = @target AND is_primary AND archived_at IS NULL
 AND EXISTS (SELECT 1 FROM contact_email s WHERE s.contact_id = @source AND s.archived_at IS NULL)`,
			`UPDATE contact_email SET is_primary = true WHERE NOT is_primary AND id = (SELECT id FROM contact_email
 WHERE contact_id = @source AND archived_at IS NULL` + ReachableEmailOrder + ` LIMIT 1)`,
		} {
			if _, err := tx.Exec(ctx, stmt, args); err != nil {
				return fmt.Errorf("contacts: handing the primary address to the preferred member: %w", err)
			}
		}
	}
	if !prefer.sourceWins(MergeFieldEmployment, sourceID) {
		return nil
	}
	_, err := mergeEdgeWrite(ctx, tx, `is_current_primary = false`, `a.contact_id = @target AND `+employment.CurrentPrimarySlotSQL("a")+`
 AND EXISTS (SELECT 1 FROM relationship s WHERE s.contact_id = @source AND `+employment.LiveCurrentPrimarySQL("s")+`)`, args)
	if err != nil {
		return fmt.Errorf("contacts: handing the current employer to the preferred member: %w", err)
	}
	return nil
}

// relinkPhones moves the merged-away contact's numbers onto the survivor by
// rule 1. A number the survivor already holds is archived onto it, so the value
// shows once and the survivor's row keeps its primary flag. When that number was
// the source's primary and the survivor has none, the survivor's row of it takes
// the slot. The count covers both moves.
func relinkPhones(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) (int64, error) {
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, archiveStateKey: time.Now().UTC()}
	if _, err := tx.Exec(ctx, `UPDATE contact_phone SET is_primary = true
 WHERE id = (SELECT b.id FROM contact_phone b JOIN contact_phone a ON a.phone = b.phone
   WHERE b.contact_id = @target AND b.archived_at IS NULL AND `+mergeContactAddressSource+` AND a.is_primary
   ORDER BY b.position, b.id LIMIT 1)
 AND NOT EXISTS (SELECT 1 FROM contact_phone p WHERE p.contact_id = @target AND p.is_primary AND p.archived_at IS NULL)`, args); err != nil {
		return 0, fmt.Errorf("contacts: keeping the primary of a number both contacts hold: %w", err)
	}
	folded, err := tx.Exec(ctx, `UPDATE contact_phone a SET contact_id = @target, is_primary = false, archived_at = @archived
 WHERE `+mergeContactAddressSource+` AND `+mergePhoneHeld, args)
	if err != nil {
		return 0, fmt.Errorf("contacts: folding a number both contacts hold: %w", err)
	}
	moved, err := relinkDemotingPrimary(ctx, tx,
		`UPDATE contact_phone a SET contact_id = @target, is_primary = `+mergePhonePrimary+` WHERE `+mergeContactAddressSource,
		sourceID.UUID, targetID.UUID)
	return folded.RowsAffected() + moved, err
}

// checkMergePreference refuses a preference for a member the decision does not
// merge.
func checkMergePreference(prefer MergePreference, selected []ids.UUID) error {
	for field, member := range prefer {
		if !slices.Contains(selected, member) {
			return &DedupeInputError{Field: "prefer." + string(field), Msg: "must be one of member_ids"}
		}
	}
	return nil
}
