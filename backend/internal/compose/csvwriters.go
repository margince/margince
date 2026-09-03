// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// csvWriters implements migration.Writers for a delimited upload.
//
// It is NOT the flip's writers with a different source, and the difference is
// the whole reason this type exists. flipWriters answers "already landed" with
// Unchanged and writes nothing, which is correct there because its source is a
// FROZEN snapshot — a re-imported row cannot carry different values than the
// row that already landed. **An uploaded file is editable.** The customer
// fixes a column and uploads the corrected file, and that same shortcut would
// report "unchanged" and write nothing, silently. So a match here compares the
// mapped fields and updates the ones that differ.
type csvWriters struct {
	pool       *pgxpool.Pool
	people     *people.Store
	identities *migration.RunStore
	runID      migration.RunID
	object     string
	// onDuplicate is the run's answer for a row naming a record the estate
	// already holds: "" or "create" lands it and files a review pair, "skip"
	// leaves the incumbent alone. Read from the stored mapping, so the commit
	// honours whatever the dry run reported on.
	onDuplicate string

	// contextTag files every record this run CREATES under one word, so a batch
	// stays findable as a batch. Zero when the run named none. Creates only: a
	// row that UPDATES a record the estate already held leaves its tags alone,
	// because the run did not put that record in the estate.
	contextTag ids.TagID
	// tags applies it, in the same transaction as the create — a crash between
	// the two would leave a row in the estate that its own batch cannot find.
	tags *collections.Store

	// nativeIDs caches external key → native id within one run. A resumed run
	// rebuilds it lazily through lookup, which falls back to the engine-owned
	// identity map.
	nativeIDs map[string]ids.UUID
	// employers is the normalized company-name index, built once per run by
	// employerIndex and nil until the first row needs it.
	employers map[string]employerCandidate
	// updated counts the rows this run rewrote. The engine's EnsureResult has
	// no "updated" member — the frozen-source model it was built for had no
	// such outcome — so the count rides here and the report reads it.
	updated int
}

var _ migration.Writers = (*csvWriters)(nil)

// newCSVWriters takes the run's mapping by pointer because a stored run may
// carry none: the object and the duplicate policy then fall back to empty,
// which is what every caller before this did by passing them separately.
func newCSVWriters(db *database.DB, runID migration.RunID, mapping *migration.RunMapping) *csvWriters {
	settled := migration.RunMapping{}
	if mapping != nil {
		settled = *mapping
	}
	return &csvWriters{
		pool:        db.Pool(),
		people:      people.NewStore(db),
		tags:        collections.NewStore(db),
		identities:  migration.NewRunStore(db),
		runID:       runID,
		object:      settled.Object,
		onDuplicate: settled.OnDuplicate,
		// An unparseable id files nothing rather than failing the run. The wire
		// refused the shapes a client can send, so what reaches here is a stored
		// mapping — and a run that already passed its dry run must not die at
		// commit over a word.
		contextTag: parseContextTag(settled.ContextTag),
		nativeIDs:  map[string]ids.UUID{},
	}
}

// csvSourceSystem namespaces the source_system imported rows carry. The
// reserved prefix is refused at the wire mappers, so a client cannot pre-plant
// a row under a guessed import id and have the store hand it back as already
// existing; the engine-owned identity map remains the authority for "already
// imported".
func csvSourceSystem() string {
	return provenance.ReservedSourceSystemPrefix + migration.ConnectorCSV
}

// provenanceOf is the imported row's source stamp: <source>:<object>:<id>, the
// UC-E11-03 convention every importer writes.
func (w *csvWriters) provenanceOf(externalID string) string {
	return fmt.Sprintf("%s%s:%s:%s", provenance.ReservedSourceSystemPrefix, migration.ConnectorCSV, w.object, externalID)
}

// Updated reports how many rows this run rewrote, for the run report.
func (w *csvWriters) Updated() int { return w.updated }

// Exists answers whether this external id already landed, through the
// engine-owned identity map — never by reading a row's own provenance, which
// outside the reserved namespace is client-writable.
func (w *csvWriters) Exists(ctx context.Context, object, externalID string) (bool, error) {
	_, found, err := w.lookup(ctx, object, externalID)
	return found, err
}

// ReconcileIdentities has nothing to repair: every landing below commits the
// record and its identity row in ONE transaction, so a crash leaves neither
// rather than a record the resume cannot name. The seam documents exactly this
// answer for a writer that lands both together.
func (w *csvWriters) ReconcileIdentities(context.Context) error { return nil }

