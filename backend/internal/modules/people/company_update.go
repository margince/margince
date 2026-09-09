// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Editing a company: what the edit is allowed to say, the locks it owes
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

// UpdateCompanyInput is one edit to one company. Every field is
// optional and a nil one is NOT SUPPLIED rather than empty — which is why the
// fields that can legitimately be emptied are named in Clear instead.
type UpdateCompanyInput struct {
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
	ParentCompanyID *ids.CompanyID
	Address     *crmcontracts.Address
	IfVersion   *int64
	// LinkedInURL, when non-nil, sets or (when empty) clears the canonical
	// LinkedIn company URL (PO-DDL-N-2). nil leaves it untouched.
	LinkedInURL *string
	// Domains, when non-nil, is the desired live domain set (replace-set:
	// add missing, archive removed, flip is_primary). nil leaves domains
	// untouched; an empty slice clears them.
	Domains *[]CompanyDomainInput
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

// UpdateCompany applies one edit to one company, in a single
// transaction: the row it reads, the row it writes and the record of what it
// wrote either all land or none of them do.
func (s *Store) UpdateCompany(ctx context.Context, id ids.CompanyID, in UpdateCompanyInput) (crmcontracts.Company, error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return crmcontracts.Company{}, err
	}
	active, err := s.activeColumns(ctx, "company")
	if err != nil {
		return crmcontracts.Company{}, err
	}
	var out crmcontracts.Company
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.updateCompanyInTx(ctx, tx, id, in, active)
		return err
	})
	return out, err
}

// updateCompanyInTx is the edit itself: the locks it owes, the patch it
// stages, the write, and the record of what the write did. It answers with the
// row as it now stands.
func (s *Store) updateCompanyInTx(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, in UpdateCompanyInput, active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	if err := auth.EnsureWritable(ctx, tx, "company", id.UUID); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := lockCompanyNameWritesForEdit(ctx, tx, in); err != nil {
		return crmcontracts.Company{}, err
	}
	current, err := readCompany(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read company before update: %w", err)
	}
	p, err := buildCompanyPatch(ctx, tx, current, in)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)

	by, err := stageCompanyReplaceSets(ctx, tx, id, current, in, p)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	if p.Empty() {
		return current, nil
	}
	if err := p.ApplyGuarded(ctx, tx, "company", id.UUID, in.IfVersion); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("apply company patch: %w", err)
	}
	if err := s.relocateIfAddressMoved(ctx, tx, id, p); err != nil {
		return crmcontracts.Company{}, err
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
		if _, err := tx.Exec(ctx, `UPDATE company SET name_source = 'human' WHERE id = $1`, id); err != nil {
			return crmcontracts.Company{}, fmt.Errorf("stamp company name provenance: %w", err)
		}
	}
	if err := recordCompanyUpdate(ctx, tx, id, p, in, by); err != nil {
		return crmcontracts.Company{}, err
	}
	out, err := readCompany(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read updated company: %w", err)
	}
	return out, nil
}

// recordCompanyUpdate is everything the write OWES once it has landed: the
// provenance a person's edit claims, the duplicate the new name may have
// revealed, the sets that ride the row's own version bump, and the audit and
// event that say what moved. All of it in the writing transaction, so a record
// of a write that did not happen cannot exist.
func recordCompanyUpdate(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID,
	p *storekit.Patch, in UpdateCompanyInput, by string,
) error {
	before, after := p.Before(), p.After()
	if err := stampEditedDescriptionAuthor(ctx, tx, id, p); err != nil {
		return err
	}
	if err := recheckRenamedCompany(ctx, tx, id, p); err != nil {
		return err
	}
	if err := reconcileCompanyReplaceSets(ctx, tx, id, by, in, before, after); err != nil {
		return err
	}
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "company", id.UUID, before, after)
	if err != nil {
		return fmt.Errorf("audit company update: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
		crmcontracts.PublicEventCompanyUpdated{ChangedFields: after}); err != nil {
		return fmt.Errorf("emit company.updated: %w", err)
	}
	return nil
}

// renamesInPatch reports whether a staged edit MOVED a name column — the pure
// half of the question above, so the precedence can be read without a database.
func renamesInPatch(p *storekit.Patch) bool {
	moved := p.Moved()
	for column := range companyNameColumns {
		if _, changed := moved[column]; changed {
			return true
		}
	}
	return false
}

