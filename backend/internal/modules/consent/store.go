// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type Store struct {
	// db binds the installation's workspace itself (ADR-0091 §9 step 3).
	db  *database.DB
	now func() time.Time
	// installationName answers what to call this installation on the public
	// preference page. Injected because the name lives in identity and a
	// module never imports a sibling; nil on any installation that has not
	// wired it, which the page renders as an omission rather than a blank.
	installationName InstallationNameReader
	// reviewRouter puts a refused send in front of somebody who may direct it.
	// Injected because approvals is a sibling module; nil on an installation
	// with no approvals surface, and routing then refuses rather than staging
	// nothing and reporting success.
	reviewRouter ReviewRouter
	// country selects which jurisdiction's messaging rules a decision is taken
	// under. Injected by compose because the setting lives in identity
	// (installationcountry.go).
	country InstallationCountryReader
	// language is which language the controller mail this store stages is
	// written in. Nil sends in the fallback rather than refusing.
	language MailLanguageReader
	// confirmSender stages the installation's own mail on the durable lane, and
	// vault holds the one-time link so the plaintext never reaches the delivery
	// row. Both nil on an installation that has not wired the lane, which
	// issueLink reports as a link that was minted and not sent — never as a
	// failure, because the token was still spent.
	confirmSender ConfirmationSender
	vault         ConfirmLinkVault
	// publicBaseURL is the canonical origin a confirm link is built on. It lives
	// on the Store rather than on Handlers because the Store is what builds the
	// link now: issueLink seals it into the vault inside its own transaction.
	publicBaseURL string
}

// NewStore binds the store to the pool every read and write runs through.
func NewStore(db *database.DB) *Store {
	return &Store{db: db, now: time.Now}
}

type Purpose struct {
	ID                  ids.PurposeID
	Key                 string
	Label               string
	RequiresDoubleOptIn bool
	CreatedAt           time.Time
}

type State struct {
	PurposeID              ids.PurposeID
	PurposeKey             string
	State                  string
	LawfulBasis            *string
	DoubleOptInConfirmedAt *time.Time
	UpdatedAt              *time.Time
	// Changed says whether this call MOVED the record. False for an
	// idempotent re-assertion and for a capture that declined to override
	// a decision already on file. The preference centre's unsubscribe
	// endpoint reports only what it actually changed, so a recipient who
	// presses the link twice is told the truth the second time rather
	// than being shown a fresh confirmation for a no-op.
	Changed bool
}

type ProofEvent struct {
	// ID is the consent_event proof row's id — an append-only ledger
	// entry, not a first-class entity in the kernel vocabulary, so it
	// stays untyped.
	ID          ids.UUID
	PurposeID   ids.PurposeID
	NewState    string
	LawfulBasis *string
	Source      *string
	CapturedBy  string
	OccurredAt  time.Time
}

// ListPurposes returns the workspace catalog. The catalog is
// config-sized (a handful of rows); the page shape exists for contract
// symmetry, not because anyone paginates it.
func (s *Store) ListPurposes(ctx context.Context) ([]Purpose, error) {
	// READ stays on contact, and only the writes moved to consent_config. The
	// catalog is a vocabulary rather than an admin screen: every seat resolves a
	// purpose_id against it to record consent from the contact page, so gating the
	// read on installation config would 403 the Contact 360 for every rep.
	//
	// The asymmetry is the point. Defining what the workspace may contact contacts
	// for is compliance configuration; knowing what it already defined is part of
	// working a record.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []Purpose
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, key, label, requires_double_opt_in, created_at
			FROM consent_purpose WHERE archived_at IS NULL ORDER BY key`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Purpose
			if err := rows.Scan(&p.ID, &p.Key, &p.Label, &p.RequiresDoubleOptIn, &p.CreatedAt); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// CreatePurpose defines one purpose. Purposes are compliance configuration, and
// consent_config is the object that says so — it replaced a borrowed
// pipeline.create, which let anyone who could add a pipeline stage define the
// vocabulary every outreach decision is judged against.
func (s *Store) CreatePurpose(ctx context.Context, key, label string, requiresDOI bool) (Purpose, error) {
	if err := auth.Require(ctx, "consent_config", principal.ActionCreate); err != nil {
		return Purpose{}, err
	}
	key = normalizedPurposeKey(key)
	if key == "" || strings.TrimSpace(label) == "" {
		return Purpose{}, &ValidationError{Field: "key", Reason: "key and label are required"}
	}
	var p Purpose
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO consent_purpose (key, label, requires_double_opt_in)
			VALUES ($1, $2, $3)
			RETURNING id, key, label, requires_double_opt_in, created_at`,
			key, label, requiresDOI).
			Scan(&p.ID, &p.Key, &p.Label, &p.RequiresDoubleOptIn, &p.CreatedAt)
		if constraint, ok := storekit.UniqueViolation(err); ok && constraint == "consent_purpose_key_unique" {
			return fmt.Errorf("purpose %q: %w", key, apperrors.ErrConflict)
		}
		return err
	})
	return p, err
}

