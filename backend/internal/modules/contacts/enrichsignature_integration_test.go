// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A mailbox that has been switched off is never SELECTED, which is a stronger
// claim than "its results are dropped" and the reason the test is here rather
// than beside the pass: what enforces it is one predicate in SQL, and a Go-side
// filter that looked identical from the outside would still have read the mail.
//
// The join it rests on is a string rather than a foreign key — capture stamps
// `connector:<provider>:<user id>` onto every activity it writes — so a test
// against real Postgres is the only place the two halves of that convention are
// checked against each other.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedSignatureCandidate plants what SignatureCandidates looks for: a contact
// captured by a connector, with no title and no phone, and one inbound email
// carrying the mailbox's own provenance stamp.
func (e *dedupeEnv) seedSignatureCandidate(
	ctx context.Context,
	t *testing.T,
	name string,
	capturedBy string,
) {
	e.seedSignatureCandidateWithAudience(ctx, t, name, capturedBy, "workspace")
}

// seedSignatureCandidateWithAudience is the same fixture with the mail's
// audience under the caller's control, so a test can tell a candidate skipped
// for its MAILBOX apart from one skipped for its message's audience.
func (e *dedupeEnv) seedSignatureCandidateWithAudience(
	ctx context.Context,
	t *testing.T,
	name string,
	capturedBy string,
	audience string,
) {
	t.Helper()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name,
		Source:   "connector:gmail",
		Emails: []ContactEmailInput{{
			Email: "sig-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))

	// captured_by on the CONTACT is what the candidate predicate filters on;
	// captured_by on the ACTIVITY is what the mailbox switch reads.
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE contact SET captured_by = $2, title = NULL WHERE id = $1`,
			contactID, capturedBy); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Regards, Dana | VP Finance | +49 30 1234', 'inbound', now(), 'gmail:seed', $2, $3)`,
			activityID, capturedBy, audience); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
			VALUES ($1, $2, 'contact', $3)`,
			ids.NewV7(), activityID, contactID); err != nil {
			return err
		}
		// THEY sent it. A signature is only theirs to read off a message they
		// wrote, so the candidate query asks for this row and a mail seeded
		// without one is a mail nobody wrote.
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'from')`, activityID, contactID)
		return err
	}); err != nil {
		t.Fatalf("seed the captured mail: %v", err)
	}
}

