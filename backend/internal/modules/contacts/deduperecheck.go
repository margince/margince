// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The open duplicate queue, held to today's filing rule. Nothing else re-reads
// an open pair, so a stricter rule left the pairs it no longer files waiting for
// a reviewer. The re-check closes them with the filing paths' own scores. A
// closed pair is archived, not disposed, and filing it again reopens the row.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/filedpairs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dedupeRecheckBatch is how many open pairs one transaction re-checks, so a
// queue of thousands never holds its row locks for the whole pass.
const dedupeRecheckBatch = 500

// The keys of a pair's audit images: whether the system has archived it
// undecided, why, and which record type the pair joins.
const (
	auditKeyArchived   = "archived"
	auditKeyReason     = "reason"
	auditKeyEntityType = "entity_type"
)

// The reasons an open pair is archived without a decision.
const (
	archiveReasonNoLongerFiled  = "no_longer_filed"
	archiveReasonEndpointMerged = "endpoint_merged"
)

// DedupeRecheckResult counts one pass: the open pairs it read, and the ones it
// closed because the rule no longer files them.
type DedupeRecheckResult struct {
	Examined int
	Closed   int
}

// openPair is one open, undecided dedupe candidate and its two ends.
type openPair struct {
	id          ids.UUID
	entityType  string
	left, right ids.UUID
	confidence  float64
}

// RecheckOpenDedupeCandidates closes every open pair the filing rule would no
// longer file. Each batch is its own transaction and skips rows another
// transaction holds, so someone deciding a pair at that moment keeps it.
func (s *Store) RecheckOpenDedupeCandidates(ctx context.Context) (DedupeRecheckResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return DedupeRecheckResult{}, err
	}
	var result DedupeRecheckResult
	after := ids.Nil
	for {
		examined, closed, last, err := s.recheckBatch(ctx, after)
		if err != nil {
			return result, fmt.Errorf("contacts: re-check open dedupe candidates: %w", err)
		}
		result.Examined += examined
		result.Closed += closed
		if examined < dedupeRecheckBatch {
			break
		}
		after = last
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := storekit.LogSystem(ctx, tx, "dedupe_recheck", map[string]any{
			"examined": result.Examined, "closed": result.Closed,
		})
		return err
	})
	return result, err
}

// recheckBatch re-checks one batch after the given id, in its own transaction.
// It answers how many pairs it read and closed, and the last id it read.
func (s *Store) recheckBatch(ctx context.Context, after ids.UUID) (examined, closed int, last ids.UUID, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		pairs, err := lockOpenPairs(ctx, tx, after)
		if err != nil || len(pairs) == 0 {
			return err
		}
		examined, last = len(pairs), pairs[len(pairs)-1].id
		consumerMail, err := s.consumerMailMatcher(ctx, tx)
		if err != nil {
			return err
		}
		for _, pair := range pairs {
			filed, score, err := stillFiled(ctx, tx, consumerMail, pair)
			if err != nil {
				return err
			}
			if filed {
				continue
			}
			archived, err := archiveDedupeCandidateTx(ctx, tx, pair.id, archiveReasonNoLongerFiled,
				map[string]any{auditKeyEntityType: pair.entityType, auditKeyConfidence: score})
			if err != nil {
				return err
			}
			if archived {
				closed++
			}
		}
		return nil
	})
	return examined, closed, last, err
}

