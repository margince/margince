// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The capture auto-create engine (ADR-0063): mail names a counterparty, and
// this ensures a contact — and, unless suppressed, their company and the
// employment edge — exists for it, all through the ONE dedupe chokepoint
// (PO-F-1/PO-F-2) in one transaction (the §9 single-tx exception: contact +
// company + relationship + link are one atomic decision here).
// Exact match reuses; fuzzy CREATES ANYWAY and records a dedupe_candidate
// for the review queue (capture never blocks on a human,
// DEDUPE_FUZZY_AUTOMERGE is pinned never); no match creates.
//
// A connector-created row belongs to the MAILBOX OWNER until something judges
// its sender. Customer identity is shared across a workspace once somebody has
// decided the sender is a customer — before that, a mailbox with a year of
// history names a lawyer, a doctor and a school, and one email is not a reason
// to publish any of them. The verdict path ensures the same address without
// asking for owner scope, and that promotes the record. The audience of the
// correspondence itself is a separate question, decided per activity
// (platform/auth ActivityContentClause).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The repeated storage vocabulary of this engine, named once.
const (
	evidenceFieldKey  = "field"
	evidenceLeftKey   = "left_value"
	evidenceRightKey  = "right_value"
	evidenceScoreKey  = "score"
	evidenceSignalKey = "signal"
	// The fuzzy tier's evidence signals. "collide" is observable downstream:
	// the review queue renders the value as a row data attribute its
	// stylesheet selects on, so the string reaches the UI even though the
	// contract types the field as a bare string and nothing gates a rename.
	evidenceSignalCollide  = "collide"
	evidenceSignalOneSided = "one_sided"

	entityContact    = "contact"
	entityCompany    = "company"
	fieldFullName    = "full_name"
	fieldDisplayName = "display_name"
	fieldEmail       = "email"
	fieldPhone       = "phone"
	emailTypeWork    = "work"
)

// ErrCounterpartySuppressed marks an erased address (A13): deletion sticks,
// the counterparty is not re-created. The capture pipeline counts it as a
// deliberate skip.
var ErrCounterpartySuppressed = errors.New("contacts: counterparty address is on the erasure suppression list")

// EnsureCounterpartyInput is one captured message's counterparty.
type EnsureCounterpartyInput struct {
	// Replied says this counterparty wrote to US, rather than merely having
	// been written to. It decides which acquisition the created record
	// carries, and the two are different facts about the contact.
	Replied     bool
	Email       string // required; lowercased here
	DisplayName string // header display name — untrusted text
	Domain      string // lowercased mail domain

	OwnerID    ids.UUID       // the connecting human — owner of created rows
	ActivityID ids.ActivityID // the captured activity to link (contact-only)
	Source     string         // provenance channel, e.g. "gmail:<message-id>"
	CapturedBy string         // "connector:<name>"

	// SuppressCompany skips company derivation (free-mail counterparty): the
	// contact is still created — alice@gmail.com is a contact, "Gmail" is
	// not her employer.
	SuppressCompany bool

	// OwnerScoped births the contact visible to their OWNER alone rather than to
	// the workspace.
	//
	// It is what the two ensure callers disagree about. The capture sink mints a
	// contact from a message nothing has judged yet — connecting a mailbox with a
	// year of history would otherwise put every correspondent, a lawyer and a
	// doctor included, in front of every colleague on the strength of one email.
	// The VERDICT path mints one because something judged the sender a business
	// counterparty, which is the moment the record becomes the workspace's.
	//
	// False is the workspace default, so a caller that says nothing gets the
	// behaviour that existed before this field. The wrong direction to fail in
	// is silently narrowing a record somebody expected to see.
	OwnerScoped bool
}

