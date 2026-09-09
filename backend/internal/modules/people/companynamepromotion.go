// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Corroborated signature company-name promotion (PO-F-2a, ADR-0072/A118).
//
// A captured company starts out named from its mail domain ("Gitex" for
// gitex.com) and stamped name_source='domain' — readable, but the company's own
// name for itself is usually in the signature blocks of the people who work
// there. Promoting that name is only safe with corroboration, because ONE
// signature is one sender's unverified claim about which company an entire
// record belongs to: an attacker who mails a connected mailbox from a domain
// nobody has written to yet would otherwise get to name the company.
//
// So a name is promoted directly only when a source the SENDER DOES NOT CONTROL
// agrees: the site dossier's own stated name. Signatures never carry that
// authority, however many of them agree — the people at one company share
// one mail domain, so two signatures are two mailboxes an actor who controls (or
// can forge From: for) that domain controls both of, and the capture path
// authenticates no From header. Counting them as independent sources would hand
// the naming decision back to exactly the attacker this rule exists to stop.
// Agreeing signatures still rank a claim above a lone one; they just stage a 🟡
// proposal instead of writing, which is the same answer with a human in it.
//
// The promotion never wins against a stronger source: the write is a CAS on
// name_source='domain' under a row lock, so a human edit (or a dossier name)
// landing first simply makes the promotion a no-op. Weaker never overwrites
// stronger, and the loser is silent rather than an error.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The corroboration a promotion rests on — recorded on the audit row so the
// reason a name changed is readable years later, not re-derived.
const (
	// CompanyNameCorroborationDossier: the company's own site states this name.
	// The only corroboration that authorizes an unattended write — a sender
	// cannot edit the victim's website.
	CompanyNameCorroborationDossier = "dossier"
	// CompanyNameCorroborationSignatures: two or more people at the company
	// signed with it. It outranks a lone signature but still stages for a human:
	// those people share one mail domain, so the agreement is one forgeable
	// source repeated, not two independent ones.
	CompanyNameCorroborationSignatures = "signatures"
	// CompanyNameCorroborationNone: one person said so and nothing agrees — this
	// verdict is staged for a human, never written.
	CompanyNameCorroborationNone = "uncorroborated"
)

// companyNamePromotionSource is the DM-CONV-11 channel on the audit row.
const companyNamePromotionSource = "signature_promotion"

// SignatureCompanyName is one person's accepted `company_name` signature evidence:
// the company that person's own mail signature names.
type SignatureCompanyName struct {
	PersonID ids.PersonID
	Value    string
}

// CompanyNameCandidate is one provisionally-named company together with
// every name that could replace the domain-derived one.
type CompanyNameCandidate struct {
	CompanyID ids.CompanyID
	DisplayName    string
	Signatures     []SignatureCompanyName
	// DossierNames are the site-read profile values that state what the
	// company calls itself (display_name, legal_name).
	DossierNames []string
}

// CompanyNameVerdict is what one candidate's evidence adds up to.
type CompanyNameVerdict struct {
	// Name is the display name to write, verbatim as a signature spelled it.
	Name string
	// NameKey is Name's normalized form — the identity of the CLAIM rather
	// than of one spelling of it. A refusal keyed on this survives the
	// evidence moving; keyed on the payload it would not.
	NameKey string
	// Corroborated says whether it may be written without asking a human.
	Corroborated bool
	// Corroboration names WHICH source agreed (one of the constants above).
	Corroboration string
	// Persons are the people whose signatures carry the name — the evidence a
	// reviewer reads when the verdict is staged instead of applied.
	Persons []ids.PersonID
}