// Associate applies the one edge a delimited file can carry — a person's
// employer, named by a company column — and discloses anything else.
//
// The default arm is kept rather than replaced. An edge shape this writer does
// not understand still has to be reported as unapplied: swallowing it as applied
// would report work that never happened, which is the reason this method existed
// at all before there was an edge to write.
func (w *csvWriters) Associate(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	if a.FromType == migration.ObjectPerson && a.ToType == migration.AssocTargetOrganizationName {
		return w.linkEmployer(ctx, a)
	}
	return migration.AssocResult{
		Applied: false,
		Reason:  fmt.Sprintf("a delimited import carries no %s→%s edge; it was not applied", a.FromType, a.ToType),
	}, nil
}

func (w *csvWriters) lookup(ctx context.Context, object, externalID string) (ids.UUID, bool, error) {
	if object != w.object {
		return ids.UUID{}, false, fmt.Errorf("import: this run carries %q, not %q", w.object, object)
	}
	if id, ok := w.nativeIDs[externalID]; ok {
		return id, true, nil
	}
	id, found, err := w.identities.LookupIdentity(ctx, csvSourceSystem(), object, externalID)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if found {
		w.nativeIDs[externalID] = id
	}
	return id, found, nil
}

// Ensure lands one row: created the first time, updated when the file has
// since changed, unchanged when it has not.
func (w *csvWriters) Ensure(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	// The same refusal the dry run reported, so the two cannot disagree about
	// this row. Without it the preview would disclose a skip and the commit
	// would then fail on the store's own constraint instead — a different
	// answer to the question a human already approved.
	if reason := unwritableReason(object, textFields(row.Fields)); reason != "" {
		return migration.EnsureResult{Skipped: true, SkipReason: reason}, nil
	}
	// A row that NAMES its record wins over every other way of finding one. The
	// file said which company this is; nothing the importer could infer beats
	// that, and inferring anyway is what made name-based updating unsafe.
	target := w.targetIDOf(ctx, row)
	if target.named {
		if target.reason != "" {
			return migration.EnsureResult{Skipped: true, SkipReason: target.reason}, nil
		}
		return w.reconcile(ctx, target.id, row)
	}
	id, found, err := w.lookup(ctx, object, row.ExternalID)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if found {
		return w.reconcile(ctx, id, row)
	}
	// Not a row this importer has landed before — but the estate may hold the
	// record anyway, and the run says what to do about that. The check runs on
	// the commit as well as the dry run so the two cannot disagree.
	//
	// It runs on BOTH branches of that decision, and it did not always. Asking
	// only when the run said "skip" made a duplicate that CREATES invisible to
	// the commit, so a finished report could only ever repeat the prediction —
	// and the estate moves between the preview and the approval, which is the
	// whole reason a duplicate count is worth reading. The cost is one
	// dedupe query per row this importer has not landed before, which is what
	// the preview already spends on the same rows.
	collides, err := w.collidesWithExisting(ctx, row)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if collides && w.onDuplicate == string(crmcontracts.Skip) {
		// discloseOnly, so a collision this caller may not see is answered as no
		// collision — and the row CREATES.
		//
		// Skipping it instead is the intuitive choice and it leaks. An opaque
		// reason hides WHICH company was hit; the outcome still says one was.
		// "Your row was not created" is an answer to "is this company in your
		// CRM", and a finished run's report is readable on the import_run grant,
		// so a caller could probe a colleague's owner-private estate one CSV row
		// at a time. Wording cannot fix that — only the outcome can.
		//
		// What creating costs is a twin of a record the caller cannot see, which
		// the dedupe review queue picks up like any other and a merge resolves.
		// What skipping costs is a disclosure that no merge undoes. It also keeps
		// the preview and the commit answering alike, since the preview is
		// disclosure-filtered for the same reason.
		_, skipReason := collisionWordingFor(object)
		return migration.EnsureResult{Skipped: true, SkipReason: skipReason, Duplicate: true}, nil
	}
	created, err := w.create(ctx, object, row)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	// The flag rides the create rather than replacing it: a duplicate the run
	// asked to keep IS a created row, and the review queue picks up the pair.
	created.Duplicate = collides
	return created, nil
}