// EnsureCounterpartyResult reports what the ensure did — every flag maps to
// rows the caller can count honestly.
type EnsureCounterpartyResult struct {
	ContactID      ids.ContactID
	ContactCreated bool
	// CompanyID is the company this counterparty was ATTACHED to, never
	// one this ensure made: capture no longer creates companies at all, so
	// there is no created-flag to report. A domain with no company yet reports
	// TriagePending instead.
	CompanyID      *ids.CompanyID
	DedupeRecorded bool
	// NameFilled reports that this ensure completed an incumbent's split name
	// that was previously unknown — the fill-only-if-empty path, never an
	// overwrite. Counting it separately keeps "created a contact" honest.
	NameFilled bool

	// TriagePending reports that this ensure OPENED a domain's company
	// question, and TriageDomain names the domain to ask about. A later message
	// that merely finds the question still open reports nothing: it is the same
	// question, it needs no second crawl, and counting it again would report a
	// hundred companies for the one domain a backfill actually met. The ensure only
	// RECORDS the question — enqueueing the crawl that answers it belongs to
	// compose, after this transaction commits, so a queue outage can never cost
	// the message that raised it. The sweep re-finds anything a missed trigger
	// dropped, so a lost signal costs latency and nothing else.
	TriagePending bool
	TriageDomain  string
}

// EnsureCounterparty resolves-or-creates the contact (and company) behind one
// captured message and links the activity to the contact. Idempotent by
// construction: the exact tier lands repeats on the same row, and the link
// insert is conflict-free on replay.
func (s *Store) EnsureCounterparty(ctx context.Context, in EnsureCounterpartyInput) (EnsureCounterpartyResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" {
		return EnsureCounterpartyResult{}, errors.New("contacts: a counterparty needs an email")
	}
	var res EnsureCounterpartyResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		res, err = s.EnsureCounterpartyTx(ctx, tx, in)
		return err
	})
	if err != nil {
		return EnsureCounterpartyResult{}, err
	}
	return res, nil
}

// EnsureCounterpartyTx is the same resolve-or-create on a transaction the CALLER
// owns, for the paths that must commit records together with the decision that
// authorized them — the ADR-0072 verdict engine resolving a deferred
// disposition, and the review-queue accept that redeems a staged proposal.
// Neither may leave a ledger row reading `real` while the records it promised
// rolled back, so neither can use the pool-owning form above.
//
// The caller is responsible for the workspace GUC; it is already set by the
// WithWorkspaceTx that produced tx.
func (s *Store) EnsureCounterpartyTx(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput) (EnsureCounterpartyResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" {
		return EnsureCounterpartyResult{}, errors.New("contacts: a counterparty needs an email")
	}
	var res EnsureCounterpartyResult
	suppressed, err := storekit.EmailSuppressed(ctx, tx, in.Email)
	if err != nil {
		return EnsureCounterpartyResult{}, err
	}
	if suppressed {
		return EnsureCounterpartyResult{}, ErrCounterpartySuppressed
	}
	if err := s.ensureContact(ctx, tx, in, &res); err != nil {
		return EnsureCounterpartyResult{}, err
	}
	if !in.SuppressCompany && in.Domain != "" {
		if err := s.ensureCompanyAndEmployment(ctx, tx, in, &res); err != nil {
			return EnsureCounterpartyResult{}, err
		}
	}
	if err := s.linkActivityToContact(ctx, tx, in.ActivityID, res.ContactID); err != nil {
		return EnsureCounterpartyResult{}, err
	}
	return res, nil
}

