// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// csvWriters implements migration.Writers for a delimited upload.
//
// A writer whose source is FROZEN may answer "already landed" with Unchanged
// and write nothing: a re-imported row cannot carry different values than the
// row that already landed. **An uploaded file is editable**, so that shortcut
// is wrong here. The customer fixes a column and uploads the corrected file,
// and answering "unchanged" would write nothing, silently. So a match here
// compares the mapped fields and updates the ones that differ.
type csvWriters struct {
	reviewRows map[string]migration.ReviewRow
	pool       *pgxpool.Pool
	contacts   *contacts.Store
	identities *migration.RunStore
	runID      migration.RunID
	object     string
	// sourceSystem is the namespace the run files its rows under, from its
	// source name (importSourceSystem).
	sourceSystem string
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

	// bindings is the identity map of this part's source, preloaded by a dry
	// run; nil reads the store per key.
	bindings map[string]migration.Binding
	// heldEmails answers, for the page a dry run is on, which of its addresses
	// a live contact already holds; nil reads the store per row.
	heldEmails map[string]bool
	// consumerMail is the workspace's consumer-mail list, read once per run.
	consumerMail *freemail.Matcher

	// nativeIDs caches external key → native id within one run. A resumed run
	// rebuilds it lazily through lookup, which falls back to the engine-owned
	// identity map.
	nativeIDs map[string]ids.UUID
	// links finds the record a link cell names. One resolver serves every part
	// of the run, so a contact can name a company of the same upload.
	links *linkResolver
	// preset is the run's record every activity row is filed under.
	preset presetSet
	// companyRefColumn is the file's column mapped to company_ref, empty when
	// none is; onBrokenLink decides a row whose ref finds nothing.
	companyRefColumn, onBrokenLink string
	// companyNameColumn is a contact part's company_name column in a ZIP run. It
	// is empty in a single CSV, and for a lead, whose company_name is plain text.
	companyNameColumn string
	// authors caches the seat each author cell resolves to, per run.
	authors authorSeats
	// owners and tagNames resolve the owner and tag cells; the bundle reads
	// them once for every part.
	owners   *ownerResolver
	tagNames *tagResolver
	// cols is the part's custom fields, read before any row transaction opens.
	cols importColumns
	// dealStages checks a deal part's stage and close date; read once per run.
	dealStages dealStages
	// deals is the store a deal part's rows land through.
	deals *deals.Store
	// activities is the store an activity part's rows land through.
	activities *activities.Store
	// columns is the part's mapping, file header → target.
	columns map[string]string
	// now is the clock an activity's timestamp is held against; nil is the
	// system clock.
	now func() time.Time
	// zone is the record zone a day cell's date is read in, read once per run.
	zone *time.Location
	// updated counts the rows this run rewrote. The engine's EnsureResult has
	// no "updated" member — the frozen-source model it was built for had no
	// such outcome — so the count rides here and the report reads it.
	updated int
}

var _ migration.Writers = (*csvWriters)(nil)

// newCSVWriters builds the writer of one part: one object, one file.
func newCSVWriters(
	db *database.DB, runID migration.RunID, part migration.PartMapping, run runOptions, links *linkResolver,
	cols importColumns,
) *csvWriters {
	w := &csvWriters{
		pool: db.Pool(),
		// An imported contact can take a lead over, which carries the lead's
		// stops and consent links (see newCounterpartyStore).
		contacts: contacts.NewStore(db).WithFieldCatalog(customfields.NewService(db.Pool(), nil)).
			WithStopCarrier(consent.NewStore(db)).
			WithSatelliteCarriers(consent.NewStore(db), introductions.NewStore(db, time.Now)).
			WithGrantCarrier(identity.NewServiceFor(db)).
			WithInstallationRenamer(installationRenamer(db.Pool())),
		deals:            dealsStoreOver(db),
		activities:       activities.NewStore(db).WithRecordZone(installationZoneTx),
		tags:             collections.NewStore(db),
		identities:       migration.NewRunStore(db),
		runID:            runID,
		object:           part.Object,
		sourceSystem:     links.sourceSystem,
		onDuplicate:      run.OnDuplicate,
		links:            links,
		preset:           presetSet{links: run.PresetLinks},
		companyRefColumn: columnFor(part.Fields, csvTargetCompanyRef),
		onBrokenLink:     run.OnBrokenLink,
		// An unparseable id files nothing rather than failing the run. The wire
		// refused the shapes a client can send, so what reaches here is a stored
		// mapping — and a run that already passed its dry run must not die at
		// commit over a word.
		contextTag: parseContextTag(run.ContextTag),
		nativeIDs:  map[string]ids.UUID{},
		authors:    authorSeats{},
		cols:       cols,
		columns:    part.Fields,
	}
	if run.FromZip && linksEmployer(part.Object) {
		w.companyNameColumn = columnFor(part.Fields, csvEmployerName)
	}
	return w
}

