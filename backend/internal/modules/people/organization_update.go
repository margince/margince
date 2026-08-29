// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Editing an organization: what the edit is allowed to say, the locks it owes
// before it reads the row it will judge itself against, and the record it
// leaves once the write has landed.

package people

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// UpdateOrganizationInput is one edit to one organization. Every field is
// optional and a nil one is NOT SUPPLIED rather than empty — which is why the
// fields that can legitimately be emptied are named in Clear instead.
type UpdateOrganizationInput struct {
	// Clear names the wire fields to set to NULL. A JSON null cannot say so —
	// it decodes to a nil pointer and reads as "not supplied" — so the
	// reversal path names them here instead.
	Clear []string
	// Trail names what the audit trail calls this write; zero is an update.
	Trail       storekit.AuditTrail
	DisplayName *string
	LegalName   *string
	// Description, when non-nil, sets or (when empty) clears the one-line
	// summary the company page shows under the title. nil leaves it untouched.
	Description *string
	Industry    *string
	SizeBand    *string
	OwnerID     *ids.UserID
	ParentOrgID *ids.OrganizationID
	Address     *crmcontracts.Address
	IfVersion   *int64
	// LinkedInURL, when non-nil, sets or (when empty) clears the canonical
	// LinkedIn company URL (PO-DDL-N-2). nil leaves it untouched.
	LinkedInURL *string
	// Domains, when non-nil, is the desired live domain set (replace-set:
	// add missing, archive removed, flip is_primary). nil leaves domains
	// untouched; an empty slice clears them.
	Domains *[]OrgDomainInput
	// Lifecycle, when non-nil, moves where the account stands with us
	// (ADR-0079/A124). nil leaves it untouched.
	Lifecycle *string
	// RelationshipTypes, when non-nil, is the desired live type set — the same
	// replace-set shape as Domains. nil leaves them untouched; an empty slice
	// clears them, except that 'partner' cannot be dropped while the partner
	// extension row lives.
	RelationshipTypes *[]string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

// UpdateOrganization applies one edit to one organization, in a single
// transaction: the row it reads, the row it writes and the record of what it
// wrote either all land or none of them do.
func (s *Store) UpdateOrganization(ctx context.Context, id ids.OrganizationID, in UpdateOrganizationInput) (crmcontracts.Organization, error) {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return crmcontracts.Organization{}, err
	}
	active, err := s.activeColumns(ctx, "organization")
	if err != nil {
		return crmcontracts.Organization{}, err
	}
	var out crmcontracts.Organization
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.updateOrganizationInTx(ctx, tx, id, in, active)
		return err
	})
	return out, err
}

// updateOrganizationInTx is the edit itself: the locks it owes, the patch it
// stages, the write, and the record of what the write did. It answers with the
// row as it now stands.
func (s *Store) updateOrganizationInTx(
	ctx context.Context, tx pgx.Tx, id ids.OrganizationID, in UpdateOrganizationInput, active []fieldcatalog.Column,
) (crmcontracts.Organization, error) {
	if err := auth.EnsureWritable(ctx, tx, "organization", id.UUID); err != nil {
		return crmcontracts.Organization{}, err
	}
	if err := lockOrgNameWritesForEdit(ctx, tx, in); err != nil {
		return crmcontracts.Organization{}, err
	}
	current, err := readOrganization(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("read organization before update: %w", err)
	}
	p, err := buildOrganizationPatch(ctx, tx, current, in)
	if err != nil {
		return crmcontracts.Organization{}, err
	}
	storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)

	by, err := stageOrgReplaceSets(ctx, tx, id, current, in, p)
	if err != nil {
		return crmcontracts.Organization{}, err
	}
	if p.Empty() {
		return current, nil
	}
	if err := p.ApplyGuarded(ctx, tx, "organization", id.UUID, in.IfVersion); err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("apply organization patch: %w", err)
	}
	if err := s.relocateIfAddressMoved(ctx, tx, id, p); err != nil {
		return crmcontracts.Organization{}, err
	}
	// MOVED, not merely set. A human editing the display name is the top of the
	// name-source lattice (ADR-0072/A118): stamp 'human' so no automated source
	// ever overwrites it. Re-sending the same value is not a re-authoring, and
	// reading After() made it one — an agent round-tripping a record it had just
	// read froze a provisional domain-derived name for good, with nothing in the
	// record saying a person had never chosen it.
	//
	// Spelled HERE rather than beside the description stamp it belongs with,
	// because it is a by-id UPDATE of a shareable record and its guard is the
	// row this function's ApplyGuarded just locked. Moved into a helper, the
	// write leaves its guard and its write-authority probe behind, and the two
	// gates that check for them are right to say so.
	if _, changed := p.Moved()[fieldDisplayName]; changed {
		if _, err := tx.Exec(ctx, `UPDATE organization SET name_source = 'human' WHERE id = $1`, id); err != nil {
			return crmcontracts.Organization{}, fmt.Errorf("stamp organization name provenance: %w", err)
		}
	}
	if err := recordOrganizationUpdate(ctx, tx, id, p, in, by); err != nil {
		return crmcontracts.Organization{}, err
	}
	out, err := readOrganization(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("read updated organization: %w", err)
	}
	return out, nil
}

