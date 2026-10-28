// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// The §1.3 two-record merge (features/01, data-model §3.2): A→B relinks
// everything that points at A to B in ONE transaction with zero orphaned
// FKs, fills B's gaps from A without overwriting anything B already has,
// and archives A with merged_into_id = B so it stays fetchable by id.
// Relinking is collision-aware, not a blind UPDATE: every unique index
// and shape constraint on the child tables encodes an invariant the
// surviving record must still satisfy afterwards —
//
//   - ≤1 primary email/phone per (contact, type) and ≤1 primary domain per
//     company: A's primaries demote when B already holds that slot.
//   - ≤1 current-primary employer per contact: same demotion rule.
//   - an activity/list/tag linked to BOTH records keeps B's link and
//     drops A's (pure link rows, deletion loses nothing).
//   - a relationship edge A already shares with B (same kind + same far
//     end) archives instead of relinking — a duplicate edge is noise,
//     and archived rows keep the provenance.
//   - a partner edge BETWEEN A and B can survive on neither (a company
//     cannot partner with itself): it archives.
//
// Consent merges restrictively where the two records disagree: A's
// withdrawal propagates to B (with an appended proof event — a state
// change without proof would break the Art. 7(1) invariant), and where
// B already holds a row for a purpose, B's state stands — A's grant
// never overrides it. For purposes B has NO row for, A's rows travel to
// B together with their original proof chain: a merge asserts the two
// records are the same human, so a consent that human granted remains
// proven (the same carry-through the lead→contact promotion does).

// The audit keys a merge writes, spelled once so the contact, company
// and lead merges cannot drift apart in what they record.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const (
	auditKeyMergedInto = "merged_into_id"
	auditKeyMergedFrom = "merged_from_id"
	auditKeyFilled     = "filled"
	// The tally a contact merge writes on both rows: how many references of
	// each kind moved over. Counts only, never the values.
	auditKeyRelinkedEmails        = "relinked_emails"
	auditKeyRelinkedPhones        = "relinked_phones"
	auditKeyRelinkedRelationships = "relinked_relationships"
	auditKeyRelinkedActivityLinks = "relinked_activity_links"
)

// filledColumns names the columns a merge filled on the survivor, for the
// archived record's row, which must not carry the values themselves.
func filledColumns(filled map[string]any) []string {
	return slices.Sorted(maps.Keys(filled))
}

// heldNothing is the before image of a fill-only write: every column it set
// held nothing until now, which is the only case a fill covers.
func heldNothing(filled map[string]any) map[string]any {
	before := make(map[string]any, len(filled))
	for column := range filled {
		before[column] = nil
	}
	return before
}

// MergeSelfError maps to 422: a record cannot merge into itself.
type MergeSelfError struct{}

func (e *MergeSelfError) Error() string { return "source and target of a merge must differ" }

// FieldFault refuses a merge whose source and target are the same record.
func (e *MergeSelfError) FieldFault() (field, code, message string) {
	return "target_id", "merge_self", e.Error()
}

// AlreadyMergedError maps to 409: the source was already merged away; the
// pointer names where it went.
type AlreadyMergedError struct {
	Kind   string
	IntoID ids.UUID
}

func (e *AlreadyMergedError) Error() string { return e.Kind + " is already merged" }

// MergedTargetError maps to 422: the chosen survivor is itself archived
// or merged away — nothing can merge INTO a dead record.
type MergedTargetError struct{ Kind string }

func (e *MergedTargetError) Error() string {
	return "merge target " + e.Kind + " is archived; the survivor must be live"
}

// FieldFault refuses merging INTO a record that was itself already merged away.
func (e *MergedTargetError) FieldFault() (field, code, message string) {
	return "target_id", "merged_target", e.Error()
}

// relinkCounts is the event payload's accounting (events.md §contact.merged):
// downstream consumers re-home their references from these numbers.
type relinkCounts struct {
	Emails        int64 `json:"emails"`
	Phones        int64 `json:"phones"`
	Relationships int64 `json:"relationships"`
	ActivityLinks int64 `json:"activity_links"`
}

// targetIDField names the wire path the merge bodies use for the SURVIVOR. Named
// once because both merge verbs refuse on it, and a field slot holds a wire
// field path, never prose.
const targetIDField = "target_id"

