// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What the export says about the subject's own rights requests, and about the
// links that can stop our mail to them.
//
// Both tables held subject data and neither reached the export. A controller
// holding the record of somebody's erasure request holds something about them,
// and a subject who asked to be erased and was refused could not see that the
// refusal existed — the one thing they would need to appeal it.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedRightsCase writes one case the way the public submission door does.
//
// assignee_id is deliberately NOT set: it references a real user row, and
// inventing one to prove the export withholds it would be a fixture fighting a
// foreign key to make a point the column list already settles.
func seedRightsCase(
	ctx context.Context, t *testing.T, owner *pgx.Conn, contact ids.ContactID,
	kind, status, resolution string,
) string {
	t.Helper()
	// RETURNED, so a test reads back the row IT planted. These tests share a
	// database, and matching on kind finds another test's case — which reads as
	// "the erasure did nothing" and is really "you asked about somebody else's
	// row".
	receipt := "RCPT-" + ids.NewV7().String()
	if _, err := owner.Exec(ctx, `
		INSERT INTO data_subject_request
		  (kind, subject_ref, contact_id, status, resolution, received_at, due_at,
		   channel, receipt_reference)
		VALUES ($1, $2, $3, $4, $5, now(), now() + interval '30 days',
		        'confirm_link', $6)`,
		kind, contact.String(), contact.UUID, status, resolution, receipt); err != nil {
		t.Fatalf("seeding the %s case: %v", kind, err)
	}
	return receipt
}

// seedWithdrawalCredential writes one link the way a send path mints it.
//
// token_hash is set because the export must NOT return it. The row holds a hash
// precisely so the plaintext lives only in the mail that carried it.
func seedWithdrawalCredential(
	ctx context.Context, t *testing.T, owner *pgx.Conn, contact ids.ContactID, scope string,
) {
	t.Helper()
	if _, err := owner.Exec(ctx, `
		INSERT INTO withdrawal_credential
		  (token_hash, address, contact_id, scope, issued_at, expires_at)
		VALUES ($1, $2, $3, $4, now(), now() + interval '730 days')`,
		"hash-"+ids.NewV7().String(), "subject@example.test", contact.UUID, scope); err != nil {
		t.Fatalf("seeding the withdrawal credential: %v", err)
	}
}

// TestTheExportCarriesTheSubjectsOwnRightsRequests.
//
// A REFUSED one especially. That is the row a subject needs to see in order to
// appeal it, and the one an export that showed only fulfilled requests would
// quietly drop.
func TestTheExportCarriesTheSubjectsOwnRightsRequests(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedRightsCase(e.ctx, t, e.owner, e.contact, "erasure", "rejected",
		"Rejected: an open contract requires us to keep the billing record.")
	seedRightsCase(e.ctx, t, e.owner, e.contact, "access", "fulfilled", "Sent 12 March.")

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.RightsRequests) != 2 {
		t.Fatalf("the export carries %d rights request(s), want 2 — a subject cannot appeal a "+
			"refusal the package omits", len(pkg.RightsRequests))
	}

	var sawRefusal bool
	for _, row := range pkg.RightsRequests {
		if row["status"] == "rejected" {
			sawRefusal = true
		}
		if _, present := row["assignee_id"]; present {
			t.Error("the export names which colleague was given the work — that is a fact " +
				"about the workspace, and a bare id names a seat the subject cannot resolve")
		}
		if row["receipt_reference"] == nil || row["receipt_reference"] == "" {
			t.Error("the export carries no receipt reference, which is what the subject quotes " +
				"when asking what happened to their request")
		}
	}
	if !sawRefusal {
		t.Error("the rejected request is missing from the export")
	}
}

// TestTheExportCarriesTheLinksThatCanStopOurMail, and never their tokens.
func TestTheExportCarriesTheLinksThatCanStopOurMail(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedWithdrawalCredential(e.ctx, t, e.owner, e.contact, "all_marketing")

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.WithdrawalCredentials) != 1 {
		t.Fatalf("the export carries %d withdrawal link(s), want 1 — 'does the link in this old "+
			"message still work' is a question only the export answers",
			len(pkg.WithdrawalCredentials))
	}
	row := pkg.WithdrawalCredentials[0]
	if _, present := row["token_hash"]; present {
		t.Error("the export carries the token hash. The row holds a hash so the plaintext lives " +
			"only in the mail that carried it, and the hash proves nothing to the subject while " +
			"narrowing the search for anybody else")
	}
	if row["scope"] != "all_marketing" {
		t.Errorf("the export says the link's scope is %v, and the subject is asking what it "+
			"would stop", row["scope"])
	}
	if row["expires_at"] == nil {
		t.Error("the export does not say when the link stops working")
	}
}

