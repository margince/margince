// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What capture RECORDS about a counterparty, as opposed to how it ensures one:
// the review pair a fuzzy match files, and the small readings that turn header
// text into columns — a name part, an impersonation tell, the acquisition a
// message evidences.
//
// They sit apart from the ensure ladder because none of them decides whether a
// record exists. Each answers a narrower question ABOUT the row being written,
// and each is read on its own by somebody auditing what capture claimed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// reviewPair is the record capture just minted, as the review queue needs to
// describe it: who it is, what address named them, and what wrote it down.
type reviewPair struct {
	created ids.ContactID
	name    string
	email   string
	source  string
	by      string
}

// fileReviewPair puts the new contact and the incumbent it resembles in front of
// a human, for the two decisions that mean "created anyway, but somebody should
// look". Capture never blocks on a reviewer, so both arms have already created
// the record; what differs is only what the queue is told.
//
// One function rather than two arms inline, because the two lanes disagree about
// exactly one thing — what the evidence says — and spelling the record-and-report
// half twice is how the two drift into describing the same queue differently.
func fileReviewPair(ctx context.Context, tx pgx.Tx, match ContactResolution,
	pair reviewPair, res *EnsureCounterpartyResult,
) error {
	var evidence []map[string]any
	switch match.Decision {
	case DecisionNameCollisionReview:
		// The name is the whole evidence, so the pair carries it alone. An exact
		// spelling is not a probability: rendering a score beside it would invite
		// a reviewer to read 1.0 as a confidence the lane never computed, and the
		// stored confidence stays 0 for the same reason.
		evidence = []map[string]any{
			{
				evidenceFieldKey: fieldFullName, evidenceLeftKey: pair.name, evidenceRightKey: pair.name,
				evidenceSignalKey: evidenceSignalCollide,
			},
		}
	case DecisionFuzzyReview:
		// The detection-time snapshot the queue renders (DH-N-8): captured NOW,
		// against the incumbent as it looked when the score was computed — never
		// re-derived later.
		var incumbentName string
		if err := tx.QueryRow(ctx,
			`SELECT full_name FROM contact WHERE id = $1`, match.ContactID).Scan(&incumbentName); err != nil {
			return fmt.Errorf("contacts: reading dedupe incumbent: %w", err)
		}
		evidence = []map[string]any{
			{
				evidenceFieldKey: fieldFullName, evidenceLeftKey: pair.name, evidenceRightKey: incumbentName,
				evidenceSignalKey: evidenceSignalCollide, evidenceScoreKey: match.Confidence,
			},
			{
				evidenceFieldKey: fieldEmail, evidenceLeftKey: pair.email, evidenceRightKey: nil,
				evidenceSignalKey: evidenceSignalOneSided,
			},
		}
	default:
		return nil
	}
	// The name lane stores NO score. Its resolution carries the name-similarity
	// term the fuzzy tier computed, but that number decided nothing here — the
	// lane fired on an exact spelling — and showing it to a reviewer beside two
	// identical names offers a confidence nobody measured.
	confidence := match.Confidence
	if match.Decision == DecisionNameCollisionReview {
		confidence = 0
	}
	recorded, err := recordDedupeCandidate(ctx, tx, entityContact,
		pair.created.UUID, match.ContactID.UUID, confidence, evidence, pair.source, pair.by)
	if err != nil {
		return err
	}
	res.DedupeRecorded = recorded
	return nil
}