// MergeContact merges contact source→target and returns the survivor.
func (s *Store) MergeContact(ctx context.Context, sourceID, targetID ids.ContactID, ifVersion *int64) (crmcontracts.Contact, error) {
	// target_id is required by the contract, which is true only if checked. An
	// absent key decodes to the zero UUID, and the self-merge guard below does
	// not catch it (a real source id never equals the zero one), so it reaches
	// the pair lock and answers not-found for a survivor nobody named.
	if err := httperr.RequireBodyID(targetIDField, targetID.UUID); err != nil {
		return crmcontracts.Contact{}, err
	}
	// authz.go maps the merge verb to update: rewriting where records
	// point is curation of both rows, not deletion of one.
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return crmcontracts.Contact{}, err
	}
	if sourceID == targetID {
		return crmcontracts.Contact{}, &MergeSelfError{}
	}

	active, err := s.activeColumns(ctx, contactEntity)
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	var out crmcontracts.Contact
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.mergeContactTx(ctx, tx, sourceID, targetID, ifVersion, active)
		return err
	})
	return out, err
}

// mergeContactTx locks both ends, resolves them, relinks every
// reference, fills the survivor's gaps, retires the source, and runs
// the write shape — all inside the caller's transaction. The pair lock
// is what makes the target check hold until commit: without it a
// concurrent merge(target→elsewhere) could archive the survivor
// mid-merge, leaving relinked children pointing at a dead record.
func (s *Store) mergeContactTx(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID, ifVersion *int64, active []fieldcatalog.Column) (crmcontracts.Contact, error) {
	out, _, err := s.mergeContactCarryingTx(ctx, tx, sourceID, targetID, ifVersion, active)
	return out, err
}

// mergeContactCarryingTx is mergeContactTx answering what it did about the
// merged-away contact's owner, for a caller that reports it.
func (s *Store) mergeContactCarryingTx(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID, ifVersion *int64, active []fieldcatalog.Column) (crmcontracts.Contact, *authz.OwnerAccessCarry, error) {
	// BEFORE LockPair, because recording a stop takes consent's lock and then
	// reads the contact row. Locking the rows first and reaching for consent's
	// lock later (inside carryStopsTx, below) inverts the order between the two
	// transactions and deadlocks — see StopCarrier.LockStopsTx.
	if err := lockStopsOrSkip(ctx, tx, s.stopCarrier,
		commsauthz.ContactStopSubject(sourceID),
		commsauthz.ContactStopSubject(targetID)); err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	_, tgtLock, err := storekit.LockPair(ctx, tx, contactEntity, sourceID.UUID, targetID.UUID)
	if err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	src, tgt, err := mergePair(ctx, tx, contactEntity, sourceID, targetID, ifVersion, readContactMergeState)
	if err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	counts, err := relinkContactReferences(ctx, tx, sourceID, targetID)
	if err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	if err := s.carryContactSatellitesTx(ctx, tx, sourceID, targetID); err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	carry, err := s.carryOwnerAccessTx(ctx, tx, targetID, (*ids.UUID)(tgt.OwnerId), (*ids.UUID)(src.OwnerId), sourceID.UUID)
	if err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	// A third contact's open pair with the merged-away one now asks about the
	// survivor, so it stays in front of a human instead of going quiet.
	if err := moveDedupeCandidates(ctx, tx, entityContact, sourceID.UUID, targetID.UUID); err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	p := buildSurvivorshipPatch(tgt, src)
	if !p.Empty() {
		if err := p.ApplyLocked(ctx, tx, tgtLock); err != nil {
			return crmcontracts.Contact{}, nil, fmt.Errorf("apply survivorship fill: %w", err)
		}
	}
	if err := archiveMergedAway(ctx, tx, contactEntity, sourceID.UUID, targetID.UUID); err != nil {
		return crmcontracts.Contact{}, nil, fmt.Errorf("retire merged-away contact: %w", err)
	}
	if err := auditContactMerge(ctx, tx, sourceID, targetID, counts, p); err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	out, err := readContact(ctx, tx, targetID, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Contact{}, nil, fmt.Errorf("read surviving contact: %w", err)
	}
	return out, carry, nil
}