// connectedMailbox seeds a capture_connection whose provenance string is the
// one the activities above carry, with the switch in the given position.
func (e *dedupeEnv) connectedMailbox(
	ctx context.Context,
	t *testing.T,
	userID ids.UUID,
	enabled *bool,
) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// The connection's owner is a real seat: capture_connection carries a
		// foreign key to app_user, and a mailbox belonging to nobody is not a
		// state the product can reach.
		if _, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Mailbox Owner')`,
			userID, "mailbox-"+userID.String()+"@seed.test"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (id, provider, user_id, status, signature_enrich_enabled)
			VALUES ($1, 'gmail', $2, 'connected', $3)`,
			ids.NewV7(), userID, enabled)
		return err
	}); err != nil {
		t.Fatalf("seed the mailbox: %v", err)
	}
}

// seedOwnerScopedCandidate is the fixture for a contact capture minted for ONE
// seat: visibility='owner' with that seat as the owner, and one inbound mail
// whose audience the caller chooses.
//
// The seat is also stamped on capture_import, which is how production records
// the mailbox that delivered a message — the same row auth.activityMembershipArm
// reads to decide that this seat was on it.
func (e *dedupeEnv) seedOwnerScopedCandidate(
	ctx context.Context,
	t *testing.T,
	name string,
	owner ids.UUID,
	audience string,
) {
	t.Helper()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: name,
		Source:   "connector:gmail",
		Emails: []ContactEmailInput{{
			Email: "own-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))

	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// The record capture actually mints while its sender is unjudged: the
		// owning seat's alone, which is what makes mining their mail no wider a
		// disclosure than the mail already is.
		if _, err := tx.Exec(ctx, `
			UPDATE contact SET captured_by = 'connector:gmail', title = NULL,
			       visibility = 'owner', owner_id = $2
			 WHERE id = $1`, contactID, owner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Regards, Dana | VP Finance | +49 30 1234', 'inbound', now(), 'gmail:seed', 'connector:gmail', $2)`,
			activityID, audience); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`,
			activityID, owner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
			VALUES ($1, $2, 'contact', $3)`,
			ids.NewV7(), activityID, contactID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'from')`, activityID, contactID)
		return err
	}); err != nil {
		t.Fatalf("seed the owner-scoped mail: %v", err)
	}
}

// ELIGIBILITY FOLLOWS THE CONTACT'S OWN VISIBILITY.
//
// The flat audience test read as a privacy rule and was one only for a
// workspace-visible contact: their fields land on a record every seat reads, so
// mail those seats may not open must not be mined. An OWNER-SCOPED contact's
// fields are readable by their owner alone — the same audience the participants
// mail already has — so the same read publishes nothing.
//
// It is not a corner: capture mints owner-scoped until a verdict widens the
// record, so the flat test left every unjudged contact permanently ineligible
// whatever the pass budget said. In the audited import that was 777 of 814.
func TestSignatureCandidatesMineAnOwnerScopedContactsOwnMail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Owner Scoped Limited Mail", e.rep, "participants")

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	if !contains(candidateNames(got), "Owner Scoped Limited Mail") {
		t.Errorf("an owner-scoped contact's own limited mail was not offered: %v — "+
			"their fields are readable by that owner alone, the same audience the mail already has", candidateNames(got))
	}
}

// The republishing guard still holds where it was always the point: a contact
// every seat can read, whose mail those seats may not open.
//
// This is the negative that keeps the arm above from becoming "mine everything".
// Without it a widened predicate would pass every test in this file.
//
// The contact is WORKSPACE-visible and the owner IS on the message — both arms
// of the owner path are otherwise satisfied — so what this pins is the
// visibility requirement alone. Seeded owner-scoped, it would be a candidate.
func TestSignatureCandidatesStillSkipLimitedMailForAWorkspaceContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Workspace Contact Limited Mail", e.rep, "participants")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// Widened to the workspace, exactly as a verdict promotes a captured
		// record. The mail and the owner's presence on it are untouched.
		_, err := tx.Exec(ctx, `
			UPDATE contact SET visibility = 'workspace'
			 WHERE full_name = 'Workspace Contact Limited Mail'`)
		return err
	}); err != nil {
		t.Fatalf("widening the contact: %v", err)
	}

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	if contains(candidateNames(got), "Workspace Contact Limited Mail") {
		t.Errorf("a workspace-visible contact was mined from limited mail: %v — "+
			"their title, phone and employer would reach seats that may not open the message", candidateNames(got))
	}
}

// The arm reads the ROW's owner, never the caller's seat. The pass runs as the
// system principal with no human behind it, so a contact owned by any colleague
// is mined from their own mail exactly as the caller's would be — and a version
// that compared against the caller would quietly enrich one seat's contacts and
// starve everybody else's.
func TestSignatureCandidatesMineAContactOwnedByAnyColleague(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Owned By A Colleague", e.otherRep, "participants")

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	// The owner IS on this mail, so it is a candidate — the assertion is that
	// the arm read the row's own owner rather than the caller's seat.
	if !contains(candidateNames(got), "Owned By A Colleague") {
		t.Errorf("an owner-scoped contact's mail was skipped because the owner is not the caller: %v — "+
			"the pass runs as the system principal and the arm is about the ROW's owner", candidateNames(got))
	}
}

func candidateNames(candidates []SignatureCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.FullName)
	}
	return names
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func TestSignatureCandidatesSkipASwitchedOffMailbox(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	on := true
	off := false
	willing := ids.NewV7()
	refusing := ids.NewV7()
	e.connectedMailbox(ctx, t, willing, &on)
	e.connectedMailbox(ctx, t, refusing, &off)

	e.seedSignatureCandidate(ctx, t, "From A Willing Mailbox", "connector:gmail:"+willing.String())
	e.seedSignatureCandidate(ctx, t, "From A Refusing Mailbox", "connector:gmail:"+refusing.String())

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	names := candidateNames(got)
	if !contains(names, "From A Willing Mailbox") {
		t.Errorf("the willing mailbox's contact is absent from %v", names)
	}
	if contains(names, "From A Refusing Mailbox") {
		t.Errorf("a switched-off mailbox's contact was selected: %v", names)
	}
}

// A mailbox that never chose follows the workspace, in both directions — which
// is what makes the null a third state rather than a missing value.
func TestSignatureCandidatesFollowTheWorkspaceWhenAMailboxHasNotChosen(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	undecided := ids.NewV7()
	e.connectedMailbox(ctx, t, undecided, nil)
	e.seedSignatureCandidate(ctx, t, "Undecided Mailbox", "connector:gmail:"+undecided.String())

	enabled, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting with the workspace on: %v", err)
	}
	if !contains(candidateNames(enabled), "Undecided Mailbox") {
		t.Error("a mailbox with no choice of its own was skipped while the workspace was on")
	}

	disabled, err := e.store.SignatureCandidates(ctx, 50, false)
	if err != nil {
		t.Fatalf("selecting with the workspace off: %v", err)
	}
	if contains(candidateNames(disabled), "Undecided Mailbox") {
		t.Error("a mailbox with no choice of its own was selected while the workspace was off")
	}
}

// Mail stamped with the bare `connector:<name>` form — no granting user bound —
// matches no connection row. It follows the workspace, which is the answer it
// had before the switch existed.
func TestSignatureCandidatesTreatUnboundMailAsTheWorkspaceDefault(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedSignatureCandidate(ctx, t, "Unbound Provenance", "connector:gmail")

	enabled, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting with the workspace on: %v", err)
	}
	if !contains(candidateNames(enabled), "Unbound Provenance") {
		t.Error("unbound mail was skipped while the workspace default was on")
	}

	disabled, err := e.store.SignatureCandidates(ctx, 50, false)
	if err != nil {
		t.Fatalf("selecting with the workspace off: %v", err)
	}
	if contains(candidateNames(disabled), "Unbound Provenance") {
		t.Error("unbound mail was selected while the workspace default was off")
	}
}

// A limited message is not signature material. The pass writes what it extracts
// onto a contact every seat can read, so mining a message whose audience
// excludes those seats republishes its content as fields — and narrowing the
// message afterwards does not take the fields back.
//
// The switched-on mailbox is what makes this a claim about the AUDIENCE: both
// contacts below sit behind the same willing mailbox, and only the audience of
// the mail they were last written from differs.
func TestSignatureCandidatesSkipALimitedMessage(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	on := true
	mailbox := ids.NewV7()
	e.connectedMailbox(ctx, t, mailbox, &on)
	stamp := "connector:gmail:" + mailbox.String()

	e.seedSignatureCandidateWithAudience(ctx, t, "Wrote From Open Mail", stamp, "workspace")
	e.seedSignatureCandidateWithAudience(ctx, t, "Wrote From Limited Mail", stamp, "participants")

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	names := candidateNames(got)
	if !contains(names, "Wrote From Open Mail") {
		t.Errorf("the open message's contact is absent from %v — the fixture cannot tell a working gate from a broken query", names)
	}
	if contains(names, "Wrote From Limited Mail") {
		t.Errorf("a contact whose only mail is limited was offered for signature mining: %v — "+
			"their title, phone and employer would be written onto a workspace-readable record from a message those readers may not open", names)
	}
}

// SELECTION AND APPLICATION ASK THE SAME QUESTION.
//
// The candidate query offering an owner's own participants mail buys nothing if
// the apply then refuses it: the pass marks the message read either way, so the
// mail is consumed, no field is written, and it is never reconsidered. That is
// worse than not selecting it at all, and it is exactly what shipped when the
// two statements spelled the rule separately.
//
// The test drives BOTH halves in the pass's own order — select, then apply the
// row that selection returned. An apply-only test cannot see a disagreement,
// because the disagreement is between the two statements rather than inside
// either one.
func TestApplySignatureFieldsLandOwnerScopedMailTheCandidateQueryOffers(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Owner Scoped Apply", e.rep, "participants")

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	var picked SignatureCandidate
	for _, c := range got {
		if c.FullName == "Owner Scoped Apply" {
			picked = c
		}
	}
	if picked.FullName == "" {
		t.Fatalf("the owner-scoped contact was not offered: %v", candidateNames(got))
	}

	res, err := e.store.ApplySignatureFields(ctx, picked.ContactID, picked.ActivityID, []SignatureField{
		{Name: "title", Value: "VP Finance", Evidence: "Regards, Dana | VP Finance", Confidence: 0.95},
	})
	if err != nil {
		t.Fatalf("applying the message selection just offered: %v", err)
	}
	if res.Applied != 1 {
		t.Fatalf("applied %d field(s) from the message SELECTION offered, want 1 — "+
			"the pass marks it read either way, so an apply that skips it consumes the mail and writes nothing", res.Applied)
	}
}

// EITHER fact of presence is enough: delivered to the owner's mailbox, OR
// stamped as a participant by seat. The two arms are alternatives, so a fixture
// carrying both proves neither on its own — this one carries ONLY the
// participant stamp, and its sibling above carries only the capture_import row.
func TestSignatureCandidatesMineMailTheOwnerIsStampedOnWithoutImporting(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Stamped Not Imported",
		Source:   "connector:gmail",
		Emails: []ContactEmailInput{{
			Email: "stamp-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE contact SET captured_by = 'connector:gmail', title = NULL,
			       visibility = 'owner', owner_id = $2 WHERE id = $1`, contactID, e.rep); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Regards, Dana | VP Finance | +49 30 1234', 'inbound', now(), 'gmail:seed', 'connector:gmail', 'participants')`,
			activityID); err != nil {
			return err
		}
		// NO capture_import row. The owner is present only as a seat-stamped
		// participant, which is the other half of activityMembershipArm.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, user_id, role, address)
			VALUES ($1, $2, 'to', $3)`, activityID, e.rep, "owner-"+e.rep.String()+"@seed.test"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
			VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activityID, contactID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'from')`, activityID, contactID)
		return err
	}); err != nil {
		t.Fatalf("seed the stamped mail: %v", err)
	}

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	if !contains(candidateNames(got), "Stamped Not Imported") {
		t.Errorf("mail the owner is stamped on was refused: %v — "+
			"a seat-stamped participant row is presence, the same as delivery", candidateNames(got))
	}
}

