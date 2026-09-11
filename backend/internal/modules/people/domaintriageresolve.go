// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Where a triage verdict becomes rows. The triage read decides what a domain
// is; this settles the ledger and, for a company, creates the company the
// capture path deliberately did not create earlier — named from the dossier
// rather than from the raw domain label, with an employment edge for every
// person who has accumulated on that domain while the question was open.
//
// One transaction: the verdict, the company, its domain, the edges, the
// dossier's findings and the dossier binding all commit together or not at all.
// A ledger row reading 'company' beside no company would be a lie the
// ensure ladder then acts on.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ResolveDomainTriageInput is one answered triage: the verdict, what produced
// it, and — for a company — the dossier to name the company from and fill
// it with.
type ResolveDomainTriageInput struct {
	Domain   string
	Status   string
	Source   string
	Evidence string
	ReadID   ids.UUID

	// DossierName is the company name the site stated. Empty falls back to the
	// domain's registrable label, which is what the pre-triage path always
	// used — a worse name, but never a fabricated one.
	DossierName string
	SeedURL     string
	Fields      []DeepReadField
	Facts       []DeepReadFact
}

// ResolveDomainTriageResult reports what the verdict actually did.
type ResolveDomainTriageResult struct {
	CompanyID      *ids.CompanyID
	CompanyCreated bool
	EdgesPlanted   int
}

// ResolveDomainTriage settles a domain's verdict. A non-company answer writes
// the ledger and stops; a company answer creates or adopts the company and
// wires everything the deferred ensures could not.
//
// Idempotent on replay: the dedupe lands a re-run on the company the first
// run created, the edge insert is conflict-free, and the field apply fills only
// what is still empty. A worker that dies mid-verdict and retries therefore
// converges rather than duplicating.
func (s *Store) ResolveDomainTriage(ctx context.Context, in ResolveDomainTriageInput) (ResolveDomainTriageResult, error) {
	var res ResolveDomainTriageResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		res, err = s.resolveDomainTriageTx(ctx, tx, in)
		return err
	})
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	return res, nil
}

// ResolveUnreadableDomainTriage answers a domain whose site gave no answer —
// unreachable, or read and identifying nobody. The sender's own name is the
// last evidence available, and it is tested HERE, inside the same transaction
// and under the same row lock as the verdict it produces, against the very
// people a company answer would have employed.
//
// A domain that is somebody's name is theirs, and that settles.
//
// Anything else is WITHHELD, not created. It used to get "the company it
// would have got before triage existed" on the reasoning that a real business
// whose site is down must not lose its record over an outage — but the record
// it got was a title-cased domain label with every field empty, and the
// disposition settled so nothing ever asked again. That produced 40 of 108
// companies in a real import, "Pwc" and "Mckinsey" among them, each frozen
// as a shell. A withheld domain stays PENDING with its reason recorded, so the
// site can be read again later and a human can decide meanwhile.
func (s *Store) ResolveUnreadableDomainTriage(ctx context.Context, in ResolveDomainTriageInput) (ResolveDomainTriageResult, error) {
	var res ResolveDomainTriageResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		persons, err := PersonsOnDomain(ctx, tx, in.Domain)
		if err != nil {
			return err
		}
		if DomainLooksPersonal(freemail.RegistrableLabel(in.Domain), persons) {
			in.Status, in.Source = DomainPersonal, DomainSourceHeuristic
			res, err = s.resolveDomainTriageTx(ctx, tx, in)
			return err
		}
		// No site, no name that explains the domain: nothing has EARNED a
		// company. Left open, marked, and answerable later.
		return markDispositionUnevidenced(ctx, tx, in.Domain, in.Evidence)
	})
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	return res, nil
}