// recordOrganizationUpdate is everything the write OWES once it has landed: the
// provenance a person's edit claims, the duplicate the new name may have
// revealed, the sets that ride the row's own version bump, and the audit and
// event that say what moved. All of it in the writing transaction, so a record
// of a write that did not happen cannot exist.
func recordOrganizationUpdate(
	ctx context.Context, tx pgx.Tx, id ids.OrganizationID,
	p *storekit.Patch, in UpdateOrganizationInput, by string,
) error {
	before, after := p.Before(), p.After()
	if err := stampEditedDescriptionAuthor(ctx, tx, id, p); err != nil {
		return err
	}
	if err := recheckRenamedOrganization(ctx, tx, id, after); err != nil {
		return err
	}
	if err := reconcileOrgReplaceSets(ctx, tx, id, by, in, before, after); err != nil {
		return err
	}
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "organization", id.UUID, before, after)
	if err != nil {
		return fmt.Errorf("audit organization update: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
		crmcontracts.PublicEventOrganizationUpdated{ChangedFields: after}); err != nil {
		return fmt.Errorf("emit organization.updated: %w", err)
	}
	return nil
}

// lockOrgNameWritesForEdit takes the name lock when — and only when — this edit
// writes a name.
//
// Only a rename needs it, and only a rename should pay for it: the key is
// workspace-wide, so taking it for an owner change would serialize every
// organization write behind an edit that cannot create a duplicate. It is taken
// ahead of the patch's row lock, per the ordering rule on lockOrgNameWrites.
//
// And ahead of the READ, because the read is what the rename is judged against.
// Taken after it, a concurrent rename landing in between left `current` holding
// a name nobody stored any more — so this write overwrote that rename while its
// before-image said the name had not moved, and the provenance stamp was
// skipped for an edit that really did change the name.
func lockOrgNameWritesForEdit(ctx context.Context, tx pgx.Tx, in UpdateOrganizationInput) error {
	if !renamesAnOrganization(in) {
		return nil
	}
	return lockOrgNameWrites(ctx, tx)
}

// relocateIfAddressMoved queues the coordinate lookup a moved address needs.
//
// INVALIDATION IS THE TRIGGER'S JOB, not this call's — the schema marks the
// coordinates stale on any address column that actually changed, so no writer
// has to remember and none can forget. What is left here is the enqueue, and it
// is keyed off what the patch actually MOVED rather than off the request
// carrying an address field: re-submitting a form with an unchanged address
// would otherwise spend a lookup, and every lookup is fifteen seconds of a rate
// the whole installation shares.
func (s *Store) relocateIfAddressMoved(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, p *storekit.Patch) error {
	if !movedAddress(p.After()) {
		return nil
	}
	if err := s.enqueueGeocode(ctx, tx, id); err != nil {
		return fmt.Errorf("re-locating a moved company: %w", err)
	}
	return nil
}