// WORKSPACE MAIL IS MINABLE WHATEVER THE RECORD'S VISIBILITY, and a grant on the
// record does not change that: every seat can already open the message, so
// writing its content into a field discloses it to nobody new.
//
// Hanging the grant veto off the whole owner arm refused exactly this — an
// owner-scoped contact with open mail is the ORDINARY case, and a single share
// silently made their open mail unminable.
func TestSignatureCandidatesMineWorkspaceMailForASharedOwnerScopedContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Shared With Open Mail", e.rep, "workspace")

	var contactID ids.ContactID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM contact WHERE full_name = 'Shared With Open Mail'`).Scan(&contactID)
	}); err != nil {
		t.Fatalf("reading the seeded contact: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('contact', $1, 'user', $2, 'read', $3)`, contactID, e.otherRep, e.rep)
		return err
	}); err != nil {
		t.Fatalf("granting a colleague read access: %v", err)
	}

	got, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting candidates: %v", err)
	}
	if !contains(candidateNames(got), "Shared With Open Mail") {
		t.Errorf("open mail was refused because the record is shared: %v — "+
			"the grantee can already read the message, so the field tells them nothing new", candidateNames(got))
	}
}

// A SHARED owner-scoped record is the workspace case again.
//
// contact is a shareable table: a live record_grant lets another seat read the
// record's fields with no tie to the source message's audience. Mining narrowed
// mail would then put its content in front of somebody who could not open it —
// which is the same republishing the workspace arm exists to prevent, reached
// by a different door.
func TestSignatureMiningStopsWhenAnOwnerScopedContactIsShared(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	e.seedOwnerScopedCandidate(ctx, t, "Shared With A Colleague", e.rep, "participants")

	var contactID ids.ContactID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM contact WHERE full_name = 'Shared With A Colleague'`).Scan(&contactID)
	}); err != nil {
		t.Fatalf("reading the seeded contact: %v", err)
	}
	// Before the grant it is a candidate: without this the assertion below
	// would pass against a query that never offered the row at all.
	before, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting before the grant: %v", err)
	}
	if !contains(candidateNames(before), "Shared With A Colleague") {
		t.Fatal("the unshared owner-scoped contact was not offered; the fixture proves nothing about sharing")
	}

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ('contact', $1, 'user', $2, 'read', $3)`, contactID, e.otherRep, e.rep)
		return err
	}); err != nil {
		t.Fatalf("granting a colleague read access: %v", err)
	}

	after, err := e.store.SignatureCandidates(ctx, 50, true)
	if err != nil {
		t.Fatalf("selecting after the grant: %v", err)
	}
	if contains(candidateNames(after), "Shared With A Colleague") {
		t.Errorf("a shared owner-scoped contact was still mined from limited mail: %v — "+
			"the granted seat reads the title without being able to open the message it came from", candidateNames(after))
	}
}