// lockOpenPairs reads the next batch of open pairs nobody has decided. A pair
// with a `resolve` in its history was judged by someone, even if the verdict
// was later undone, and the re-check never second-guesses one.
func lockOpenPairs(ctx context.Context, tx pgx.Tx, after ids.UUID) ([]openPair, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.entity_type,
		       coalesce(d.left_contact_id, d.left_company_id, d.left_lead_id),
		       coalesce(d.right_contact_id, d.right_company_id, d.right_lead_id),
		       d.confidence::float8
		  FROM dedupe_candidate d
		 WHERE d.disposition = 'open' AND d.archived_at IS NULL AND d.id > $1
		   AND NOT EXISTS (SELECT 1 FROM audit_log a
		                    WHERE a.entity_type = 'dedupe_candidate' AND a.entity_id = d.id
		                      AND a.action = 'resolve')
		 ORDER BY d.id
		 LIMIT $2
		   FOR UPDATE OF d SKIP LOCKED`, after, dedupeRecheckBatch)
	if err != nil {
		return nil, fmt.Errorf("read open dedupe candidates: %w", err)
	}
	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (openPair, error) {
		var p openPair
		err := row.Scan(&p.id, &p.entityType, &p.left, &p.right, &p.confidence)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read open dedupe candidates: %w", err)
	}
	return pairs, nil
}

// stillFiled answers whether today's rule would file this pair, and its score
// now. An identity conflict (a shared phone, a domain two companies claim) is a
// fact rather than a score and stands while both companies are live. A contact
// or lead pair with an end this pass cannot read stands. Leads have no
// identity-conflict lane, so a lead pair at 1.0 is scored again.
func stillFiled(ctx context.Context, tx pgx.Tx, consumerMail *freemail.Matcher, pair openPair) (bool, float64, error) {
	if pair.confidence >= identityConflictConfidence && pair.entityType == entityContact {
		return true, pair.confidence, nil
	}
	switch pair.entityType {
	case entityContact:
		return contactPairStillFiled(ctx, tx, consumerMail, pair)
	case entityLead:
		return leadPairStillFiled(ctx, tx, consumerMail, pair)
	case entityCompany:
		return companyPairStillFiled(ctx, tx, pair)
	}
	return true, pair.confidence, nil
}

// archiveDedupeCandidateTx archives one open pair nobody decided, with the
// reason on its audit row. It is the system's closure and never records a
// verdict, which setDedupeDispositionTx alone does for someone.
func archiveDedupeCandidateTx(ctx context.Context, tx pgx.Tx, id ids.UUID, reason string, detail map[string]any) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE dedupe_candidate SET archived_at = now(), version = version + 1
		 WHERE id = $1 AND disposition = 'open' AND archived_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("archive dedupe candidate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	after := map[string]any{auditKeyArchived: true, auditKeyReason: reason}
	maps.Copy(after, detail)
	if _, err := storekit.Audit(ctx, tx, "archive", auditEntityDedupe, id,
		map[string]any{auditKeyArchived: false}, after); err != nil {
		return false, fmt.Errorf("audit the archived dedupe candidate: %w", err)
	}
	return true, nil
}

// contactSide is what the contact score reads about one end of a pair.
type contactSide struct {
	name           string
	emails         []string
	companyID      *ids.CompanyID
	companyDomains []string
	mailDomains    []string
}

func contactPairStillFiled(ctx context.Context, tx pgx.Tx, consumerMail *freemail.Matcher, pair openPair) (bool, float64, error) {
	left, found, err := readContactSide(ctx, tx, pair.left)
	if err != nil || !found {
		return true, pair.confidence, err
	}
	right, found, err := readContactSide(ctx, tx, pair.right)
	if err != nil || !found {
		return true, pair.confidence, err
	}
	score := max(contactScore(left, right, consumerMail), contactScore(right, left, consumerMail))
	// The review lanes' own pair tests. A pair stays while the evidence it was
	// filed on still holds, and retires once an edit removes it.
	_, _, variant := addressVariantPair(left.emails, right.emails)
	return score >= dedupeReviewThreshold || contactNamesCollide(left.name, right.name) || variant, score, nil
}

// contactScore is contactConfidence with one side as the candidate and the
// other as the stored row. The filing query returns one row per employer
// domain, so each domain is scored.
func contactScore(candidate, stored contactSide, consumerMail *freemail.Matcher) float64 {
	c := ContactCandidate{
		FullName: candidate.name, Emails: candidate.emails,
		ConsumerMail: consumerMail, CurrentPrimaryCompanyID: candidate.companyID,
	}
	row := contactCandidateRow{fullName: stored.name, companyID: stored.companyID, mailDomains: stored.mailDomains}
	best := contactConfidence(c, row)
	for _, domain := range stored.companyDomains {
		row.companyDomain = &domain
		best = max(best, contactConfidence(c, row))
	}
	return best
}

func readContactSide(ctx context.Context, tx pgx.Tx, id ids.UUID) (contactSide, bool, error) {
	var side contactSide
	err := tx.QueryRow(ctx, `
		SELECT p.full_name,
		       coalesce((SELECT array_agg(pe.email) FROM contact_email pe
		                  WHERE pe.contact_id = p.id AND pe.archived_at IS NULL), '{}')
		  FROM contact p
		 WHERE p.id = $1 AND p.archived_at IS NULL`, id).
		Scan(&side.name, &side.emails)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactSide{}, false, nil
	}
	if err != nil {
		return contactSide{}, false, fmt.Errorf("read dedupe contact %s: %w", id, err)
	}
	side.mailDomains = mailDomainsOf(side.emails)
	side.companyID, side.companyDomains, err = readCurrentEmployer(ctx, tx, id)
	return side, true, err
}

// readCurrentEmployer is the contact's current primary employer and the domains
// it holds, as the filing query joins them, or nothing when none is known.
func readCurrentEmployer(ctx context.Context, tx pgx.Tx, contact ids.UUID) (*ids.CompanyID, []string, error) {
	var company ids.CompanyID
	var domains []string
	err := tx.QueryRow(ctx, `
		SELECT r.company_id,
		       coalesce((SELECT array_agg(od.domain) FROM company_domain od
		                  WHERE od.company_id = r.company_id AND od.archived_at IS NULL), '{}')
		  FROM relationship r
		 WHERE r.contact_id = $1 AND r.kind = 'employment' AND r.archived_at IS NULL
		   AND `+employment.CurrentPrimarySQL("r")+`
		 ORDER BY r.company_id LIMIT 1`, contact).Scan(&company, &domains)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read dedupe contact employer %s: %w", contact, err)
	}
	return &company, domains, nil
}

// leadSide is what the lead score reads about one end of a pair.
type leadSide struct {
	name, company, email string
}

func leadPairStillFiled(ctx context.Context, tx pgx.Tx, consumerMail *freemail.Matcher, pair openPair) (bool, float64, error) {
	left, found, err := readLeadSide(ctx, tx, pair.left)
	if err != nil || !found {
		return true, pair.confidence, err
	}
	right, found, err := readLeadSide(ctx, tx, pair.right)
	if err != nil || !found {
		return true, pair.confidence, err
	}
	// fuzzyLead is the only lead filer, and it has no same-name lane.
	score := max(leadScore(left, right, consumerMail), leadScore(right, left, consumerMail))
	return score >= dedupeReviewThreshold, score, nil
}

// leadScore is leadConfidence with one side as the lead being written and the
// other as the stored row. A consumer mail host says nothing about an
// employer, so its domain is dropped as fuzzyLead drops it.
func leadScore(candidate, stored leadSide, consumerMail *freemail.Matcher) float64 {
	domain := emailDomain(candidate.email)
	if consumerMail.IsConsumer(domain) {
		domain = ""
	}
	name := stored.name
	company := stored.company
	return leadConfidence(candidate.name, strings.TrimSpace(candidate.company), domain, leadCandidateRow{
		fullName: &name, company: &company, domain: emailDomain(stored.email),
	})
}

func readLeadSide(ctx context.Context, tx pgx.Tx, id ids.UUID) (leadSide, bool, error) {
	var side leadSide
	err := tx.QueryRow(ctx, `
		SELECT coalesce(full_name, ''), coalesce(company_name, ''), coalesce(email, '')
		  FROM lead WHERE id = $1 AND archived_at IS NULL`, id).
		Scan(&side.name, &side.company, &side.email)
	if errors.Is(err, pgx.ErrNoRows) {
		return leadSide{}, false, nil
	}
	if err != nil {
		return leadSide{}, false, fmt.Errorf("read dedupe lead %s: %w", id, err)
	}
	return side, true, nil
}

// companyPairStillFiled re-scores two companies with the name pairing the
// filing path uses and holds it to the same second fact (keepCorroborated). It
// keeps a pair whose domains still nest (domainNests).
func companyPairStillFiled(ctx context.Context, tx pgx.Tx, pair openPair) (bool, float64, error) {
	var leftDisplay, leftLegal, rightDisplay, rightLegal string
	err := tx.QueryRow(ctx, `
		SELECT l.display_name, coalesce(l.legal_name, ''), r.display_name, coalesce(r.legal_name, '')
		  FROM company l, company r
		 WHERE l.id = $1 AND r.id = $2 AND l.archived_at IS NULL AND r.archived_at IS NULL`,
		pair.left, pair.right).Scan(&leftDisplay, &leftLegal, &rightDisplay, &rightLegal)
	// An archived company is no duplicate to review. Restoring it writes the
	// row, so the duplicate scan reads it and files the pair again.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, pair.confidence, nil
	}
	if err != nil {
		return true, pair.confidence, fmt.Errorf("read dedupe companies: %w", err)
	}
	if pair.confidence >= identityConflictConfidence {
		return true, pair.confidence, nil
	}
	forward := bestCompanyNamePairing(leftDisplay, leftLegal, rightDisplay, rightLegal)
	backward := bestCompanyNamePairing(rightDisplay, rightLegal, leftDisplay, leftLegal)
	score := max(forward.Confidence, backward.Confidence)
	if score >= dedupeReviewThreshold || forward.ExactName || backward.ExactName {
		forward.CompanyID = ids.From[ids.CompanyKind](pair.right)
		subject := ids.From[ids.CompanyKind](pair.left)
		kept, err := keepCorroborated(ctx, tx, CompanyCandidate{ExcludeID: &subject}, []CompanyCandidateScore{forward})
		if err != nil || len(kept) > 0 {
			return true, score, err
		}
	}
	nests, err := companyDomainsNest(ctx, tx, pair.left, pair.right)
	return nests, score, err
}

// refileArchivedCandidateTx brings back an open pair the system archived without
// a decision, with the evidence that files it now. A pair someone decided is
// never archived-open, so this cannot revive a verdict.
func refileArchivedCandidateTx(ctx context.Context, tx pgx.Tx, entityType, leftCol, rightCol string,
	left, right ids.UUID, confidence float64, evidence []byte,
) (bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE dedupe_candidate
		   SET archived_at = NULL, confidence = $4, evidence = $5, version = version + 1
		 WHERE entity_type = $1 AND %s = $2 AND %s = $3
		   AND disposition = 'open' AND archived_at IS NOT NULL
		RETURNING id`, leftCol, rightCol),
		entityType, left, right, confidence, evidence).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reopening dedupe candidate: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "restore", auditEntityDedupe, id, map[string]any{auditKeyArchived: true},
		map[string]any{
			auditKeyArchived: false, auditKeyEntityType: entityType, auditKeyConfidence: confidence,
			auditKeyReason: "filed_again",
		}); err != nil {
		return false, fmt.Errorf("contacts: audit the reopened dedupe candidate: %w", err)
	}
	filedpairs.Note(ctx, id)
	return true, nil
}