// stampEditedDescriptionAuthor records that a PERSON wrote the description this
// edit moved. A site read may replace a description no person authored, so an
// edited one has to say in field_provenance that somebody typed it — otherwise
// the next crawl reads "no row" as "no owner" and takes the sentence they wrote.
//
// The test is whether the VALUE moved, not whether the field was sent.
// storekit.Patch records an assignment unconditionally, so `after` holds every
// key the request named — and an agent re-sending a human's description
// unchanged would otherwise write an agent: row on top of the human's and hand
// the column to the next crawl. The name stamp asks the same question through
// Patch.Moved. This one cannot: the description's two images hold different
// TYPES by construction — *string as read from the row, string as the request
// supplied it — and Moved counts a type change as a move, so it would call
// every send a rewrite. describedDifferently compares the values themselves.
func stampEditedDescriptionAuthor(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, p *storekit.Patch) error {
	if !describedDifferently(p.Before(), p.After()) {
		return nil
	}
	// Not the replace-set principal: that one is empty unless the edit also
	// carried a replace-set, and a description edit usually carries neither,
	// which would stamp the field to nobody.
	editor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return stampDescriptionAuthor(ctx, tx, id, editor)
}

// stageOrgReplaceSets validates the domain and relationship-type replace-sets
// up front, so a bad request fails before the version bump, and folds the
// updated_at bump each of them rides into the patch: the reconciles happen
// against the org row's own guarded write, so If-Match still guards them and
// the audit row records the transition — the same shape as UpdatePerson/social.
// It answers with the captured-by principal those reconciles stamp, empty when
// the edit touches neither set.
func stageOrgReplaceSets(ctx context.Context, tx pgx.Tx, id ids.OrganizationID,
	current crmcontracts.Organization, in UpdateOrganizationInput, p *storekit.Patch,
) (string, error) {
	if in.RelationshipTypes == nil && in.Domains == nil {
		return "", nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return "", err
	}
	if in.RelationshipTypes != nil {
		deduped, err := dedupeRelationshipTypes(*in.RelationshipTypes)
		if err != nil {
			return "", err
		}
		*in.RelationshipTypes = deduped
	}
	if in.Domains != nil {
		if err := parseOrgDomains(*in.Domains); err != nil {
			return "", err
		}
		// Collapse hosts that normalize to the same domain so the probe,
		// reconcile, and audit-after all see one row per host.
		*in.Domains = dedupeDomains(*in.Domains)
		if err := ensureOrgDomainsUnclaimedExcept(ctx, tx, id, *in.Domains); err != nil {
			return "", err
		}
	}
	// A replace-set changes no column on the row itself, so this bump is what
	// makes the patch non-empty and carries the version guard. Once, after both
	// branches: a request naming both sets is still one write at one instant, and
	// which branch happened to run last should not pick the timestamp.
	p.Set("updated_at", current.UpdatedAt, time.Now().UTC())
	return by, nil
}

// reconcileOrgReplaceSets lands the two replace-sets on the row the patch just
// wrote and records each transition in the audit images the caller commits.
func reconcileOrgReplaceSets(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, by string,
	in UpdateOrganizationInput, before, after map[string]any,
) error {
	if in.Domains != nil {
		domainsBefore, err := reconcileOrgDomains(ctx, tx, workspaceID(ctx), id, by, *in.Domains)
		if err != nil {
			return err
		}
		before["domains"] = domainsBefore
		after["domains"] = domainSummaries(*in.Domains)
	}
	if in.RelationshipTypes != nil {
		typesBefore, err := reconcileOrgRelationshipTypes(ctx, tx, workspaceID(ctx), id, "manual", by, *in.RelationshipTypes)
		if err != nil {
			return err
		}
		before["relationship_types"] = typesBefore
		after["relationship_types"] = *in.RelationshipTypes
	}
	return nil
}

// recheckRenamedOrganization asks whether an edited organization now resembles
// another one, given the patch's applied delta.
//
// Either name axis can reveal it alone: two records of one company converging
// on the same registered name is exactly the shape that doubled a company in a
// live workspace, and a legal-name-only edit changes no display name at all.
// The edit stands regardless — this only files a pair for the review queue.
func recheckRenamedOrganization(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, after map[string]any) error {
	renamed := false
	for column := range orgNameColumns {
		if _, wrote := after[column]; wrote {
			renamed = true
		}
	}
	if !renamed {
		return nil
	}
	editor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return recheckOrgNameForDuplicates(ctx, tx, id, editor)
}