// create lands the row as a new record of its class.
func (w *csvWriters) create(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	switch object {
	case migration.ObjectLead:
		return w.createLead(ctx, row)
	case migration.ObjectOrganization:
		return w.createOrganization(ctx, row)
	case migration.ObjectPerson:
		return w.createPerson(ctx, row)
	default:
		return migration.EnsureResult{}, fmt.Errorf("import: %q is not an importable object", object)
	}
}

// reconcile brings an already-landed record up to what the file now says.
func (w *csvWriters) reconcile(ctx context.Context, id ids.UUID, row migration.Row) (migration.EnsureResult, error) {
	current, err := w.read(ctx, id)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	changed, err := changedFields(current, textFields(row.Fields))
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if len(changed) == 0 {
		// Counted as neither a create nor an update: reporting work that never
		// happened would inflate both the disposition table and the audit log.
		return migration.EnsureResult{Unchanged: true}, nil
	}
	if err := w.apply(ctx, id, changed, current, w.provenanceOf(row.ExternalID)); err != nil {
		// A corrected file moving an address onto a person who is not its owner
		// is one bad row, not a failed run: skip it with the reason and let the
		// rest of the file land. Unhandled, this error aborts the whole
		// migration, which is the wrong answer to a typo in a spreadsheet.
		var dup *people.DuplicateEmailError
		if errors.As(err, &dup) {
			return migration.EnsureResult{Skipped: true, SkipReason: skipReasonDuplicateEmail}, nil
		}
		var takenDomain *people.DuplicateDomainError
		if errors.As(err, &takenDomain) {
			// The company half of the same case: a corrected file moving a domain
			// onto a company that is not its owner.
			return migration.EnsureResult{Skipped: true, SkipReason: domainClaimedReason}, nil
		}
		return migration.EnsureResult{}, err
	}
	w.updated++
	return migration.EnsureResult{Disclosure: fmt.Sprintf("row %s: %d field(s) rewritten from the file", row.ExternalID, len(changed))}, nil
}