// DecideCompanyName weighs one candidate's signatures against its dossier and the
// name the company already carries. It reports no verdict (ok=false) when
// nothing proposes a different name.
//
// Pure, so the rule is testable without a database and reads as the rule
// rather than as a query plan.
func DecideCompanyName(c CompanyNameCandidate) (CompanyNameVerdict, bool) {
	groups := groupSignatureNames(c.Signatures, NormalizeCompanyName(c.DisplayName))
	if len(groups) == 0 {
		return CompanyNameVerdict{}, false
	}
	winner := bestNameClaim(groups, dossierKeys(c.DossierNames))
	corroboration := winner.corroboration
	return CompanyNameVerdict{
		Name:    dominantSpelling(winner.spelling),
		NameKey: winner.key,
		// ONLY the dossier authorizes an unattended write. Agreeing signatures
		// are the same mail domain speaking twice, and the sender chose it.
		Corroborated:  corroboration == CompanyNameCorroborationDossier,
		Corroboration: corroboration,
		Persons:       sortedPersonIDs(winner.persons),
	}, true
}

// nameClaim is one company name several signatures agree on, with the spellings
// they used and the people who used them.
type nameClaim struct {
	key      string
	spelling map[string]int
	persons  map[ids.PersonID]bool
	// corroboration is filled by bestNameClaim, which is where the dossier is
	// known; until then a claim is just a claim.
	corroboration string
}

// dossierKeys reduces the site-stated names to the normalized set a signature
// can agree with.
func dossierKeys(names []string) map[string]bool {
	keys := make(map[string]bool, len(names))
	for _, n := range names {
		if key := NormalizeCompanyName(n); key != "" {
			keys[key] = true
		}
	}
	return keys
}

// groupSignatureNames folds the signatures into one claim per normalized name:
// "Acme GmbH" and "ACME" are one claim made twice, which is exactly what
// corroboration counts. A signature restating the name already on the record
// proposes nothing and is dropped — it cannot corroborate a change either.
func groupSignatureNames(signatures []SignatureCompanyName, current string) map[string]*nameClaim {
	claims := map[string]*nameClaim{}
	for _, s := range signatures {
		key := NormalizeCompanyName(s.Value)
		if key == "" || key == current {
			continue
		}
		c, ok := claims[key]
		if !ok {
			c = &nameClaim{key: key, spelling: map[string]int{}, persons: map[ids.PersonID]bool{}}
			claims[key] = c
		}
		c.spelling[s.Value]++
		c.persons[s.PersonID] = true
	}
	return claims
}

// bestNameClaim ranks the claims and returns the winner with its corroboration
// resolved: corroborated beats uncorroborated, more people beats fewer, and the
// normalized name breaks the remaining tie — so two workers reading the same
// evidence always reach the same answer rather than renaming the company
// back and forth.
func bestNameClaim(claims map[string]*nameClaim, dossier map[string]bool) *nameClaim {
	ranked := make([]*nameClaim, 0, len(claims))
	for _, c := range claims {
		c.corroboration = corroborationFor(c, dossier)
		ranked = append(ranked, c)
	}
	sort.Slice(ranked, func(i, j int) bool {
		ci := ranked[i].corroboration != CompanyNameCorroborationNone
		cj := ranked[j].corroboration != CompanyNameCorroborationNone
		if ci != cj {
			return ci
		}
		if len(ranked[i].persons) != len(ranked[j].persons) {
			return len(ranked[i].persons) > len(ranked[j].persons)
		}
		return ranked[i].key < ranked[j].key
	})
	return ranked[0]
}

// corroborationFor names the second source that agrees with a claim, if any.
func corroborationFor(c *nameClaim, dossier map[string]bool) string {
	switch {
	case dossier[c.key]:
		return CompanyNameCorroborationDossier
	case len(c.persons) >= 2:
		return CompanyNameCorroborationSignatures
	default:
		return CompanyNameCorroborationNone
	}
}

// dominantSpelling picks the raw form to write: the one most signatures used,
// ties broken lexicographically so the choice is reproducible.
func dominantSpelling(counts map[string]int) string {
	spellings := make([]string, 0, len(counts))
	for s := range counts {
		spellings = append(spellings, s)
	}
	sort.Slice(spellings, func(i, j int) bool {
		if counts[spellings[i]] != counts[spellings[j]] {
			return counts[spellings[i]] > counts[spellings[j]]
		}
		return spellings[i] < spellings[j]
	})
	return spellings[0]
}

