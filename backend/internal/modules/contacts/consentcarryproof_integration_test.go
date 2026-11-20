// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Question 4 of the consent carry — whether the retiring record's
// consent_event proof rows move with the state — is the one place the three
// carries genuinely differ, and before this gate the difference lived in three
// prose comments in three files. A reader could only find it by comparing them
// side by side, and a fix applied to two of the three would have been a
// lawful-processing bug nothing failed on.
//
// So each carry's rule is asserted against real rows, and in BOTH directions:
// reversing rehomeProof in consentCarries fails here, and reversing the
// expectation below fails against the database. The corpus is derived from
// consentCarries rather than written out, so a fourth carry cannot be added
// without declaring what it does with the proof.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// proofRehomesExpected is what each carry must DO with the proof rows, written
// independently of the spec production reads so the two can disagree.
//
// TRUE for both merges: the delivery gate reads the double-opt-in proof off
// the LIVE record, so a grant that moved while its confirmation stayed on the
// archived one would be a grant nobody can act on.
//
// FALSE for promotion: the lead-scoped events are the evidence that the
// consent predates the promotion. Re-keying them to the contact destroys the
// only record of when it was given and to whom. A confirmed double opt-in
// proof is copied, not moved (TestAPromotionCopiesTheConfirmedProofOfACarriedGrant).
var proofRehomesExpected = map[consentCarryKind]bool{
	consentCarryContactMerge:  true,
	consentCarryLeadMerge:     true,
	consentCarryLeadPromotion: false,
}

func TestEachConsentCarryProvesItsProofRule(t *testing.T) {
	e := setupPromoteConsent(t)
	if len(proofRehomesExpected) != len(consentCarries) {
		t.Fatalf("this gate expects %d carr(ies) and the package declares %d: a carry nobody asserts "+
			"chooses its proof rule silently", len(proofRehomesExpected), len(consentCarries))
	}
	for kind, spec := range consentCarries {
		want, declared := proofRehomesExpected[kind]
		if !declared {
			t.Fatalf("%s is declared in consentCarries but not here: say what it does with the proof rows", spec.name)
		}
		t.Run(spec.name, func(t *testing.T) {
			e.assertProofRule(t, kind, spec, want)
		})
	}
}

// assertProofRule seeds one retiring record with a granted consent and its
// proof event, runs the carry, and reads where the proof ended up.
func (e *promoteConsentEnv) assertProofRule(t *testing.T, kind consentCarryKind, spec consentCarrySpec, wantRehomed bool) {
	t.Helper()
	ctx := context.Background()
	retiring := e.seedConsentSubject(t, spec.from)
	survivor := e.seedConsentSubject(t, spec.to)
	e.seedCarriedConsent(t, spec.from, retiring, e.newsletter)

	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Errorf("rolling back the carry: %v", err)
		}
	}()
	if err := carryConsent(ctx, tx, kind, retiring, survivor, "human:gate"); err != nil {
		t.Fatalf("carrying consent for %s: %v", spec.name, err)
	}

	onRetiring := countProof(t, tx, spec.from, retiring)
	onSurvivor := countProof(t, tx, spec.to, survivor)
	if wantRehomed && (onRetiring != 0 || onSurvivor == 0) {
		t.Errorf("%s must carry the proof rows onto the survivor, and left %d on the retiring record "+
			"with %d on the survivor: a grant whose double-opt-in proof stayed behind is a grant the "+
			"delivery gate cannot act on", spec.name, onRetiring, onSurvivor)
	}
	if !wantRehomed && (onRetiring == 0 || onSurvivor != 0) {
		t.Errorf("%s must leave the proof rows where they were written, and left %d on the retiring "+
			"record with %d on the survivor: the retiring record's events are the evidence that the "+
			"consent predates the carry", spec.name, onRetiring, onSurvivor)
	}
	// The state itself moves either way; a proof rule that also lost the state
	// would satisfy the counts above while carrying nothing.
	if state, found := e.consentRow(t, string(spec.to), survivor, e.newsletter); !found || state != "granted" {
		t.Errorf("%s left the survivor without the carried state (found=%v state=%q)", spec.name, found, state)
	}
}

