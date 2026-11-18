// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The standing send override against real rows.
//
// communication_override is a rep's own vouch that a machine-level refusal may
// be overruled for one subject — personal data about that subject, so it must
// be reached by the same acts communication_suppression already is: the Art. 17
// cascade (contact-keyed and lead-twin-keyed), the retention sweep (which
// cannot detach an override the way it detaches a suppression, because the
// table has no address column to detach onto), and the Art. 15 export. Each
// case here mirrors an existing suppression test one statement over, proving
// the table this task added is not a standing "allow" that outlives the
// subject it was written about — a leftover vouch on an erased subject is a
// live privacy defect, not a cosmetic gap.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedOverride writes one live standing override, naming the subject by
// contact or lead id.
func seedOverride(ctx context.Context, t *testing.T, conn execer, contact, lead *ids.UUID) {
	t.Helper()
	mustExec(ctx, t, conn, `
		INSERT INTO communication_override (contact_id, lead_id, category, reason, decided_by_level, captured_by, legal_entity_id)
		VALUES ($1, $2, 'marketing', 'vouched for on the call', 'user', 'user:test', (SELECT id FROM legal_entity WHERE is_default))`,
		contact, lead)
}

// countOverrides reads how many communication_override rows still name the
// given contact or lead.
func countOverrides(ctx context.Context, t *testing.T, conn execer, column string, id ids.UUID) int {
	t.Helper()
	q, ok := conn.(interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	})
	if !ok {
		t.Fatalf("connection cannot QueryRow")
	}
	var n int
	if err := q.QueryRow(ctx,
		`SELECT count(*) FROM communication_override WHERE `+column+` = $1`, id).Scan(&n); err != nil {
		t.Fatalf("counting overrides: %v", err)
	}
	return n
}

// TestArt17ErasureDeletesTheSubjectsOverride mirrors what deleteConsentCapabilities
// already proves for communication_suppression: erasure destroys the standing
// vouch in the same transaction, because it carries no address column to keep
// it useful once the contact row is gone — unlike the suppression it sits
// beside, a leftover override could go on authorising sends to nobody's mailbox
// in particular but the erased subject's own.
func TestArt17ErasureDeletesTheSubjectsOverride(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedOverride(e.ctx, t, e.owner, &e.contact.UUID, nil)

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	if n := countOverrides(e.ctx, t, e.owner, "contact_id", e.contact.UUID); n != 0 {
		t.Errorf("%d override row(s) still name the erased contact — a standing 'allow' "+
			"survived the request that was supposed to remove every capability naming them", n)
	}
}

// TestArt17ErasureDeletesTheLeadTwinsOverride mirrors anonymizeLeadTwins's own
// suppression delete: an override written while the recipient existed only as
// a lead carries lead_id and no contact_id, so a contact-keyed statement alone
// cannot see it.
func TestArt17ErasureDeletesTheLeadTwinsOverride(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	lead := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
	mustExec(ctx, t, tx,
		`INSERT INTO lead (id, full_name, source, captured_by, promoted_contact_id)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text, $3)`, lead, user, contact)
	seedOverride(ctx, t, tx, nil, &lead)

	if _, err := anonymizeLeadTwins(ctx, tx, contact, nil); err != nil {
		t.Fatalf("anonymizing the lead twins: %v", err)
	}

	if n := countOverrides(ctx, t, tx, "lead_id", lead); n != 0 {
		t.Errorf("%d override row(s) still name the erased lead twin — the contact-keyed "+
			"delete cannot see a row carrying only lead_id, and this sweep is what has to", n)
	}
}