// TestErasureRetiresARightsCaseAndKeepsTheRecordThatItHappened.
//
// The sharpest case of the decision-row shape: the erasure being performed is
// usually the answer to one of these rows. Destroying it would leave the
// controller unable to show it answered the request — including a refusal,
// where the record is what an appeal would ask to see.
func TestErasureRetiresARightsCaseAndKeepsTheRecordThatItHappened(t *testing.T) {
	e := setupSARIdentifiers(t)
	receipt := seedRightsCase(e.ctx, t, e.owner, e.contact, "erasure", "rejected",
		"Rejected: Anna Schmidt said she wanted only the newsletter stopped.")

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	var kind, subjectRef, storedReceipt string
	var resolution, contactID *string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT kind, subject_ref, receipt_reference, resolution, contact_id::text
		  FROM data_subject_request WHERE receipt_reference = $1`, receipt).
		Scan(&kind, &subjectRef, &storedReceipt, &resolution, &contactID); err != nil {
		t.Fatalf("reading the retired case: %v — the record that a request arrived must survive "+
			"the erasure that answered it", err)
	}
	if contactID != nil {
		t.Error("the case still names the erased contact")
	}
	// Tombstoned rather than nulled: a closed case must carry a resolution
	// (dsr_resolution_shape), so the erasure replaces the prose rather than
	// removing it — a null would refuse the erasure on exactly the closed
	// cases it is most likely to find.
	if resolution == nil || *resolution != "erased" {
		got := "<nil>"
		if resolution != nil {
			got = *resolution
		}
		t.Errorf("the case carries resolution %q, want the tombstone — the prose names the "+
			"subject and a closed case may not carry none", got)
	}
	if subjectRef == e.contact.String() {
		t.Error("the case still carries the subject reference it was opened with")
	}
	if storedReceipt == "" {
		t.Error("the receipt reference was destroyed — it names no subject and it is what " +
			"somebody quotes when asking what happened to their request")
	}
}

// TestTheErasedCaseKeepsItsDeadline is the accountability half stated as a
// test: a controller must be able to show it answered within the month.
func TestTheErasedCaseKeepsItsDeadline(t *testing.T) {
	e := setupSARIdentifiers(t)
	receipt := seedRightsCase(e.ctx, t, e.owner, e.contact, "access", "fulfilled", "Sent.")

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	var received, due time.Time
	var status string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT received_at, due_at, status FROM data_subject_request
		 WHERE receipt_reference = $1`, receipt).
		Scan(&received, &due, &status); err != nil {
		t.Fatalf("reading the retired case: %v", err)
	}
	if received.IsZero() || due.IsZero() || status == "" {
		t.Errorf("the erasure destroyed the case's own history: received %v due %v status %q",
			received, due, status)
	}
}

// eraseCapabilities runs the real Art. 17 consent cascade on its own
// transaction, which is the shape it runs in production.
func eraseCapabilities(
	ctx context.Context, t *testing.T, owner *pgx.Conn, contact ids.ContactID,
) {
	t.Helper()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the erasure: %v", err)
	}
	// Released here rather than on each failure path, so every exit closes it —
	// including the t.Fatalf below, which returns through no line of this
	// function. ErrTxClosed is expected rather than tolerated: whatever failed
	// may have closed the transaction on its way out, and reporting that as a
	// second failure would print noise ahead of the cause.
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the erasure transaction: %v", err)
		}
	})
	if err := deleteConsentCapabilities(ctx, tx, contact, nil, "test"); err != nil {
		t.Fatalf("erasing the subject's capabilities: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the erasure: %v", err)
	}
}

// TestAnOfficerOpenedCaseIsRetiredToo is Codex's finding, and it was the
// largest hole in this change.
//
// CreateDSR never sets contact_id, even when the reference IS a contact uuid.
// So a case an officer opened by hand carried its subject forever: keyed on the
// link alone, the erasure never saw it and neither did the export.
func TestAnOfficerOpenedCaseIsRetiredToo(t *testing.T) {
	e := setupSARIdentifiers(t)
	receipt := "RCPT-" + ids.NewV7().String()
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO data_subject_request
		  (kind, subject_ref, status, resolution, received_at, due_at, channel, receipt_reference)
		VALUES ('access', $1, 'fulfilled', 'Handed over in person.', now(),
		        now() + interval '30 days', 'phone', $2)`,
		e.contact.String(), receipt); err != nil {
		t.Fatalf("seeding the officer's case: %v", err)
	}

	eraseCapabilities(e.ctx, t, e.owner, e.contact)

	var subjectRef string
	var resolution *string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT subject_ref, resolution FROM data_subject_request
		 WHERE receipt_reference = $1`, receipt).Scan(&subjectRef, &resolution); err != nil {
		t.Fatalf("reading the officer's case: %v", err)
	}
	if subjectRef == e.contact.String() {
		t.Error("a case an officer opened by hand still names the erased subject — it carries " +
			"no contact link, so an erasure keyed on the link alone never reaches it")
	}
	if resolution == nil || *resolution != "erased" {
		t.Errorf("the officer's prose survived the erasure: %v", resolution)
	}
}

// TestTheExportShowsACaseAnOfficerOpened, the other half of the same gap: a
// subject could see the requests they made online and not the ones somebody
// recorded for them.
func TestTheExportShowsACaseAnOfficerOpened(t *testing.T) {
	e := setupSARIdentifiers(t)
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO data_subject_request
		  (kind, subject_ref, status, resolution, received_at, due_at, channel, receipt_reference)
		VALUES ('rectify', $1, 'rejected', 'Rejected: the contract requires it.', now(),
		        now() + interval '30 days', 'phone', $2)`,
		e.contact.String(), "RCPT-"+ids.NewV7().String()); err != nil {
		t.Fatalf("seeding the officer's case: %v", err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.RightsRequests) != 1 {
		t.Fatalf("the export carries %d rights request(s), want 1 — a case recorded FOR the "+
			"subject is still a case about them", len(pkg.RightsRequests))
	}
	// And the refusal reason travels, which is what a subject appeals against.
	if got := pkg.RightsRequests[0]["resolution"]; got == nil || got == "" {
		t.Error("the export withholds why the request was rejected, which is the one thing " +
			"the subject needs in order to appeal it")
	}
}