// A lead's confirmed double opt-in grant must still authorize mail after
// promotion. The gate reads the proof off the contact, so the carry copies it.
// The lead's own events stay in place.
func TestAPromotionCopiesTheConfirmedProofOfACarriedGrant(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	lead := e.seedConsentSubject(t, consentSubjectLead)
	contact := e.seedConsentSubject(t, consentSubjectContact)
	e.seedCarriedConsent(t, consentSubjectLead, lead, e.newsletter)
	var textVersion, request ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_text_version (key, body, version) VALUES ('carry-proof-'||$1::text, 'Seeded question', 'v1')
		RETURNING id`, lead).Scan(&textVersion); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_qualifying_event (lead_id, kind, note, occurred_at, captured_by, source)
		VALUES ($1, 'requested_by_subject', 'asked to subscribe', now(), 'human:x', 'captured')
		RETURNING id`, lead).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO consent_event (lead_id, purpose_id, new_state, source, policy_text, policy_version,
		                           double_opt_in_confirmed_at, captured_at, captured_by, issuance_trigger,
		                           consent_text_version_id, subject_request_id)
		VALUES ($1, $2, 'granted', 'consent_link', 'seeded wording', 'v1', now(), now(), 'human:x', 'confirm_link', $3, $4)`,
		lead, e.newsletter, textVersion, request); err != nil {
		t.Fatal(err)
	}
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Errorf("rolling back the carry: %v", err)
		}
	}()
	if err := carryConsent(ctx, tx, consentCarryLeadPromotion, lead, contact, "human:gate"); err != nil {
		t.Fatal(err)
	}
	var confirmed int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM consent_event WHERE contact_id = $1 AND purpose_id = $2
		   AND double_opt_in_confirmed_at IS NOT NULL AND issuance_trigger IS NOT NULL
		   AND consent_text_version_id = $3 AND subject_request_id = $4`,
		contact, e.newsletter, textVersion, request).Scan(&confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed != 1 || countProof(t, tx, consentSubjectLead, lead) == 0 {
		t.Errorf("contact holds %d proofs naming the lead's text version and request, lead keeps %d events; want 1 copied and the lead's left in place",
			confirmed, countProof(t, tx, consentSubjectLead, lead))
	}
}

// seedConsentSubject writes an empty record of the kind the column names and
// returns its id.
func (e *promoteConsentEnv) seedConsentSubject(t *testing.T, column consentSubject) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	statement := `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Pat Contact', 'manual', 'human:x')`
	if column == consentSubjectLead {
		// The email is unique per lead: two leads seeded for one carry would
		// otherwise collide on the dedupe key rather than on the rule under test.
		statement = `INSERT INTO lead (id, full_name, email, status, source, captured_by)
			VALUES ($1, 'Lena Lead', $2, 'contacted', 'inbound', 'human:x')`
		if _, err := e.owner.Exec(context.Background(), statement, id, "carry-"+id.String()+"@gate.test"); err != nil {
			t.Fatal(err)
		}
		return id
	}
	if _, err := e.owner.Exec(context.Background(), statement, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedCarriedConsent gives the record a granted state and the proof event that
// backs it — the pair the carry has to keep together or deliberately separate.
func (e *promoteConsentEnv) seedCarriedConsent(t *testing.T, column consentSubject, subject, purpose ids.UUID) {
	t.Helper()
	now := clocktest.Now(t)
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO contact_consent (`+string(column)+`, purpose_id, state, captured_at, source)
		 VALUES ($1, $2, 'granted', $3, 'form')`, subject, purpose, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO consent_event (`+string(column)+`, purpose_id, new_state, source, policy_text, policy_version, captured_at, captured_by)
		 VALUES ($1, $2, 'granted', 'form', 'seeded wording', 'v1', $3, 'human:x')`,
		subject, purpose, now); err != nil {
		t.Fatal(err)
	}
}