// lockCompanyNameWritesForEdit takes the name lock when — and only when — this edit
// writes a name.
//
// Only a rename needs it, and only a rename should pay for it: the key is
// workspace-wide, so taking it for an owner change would serialize every
// company write behind an edit that cannot create a duplicate. It is taken
// ahead of the patch's row lock, per the ordering rule on lockCompanyNameWrites.
//
// And ahead of the READ, because the read is what the rename is judged against.
// Taken after it, a concurrent rename landing in between left `current` holding
// a name nobody stored any more — so this write overwrote that rename while its
// before-image said the name had not moved, and the provenance stamp was
// skipped for an edit that really did change the name.
func lockCompanyNameWritesForEdit(ctx context.Context, tx pgx.Tx, in UpdateCompanyInput) error {
	if !renamesAnCompany(in) {
		return nil
	}
	return lockCompanyNameWrites(ctx, tx)
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
func (s *Store) relocateIfAddressMoved(ctx context.Context, tx pgx.Tx, id ids.CompanyID, p *storekit.Patch) error {
	if !movedAddress(p.Moved()) {
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
func stampEditedDescriptionAuthor(ctx context.Context, tx pgx.Tx, id ids.CompanyID, p *storekit.Patch) error {
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

// stageCompanyReplaceSets validates the domain and relationship-type replace-sets
// up front, so a bad request fails before the version bump, and folds the
// updated_at bump each of them rides into the patch: the reconciles happen
// against the company row's own guarded write, so If-Match still guards them and
// the audit row records the transition — the same shape as UpdatePerson/social.
// It answers with the captured-by principal those reconciles stamp, empty when
// the edit touches neither set.
func stageCompanyReplaceSets(ctx context.Context, tx pgx.Tx, id ids.CompanyID,
	current crmcontracts.Company, in UpdateCompanyInput, p *storekit.Patch,
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
		if err := parseCompanyDomains(*in.Domains); err != nil {
			return "", err
		}
		// Collapse hosts that normalize to the same domain so the probe,
		// reconcile, and audit-after all see one row per host.
		*in.Domains = dedupeDomains(*in.Domains)
		if err := ensureCompanyDomainsUnclaimedExcept(ctx, tx, id, *in.Domains); err != nil {
			return "", err
		}
		// And elect the primary HERE, for the same reason and on the same
		// slice. The reconcile elects one either way, but on a copy — so an
		// election left to it lands on the row while the audit after-image,
		// built from this slice, still reports the domain as ordinary. The
		// trail would then say a company has no primary domain while the row
		// says it does, and is_primary is exactly what admits that company to
		// the website read the trail exists to explain.
		live, err := livePrimaryDomain(ctx, tx, id)
		if err != nil {
			return "", err
		}
		*in.Domains = electPrimary(*in.Domains, live)
	}
	// A replace-set changes no column on the row itself, so this bump is what
	// makes the patch non-empty and carries the version guard. Once, after both
	// branches: a request naming both sets is still one write at one instant, and
	// which branch happened to run last should not pick the timestamp.
	p.Set("updated_at", current.UpdatedAt, time.Now().UTC())
	return by, nil
}

// reconcileCompanyReplaceSets lands the two replace-sets on the row the patch just
// wrote and records each transition in the audit images the caller commits.
func reconcileCompanyReplaceSets(ctx context.Context, tx pgx.Tx, id ids.CompanyID, by string,
	in UpdateCompanyInput, before, after map[string]any,
) error {
	if in.Domains != nil {
		domainsBefore, err := reconcileCompanyDomains(ctx, tx, workspaceID(ctx), id, by, *in.Domains)
		if err != nil {
			return err
		}
		before["domains"] = domainsBefore
		after["domains"] = domainSummaries(*in.Domains)
	}
	if in.RelationshipTypes != nil {
		typesBefore, err := reconcileCompanyRelationshipTypes(ctx, tx, workspaceID(ctx), id, "manual", by, *in.RelationshipTypes)
		if err != nil {
			return err
		}
		before["relationship_types"] = typesBefore
		after["relationship_types"] = *in.RelationshipTypes
	}
	return nil
}

// recheckRenamedCompany asks whether an edited company now resembles
// another one, given the patch's applied delta.
//
// Either name axis can reveal it alone: two records of one company converging
// on the same registered name is exactly the shape that doubled a company in a
// live workspace, and a legal-name-only edit changes no display name at all.
// The edit stands regardless — this only files a pair for the review queue.
//
// MOVED, not merely assigned. storekit.Patch records an assignment for every
// field the request named, so keyed off the after-image an agent echoing a name
// it had just read spent a workspace-wide fuzzy scan — under the name lock, for
// the whole rest of the transaction — and filed a pair over an edit that
// renamed nothing.
//
// The two axes answer that question with different precision, and the weaker
// one errs the safe way. display_name's images are both plain strings, so a
// re-send of the same name is not a move. legal_name's are not: the row reads
// back as *string and the request supplies string, and a type change counts as
// a move — so an echoed legal name still spends its scan. Over-reporting costs
// a redundant pair the queue already de-duplicates; under-reporting would lose
// the rename that doubled a company, which is the failure this detector exists
// for.
func recheckRenamedCompany(ctx context.Context, tx pgx.Tx, id ids.CompanyID, p *storekit.Patch) error {
	if !renamesInPatch(p) {
		return nil
	}
	editor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return recheckCompanyNameForDuplicates(ctx, tx, id, editor)
}