func sortedPersonIDs(set map[ids.PersonID]bool) []ids.PersonID {
	out := make([]ids.PersonID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// CompanyNameCandidates lists provisionally-named companies that have at least
// one employee signature naming a company, each with the evidence to judge it.
//
// One PAGE, keyed on the company id the caller last saw — not the first N
// by age. The difference is the whole correctness of the sweep: most candidates
// reach a verdict that changes nothing (their signatures restate the name the
// record already carries, or the one name proposed is uncorroborated and waits
// on a human), and those rows stay candidates forever. A fixed prefix of a fixed
// ordering therefore fills with rows that will never resolve, and every
// company behind them — including ones whose corroborated name is ready to
// apply today — is never looked at again. Paging to exhaustion is what stops
// that, and it costs nothing: the work per company is one in-memory
// decision, no model call and no network.
func (s *Store) CompanyNameCandidates(ctx context.Context, after ids.CompanyID, limit int) ([]CompanyNameCandidate, error) {
	var out []CompanyNameCandidate
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var cursor *ids.CompanyID
		if !after.IsZero() {
			cursor = &after
		}
		rows, err := tx.Query(ctx, `
			SELECT o.id, o.display_name
			FROM company o
			WHERE o.name_source = 'domain'
			  AND o.archived_at IS NULL AND o.merged_into_id IS NULL
			  AND ($1::uuid IS NULL OR o.id > $1)
			  AND EXISTS (
				SELECT 1
				FROM relationship r
				JOIN person p ON p.id = r.person_id
				  AND p.archived_at IS NULL AND p.merged_into_id IS NULL
				JOIN person_profile_field f ON f.person_id = p.id AND f.field = 'company_name'
				WHERE r.company_id = o.id AND r.kind = 'employment' AND r.archived_at IS NULL)
			ORDER BY o.id
			LIMIT $2`, cursor, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		candidates, byID, err := scanCompanyNameCandidates(rows)
		if err != nil {
			return err
		}
		out = candidates
		if len(out) == 0 {
			return nil
		}
		companyIDs := make([]ids.CompanyID, 0, len(out))
		for _, c := range out {
			companyIDs = append(companyIDs, c.CompanyID)
		}
		if err := loadSignatureCompanyNames(ctx, tx, companyIDs, out, byID); err != nil {
			return err
		}
		return loadDossierCompanyNames(ctx, tx, companyIDs, out, byID)
	})
	if err != nil {
		return nil, fmt.Errorf("people: listing company-name promotion candidates: %w", err)
	}
	return out, nil
}

// scanCompanyNameCandidates reads the candidate rows and the index the evidence
// loaders fill them through — one pass, so a company's position is known
// before its signatures arrive.
func scanCompanyNameCandidates(rows pgx.Rows) ([]CompanyNameCandidate, map[ids.CompanyID]int, error) {
	var out []CompanyNameCandidate
	byID := map[ids.CompanyID]int{}
	for rows.Next() {
		var c CompanyNameCandidate
		if err := rows.Scan(&c.CompanyID, &c.DisplayName); err != nil {
			return nil, nil, err
		}
		byID[c.CompanyID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, byID, nil
}

func loadSignatureCompanyNames(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID,
	out []CompanyNameCandidate, byID map[ids.CompanyID]int,
) error {
	rows, err := tx.Query(ctx, `
		SELECT r.company_id, p.id, f.value
		FROM relationship r
		JOIN person p ON p.id = r.person_id
		  AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		JOIN person_profile_field f ON f.person_id = p.id AND f.field = 'company_name'
		WHERE r.company_id = ANY($1) AND r.kind = 'employment' AND r.archived_at IS NULL`, companyIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID ids.CompanyID
		var sig SignatureCompanyName
		if err := rows.Scan(&companyID, &sig.PersonID, &sig.Value); err != nil {
			return err
		}
		if i, ok := byID[companyID]; ok {
			out[i].Signatures = append(out[i].Signatures, sig)
		}
	}
	return rows.Err()
}

func loadDossierCompanyNames(ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID,
	out []CompanyNameCandidate, byID map[ids.CompanyID]int,
) error {
	rows, err := tx.Query(ctx, `
		SELECT company_id, value
		FROM company_profile_field
		WHERE company_id = ANY($1) AND field IN ('display_name', 'legal_name')`, companyIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID ids.CompanyID
		var value string
		if err := rows.Scan(&companyID, &value); err != nil {
			return err
		}
		if i, ok := byID[companyID]; ok {
			out[i].DossierNames = append(out[i].DossierNames, value)
		}
	}
	return rows.Err()
}

// PromoteCompanyName writes a corroborated name onto a still-provisional
// company in its own transaction — the sweep's entry point.
func (s *Store) PromoteCompanyName(ctx context.Context, companyID ids.CompanyID, name, corroboration string) (bool, error) {
	var promoted bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		promoted, err = s.PromoteCompanyNameTx(ctx, tx, companyID, name, corroboration)
		return err
	})
	if err != nil {
		return false, err
	}
	return promoted, nil
}

// PromoteCompanyNameTx is the same write on a caller's transaction — the review
// queue's accept redeems its approval and applies the name in one commit.
//
// Reports false, no error, when the company is gone or no longer
// provisional: a human (or a dossier) naming it while the proposal sat in the
// inbox is the stronger source winning, which is the rule working, not a
// failure to report.
func (s *Store) PromoteCompanyNameTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, name, corroboration string) (bool, error) {
	// The probe sits HERE and not on the wrapper above, because compose reaches
	// this function directly on both of its real paths — the sweep and the
	// accept executor. A gate on the wrapper would guard the spelling nobody
	// production uses, which is the same as no gate while looking like one.
	// It is a no-op for those unbounded principals today.
	if err := auth.EnsureWritable(ctx, tx, "company", companyID.UUID); err != nil {
		return false, err
	}
	// The name lock comes before the row lock, the one order every path that
	// takes both uses (UpdateCompany says why): otherwise
	// this sweep and a human's rename of the same company can each hold what
	// the other wants.
	if err := lockCompanyNameWrites(ctx, tx); err != nil {
		return false, err
	}
	var current, source string
	// The row lock serializes this against a concurrent human edit: whoever
	// commits first is read by the other, so the CAS below cannot be decided
	// against a name that has already been replaced.
	err := tx.QueryRow(ctx, `
		SELECT display_name, name_source FROM company
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE`, companyID).Scan(&current, &source)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("people: reading the company to promote: %w", err)
	}
	if source != nameSourceDomain || current == name {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE company SET display_name = $2, name_source = $3
		WHERE id = $1 AND name_source = $4`,
		companyID, name, nameSourceSignature, nameSourceDomain)
	if err != nil {
		return false, fmt.Errorf("people: promoting the company name: %w", err)
	}
	// The row lock above makes this unreachable, and it is checked anyway: an
	// audit row and a company.updated event describing a rename that did
	// not happen are worse than the rename being skipped.
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	before := map[string]any{fieldDisplayName: current, "name_source": nameSourceDomain}
	after := map[string]any{
		fieldDisplayName: name, "name_source": nameSourceSignature,
		auditKeySource: companyNamePromotionSource, "corroboration": corroboration,
	}
	auditID, err := storekit.Audit(ctx, tx, actionUpdate, "company", companyID.UUID, before, after)
	if err != nil {
		return false, fmt.Errorf("people: auditing the company-name promotion: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID,
		crmcontracts.PublicEventCompanyUpdated{ChangedFields: after}); err != nil {
		return false, fmt.Errorf("people: emitting company.updated for the promotion: %w", err)
	}
	// The name this row was created under was a guess from its mail domain;
	// the one it just took is the company's own. That is the first moment a
	// twin captured from another domain can be recognised, so ask now.
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	if err := recheckCompanyNameForDuplicates(ctx, tx, companyID, by); err != nil {
		return false, err
	}
	return true, nil
}
