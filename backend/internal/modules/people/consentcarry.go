// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The consent carry: what happens to a retiring record's consent when another
// record survives it. Three paths need it — a person merge, a lead merge, and
// a lead's promotion to a person — and they ask the same four questions in the
// same order:
//
//  1. Withdrawal wins. Where both records hold a state for one purpose and the
//     retiring one withdrew, the survivor's grant flips to withdrawn, with an
//     appended consent_event proof row in the same statement. A carry that
//     turned an opt-out back into a grant is a lawful-processing defect, not a
//     data-tidiness one.
//  2. Otherwise the survivor's own state stands, and the colliding row on the
//     retiring record is dropped.
//  3. Everything the survivor has no state for re-points to it.
//  4. The proof rows either follow the state or stay where they were written —
//     and that is the one place the three paths genuinely differ.
//
// They were three copies of one CTE, differing only in a key column and a
// literal, and they had already drifted on question 4 with the difference
// discoverable only by reading all three files side by side. Consent is the
// domain where a fix applied to two of three copies is a lawful-processing
// bug, so there is one implementation and the difference is DECLARED in the
// spec rather than argued in prose. TestEachConsentCarryProvesItsProofRule
// (consentcarryproof_integration_test.go) runs every carry declared here
// against real rows and fails when a declared rule is reversed on either side.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// consentSubject is the column naming whose consent a person_consent or
// consent_event row is. Both values are package constants, so the only
// identifiers this file formats into SQL are compile-time literals.
type consentSubject string

const (
	consentSubjectPerson consentSubject = "person_id"
	consentSubjectLead   consentSubject = "lead_id"
)

// The carry's own source vocabulary, written to consent_event.source and
// consent_event.policy_version so an audit reading the proof row alone can
// tell which act moved the state.
const (
	consentCarrySourceMerge     = "merge"
	consentCarrySourcePromotion = "promotion"
)

// consentCarryKind names each carry this package performs. Declaring them as
// a closed set is what lets the gate derive its corpus from production: a
// fourth carry has to appear here to exist at all, so it cannot be added
// without the proof rule it chooses being asserted.
type consentCarryKind int

const (
	consentCarryPersonMerge consentCarryKind = iota
	consentCarryLeadMerge
	consentCarryLeadPromotion
)

// consentCarrySpec is one carry's answers to the four questions above.
type consentCarrySpec struct {
	// name is what a reader of an error or a failing gate sees.
	name string
	// from is the retiring record's key column, to the survivor's. They are
	// the same column for a merge and different for a promotion, which is what
	// makes step 3 clear the old key on the way past.
	from, to consentSubject
	// source is written to both consent_event.source and
	// consent_event.policy_version, as the pre-shared copies did.
	source string
	// policyText is the sentence a reader of the proof row sees. Each carry
	// says what actually happened to the record, so an audit reading the event
	// alone can tell a merge from a promotion.
	policyText string
	// rehomeProof decides question 4: whether the retiring record's
	// consent_event rows move to the survivor.
	//
	// TRUE for both merges. The delivery gate reads the double-opt-in proof
	// off the LIVE record, so a grant that moved while its confirmation stayed
	// on the archived one would be a grant nobody can act on.
	//
	// FALSE for promotion, and deliberately. The lead-scoped events ARE the
	// evidence that the consent predates the promotion; re-keying them to the
	// person would destroy the only record of when it was given and to whom.
	//
	// What makes that safe rather than merely principled is that the lead arm
	// cannot hold the one kind of proof the delivery gate reads: a double
	// opt-in grant on a lead subject is REFUSED at the writer, because the
	// round-trip is person-keyed (consent.Store.resolveDOIConfirmation — "promote
	// the lead before granting it"). So there is no DOI confirmation on a lead
	// to strand behind on the archived record. Held by
	// TestLeadScopedDOIGrantIsRefused; if that refusal is ever lifted, this
	// answer to question 4 has to be revisited in the same change.
	rehomeProof bool
}

// consentCarries is every carry this package performs. The three entry points
// read their spec from here rather than each building its own.
//
// Held by: TestTheConsentCarryIsSpelledOnceInPeople (backend/gates/oneconsentcarry_test.go)
var consentCarries = map[consentCarryKind]consentCarrySpec{
	consentCarryPersonMerge: {
		name: "a person merge",
		from: consentSubjectPerson, to: consentSubjectPerson,
		source:      consentCarrySourceMerge,
		policyText:  "withdrawal carried over from a merged duplicate record",
		rehomeProof: true,
	},
	consentCarryLeadMerge: {
		name: "a lead merge",
		from: consentSubjectLead, to: consentSubjectLead,
		source:      consentCarrySourceMerge,
		policyText:  "withdrawal carried over from the merged-away lead",
		rehomeProof: true,
	},
	consentCarryLeadPromotion: {
		name: "a lead promotion",
		from: consentSubjectLead, to: consentSubjectPerson,
		source:      consentCarrySourcePromotion,
		policyText:  "withdrawal carried over from the promoted lead",
		rehomeProof: false,
	},
}