// ContactConsent reads one contact's per-purpose state plus the full
// proof log (Art. 7 demonstrability). The contact is the read target —
// row scope gates the whole answer.
func (s *Store) ContactConsent(ctx context.Context, contactID ids.ContactID) ([]State, []ProofEvent, error) {
	return s.subjectConsent(ctx, subject{entityType: entityContact, column: subjectColumnContact, id: contactID.UUID})
}

// LeadConsent is the lead arm of the same read (E12.20): the per-purpose
// state and proof log a capture surface recorded before promotion.
func (s *Store) LeadConsent(ctx context.Context, leadID ids.LeadID) ([]State, []ProofEvent, error) {
	return s.subjectConsent(ctx, subject{entityType: "lead", column: "lead_id", id: leadID.UUID})
}

// subjectConsent answers either arm — the subject is the read target, so
// its object grant and row scope gate the whole answer.
func (s *Store) subjectConsent(ctx context.Context, sub subject) ([]State, []ProofEvent, error) {
	if err := auth.Require(ctx, sub.entityType, principal.ActionRead); err != nil {
		return nil, nil, err
	}
	var states []State
	var events []ProofEvent
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		states, events, err = subjectConsentInTx(ctx, tx, sub)
		return err
	})
	return states, events, err
}

// ContactConsentTx is ContactConsent inside a caller-opened transaction — the
// composite record read. Same gates in the same order.
func (s *Store) ContactConsentTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]State, []ProofEvent, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	return subjectConsentInTx(ctx, tx, subject{entityType: "contact", column: "contact_id", id: contactID.UUID})
}