// provenanceOf is the imported row's source stamp: <source>:<object>:<id>, the
// UC-E11-03 convention every importer writes.
func (w *csvWriters) provenanceOf(externalID string) string {
	return fmt.Sprintf("%s:%s:%s", w.sourceSystem, w.object, externalID)
}

// Updated reports how many rows this run rewrote, for the run report.
func (w *csvWriters) Updated() int { return w.updated }

// Exists answers whether this external id already landed, through the
// engine-owned identity map — never by reading a row's own provenance, which
// outside the reserved namespace is client-writable.
func (w *csvWriters) Exists(ctx context.Context, object, externalID string) (bool, error) {
	_, found, err := w.lookup(ctx, object, externalID)
	if errors.Is(err, errBoundRecordArchived) {
		return true, nil
	}
	return found, err
}

// ReconcileIdentities has nothing to repair: every landing below commits the
// record and its identity row in ONE transaction, so a crash leaves neither
// rather than a record the resume cannot name. The seam documents exactly this
// answer for a writer that lands both together.
func (w *csvWriters) ReconcileIdentities(context.Context) error { return nil }

// Associate applies the one edge a delimited file can carry — a contact's
// employer, named by a company column — and discloses anything else.
//
// The default arm is kept rather than replaced. An edge shape this writer does
// not understand still has to be reported as unapplied: swallowing it as applied
// would report work that never happened, which is the reason this method existed
// at all before there was an edge to write.
func (w *csvWriters) Associate(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	if a.FromType == migration.ObjectContact && a.ToType == migration.AssocTargetCompanyName {
		return w.linkEmployer(ctx, a)
	}
	if a.FromType == migration.ObjectContact && a.ToType == migration.AssocTargetCompanyRef {
		return w.linkCompanyRef(ctx, a)
	}
	return notLinkedFor(migration.Why(migration.ReasonLinkEdgeUnsupported, "from", a.FromType, "to", a.ToType)), nil
}

func (w *csvWriters) ensureOne(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	if refused := w.createRefusal(ctx, row); refused != nil {
		return skippedWithProblem(refused), nil
	}
	typed, invalid, unknownOwner := w.landingCells(row)
	if invalid != nil {
		return skippedWithProblem(invalid), nil
	}
	link, err := w.linkProblemOf(ctx, row)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if link.skip {
		return skippedWithProblem(link.problem), nil
	}
	landed, err := w.ensureRow(ctx, object, row, link.into(typed))
	if err == nil && !landed.Skipped {
		landed.Problem = cmp.Or(link.problem, unknownOwner)
	}
	return landed, err
}