// auditContactMerge records the merge on both rows and emits contact.merged.
func auditContactMerge(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID, counts relinkCounts, p *storekit.Patch) error {
	auditID, err := storekit.Audit(ctx, tx, "merge", contactEntity, sourceID.UUID,
		map[string]any{auditKeyMergedInto: nil},
		map[string]any{
			auditKeyMergedInto: targetID, auditKeyFilled: filledColumns(p.After()),
			auditKeyRelinkedEmails: counts.Emails, auditKeyRelinkedPhones: counts.Phones,
			auditKeyRelinkedRelationships: counts.Relationships, auditKeyRelinkedActivityLinks: counts.ActivityLinks,
		})
	if err != nil {
		return fmt.Errorf("audit contact merge: %w", err)
	}
	// One event, its own verb: the context graph collapses two nodes,
	// which neither contact.updated nor contact.archived can say.
	if err := storekit.EmitEvent(ctx, tx, auditID, sourceID.UUID, contactMergedPayload(sourceID, targetID, counts)); err != nil {
		return fmt.Errorf("emit contact.merged: %w", err)
	}
	// Each filled column is its own top-level key, as in an update row. The
	// history read masks by key, so a nested value would pass the mask.
	before := map[string]any{auditKeyMergedFrom: nil}
	after := map[string]any{
		auditKeyMergedFrom: sourceID.UUID, auditKeyRelinkedEmails: counts.Emails, auditKeyRelinkedPhones: counts.Phones,
		auditKeyRelinkedRelationships: counts.Relationships, auditKeyRelinkedActivityLinks: counts.ActivityLinks,
	}
	maps.Copy(before, p.Before())
	maps.Copy(after, p.After())
	if _, err := storekit.Audit(ctx, tx, "merge", contactEntity, targetID.UUID, before, after); err != nil {
		return fmt.Errorf("audit contact merge on the survivor: %w", err)
	}
	return nil
}

// carryContactSatellitesTx moves what other modules keep about the merged-away
// contact onto the survivor. None of it is a relink of this module's own rows.
func (s *Store) carryContactSatellitesTx(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	from, to := commsauthz.ContactStopSubject(sourceID), commsauthz.ContactStopSubject(targetID)
	// In this order. The stops come across first and are not a relink (see
	// stopcarry.go). Without them marketing resumed against somebody who had
	// refused it. A rep's standing override follows for the same reason
	// (overridecarry.go). Then come the links, bases and asks other modules
	// keep about them (satellitecarry.go).
	steps := []struct {
		what string
		run  func() error
	}{
		{"stops", func() error { return s.carryStopsTx(ctx, tx, from, to) }},
		{"overrides", func() error { return s.carryOverridesTx(ctx, tx, from, to) }},
		{"consent links", func() error { return s.carryConsentSatellitesTx(ctx, tx, from, to) }},
		{"introduction asks", func() error { return s.carryIntrosTx(ctx, tx, sourceID, targetID) }},
		{"grants", func() error { return s.carryGrantsTx(ctx, tx, contactEntity, sourceID.UUID, targetID.UUID) }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return fmt.Errorf("carry the merged-away contact's %s: %w", step.what, err)
		}
	}
	return nil
}

// contactMergedPayload builds the contact.merged wire payload from
// MergeContact's resolved ids and relink tally — the ONE place that maps
// the local relinkCounts shape onto the published schema, so a future
// field rename shows up here rather than at an independently-drifting
// map literal.
func contactMergedPayload(sourceID, targetID ids.ContactID, counts relinkCounts) crmcontracts.PublicEventContactMerged {
	return crmcontracts.PublicEventContactMerged{
		MergedFromId: openapi_types.UUID(sourceID.UUID),
		MergedIntoId: openapi_types.UUID(targetID.UUID),
		Relinked: crmcontracts.PublicEventContactMergedRelinkCounts{
			Emails:        counts.Emails,
			Phones:        counts.Phones,
			Relationships: counts.Relationships,
			ActivityLinks: counts.ActivityLinks,
		},
	}
}

// buildSurvivorshipPatch folds A's values onto B fill-only: B never loses
// what it already holds; only its empty fields take A's non-empty values.
func buildSurvivorshipPatch(target, source crmcontracts.Contact) *storekit.Patch {
	p := storekit.NewPatch()
	fillString(p, columnFirstName, target.FirstName, source.FirstName)
	fillString(p, columnLastName, target.LastName, source.LastName)
	fillString(p, "title", target.Title, source.Title)
	if target.ConvertedFromLeadId == nil && source.ConvertedFromLeadId != nil {
		p.Set("converted_from_lead_id", nil, ids.UUID(*source.ConvertedFromLeadId))
	}
	fillMergeAddress(p, target.Address, source.Address)
	return p
}