// subjectConsentInTx is the shared body of the store-opened and
// caller-opened consent reads.
func subjectConsentInTx(ctx context.Context, tx pgx.Tx, sub subject) ([]State, []ProofEvent, error) {
	if err := auth.EnsureVisible(ctx, tx, sub.entityType, sub.id); err != nil {
		return nil, nil, err
	}
	var states []State
	var events []ProofEvent
	// Every tracked purpose appears — absent rows read as the honest
	// 'unknown', never as an implicit grant.
	rows, err := tx.Query(ctx, `
		SELECT cp.id, cp.key, coalesce(pc.state, 'unknown'), pc.lawful_basis, pc.captured_at
		FROM consent_purpose cp
		LEFT JOIN contact_consent pc ON pc.purpose_id = cp.id AND pc.`+sub.column+` = $1
		WHERE cp.archived_at IS NULL
		ORDER BY cp.key`, sub.id)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var st State
		if err := rows.Scan(&st.PurposeID, &st.PurposeKey, &st.State, &st.LawfulBasis, &st.UpdatedAt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		states = append(states, st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rows, err = tx.Query(ctx, `
		SELECT id, purpose_id, new_state, lawful_basis, source, captured_by, captured_at
		FROM consent_event WHERE `+sub.column+` = $1 ORDER BY captured_at DESC, id DESC`, sub.id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ev ProofEvent
		if err := rows.Scan(&ev.ID, &ev.PurposeID, &ev.NewState, &ev.LawfulBasis, &ev.Source, &ev.CapturedBy, &ev.OccurredAt); err != nil {
			return nil, nil, err
		}
		events = append(events, ev)
	}
	return states, events, rows.Err()
}

type RecordInput struct {
	// ContactID / LeadID name the consent subject — exactly one is set
	// (data-model §7: a public form or LinkedIn capture obtains consent
	// from someone who is still a lead). The DB CHECK only rules out
	// both-null; the XOR is enforced here.
	ContactID   ids.ContactID
	LeadID      ids.LeadID
	PurposeID   ids.PurposeID
	NewState    string // granted | withdrawn
	LawfulBasis *string
	Source      *string
	// MailboxProof names how the caller established that the subject controls
	// the address, for a grant made on a surface the subject reached through a
	// single-use link delivered to it. It is the ONLY way a double-opt-in
	// purpose confirms, and lands on the proof row's issuance_trigger so the
	// chain stays demonstrable.
	//
	// No transport sets this: it is not on the wire, and every handler building
	// a RecordInput from a request body leaves it zero. The one writer is the
	// confirm submit, which sets it only after spending the token that earns it.
	//
	// Held by: TestOnlyTheConfirmSubmitClaimsAProvenMailbox
	// (backend/gates/mailboxproofwriters_test.go) — which fails if any other
	// file sets the field, and fails again if the submit stops spending the
	// token that makes the claim true.
	MailboxProof MailboxProof
	// PolicyText/PolicyVersion carry the CaptureConsent passthrough of a
	// capture surface (feedback/14): the EXACT wording and version shown
	// to the subject, stored verbatim on the proof row (Art 7(1)
	// demonstrability). Nil keeps the API-surface defaults.
	PolicyText    *string
	PolicyVersion *string
	// TextVersionID names the PUBLISHED wording this grant rests on, where the
	// door knows which one the subject read. It is what makes the proof
	// checkable: a reader follows it to the row the controller published rather
	// than trusting policy_text, which on a public door arrived in the same
	// request as the answer it evidences.
	//
	// Zero on every door that cannot say — a link minted before the question
	// was pinned, an API caller stating their own wording — and those rows keep
	// the weaker evidence they always had.
	TextVersionID ids.UUID
	// NeverOverrideExisting is the anonymous-capture rule: a public
	// surface asserting "granted" must not flip a decision already on
	// record — above all a WITHDRAWAL, which an attacker knowing only an
	// email address could otherwise anonymously reverse. When set, an
	// existing different state is left untouched and returned as-is
	// (silently: refusing loudly would make the surface a consent-state
	// oracle).
	NeverOverrideExisting bool
}

// subject is the resolved consent subject: which entity the state and
// proof rows hang on, and which column carries it.
type subject struct {
	entityType string // contact | lead — the RBAC object and the audit/event entity
	column     string // contact_id | lead_id
	id         ids.UUID
}

// consentSubject enforces the exactly-one-subject rule (data-model §7):
// contact XOR lead. The DB CHECK only guards both-null, so both-set and
// neither-set are refused here, before any grant is admitted.
func consentSubject(in RecordInput) (subject, error) {
	contactSet, leadSet := !in.ContactID.IsZero(), !in.LeadID.IsZero()
	switch {
	case contactSet && leadSet:
		return subject{}, &ValidationError{Field: fieldSubject, Reason: "consent takes exactly one subject — a contact or a lead, not both"}
	case contactSet:
		return subject{entityType: entityContact, column: subjectColumnContact, id: in.ContactID.UUID}, nil
	case leadSet:
		return subject{entityType: "lead", column: "lead_id", id: in.LeadID.UUID}, nil
	}
	return subject{}, &ValidationError{Field: "subject", Reason: "consent needs a subject — a contact or a lead"}
}

// admitRecord settles everything decidable before the transaction opens: which
// subject the request is about, that it names a purpose at all, that the caller
// may write consent for that subject, and that the state is one a caller may
// record. Extracted so Record itself is about the write.
//
// The ORDER is the interesting part, and it is deliberate in two places.
//
// The subject comes first because a body naming both a contact and a lead is not a
// well-formed consent request at all, so "which subject" outranks "which
// purpose" — and the authority check cannot even run before it, since which
// object grant applies depends on the answer.
//
// The purpose guard comes before that authority check, which puts every
// input-shape refusal together at the front, the order CreateRelationship
// already uses for an unknown kind. A required field's NAME is published
// contract, so answering it ahead of authority discloses nothing.
func admitRecord(ctx context.Context, in RecordInput) (subject, ConsentState, error) {
	sub, err := consentSubject(in)
	if err != nil {
		return subject{}, "", err
	}
	// purpose_id is required by the contract, which is a claim only a check makes
	// true: an absent key decodes to the zero UUID with no error, and the purpose
	// read inside the transaction would answer not-found for a purpose the caller
	// never named.
	if err := httperr.RequireBodyID(purposeIDField, in.PurposeID.UUID); err != nil {
		return subject{}, "", err
	}
	if err := auth.Require(ctx, sub.entityType, principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	// Returned rather than discarded: Record decides which row probe to run
	// from this value, and re-deriving it there with a string conversion would
	// be a second parse that could disagree with this one.
	state, err := ParseRecordableState(in.NewState)
	if err != nil {
		return subject{}, "", err
	}
	if err := requireRecordableWording(state, in.PolicyText, in.PolicyVersion); err != nil {
		return subject{}, "", err
	}
	return sub, state, nil
}

// Record sets one subject×purpose state and appends the proof row —
// audited (consent_grant/consent_withdraw) and emitted (consent.changed)
// in the same transaction as every other mutation. The subject is a
// contact or, before promotion, a lead (E12.20). Re-asserting the
// current state is idempotent: no second proof row, no second event.
func (s *Store) Record(ctx context.Context, in RecordInput) (State, error) {
	// Admitted before a connection is taken: a malformed subject or a caller
	// without authority is refused without opening a transaction, which is what
	// keeps a bad request off the pool. The write below does not repeat it.
	sub, state, err := admitRecord(ctx, in)
	if err != nil {
		return State{}, err
	}
	var out State
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.recordAdmittedTx(ctx, tx, in, sub, state)
		return err
	})
	return out, err
}

func stateOrUnknown(state string) string {
	if state == "" {
		return "unknown"
	}
	return state
}

// ValidationError maps to a 422 at the transport.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return "consent: " + e.Field + ": " + e.Reason }