// The apply path re-tests the SOURCE, not only the candidate query that
// selected it.
//
// SignatureCandidates selects an open message, a model call runs, and a human
// or a verdict can limit that message before the fields land. What lands is a
// title, a phone and an employer on a contact every seat reads, and the audience
// rescope deliberately does not retract profile fields — so a field written
// after the narrowing stays readable for good. That is the one outcome no later
// correction reaches, which is why the test is at the write.
func TestApplySignatureFieldsSkipsASourceLimitedWhileTheModelRan(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	apply := func(audience string) SignatureApplyResult {
		t.Helper()
		contact, err := e.store.CreateContact(ctx, CreateContactInput{
			FullName: "Signature Subject " + audience,
			Source:   "connector:gmail",
			Emails: []ContactEmailInput{{
				Email: "sig-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true,
			}},
		})
		if err != nil {
			t.Fatalf("seed contact: %v", err)
		}
		contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
		activityID := ids.NewV7()
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
				VALUES ($1, 'email', 'Regards, Dana | VP Finance', 'inbound', now(), 'gmail:seed', 'connector:gmail', $2)`,
				activityID, audience)
			return err
		}); err != nil {
			t.Fatalf("seed the source message: %v", err)
		}
		res, err := e.store.ApplySignatureFields(ctx, contactID, activityID, []SignatureField{
			{Name: "title", Value: "VP Finance", Evidence: "Regards, Dana | VP Finance", Confidence: 0.95},
		})
		if err != nil {
			t.Fatalf("applying signature fields from %s mail: %v", audience, err)
		}
		return res
	}

	// The open case first: without it a re-check that refused everything would
	// pass the assertion below and silently switch signature enrichment off.
	if open := apply("workspace"); open.Applied != 1 {
		t.Fatalf("an open source applied %d field(s), want 1 — the re-check refuses more than the audience", open.Applied)
	}
	if limited := apply("participants"); limited.Applied != 0 {
		t.Errorf("a source limited while the model ran applied %d field(s) — a title read from a message its readers may not open, "+
			"written onto a contact every seat sees, and the audience rescope does not retract profile fields", limited.Applied)
	}
}