// ensureContact runs PO-F-1 and creates when it does not exactly match; a
// fuzzy hit creates AND records the pair for the review queue.
func (s *Store) ensureContact(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, res *EnsureCounterpartyResult) error {
	if err := auth.Require(ctx, entityContact, principal.ActionCreate); err != nil {
		return err
	}
	parsed, err := s.nameCounterparty(ctx, tx, in)
	if err != nil {
		return err
	}
	name := parsed.Full
	// The workspace's own consumer-mail list travels with the candidate: the
	// employer-agreement term must judge a shared domain the same way capture
	// does, or an admin's carve-out would hold on one side and not the other.
	consumerMail, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return err
	}
	match, err := DedupeContact(ctx, tx, ContactCandidate{
		FullName: name, Emails: []string{in.Email}, ConsumerMail: consumerMail,
	})
	if err != nil {
		return err
	}
	if match.Decision == DecisionExactCollision {
		res.ContactID = match.ContactID
		// A workspace-scoped ensure over an owner-scoped record PROMOTES it.
		//
		// This is how a contact the sink minted while its sender was unjudged
		// becomes the workspace's: the verdict path ensures the same address
		// with OwnerScoped false, and that is the decision. It runs before the
		// quarantine return below, because an impersonation tell is a reason to
		// learn nothing NEW from this message and not a reason to withhold a
		// promotion something already decided.
		if err := promoteIfWorkspaceScoped(ctx, tx, match.ContactID, in.OwnerScoped); err != nil {
			return err
		}
		if quarantineSuspect(in.DisplayName, in.Domain) {
			// The header carries an impersonation tell. A new record would be
			// created quarantined for review; an EXISTING one has no such
			// label to wear, so the only safe answer is to learn nothing from
			// this message. The activity is still captured either way.
			return nil
		}
		return fillMissingContactName(ctx, tx, match.ContactID, parsed, res)
	}

	id, err := createContact(ctx, tx, match, ContactSpec{
		FullName:    name,
		FirstName:   nameColumn(parsed.First),
		LastName:    nameColumn(parsed.Last),
		OwnerID:     ownerFromUUID(&in.OwnerID),
		Visibility:  visibilityFor(in.OwnerScoped),
		Quarantined: quarantineSuspect(in.DisplayName, in.Domain),
		Emails:      []ContactEmailInput{{Email: in.Email, EmailType: emailTypeWork, IsPrimary: true}},
		Source:      in.Source,
		CapturedBy:  in.CapturedBy,
		// Only a REPLY is the contact initiating contact. Capture also mints a
		// record for somebody we wrote to twice who never answered, and
		// recording that as subject_initiated would put the vocabulary's
		// strongest claim on a cold prospect's file — the exact confusion this
		// table exists to prevent, manufactured by the table itself.
		Acquisition: Acquisition{Kind: acquiredFromCapture(in.Replied)},
	})
	if err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "create", entityContact, id.UUID, nil, map[string]any{fieldFullName: name})
	if err != nil {
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactCreated{FullName: name}); err != nil {
		return err
	}
	res.ContactID = id
	res.ContactCreated = true

	if match.Decision == DecisionFuzzyReview {
		// The detection-time snapshot the queue renders (DH-N-8): captured
		// NOW, against the incumbent as it looked when the score was
		// computed — never re-derived later.
		var incumbentName string
		if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, match.ContactID).Scan(&incumbentName); err != nil {
			return fmt.Errorf("contacts: reading dedupe incumbent: %w", err)
		}
		evidence := []map[string]any{
			{evidenceFieldKey: fieldFullName, evidenceLeftKey: name, evidenceRightKey: incumbentName, evidenceSignalKey: evidenceSignalCollide, evidenceScoreKey: match.Confidence},
			{evidenceFieldKey: fieldEmail, evidenceLeftKey: in.Email, evidenceRightKey: nil, evidenceSignalKey: evidenceSignalOneSided},
		}
		recorded, err := recordDedupeCandidate(ctx, tx, entityContact, id.UUID, match.ContactID.UUID, match.Confidence,
			evidence, in.Source, in.CapturedBy)
		if err != nil {
			return err
		}
		res.DedupeRecorded = recorded
	}
	return nil
}