// ensureRow is Ensure once the row's cells and links allow it to land.
func (w *csvWriters) ensureRow(ctx context.Context, object string, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	target := w.targetIDOf(ctx, row)
	if target.why.Code != "" {
		return skippedFor(target.why), nil
	}
	// A row that NAMES its record wins over every other way of finding one. The
	// file said which company this is; nothing the importer could infer beats
	// that, and inferring anyway is what made name-based updating unsafe.
	if target.named {
		if err := w.reviewedTarget(row.ExternalID, target.id); err != nil {
			return migration.EnsureResult{}, err
		}
		if err := w.bind(ctx, row, target.id); err != nil {
			return migration.EnsureResult{}, err
		}
		return w.reconcile(ctx, target.id, row, typed)
	}
	id, found, err := w.lookup(ctx, object, row.ExternalID)
	if errors.Is(err, errBoundRecordArchived) {
		return skippedFor(boundArchivedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if found {
		if replay, err := w.reviewedBinding(ctx, row, id); err != nil || replay {
			return migration.EnsureResult{Unchanged: replay}, err
		}
		return w.reconcile(ctx, id, row, typed)
	}
	if expected, reviewed := w.reviewRows[row.ExternalID]; reviewed && expected.TargetID != "" {
		return migration.EnsureResult{}, apperrors.ErrVersionSkew
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
	if collides && w.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
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
		_, skipWhy := collisionWordingFor(object)
		skipped := skippedFor(skipWhy)
		skipped.Duplicate = true
		return skipped, nil
	}
	created, err := w.create(ctx, object, row, typed)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	// The flag rides the create rather than replacing it: a duplicate the run
	// asked to keep IS a created row, and the review queue picks up the pair.
	created.Duplicate = collides
	return created, nil
}

// create lands the row as a new record of its class.
func (w *csvWriters) create(ctx context.Context, object string, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	switch object {
	case migration.ObjectLead:
		return w.createLead(ctx, row, typed)
	case migration.ObjectCompany:
		return w.createCompany(ctx, row, typed)
	case migration.ObjectContact:
		return w.createContact(ctx, row, typed)
	case migration.ObjectDeal:
		return w.createDeal(ctx, row, typed)
	case migration.ObjectActivity:
		return w.createActivity(ctx, row, typed)
	default:
		return migration.EnsureResult{}, fmt.Errorf("import: %q is not an importable object", object)
	}
}

// reconcile brings an already-landed record up to what the file now says.
func (w *csvWriters) reconcile(ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	switch w.object {
	case migration.ObjectDeal:
		return w.reconcileDeal(ctx, id, row, typed)
	case migration.ObjectActivity:
		return w.reconcileActivity(ctx, id, row, typed)
	}
	current, err := w.read(ctx, id)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	changed, err := changedFields(current, textFields(row.Fields), typed)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if len(changed) == 0 {
		// Counted as neither a create nor an update: reporting work that never
		// happened would inflate both the disposition table and the audit log.
		return migration.EnsureResult{Unchanged: true}, w.tagExisting(ctx, id, row)
	}
	if !auth.Allows(ctx, w.object, principal.ActionUpdate) {
		return skippedFor(cannotEditWhy(w.object)), nil
	}
	// A reviewed plan lands the row inside one shared transaction, so a refused
	// write is rolled back to here before the row is skipped.
	if err := database.WithSavepoint(ctx, func(ctx context.Context) error {
		return w.apply(ctx, id, changed, typed, current, w.provenanceOf(row.ExternalID))
	}); err != nil {
		// A corrected file moving an address onto a contact who is not its owner
		// is one bad row, not a failed run: skip it with the reason and let the
		// rest of the file land. Unhandled, this error aborts the whole
		// migration, which is the wrong answer to a typo in a spreadsheet.
		if _, ok := errors.AsType[*contacts.DuplicateEmailError](err); ok {
			return skippedFor(duplicateEmailWhy), nil
		}
		if _, ok := errors.AsType[*contacts.DuplicateDomainError](err); ok {
			// The company half of the same case: a corrected file moving a domain
			// onto a company that is not its owner.
			return skippedFor(domainClaimedWhy), nil
		}
		if _, ok := errors.AsType[*contacts.DuplicateLeadError](err); ok {
			return skippedFor(naturalKeyTakenWhy), nil
		}
		if linkedInClaimed(err) {
			return skippedFor(linkedInClaimedWhy), nil
		}
		return migration.EnsureResult{}, err
	}
	if err := w.tagExisting(ctx, id, row); err != nil {
		return migration.EnsureResult{}, err
	}
	w.updated++
	return migration.EnsureResult{Disclosure: fmt.Sprintf("row %s: %d field(s) rewritten from the file", row.ExternalID, len(changed))}, nil
}

// apply writes the changed fields. `current` is the stored record's JSON, which
// the company and contact paths need because an address patch is
// all-or-nothing: the store assigns all six address columns whenever an Address
// is given, so a file carrying only a City would blank the street, postal code
// and country a human entered. The mapped components are merged onto what is
// stored, and only then sent.
//
// The contact path carries a second version of the same hazard. Its emails are
// child rows and the patch REPLACES them, so a file naming one address would
// archive every other address a human added by hand. The same rule applies: a
// file whose columns are whatever the customer exported may not delete what it
// never mentioned.
func (w *csvWriters) apply(
	ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any, current []byte, source string,
) error {
	// The update half of the same rule — see land above.
	ctx = contacts.WithBulkWrite(ctx)
	switch w.object {
	case migration.ObjectLead:
		in := leadUpdateFrom(changed, typed)
		in.IfVersion = w.reviewedVersion(id)
		_, err := w.contacts.UpdateLead(ctx, ids.From[ids.LeadKind](id), in)
		return err
	case migration.ObjectCompany:
		in := companyUpdateFrom(changed, typed)
		in.IfVersion = w.reviewedVersion(id)
		merged, given, err := addressMergedOnto(current, in.Address)
		if err != nil {
			return err
		}
		if given {
			in.Address = merged
		}
		// The same rule for domains, which the store also replaces wholesale: a
		// file naming one must not archive the others.
		mergedDomains, hasDomain, err := domainsMergedOnto(current, companyDomainsFrom(changed))
		if err != nil {
			return err
		}
		if hasDomain {
			in.Domains = &mergedDomains
		}
		_, err = w.contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), in)
		return err
	case migration.ObjectContact:
		return w.applyContact(ctx, id, contactUpdateFrom(changed, typed), current, source)
	default:
		return fmt.Errorf("import: %q is not an importable object", w.object)
	}
}