// recordDedupeCandidate stores the pair canonically (lower id left,
// DH-DDL-1); the unique pair index makes a re-detection a no-op — reported
// as recorded=false so counters stay honest.
func recordDedupeCandidate(ctx context.Context, tx pgx.Tx, entityType string, a, b ids.UUID, confidence float64, evidence []map[string]any, source, by string) (bool, error) {
	left, right := a, b
	if right.String() < left.String() {
		left, right = right, left
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return false, err
	}
	leftCol, rightCol := "left_contact_id", "right_contact_id"
	switch entityType {
	case entityCompany:
		leftCol, rightCol = "left_company_id", "right_company_id"
	case entityLead:
		leftCol, rightCol = "left_lead_id", "right_lead_id"
	}
	var candidateID ids.UUID
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO dedupe_candidate (entity_type, %s, %s, confidence, evidence, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING
		RETURNING id`, leftCol, rightCol),
		entityType, left, right, confidence, payload, source, by).Scan(&candidateID)
	if errors.Is(err, pgx.ErrNoRows) {
		// This pair is already proposed. Nothing was written, so nothing is
		// audited — a no-op must not mint history.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: recording dedupe candidate: %w", err)
	}
	// Audited, NOT published. A dedupe candidate is a question the system is
	// asking about two records, not something that happened to either — nothing
	// downstream acts on one, and an outbox row nobody consumes would be an
	// event kind invented to satisfy a rule rather than a reader. What the
	// audit row buys is the part that was actually missing: who proposed this
	// merge, when, and on what evidence, on the record's own history rather
	// than only in operator telemetry.
	if _, err := storekit.Audit(ctx, tx, "create", "dedupe_candidate", candidateID, nil, map[string]any{
		"entity_type": entityType, "confidence": confidence, auditKeySource: source,
	}); err != nil {
		return false, fmt.Errorf("contacts: audit the dedupe candidate: %w", err)
	}
	return true, nil
}

// nameColumn renders a parsed name part for the nullable split-name columns.
// An unconfident parse leaves them NULL rather than storing "" — a column that
// says "we do not know" must not be spelled the same as one that says "empty".
func nameColumn(part string) *string {
	if part == "" {
		return nil
	}
	return &part
}

// quarantineSuspect flags the cheap impersonation tells (ADR-0063): a
// punycode domain (homoglyph vector) or a display name that embeds an
// address on a DIFFERENT domain ("ceo@acme.com <attacker@evil.example>").
// Flagged rows carry quarantined_at for the review surface; capture still
// records them — hiding suspicious mail would be worse than labeling it.
//
// Both tells are statements ABOUT the sender's mail domain, so with no domain
// there is nothing for either to contradict and the answer is no. Without that
// floor the second tell compares an embedded address against "" and matches
// every display name that merely contains an "@" — quarantining a record for a
// reason that cannot apply to it.
func quarantineSuspect(displayName, domain string) bool {
	if domain == "" {
		return false
	}
	if strings.HasPrefix(domain, "xn--") || strings.Contains(domain, ".xn--") {
		return true
	}
	name := strings.ToLower(displayName)
	at := strings.Index(name, "@")
	if at < 0 {
		return false
	}
	embedded := name[at+1:]
	if end := strings.IndexAny(embedded, " >,;"); end >= 0 {
		embedded = embedded[:end]
	}
	embedded = strings.Trim(embedded, ".")
	return embedded != "" && embedded != strings.ToLower(domain)
}

// acquiredFromCapture names what capture actually observed.
//
// A reply is the contact writing to us, which is the strongest acquisition in
// the vocabulary and the one a lawful reply is later argued from. Two outbound
// threads with no answer is US writing to THEM: worth a record, and not
// something the contact did. Unknown rather than a weaker positive kind, because
// capture cannot see how the address was obtained in the first place.
func acquiredFromCapture(replied bool) string {
	if replied {
		return AcquiredSubjectInitiated
	}
	return AcquiredUnknownLegacy
}

// acquiredFromCaptureTx widens acquiredFromCapture by the mail itself: an
// address that SENT us a captured message gave us its data by writing, whether
// or not we had written first. Bulk mail does not count, as it does not for a
// reply (capture's wroteBackTx): a newsletter is a list writing to everyone,
// not the sender writing to us. A first mail from a stranger is them contacting
// us, and a disclosure duty for it would be owed to nobody. Only the reverse —
// an address we wrote to, or saw on a Cc, that never wrote — stays unknown,
// unless a seat's mailbox already held mail with it before that seat connected
// the mailbox (mailbox_history): mail the seat sent to it, or received mail
// that named it on To or Cc.
func acquiredFromCaptureTx(ctx context.Context, tx pgx.Tx, replied bool, email string) (string, error) {
	if kind := acquiredFromCapture(replied); kind == AcquiredSubjectInitiated {
		return kind, nil
	}
	var wrote bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity_participant p
		    JOIN activity a ON a.id = p.activity_id
		   WHERE p.role = 'from' AND p.address = lower($1)
		     AND a.direction = 'inbound' AND a.archived_at IS NULL
		     AND NOT a.bulk_mail_attested)`, email).Scan(&wrote); err != nil {
		return "", fmt.Errorf("contacts: did this address write to us: %w", err)
	}
	if wrote {
		return AcquiredSubjectInitiated, nil
	}
	migrated, err := heldByMigratedLeadTx(ctx, tx, email)
	if err != nil {
		return "", err
	}
	if migrated {
		return AcquiredCRMMigration, nil
	}
	history, err := writtenToBeforeConnectedTx(ctx, tx, email)
	if err != nil {
		return "", err
	}
	if !history {
		if history, err = receivedBeforeConnectedTx(ctx, tx, email); err != nil {
			return "", err
		}
	}
	if history {
		return AcquiredMailboxHistory, nil
	}
	return AcquiredUnknownLegacy, nil
}

// heldByMigratedLeadTx reports that a lead carried over from the previous CRM
// already holds this address.
//
// The import files a counterparty with no deal and no conversation as a LEAD,
// not a contact. When a connected mailbox later finds mail with them, capture
// mints a contact beside that lead — and without this, the contact read as a
// stranger found in old mail and owed an Art. 14 notice, though they came over
// from the old CRM like every other migrated record (DutyFor, "crm_migration").
//
// The address must be the one the lead was IMPORTED with, read from its create
// audit row. Only an importer may write the mirror: prefix, but anyone who may
// edit leads may change a lead's email — and an imported lead retargeted at a
// stranger's address would otherwise excuse that stranger's notice. An email
// changed after the import therefore counts for nothing; changed back, it is
// the imported address again.
//
// Only a LIVE lead counts. A promoted lead already has its contact, so capture
// never mints one beside it; a retired lead leaves the duty owed, which is the
// safe direction. It also keeps the read on the lead email index, which covers
// live rows only. An erased lead has had its address scrubbed and matches
// nothing.
func heldByMigratedLeadTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM lead l
		   WHERE l.email = lower($1)
		     AND l.archived_at IS NULL
		     AND starts_with(l.source_system, $2)
		     AND EXISTS (
		           SELECT 1 FROM audit_log a
		            WHERE a.entity_type = 'lead' AND a.entity_id = l.id
		              AND a.action = 'create'
		              AND lower(a.after->>'email') = l.email))`,
		email, provenance.ReservedSourceSystemPrefix).Scan(&held); err != nil {
		return false, fmt.Errorf("contacts: did a migrated lead hold this address: %w", err)
	}
	return held, nil
}