// ensureCompanyAndEmployment decides what this mail domain may create, and creates
// only that. It runs PO-F-2 on the domain, and where the domain is not yet
// understood it creates NOTHING and opens the question instead — the contact
// already exists, and a company invented from a domain label is the
// defect this ladder exists to stop.
//
// The order is load-bearing at every step:
//
//	a suppressed domain                     → no company, no question, ever
//	a company already on this domain  → attach; a human's row always wins
//	consumer mail                           → no company, and no question to ask
//	a settled verdict                       → obey it
//	anything else                           → open the question, create nothing
func (s *Store) ensureCompanyAndEmployment(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, res *EnsureCounterpartyResult) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionCreate); err != nil {
		return err
	}
	// A From: header is forgeable and net/mail parses far more loosely than DNS
	// allows, so a counterparty "domain" may be any atext string — `jane@%`
	// parses. Nothing that is not a hostname may become a company, a crawl seed,
	// or a key in a query: the contact still lands, with no employer.
	base, ok := freemail.Hostname(in.Domain)
	if !ok {
		return nil
	}
	// The standing refusal runs FIRST, ahead of the dedupe attach.
	//
	// A vendor the business merely uses — an expense tool, a ticket shop — has a
	// real corporate website, so every evidence test the triage can run says
	// "company". The refusal is therefore a decision about the DOMAIN, and it
	// has to be consulted before anything can attach a contact to a company on
	// it: consulted later, a named employee writing from that domain would find
	// the company a previous message created and quietly re-employ
	// everyone onto it.
	suppressed, err := domainSuppressedTx(ctx, tx, base)
	if err != nil {
		return err
	}
	if suppressed {
		return nil
	}
	match, err := DedupeCompany(ctx, tx, CompanyCandidate{Domains: []string{in.Domain, base}})
	if err != nil {
		return err
	}
	if match.Decision != DecisionExactCollision {
		// No company yet. Whether one may be created is not this path's
		// call any more.
		return s.deferCompanyToTriage(ctx, tx, in, base, res)
	}
	companyID := match.CompanyID
	res.CompanyID = &companyID
	// A human put a company on this domain. That overrides whatever a crawl
	// concluded — including a refusal — and the ledger records it as theirs, so
	// the next message stops re-asking and the trail says who overruled what.
	if err := adoptDispositionForCompany(ctx, tx, base, companyID); err != nil {
		return err
	}

	return plantEmploymentEdge(ctx, tx, in, res.ContactID, companyID)
}

// linkActivityToContact attaches the captured activity to the contact —
// contact-only by decision (the company rolls up through employment, a direct
// company link would double-count the same mail). Shared with the channel ensure,
// which links the same way: it takes the activity id rather than either
// path's input so neither has to know the other's shape.
//
// Being the ONE point both paths reach, it is also where the contact is settled
// against a merge. Every step above it — the dedupe ladder, the identity bind,
// the handle refresh — named its contact from a read that a merge can invalidate
// before this insert runs, and no reader of activity_link walks merged_into_id,
// so a link written to the retired id leaves the message on a record nobody
// opens. Resolving it here covers both callers at once, and it is the last read
// before the write, so nothing can overtake it.
func (s *Store) linkActivityToContact(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, contactID ids.ContactID) error {
	// FOR UPDATE serializes this behind the merge's own LockPair, so the
	// redirect this reads is the committed one; one hop is enough because a
	// merge repoints its source's redirect rather than chaining (merge.go).
	// An archived survivor is still the right subject: the message happened,
	// and this call's caller logs faults rather than failing the capture, so
	// refusing here would drop the link silently rather than loudly.
	var canonical ids.ContactID
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(merged_into_id, id) FROM contact WHERE id = $1 FOR UPDATE`,
		contactID).Scan(&canonical); err != nil {
		return fmt.Errorf("contacts: resolving the contact this activity belongs to: %w", err)
	}
	contactID = canonical
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		SELECT $1, 'contact', $2
		WHERE NOT EXISTS (
			SELECT 1 FROM activity_link WHERE activity_id = $1 AND entity_type = 'contact' AND contact_id = $2)`,
		activityID, contactID); err != nil {
		return fmt.Errorf("contacts: linking activity to contact: %w", err)
	}
	return nameContactAmongParticipants(ctx, tx, activityID, contactID)
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