// carryConsent applies one carry from the retiring record to the survivor.
//
// `by` is the authenticated principal the proof row records; every caller has
// already resolved it, because a carry inside a merge writes as whoever asked
// for the merge.
func carryConsent(ctx context.Context, tx pgx.Tx, kind consentCarryKind, fromID, toID ids.UUID, by string) error {
	spec, ok := consentCarries[kind]
	if !ok {
		return fmt.Errorf("no consent carry is declared for kind %d; declare it in consentCarries with the proof rule it chooses", int(kind))
	}
	now := time.Now().UTC()
	if err := flipCarriedWithdrawals(ctx, tx, spec, fromID, toID, by, now); err != nil {
		return err
	}
	if err := dropCollidingConsent(ctx, tx, spec, fromID, toID); err != nil {
		return err
	}
	if err := repointCarriedConsent(ctx, tx, spec, fromID, toID); err != nil {
		return err
	}
	if !spec.rehomeProof {
		return nil
	}
	if _, err := tx.Exec(ctx, storekit.SQLf(
		`UPDATE consent_event SET %[2]s = $2 WHERE %[1]s = $1`, spec.from, spec.to,
	),
		fromID, toID); err != nil {
		return fmt.Errorf("carry the consent proof rows onto the surviving record: %w", err)
	}
	return nil
}

// flipCarriedWithdrawals answers question 1: a withdrawal on the retiring
// record overrides the survivor's grant, and the flip lands with its proof row
// in the same statement — a state change without proof would break the
// Art. 7(1) demonstrability invariant.
func flipCarriedWithdrawals(ctx context.Context, tx pgx.Tx, spec consentCarrySpec,
	fromID, toID ids.UUID, by string, now time.Time,
) error {
	_, err := tx.Exec(ctx, storekit.SQLf(`
		WITH flipped AS (
		  UPDATE person_consent b SET state = 'withdrawn', captured_at = $3, source = $5
		  FROM person_consent a
		  WHERE a.%[1]s = $1 AND b.%[2]s = $2
		    AND a.purpose_id = b.purpose_id
		    AND a.state = 'withdrawn' AND b.state <> 'withdrawn'
		  RETURNING b.purpose_id
		)
		INSERT INTO consent_event (%[2]s, purpose_id, new_state, source,
		                           policy_text, policy_version, captured_at, captured_by)
		SELECT $2, purpose_id, 'withdrawn', $5, $6, $5, $3, $4
		FROM flipped`, spec.from, spec.to),
		fromID, toID, now, by, spec.source, spec.policyText)
	if err != nil {
		return fmt.Errorf("carry withdrawals onto the surviving record: %w", err)
	}
	return nil
}

// dropCollidingConsent answers question 2: where the survivor already holds a
// state for a purpose, its own stands and the retiring record's row goes.
func dropCollidingConsent(ctx context.Context, tx pgx.Tx, spec consentCarrySpec, fromID, toID ids.UUID) error {
	_, err := tx.Exec(ctx, storekit.SQLf(`
		DELETE FROM person_consent a
		WHERE a.%[1]s = $1 AND EXISTS (
		  SELECT 1 FROM person_consent b
		  WHERE b.%[2]s = $2 AND b.purpose_id = a.purpose_id)`, spec.from, spec.to),
		fromID, toID)
	if err != nil {
		return fmt.Errorf("drop the colliding consent rows: %w", err)
	}
	return nil
}

// repointCarriedConsent answers question 3. A carry between two records of the
// same kind only sets the survivor's key; a promotion also CLEARS the lead key,
// so the carried state stops riding the retired lead's lifecycle
// (person_consent.lead_id cascades on lead deletion).
func repointCarriedConsent(ctx context.Context, tx pgx.Tx, spec consentCarrySpec, fromID, toID ids.UUID) error {
	clearOldKey := ""
	if spec.from != spec.to {
		clearOldKey = storekit.SQLf(", %s = NULL", spec.from)
	}
	_, err := tx.Exec(ctx, storekit.SQLf(
		`UPDATE person_consent SET %[2]s = $2%[3]s WHERE %[1]s = $1`, spec.from, spec.to, clearOldKey,
	),
		fromID, toID)
	if err != nil {
		return fmt.Errorf("re-point the carried consent rows: %w", err)
	}
	return nil
}