func relinkContactEdges(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) (int64, error) {
	if err := relinkEmploymentSupport(ctx, tx, sourceID.UUID, targetID.UUID, true); err != nil {
		return 0, err
	}
	moved, err := relinkWorksWithEdges(ctx, tx, sourceID, targetID)
	if err != nil {
		return 0, err
	}
	// works_with is handled above and excluded here: its duplicate is a PAIR
	// duplicate, not a (kind, company, deal) one, and this predicate would archive
	// a source edge because the target pairs with ANYBODY.
	// The role counts for the kinds keyed on it — see roleKeyedDuplicateSQL.
	if err := foldFirstObservations(ctx, tx, contactFK, sourceID.UUID, targetID.UUID); err != nil {
		return 0, err
	}
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, archiveStateKey: time.Now().UTC()}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at = @archived`, mergeContactEdgeSource+` AND `+mergeContactEdgeDuplicate, args); err != nil {
		return 0, err
	}
	relinked, err := mergeEdgeWrite(ctx, tx, `contact_id = @target, is_current_primary = `+mergeContactEdgePrimary, mergeContactEdgeSource, args)
	return moved + relinked, err
}

// relinkWorksWithEdges re-homes the merged contact's works_with pairs, on
// WHICHEVER column the source sits in. A pair whose other end IS the target
// would relink into a self-pair, and one the target already holds — in either
// orientation — would double it; both are archived instead, because the
// surviving record already states the fact.
func relinkWorksWithEdges(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) (int64, error) {
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, archiveStateKey: time.Now().UTC()}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at = @archived`, mergeWorksWithSource+` AND `+mergeWorksWithDuplicate, args); err != nil {
		return 0, err
	}
	return mergeEdgeWrite(ctx, tx, `
 contact_id = CASE WHEN a.contact_id = @source THEN @target::uuid ELSE a.contact_id END,
 counterparty_contact_id = CASE WHEN a.counterparty_contact_id = @source THEN @target::uuid ELSE a.counterparty_contact_id END`,
		mergeWorksWithSource, args)
}

// relinkLinkRows re-homes the pure link tables (activity_link,
// list_member, taggable): a link the survivor already holds stands, and the
// retired copy is dropped. activity_link has no provenance; the list note is
// kept, and the earlier adder or tagger of a duplicate is not.
func relinkLinkRows(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) (int64, error) {
	column, source, duplicate, err := mergeActivityPredicates(entityType)
	if err != nil {
		return 0, err
	}
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, recordKindParam: entityType}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_link a
 WHERE `+source+` AND `+duplicate, args); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `UPDATE activity_link a SET `+column+` = @target WHERE `+source, args)
	if err != nil {
		return 0, err
	}
	relinked := tag.RowsAffected()

	// Spelled out per table: the company-merge coverage census reads these
	// statements, and a table name assembled at runtime is invisible to it.
	for _, stmt := range []string{
		`UPDATE list_member b SET note = a.note FROM list_member a
 WHERE ` + mergeMembershipSource + ` AND ` + mergeMembershipTarget + ` AND b.list_id = a.list_id AND ` + mergeListNoteFill,
		`DELETE FROM list_member a
 WHERE ` + mergeMembershipSource + ` AND ` + mergeListDuplicate,
		`UPDATE list_member a SET entity_id = @target WHERE ` + mergeMembershipSource,
		`DELETE FROM taggable a
 WHERE ` + mergeMembershipSource + ` AND ` + mergeTagDuplicate,
		`UPDATE taggable a SET entity_id = @target WHERE ` + mergeMembershipSource,
	} {
		if _, err := tx.Exec(ctx, stmt, args); err != nil {
			return 0, err
		}
	}
	return relinked, nil
}

// mergeConsent carries the merged-away contact's consent onto the survivor
// (consentcarry.go). The contact merge re-homes the proof rows with the state:
// the delivery gate reads the double-opt-in proof off the live record.
func mergeConsent(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return carryConsent(ctx, tx, consentCarryContactMerge, sourceID.UUID, targetID.UUID, by)
}

// archiveMergedAway retires the source row: archived + the redirect
// pointer, in one statement so a concurrent merge of the same source
// loses the race instead of double-writing.
func archiveMergedAway(ctx context.Context, tx pgx.Tx, table string, sourceID, targetID ids.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE `+table+` SET archived_at = $3, merged_into_id = $2
		 WHERE id = $1 AND archived_at IS NULL`,
		sourceID, targetID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrConflict
	}
	return nil
}

// fillString sets a nullable text column from the source only when the
// survivor has none (fill-only survivorship).
func fillString(p *storekit.Patch, column string, target, source *string) {
	if target == nil && source != nil {
		p.Set(column, nil, *source)
	}
}