// read answers the stored record as its own JSON — the surface changedFields
// compares against, and the same shape the contract serves for it.
func (w *csvWriters) read(ctx context.Context, id ids.UUID) ([]byte, error) {
	switch w.object {
	case migration.ObjectLead:
		lead, err := w.people.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(lead)
	case migration.ObjectOrganization:
		org, err := w.people.GetOrganization(ctx, ids.From[ids.OrganizationKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(org)
	case migration.ObjectPerson:
		person, err := w.people.GetPerson(ctx, ids.From[ids.PersonKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(person)
	default:
		return nil, fmt.Errorf("import: %q is not an importable object", w.object)
	}
}

// encodeRecord renders one stored record as JSON. Generic so no wire type
// is widened to an empty interface on the way through.
func encodeRecord[T crmcontracts.Lead | crmcontracts.Organization | crmcontracts.Person](record T) ([]byte, error) {
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("import: reading the stored record: %w", err)
	}
	return encoded, nil
}

// apply writes the changed fields. `current` is the stored record's JSON, which
// the organization and person paths need because an address patch is
// all-or-nothing: the store assigns all six address columns whenever an Address
// is given, so a file carrying only a City would blank the street, postal code
// and country a human entered. The mapped components are merged onto what is
// stored, and only then sent.
//
// The person path carries a second version of the same hazard. Its emails are
// child rows and the patch REPLACES them, so a file naming one address would
// archive every other address a human added by hand. The same rule applies: a
// file whose columns are whatever the customer exported may not delete what it
// never mentioned.
func (w *csvWriters) apply(ctx context.Context, id ids.UUID, changed map[string]string, current []byte, source string) error {
	// The update half of the same rule — see land above.
	ctx = people.WithBulkWrite(ctx)
	switch w.object {
	case migration.ObjectLead:
		_, err := w.people.UpdateLead(ctx, ids.From[ids.LeadKind](id), leadUpdateFrom(changed))
		return err
	case migration.ObjectOrganization:
		in := organizationUpdateFrom(changed)
		merged, given, err := addressMergedOnto(current, in.Address)
		if err != nil {
			return err
		}
		if given {
			in.Address = merged
		}
		// The same rule for domains, which the store also replaces wholesale: a
		// file naming one must not archive the others.
		mergedDomains, hasDomain, err := domainsMergedOnto(current, orgDomainsFrom(changed))
		if err != nil {
			return err
		}
		if hasDomain {
			in.Domains = &mergedDomains
		}
		_, err = w.people.UpdateOrganization(ctx, ids.From[ids.OrganizationKind](id), in)
		return err
	case migration.ObjectPerson:
		in := personUpdateFrom(changed)
		// Emails added by an update carry the same provenance the create path
		// stamps; without it person_email.source lands empty.
		in.Source = source
		merged, given, err := addressMergedOnto(current, in.Address)
		if err != nil {
			return err
		}
		if given {
			in.Address = merged
		}
		mergedEmails, given, err := emailsMergedOnto(current, in.Emails)
		if err != nil {
			return err
		}
		if given {
			in.Emails = mergedEmails
		}
		_, err = w.people.UpdatePerson(ctx, ids.From[ids.PersonKind](id), in)
		return err
	default:
		return fmt.Errorf("import: %q is not an importable object", w.object)
	}
}

// errImportReplayed aborts a landing that wrote nothing: the store answered
// with a record that already existed under its natural key. Rolling back keeps
// the identity row out of the map — adopting a record this run did not create
// would make the next attempt resolve it as already-imported, turning a
// one-shot disclosure into none.
var errImportReplayed = errors.New("import: the record replayed under its natural key")

func (w *csvWriters) createLead(ctx context.Context, row migration.Row) (migration.EnsureResult, error) {
	in := leadCreateFrom(textFields(row.Fields), csvSourceSystem(), row.ExternalID, w.provenanceOf(row.ExternalID))
	err := w.land(ctx, row.ExternalID, func(tx pgx.Tx) (ids.UUID, error) {
		lead, created, err := w.people.CreateLeadTx(ctx, tx, in)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating lead %s: %w", row.ExternalID, err)
		}
		if !created {
			return ids.UUID{}, errImportReplayed
		}
		return ids.UUID(lead.Id), nil
	})
	if errors.Is(err, errImportReplayed) {
		return migration.EnsureResult{Skipped: true, SkipReason: skipReasonNaturalKeyTaken}, nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

func (w *csvWriters) createOrganization(ctx context.Context, row migration.Row) (migration.EnsureResult, error) {
	in := organizationCreateFrom(textFields(row.Fields), w.provenanceOf(row.ExternalID))
	if in.DisplayName == "" {
		return migration.EnsureResult{Skipped: true, SkipReason: "the mapped display_name is empty, so the row names no company"}, nil
	}
	err := w.land(ctx, row.ExternalID, func(tx pgx.Tx) (ids.UUID, error) {
		org, err := w.people.CreateOrganizationTx(ctx, tx, in)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating organization %s: %w", row.ExternalID, err)
		}
		return ids.UUID(org.Id), nil
	})
	var dup *people.DuplicateDomainError
	if errors.As(err, &dup) {
		// A domain names ONE company across the estate, so a row claiming one
		// another company already holds is refused by the store — the same shape
		// a person's claimed email has. One bad row is a skip with a reason, not
		// a failed run: the rest of the file still lands.
		return migration.EnsureResult{Skipped: true, SkipReason: domainClaimedReason}, nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// domainClaimedReason is what the report says for such a row. It names the
// row's own value and nothing about the incumbent — not which company holds the
// domain, nor whether the caller could have seen it.
const domainClaimedReason = "this domain is already held by another company in the CRM, " +
	"so the row cannot create a second company under it"

// land commits one native record and its identity-map row in ONE transaction,
// then caches the binding — after the commit, never inside it: an entry for a
// landing that then rolled back would make this run's later pages resolve an id
// that does not exist, and lookup answers from the cache before it asks the map.
func (w *csvWriters) land(ctx context.Context, externalID string, create func(tx pgx.Tx) (ids.UUID, error)) error {
	// Every row this importer writes is one of many under a single approval, so
	// it carries the marker that says so. What it buys today is one thing: a
	// standing domain refusal is NOT lifted by a spreadsheet, because approving
	// a file is not the same act as a person putting one domain on one company.
	ctx = people.WithBulkWrite(ctx)
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		if id, err = create(tx); err != nil {
			return err
		}
		if err := w.identities.RecordIdentityTx(ctx, tx, w.runID, csvSourceSystem(), w.object, externalID, id); err != nil {
			return err
		}
		return w.fileUnderContextTag(ctx, tx, id)
	}); err != nil {
		return err
	}
	w.nativeIDs[externalID] = id
	return nil
}