// TestRetentionAnonymizeDeletesTheContactsOverride mirrors clearCommunicationRecord's
// own suppression handling — DETACHED there, because an anonymized subject may
// lawfully return and a deleted objection would let them return mailable. An
// override cannot be carried the same way: there is no address to detach it
// onto, and a subject who returns arrives as a new record with nobody yet
// vouching for them, so it is deleted outright.
func TestRetentionAnonymizeDeletesTheContactsOverride(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
	seedOverride(ctx, t, tx, &contact.UUID, nil)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	if n := countOverrides(ctx, t, tx, "contact_id", contact.UUID); n != 0 {
		t.Errorf("%d override row(s) still name the anonymized contact — an override has no "+
			"address to survive on, unlike the suppression beside it, so it must be deleted "+
			"outright rather than merely detached", n)
	}
}

// TestRetentionAnonymizeDeletesTheLeadsOverride is clearLeadCommunicationRecord's
// own case: a lead nobody was promoted from, reached by the lead retention
// action rather than the contact-driven twin sweep.
func TestRetentionAnonymizeDeletesTheLeadsOverride(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	lead := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO lead (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, lead, user)
	seedOverride(ctx, t, tx, nil, &lead)

	if err := (&RetentionService{}).anonymizeLead(ctx, tx, lead); err != nil {
		t.Fatalf("anonymizing the lead: %v", err)
	}

	if n := countOverrides(ctx, t, tx, "lead_id", lead); n != 0 {
		t.Errorf("%d override row(s) still name the anonymized lead — the lead retention "+
			"action's own communication clear must reach a table with no address to detach "+
			"onto by deleting it outright", n)
	}
}

// TestTheExportCarriesTheSubjectsOverride mirrors sarCommunicationSections's own
// suppression read: Art. 15 owes the subject the record that a human decided
// to write to them despite a machine refusal, and why.
func TestTheExportCarriesTheSubjectsOverride(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedOverride(e.ctx, t, e.owner, &e.contact.UUID, nil)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.CommunicationOverrides) == 0 {
		t.Fatal("the export carries no override at all, though this subject has a standing " +
			"vouch on file — a subject asking why they received something the installation " +
			"had refused is owed the override as much as the refusal")
	}
	row := pkg.CommunicationOverrides[0]
	if row["category"] != "marketing" || row["reason"] != "vouched for on the call" {
		t.Errorf("the exported override does not carry the recorded category/reason: %+v", row)
	}
}

// TestArt17ErasureDeletesAMergedPredecessorsOverride holds the other half of a
// merge: the survivor was given a copy, and the predecessor kept the original
// as evidence. Erasing the survivor that leaves the predecessor's vouch behind
// leaves the subject's own record of being vouched for.
func TestArt17ErasureDeletesAMergedPredecessorsOverride(t *testing.T) {
	e := setupSARIdentifiers(t)
	predecessor := seedMergedPredecessor(e.ctx, t, e, e.contact)
	seedOverride(e.ctx, t, e.owner, &predecessor.UUID, nil)

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	if n := countOverrides(e.ctx, t, e.owner, "contact_id", predecessor.UUID); n != 0 {
		t.Errorf("%d override row(s) survived on the merged-away predecessor of an erased subject", n)
	}
}

// TestArt17ErasureDeletesAMergedPredecessorsSuppression is the suppression twin:
// a stop and a vouch are the same statement about the same contact.
func TestArt17ErasureDeletesAMergedPredecessorsSuppression(t *testing.T) {
	e := setupSARIdentifiers(t)
	predecessor := seedMergedPredecessor(e.ctx, t, e, e.contact)
	mustExec(e.ctx, t, e.owner, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject')`, predecessor)

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	var n int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, predecessor).Scan(&n); err != nil {
		t.Fatalf("counting the predecessor's suppressions: %v", err)
	}
	if n != 0 {
		t.Errorf("%d suppression row(s) survived on the merged-away predecessor of an erased subject", n)
	}
}