// markDispositionUnevidenced records that a domain's question is still open
// because nothing evidenced a company — the site could not be read, and the
// sender's name did not explain the domain.
//
// It keeps status='pending' deliberately. Pending is the ONE value every
// due-scan, queue-mark and exhaustion query treats as open, so a withheld
// domain stays retryable and stays visible without teaching five other queries
// a second word for the same thing.
//
// Guarded on pending so it cannot overwrite an answer a HUMAN settled: an admin
// who confirmed the company owns that verdict, and a later sweep finding the
// site still unreadable must not quietly reopen their decision.
func markDispositionUnevidenced(ctx context.Context, tx pgx.Tx, domain, evidence string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET pending_reason = 'unevidenced', evidence = NULLIF($2, ''),
		       next_attempt_at = NULL, updated_at = now()
		 WHERE domain = $1 AND status = $3`,
		domain, evidence, DomainPending); err != nil {
		return fmt.Errorf("people: recording that %s evidenced no company: %w", domain, err)
	}
	return nil
}

func (s *Store) resolveDomainTriageTx(ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput) (ResolveDomainTriageResult, error) {
	// The lock every concurrent ensure on this domain waits behind, taken
	// before anything is decided so no ensure can slip between the read and
	// the write and conclude the question is still open.
	prior, known, err := readDispositionTx(ctx, tx, in.Domain)
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	if !known {
		return ResolveDomainTriageResult{}, fmt.Errorf("people: %s has no open disposition to resolve", in.Domain)
	}
	if prior.Settled() {
		// Already answered. A worker that resolved this and then died before
		// recording its dossier gets its whole run replayed by the reclaim, and
		// without this a domain settled `personal` would reach the create path
		// on the second pass and get the company the first pass refused —
		// while settleDisposition's own pending-guard kept the ledger saying
		// `personal`. The answer stands; the replay is a no-op.
		return ResolveDomainTriageResult{CompanyID: prior.CompanyID}, nil
	}
	// The last gate before a company exists, and it reads the LOCKED row: a
	// crawl already in flight when the domain was refused would otherwise land
	// its verdict here and create the very record the refusal forbids. The
	// sweeps skip suppressed domains, but a read they started earlier cannot
	// know that, and an unlocked check would lose the race against a
	// suppression committing in between.
	if in.Status == DomainCompany && prior.Suppressed() {
		return ResolveDomainTriageResult{}, nil
	}
	if in.Status != DomainCompany {
		return ResolveDomainTriageResult{}, settleDisposition(ctx, tx, in, nil)
	}

	res, err := s.adoptOrCreateTriagedCompany(ctx, tx, in, prior)
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	if res.EdgesPlanted, err = plantDomainEmployment(ctx, tx, in.Domain, *res.CompanyID); err != nil {
		return ResolveDomainTriageResult{}, err
	}
	if len(in.Fields) > 0 || len(in.Facts) > 0 {
		if err := s.ApplyDeepReadTx(ctx, tx, DeepReadProposal{
			CompanyID:  *res.CompanyID,
			SourceURL:  in.SeedURL,
			SiteReadID: in.ReadID,
			Fields:     in.Fields,
			Facts:      in.Facts,
		}); err != nil {
			return ResolveDomainTriageResult{}, err
		}
	}
	if err := bindTriageDossier(ctx, tx, in.ReadID, *res.CompanyID); err != nil {
		return ResolveDomainTriageResult{}, err
	}
	return res, settleDisposition(ctx, tx, in, res.CompanyID)
}

// adoptOrCreateTriagedCompany returns the company the verdict belongs to. It
// looks for an existing one FIRST: a human may have typed the company in while
// the crawl ran, and a second row for the same domain would be exactly the
// duplicate the dedupe chokepoint exists to prevent.
func (s *Store) adoptOrCreateTriagedCompany(ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput, prior DomainDisposition) (ResolveDomainTriageResult, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionCreate); err != nil {
		return ResolveDomainTriageResult{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	// The site stated a name, so this company is born with the name a
	// human would have typed rather than a title-cased domain label — and with
	// the provenance to say so, which keeps a later dossier from overwriting it.
	displayName, nameSource := in.DossierName, nameSourceDossier
	if displayName == "" {
		displayName, nameSource = DisplayNameFromDomain(in.Domain), nameSourceDomain
	}
	if displayName == "" {
		displayName, nameSource = in.Domain, nameSourceDomain
	}

	// The name is settled BEFORE PO-F-2 runs, because the name is half of what
	// PO-F-2 reads. Asking about the domain alone leaves the fuzzy tier nothing
	// to score, and two domains of one company ("acme.de", "acme.eu") derive
	// the same label — the shape that put one company in a workspace twice.
	match, err := DedupeCompanyForCreate(ctx, tx, CompanyCandidate{
		DisplayName: displayName,
		Domains:     []string{in.Domain},
	})
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	if match.Decision == DecisionExactCollision {
		return ResolveDomainTriageResult{CompanyID: &match.CompanyID}, nil
	}

	companyID, err := createCompany(ctx, tx, match, CompanySpec{
		DisplayName: displayName,
		NameSource:  nameSource,
		OwnerID:     ownerFromUUID(prior.OwnerID),
		Visibility:  visibilityWorkspace,
		Domains:     []CompanyDomainInput{{Domain: in.Domain, IsPrimary: true}},
		Source:      domainTriageSource(in.Domain),
		CapturedBy:  by,
	})
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	auditID, err := storekit.Audit(ctx, tx, "create", entityCompany, companyID.UUID, nil, map[string]any{
		fieldDisplayName: displayName, auditKeyNameSource: nameSource, auditKeyDomain: in.Domain,
	})
	if err != nil {
		return ResolveDomainTriageResult{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID,
		crmcontracts.PublicEventCompanyCreated{DisplayName: &displayName}); err != nil {
		return ResolveDomainTriageResult{}, err
	}
	// A near-match creates anyway — triage is resolving a question a human
	// already answered, and DEDUPE_FUZZY_AUTOMERGE is pinned never — but the
	// pair goes on the review queue so the twin is visible.
	if err := match.recordIfReview(ctx, tx, companyID, domainTriageSource(in.Domain), by); err != nil {
		return ResolveDomainTriageResult{}, err
	}
	return ResolveDomainTriageResult{CompanyID: &companyID, CompanyCreated: true}, nil
}

// plantDomainEmployment gives every live person on the domain their employment
// edge at once. They accumulated while the question was open — each ensure
// created the person and deliberately left the company undecided — so this is
// where the whole backlog is wired, not only the sender who happened to trigger
// the verdict.
//
// It never reassigns: someone whose current employer a human already recorded
// keeps it, exactly as the capture ensure never overrides one.
func plantDomainEmployment(ctx context.Context, tx pgx.Tx, domain string, companyID ids.CompanyID) (int, error) {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return 0, err
	}
	// The candidate set is read FIRST so each person's employment lock can be
	// taken before the insert decides anything from a read of their
	// employments. `FOR UPDATE` gives the same archive guarantee
	// lockPersonForAttach gives the single-person writers — an archive in
	// flight either commits first and drops the row out of this read, or waits
	// and sweeps the edge this plants.
	candidates, err := domainEmploymentCandidates(ctx, tx, domain)
	if err != nil {
		return 0, err
	}
	// Then the same per-person lock every other writer of this state takes.
	// Without it this planter reads "they already have a primary", skips them,
	// and a patch ending that employment commits beside the read: the person is
	// left employed once, unmarked. The rows come back in id order, so two runs
	// over overlapping domains take their locks in one order rather than each
	// waiting on the other's.
	for _, personID := range candidates {
		if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, personID.String()); err != nil {
			return 0, err
		}
	}
	// The NOT EXISTS is re-asked HERE, under those locks, and that is the whole
	// point of the split: the candidate read above is a snapshot, and this is
	// the decision.
	rows, err := tx.Query(ctx, `
		INSERT INTO relationship (kind, person_id, company_id, is_current_primary, source, captured_by)
		SELECT 'employment', p.id, $1, true, $2, $3
		FROM person p
		WHERE p.id = ANY($4)
		  AND NOT EXISTS (
			SELECT 1 FROM relationship r
			WHERE r.person_id = p.id AND `+employment.CurrentPrimarySlotSQL("r")+`)
		ON CONFLICT DO NOTHING
		RETURNING id, person_id`,
		companyID, domainTriageSource(domain), by, candidates)
	if err != nil {
		return 0, fmt.Errorf("people: planting the employment edges for %s: %w", domain, err)
	}
	type planted struct {
		edge   ids.UUID
		person ids.PersonID
	}
	var made []planted
	for rows.Next() {
		var one planted
		if err := rows.Scan(&one.edge, &one.person); err != nil {
			rows.Close()
			return 0, fmt.Errorf("people: planting the employment edges for %s: %w", domain, err)
		}
		made = append(made, one)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("people: planting the employment edges for %s: %w", domain, err)
	}
	// Each edge gets the write shape's other two rows, in this same transaction.
	// An employer appearing on a contact with nobody having typed it is exactly
	// the change the trail exists to explain, and the bus is how the surfaces
	// that count a company's contacts learn to recount.
	for _, one := range made {
		if err := auditCapturedEmployment(ctx, tx, one.edge, one.person, companyID, relationshipOriginCapture); err != nil {
			return 0, err
		}
	}
	return len(made), nil
}

// domainEmploymentCandidates names the live people this domain's verdict is
// about: everybody reachable at it, whatever employments they already hold.
//
// It exists so the planter can hold a lock per person before it decides, and
// it deliberately does NOT ask who has a primary employment yet. That question
// is the decision, and asking it here would answer it from an unlocked read: a
// person whose only employment ends while this list is being built would be
// excluded from it, and the insert — which can only reconsider the ids it was
// handed — would leave them employed once and unmarked. Locking a few people
// the insert then skips costs nothing; the other way costs the case this lock
// exists for.
//
// The order is the person id, which is what keeps two runs over overlapping
// domains from taking one pair of locks in opposite orders.
func domainEmploymentCandidates(ctx context.Context, tx pgx.Tx, domain string) ([]ids.PersonID, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id
		FROM person p
		WHERE p.archived_at IS NULL
		  AND p.merged_into_id IS NULL
		  AND EXISTS (
			SELECT 1 FROM person_email pe
			WHERE pe.person_id = p.id
			  -- An address somebody no longer uses must not attach them to a
			  -- new employer: a former colleague would be re-hired by a domain
			  -- they left.
			  AND pe.archived_at IS NULL
			  AND (split_part(pe.email, '@', 2) = $1
			       -- A literal suffix compare, never LIKE — see PersonsOnDomain.
			       OR right(split_part(pe.email, '@', 2), length($1) + 1) = '.' || $1))
		ORDER BY p.id
		FOR UPDATE OF p`, domain)
	if err != nil {
		return nil, fmt.Errorf("people: reading the people on %s: %w", domain, err)
	}
	defer rows.Close()
	var out []ids.PersonID
	for rows.Next() {
		var one ids.PersonID
		if err := rows.Scan(&one); err != nil {
			return nil, fmt.Errorf("people: reading the people on %s: %w", domain, err)
		}
		out = append(out, one)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading the people on %s: %w", domain, err)
	}
	return out, nil
}