// different input builder, a different store call, a different emptiness to
// refuse and a different sentence to refuse it with. Folding them together
// would mean a generic seam that exists to satisfy a linter and nothing else.
//
//nolint:dupl // createContact has the same SHAPE and not the same job: a
func (w *csvWriters) createCompany(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in := companyCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	if in.DisplayName == "" {
		return skippedFor(companyNameEmptyWhy), nil
	}
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		company, err := w.contacts.CreateCompanyTx(ctx, tx, in, w.cols.store)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating company %s: %w", row.ExternalID, err)
		}
		return ids.UUID(company.Id), nil
	})
	if _, ok := errors.AsType[*contacts.DuplicateDomainError](err); ok {
		// A domain names ONE company across the estate, so a row claiming one
		// another company already holds is refused by the store — the same shape
		// a contact's claimed email has. One bad row is a skip with a reason, not
		// a failed run: the rest of the file still lands.
		return skippedFor(domainClaimedWhy), nil
	}
	if linkedInClaimed(err) {
		return skippedFor(linkedInClaimedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// land commits one native record, its identity-map row and its tags in one
// transaction, then caches the binding. It caches after the commit: a rolled
// back landing would leave later pages resolving an id that does not exist.
func (w *csvWriters) land(ctx context.Context, row migration.Row, create func(tx pgx.Tx) (ids.UUID, error)) error {
	// Every row this importer writes is one of many under a single approval, so
	// it carries the marker that says so. What it buys today is one thing: a
	// standing domain refusal is NOT lifted by a spreadsheet, because approving
	// a file is not the same act as a contact putting one domain on one company.
	ctx = contacts.WithBulkWrite(ctx)
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		if id, err = create(tx); err != nil {
			return err
		}
		if err := w.identities.RecordIdentityTx(ctx, tx, w.runID, w.sourceSystem, w.object, row.ExternalID, id, migration.OriginCreated); err != nil {
			return err
		}
		if err := w.tagNames.applyAll(ctx, tx, w.object, id, textFields(row.Fields)[csvTargetTags]); err != nil {
			return err
		}
		return w.fileUnderContextTag(ctx, tx, id)
	}); err != nil {
		return err
	}
	w.nativeIDs[row.ExternalID] = id
	return nil
}