// countProof counts the proof rows keyed to one record, inside the carry's own
// transaction so the reads see what the carry did rather than what committed.
func countProof(t *testing.T, tx pgx.Tx, column consentSubject, subject ids.UUID) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_event WHERE `+string(column)+` = $1`, subject).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A withdrawal that flips the survivor's grant keeps the addresses it was bound
// to, because the retiring record's subject withdrew. Two withdrawals of one
// purpose keep the addresses of both.
//
// Mutation: drop recorded_for_addresses from the flip, or the joined update
// from dropCollidingConsent, and its case fails.
func TestACarriedWithdrawalKeepsTheAddressesItWasBoundTo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		survivor string
		want     []string
	}{
		{"it flips the survivor's grant", "granted", []string{"retiring@carry.test"}},
		{"the survivor withdrew too", "withdrawn", []string{"retiring@carry.test", "survivor@carry.test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			ctx := context.Background()
			retiring := e.seedConsentSubject(t, consentSubjectContact)
			survivor := e.seedConsentSubject(t, consentSubjectContact)
			for _, row := range []struct {
				subject ids.UUID
				state   string
				address string
			}{
				{retiring, "withdrawn", "retiring@carry.test"},
				{survivor, tc.survivor, "survivor@carry.test"},
			} {
				if _, err := e.owner.Exec(ctx, `
					INSERT INTO contact_consent (contact_id, purpose_id, state, captured_at, source, recorded_for_addresses)
					VALUES ($1, $2, $3, now(), 'form', ARRAY[$4::text])`,
					row.subject, e.newsletter, row.state, row.address); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := e.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
					t.Errorf("rolling back the carry: %v", err)
				}
			}()
			if err := carryConsent(ctx, tx, consentCarryContactMerge, retiring, survivor, "human:gate"); err != nil {
				t.Fatalf("carrying consent: %v", err)
			}
			var bound []string
			if err := tx.QueryRow(ctx, `
				SELECT recorded_for_addresses FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2`,
				survivor, e.newsletter).Scan(&bound); err != nil {
				t.Fatalf("reading the survivor's withdrawal: %v", err)
			}
			if !slices.Equal(bound, tc.want) {
				t.Errorf("the survivor's withdrawal is bound to %v, want %v", bound, tc.want)
			}
		})
	}
}

// A lead's withdrawal that flips the promoted contact's grant writes a proof row
// on the contact. It keeps the withdrawal's retention floor, so retention cannot
// reach the contact sooner than the lead whose subject refused.
func TestACarriedWithdrawalKeepsItsRetentionFloor(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	lead := e.seedConsentSubject(t, consentSubjectLead)
	contact := e.seedConsentSubject(t, consentSubjectContact)
	e.seedCarriedConsent(t, consentSubjectContact, contact, e.newsletter)
	kept := clocktest.Now(t).Add(366 * 24 * time.Hour)
	if _, err := e.owner.Exec(ctx, `
		WITH state AS (
		  INSERT INTO contact_consent (lead_id, purpose_id, state, captured_at, source)
		  VALUES ($1, $2, 'withdrawn', now(), 'unsubscribe_link'))
		INSERT INTO consent_event (lead_id, purpose_id, new_state, source, captured_at, captured_by, kept_until)
		VALUES ($1, $2, 'withdrawn', 'unsubscribe_link', now(), 'subject', $3)`,
		lead, e.newsletter, kept); err != nil {
		t.Fatal(err)
	}
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Errorf("rolling back the carry: %v", err)
		}
	}()
	if err := carryConsent(ctx, tx, consentCarryLeadPromotion, lead, contact, "human:gate"); err != nil {
		t.Fatalf("carrying consent: %v", err)
	}
	var carried *time.Time
	if err := tx.QueryRow(ctx, `SELECT kept_until FROM consent_event
		WHERE contact_id = $1 AND new_state = 'withdrawn'`, contact).Scan(&carried); err != nil {
		t.Fatalf("reading the carried withdrawal: %v", err)
	}
	if carried == nil || !carried.Equal(kept) {
		t.Errorf("the carried withdrawal is kept until %v, want the lead's %v", carried, kept)
	}
}