// bindTriageDossier attaches the triage read to the company it produced.
// confirmed_at is what the row's target-shape CHECK requires alongside the
// company, and it is honest here: the verdict IS the confirmation.
func bindTriageDossier(ctx context.Context, tx pgx.Tx, readID ids.UUID, companyID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE site_read
		   SET company_id = $2, confirmed_at = now(), updated_at = now()
		 WHERE id = $1 AND target_kind = $3 AND company_id IS NULL`,
		readID, companyID, TargetKindDomainTriage); err != nil {
		return fmt.Errorf("people: binding the triage dossier to its company: %w", err)
	}
	return nil
}

// settleDisposition writes the answer and closes the retry cursor, so the domain
// drops out of the sweep's due scan for good.
//
// Guarded on `status = 'pending'`: a verdict answers an OPEN question. A late
// duplicate — a re-queued job, a sweep racing a trigger — must not overwrite the
// answer that already landed, and must never undo a human who settled it by
// hand (adoptDispositionForCompany, which is deliberately not guarded because
// overriding is its whole job).
func settleDisposition(ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput, companyID *ids.CompanyID) error {
	var readID *ids.UUID
	if !in.ReadID.IsZero() {
		readID = &in.ReadID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET status = $2, source = $3, evidence = NULLIF($4, ''),
		       company_id = $5, site_read_id = $6,
		       -- The question is answered, so it is no longer waiting on
		       -- evidence. Leaving the marker would keep a settled domain in
		       -- the "needs a human" list for ever.
		       pending_reason = NULL,
		       next_attempt_at = NULL, updated_at = now()
		 WHERE domain = $1 AND status = 'pending'`,
		in.Domain, in.Status, in.Source, in.Evidence, companyID, readID); err != nil {
		return fmt.Errorf("people: settling the disposition of %s: %w", in.Domain, err)
	}
	return nil
}

// The audit-payload keys a triage-created company carries. auditKeyDomain
// is deliberately its own constant and not nameSourceDomain: one is a payload
// field name, the other a provenance value, and they collide only by spelling.
const (
	auditKeyNameSource = "name_source"
	auditKeyDomain     = "domain"
)

// domainTriageSource is the provenance string rows created by a verdict carry,
// naming the domain whose triage produced them.
func domainTriageSource(domain string) string { return "domain_triage:" + domain }
