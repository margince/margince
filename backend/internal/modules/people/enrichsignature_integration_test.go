// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

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

// seedSignatureCandidate plants what SignatureCandidates looks for: a person
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
	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: name,
		Source:   "connector:gmail",
		Emails: []PersonEmailInput{{
			Email: "sig-" + ids.NewV7().String() + "@seed.test", EmailType: emailTypeWork, IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(person.Id))

	// captured_by on the PERSON is what the candidate predicate filters on;
	// captured_by on the ACTIVITY is what the mailbox switch reads.
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE person SET captured_by = $2, title = NULL WHERE id = $1`,
			personID, capturedBy); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, body, direction, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', 'Regards, Dana | VP Finance | +49 30 1234', 'inbound', now(), 'gmail:seed', $2, $3)`,
			activityID, capturedBy, audience); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (id, activity_id, entity_type, person_id)
			VALUES ($1, $2, 'person', $3)`,
			ids.NewV7(), activityID, personID)
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
		t.Errorf("the willing mailbox's person is absent from %v", names)
	}
	if contains(names, "From A Refusing Mailbox") {
		t.Errorf("a switched-off mailbox's person was selected: %v", names)
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
// onto a person every seat can read, so mining a message whose audience
// excludes those seats republishes its content as fields — and narrowing the
// message afterwards does not take the fields back.
//
// The switched-on mailbox is what makes this a claim about the AUDIENCE: both
// people below sit behind the same willing mailbox, and only the audience of
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
		t.Errorf("the open message's person is absent from %v — the fixture cannot tell a working gate from a broken query", names)
	}
	if contains(names, "Wrote From Limited Mail") {
		t.Errorf("a person whose only mail is limited was offered for signature mining: %v — "+
			"their title, phone and employer would be written onto a workspace-readable record from a message those readers may not open", names)
	}
}