// TestRetentionAnonymizeDeletesAMergedPredecessorsOverride holds the retention
// sweep to the same reach as the eraser.
func TestRetentionAnonymizeDeletesAMergedPredecessorsOverride(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	predecessor := ids.New[ids.ContactKind]()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by, merged_into_id, archived_at)
		 VALUES ($1, 'Hedda Duplicate', 'manual', 'user:'||$2::text, $3, now())`, predecessor, user, contact)
	seedOverride(ctx, t, tx, &predecessor.UUID, nil)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	if n := countOverrides(ctx, t, tx, "contact_id", predecessor.UUID); n != 0 {
		t.Errorf("%d override row(s) survived on the merged-away predecessor of an anonymized contact", n)
	}
}

// TestRetentionAnonymizeKeepsAMergedPredecessorsSuppression is the stop twin
// of the override case above, with the opposite outcome. An override goes. A
// stop is pinned to the hash of the subject's address, so it never expires.
func TestRetentionAnonymizeKeepsAMergedPredecessorsSuppression(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	predecessor := ids.New[ids.ContactKind]()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
	mustExec(ctx, t, tx,
		`INSERT INTO contact_email (contact_id, email, source, captured_by)
		 VALUES ($1, 'hedda@anon.test', 'manual', 'user:'||$2::text)`, contact, user)
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by, merged_into_id, archived_at)
		 VALUES ($1, 'Hedda Duplicate', 'manual', 'user:'||$2::text, $3, now())`, predecessor, user, contact)
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject')`, predecessor)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	var onPredecessor, onAddress int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE contact_id = $1),
		       count(*) FILTER (WHERE contact_id IS NULL AND address_hash = $2 AND revoked_at IS NULL)
		  FROM communication_suppression`, predecessor, storekit.SuppressionHash("hedda@anon.test")).Scan(&onPredecessor, &onAddress); err != nil {
		t.Fatalf("counting the predecessor's suppressions: %v", err)
	}
	if onPredecessor != 0 || onAddress != 1 {
		t.Errorf("after the anonymize %d stop(s) name the predecessor and %d stop the address, want 0 and 1",
			onPredecessor, onAddress)
	}
}

// A lifted stop stops nothing. The anonymize keeps it on the record as
// evidence, with no address, and copies only the active stops onto addresses.
func TestRetentionAnonymizePinsOnlyActiveStops(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
	// Addresses of this test alone: the count below reads the whole table, and
	// another test may leave a stop on a shared address.
	work, home := "hedda-"+contact.String()+"@anon.test", "hedda.home-"+contact.String()+"@anon.test"
	for _, address := range []string{work, home} {
		mustExec(ctx, t, tx,
			`INSERT INTO contact_email (contact_id, email, source, captured_by)
			 VALUES ($1, $2, 'manual', 'user:'||$3::text)`, contact, address, user)
	}
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level, revoked_at)
		VALUES ($1, 'subject_request', 'operator_ui', 'human:x', 'user', now()),
		       ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject', NULL)`, contact)
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, address, kind, source, captured_by, decided_by_level, revoked_at)
		VALUES ($1, $2, 'subject_request', 'operator_ui', 'human:x', 'user', now())`, contact, work)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	var liveOnAddresses, liftedWithAddress, liftedKept int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE revoked_at IS NULL AND contact_id IS NULL AND address_hash = ANY($2)),
		       count(*) FILTER (WHERE revoked_at IS NOT NULL AND address IN ($3, $4)),
		       count(*) FILTER (WHERE revoked_at IS NOT NULL AND contact_id = $1)
		  FROM communication_suppression`, contact,
		[]string{storekit.SuppressionHash(work), storekit.SuppressionHash(home)}, work, home).Scan(&liveOnAddresses, &liftedWithAddress, &liftedKept); err != nil {
		t.Fatalf("counting the stops: %v", err)
	}
	if liveOnAddresses != 2 || liftedWithAddress != 0 || liftedKept != 2 {
		t.Errorf("after the anonymize: %d live stops on addresses (want 2), %d lifted stops holding an address (want 0), "+
			"%d lifted stops kept on the record (want 2)", liveOnAddresses, liftedWithAddress, liftedKept)
	}
}
