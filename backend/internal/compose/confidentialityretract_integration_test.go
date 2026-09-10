// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a thread judged PERSONAL does to the contact on it.
//
// The verdict almost always arrives after the contact: capture creates on
// commit and classification reads the thread later. In one real mailbox every
// contact on a personal thread — forty-six of them — predated the verdict about
// it, so refusing at creation time alone would have prevented none of them.
//
// The keeping is as much the subject as the withdrawing. A person who also
// writes about business is a business contact who happens to have a private
// thread, and every "retracted" case here is paired with the shape that must
// survive.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A personal verdict withdraws the contact capture already made.
//
// The verdict almost always arrives AFTER the contact: capture creates on
// commit and classification reads the thread later. In one real mailbox every
// contact on a personal thread — forty-six of them, a founder's aunt among them
// — predated the verdict about it, so refusing at creation time alone would
// have prevented none of them.
func TestAPersonalVerdictRetractsTheContactCaptureAlreadyMade(t *testing.T) {
	e := integration.Setup(t)
	const aunt = "aunt@family.test"
	activityID := seedHeldThreadMail(t, e, "thread-family", aunt, "Geburtstag")
	threadID := seedThreadQuestion(t, e, "thread-family", activityID)
	personID := seedCapturedContact(t, e, aunt)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if !personIsArchived(t, e, personID) {
		t.Fatal("a contact whose only correspondence is a private conversation is still in the CRM — " +
			"the classifier decided this is not the workspace's business")
	}
}

// A contact who ALSO writes about business survives. They are a business
// contact who happens to have a private thread too, and retracting them would
// lose a real counterparty.
//
// The business thread is JUDGED business, through the real engine. It used to
// be a thread nothing had looked at, which passed for the same thing and was
// not: silence about a thread is not evidence of a business relationship, and
// reading it as one is what kept a founder's private clinic in the CRM.
func TestAPersonalVerdictKeepsAContactWhoAlsoWritesAboutBusiness(t *testing.T) {
	e := integration.Setup(t)
	const both = "cousin@family.test"
	businessActivity := seedHeldThreadMail(t, e, "thread-business", both, "Angebot")
	businessID := seedThreadQuestion(t, e, "thread-business", businessActivity)
	runConfidentiality(t, e, businessID, confidentialityOrdinary, 0.95)

	privateActivity := seedHeldThreadMail(t, e, "thread-private", both, "Familie")
	threadID := seedThreadQuestion(t, e, "thread-private", privateActivity)
	personID := seedCapturedContact(t, e, both)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact who also writes about business was retracted — one private " +
			"conversation does not make somebody's whole correspondence private")
	}
}

// A thread nothing ever judged does NOT protect the contact.
//
// This is the founder's clinic, exactly. Their private clinic wrote twice: one
// thread was judged personal and held, and the second had no verdict row at all
// because it arrived at a forwarding address the seat had not yet claimed. The
// retraction read the unjudged thread as business and kept the record, so the
// clinic and its blood-test results sat in the CRM.
func TestAPersonalVerdictRetractsAContactWhoseOtherThreadWasNeverJudged(t *testing.T) {
	e := integration.Setup(t)
	const clinic = "clinic@health.test"
	privateActivity := seedHeldThreadMail(t, e, "thread-results", clinic, "Blood test results")
	threadID := seedThreadQuestion(t, e, "thread-results", privateActivity)
	// A second conversation nobody asked a question about: held, unjudged.
	seedHeldThreadMail(t, e, "thread-unjudged", clinic, "Blood test results, August")
	personID := seedCapturedContact(t, e, clinic)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if !personIsArchived(t, e, personID) {
		t.Fatal("a thread nothing has judged kept a private correspondent in the CRM — " +
			"silence about a conversation is not evidence that it is business")
	}
}

// A question still in flight is a reason to WAIT, not to retract. Resolving it
// personal runs the retraction again, and resolving it ordinary is the business
// evidence the test above is missing.
func TestAPersonalVerdictWaitsWhileAnotherThreadIsStillPending(t *testing.T) {
	e := integration.Setup(t)
	const pending = "unsure@family.test"
	privateActivity := seedHeldThreadMail(t, e, "thread-private-b", pending, "Privat")
	threadID := seedThreadQuestion(t, e, "thread-private-b", privateActivity)
	// Opened and deliberately not answered.
	otherActivity := seedHeldThreadMail(t, e, "thread-open", pending, "Offen")
	seedThreadQuestion(t, e, "thread-open", otherActivity)
	personID := seedCapturedContact(t, e, pending)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact was retracted while a question about another of their threads " +
			"was still open — the answer may yet be that it is business")
	}
}

// Mail already open to the workspace protects the contact, with no verdict row
// anywhere. A cleared sender's mail is born workspace-visible and opens no
// confidentiality question at all, so requiring a ledger row as the only
// evidence would retract exactly the contacts the workspace has already agreed
// are its own.
func TestAPersonalVerdictKeepsAContactWhoseOtherThreadIsOpenToTheWorkspace(t *testing.T) {
	e := integration.Setup(t)
	const known = "partner@business.test"
	privateActivity := seedHeldThreadMail(t, e, "thread-private-c", known, "Privat")
	threadID := seedThreadQuestion(t, e, "thread-private-c", privateActivity)
	seedOpenThreadMail(t, e, "thread-open-ws", known, "Vertrag")
	personID := seedCapturedContact(t, e, known)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact whose other conversation is already open to the whole workspace " +
			"was retracted — the workspace has agreed that correspondence is its own")
	}
}

// A contact the SENDER verdict promoted to the workspace is still retractable.
//
// Two machines, and the earlier of them used to win: the sender classifier
// publishes a contact it judges a real counterparty, and the retraction refused
// any record that was not owner-scoped. So a system promotion at 05:58 outranked
// a system verdict at 07:05, and a private medical correspondent stayed visible
// to every colleague. Nobody decided that; it fell out of the order the two
// passes happened to run in.
func TestAPersonalVerdictRetractsAContactAMachinePromoted(t *testing.T) {
	e := integration.Setup(t)
	const promoted = "doctor@health.test"
	activityID := seedHeldThreadMail(t, e, "thread-promoted", promoted, "Befund")
	threadID := seedThreadQuestion(t, e, "thread-promoted", activityID)
	personID := seedMachinePromotedContact(t, e, promoted)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if !personIsArchived(t, e, personID) {
		t.Fatal("a contact the sender classifier had promoted survived a personal verdict — " +
			"one machine's guess outranked another machine's answer about the same person")
	}
	if got := personVisibilityIs(t, e, personID); got != "owner" {
		t.Fatalf("the retracted contact is still %q-visible: an archived record is still "+
			"listable by colleagues who ask for archived ones, so it has to be narrowed too", got)
	}
}

// A record a human PUBLISHED is theirs, exactly as an edited one is. The
// difference from the test above is only who did the publishing, and it is the
// whole difference: a person deciding a contact belongs to the workspace is a
// decision, and a classifier's opinion about one conversation does not overrule
// it.
func TestAPersonalVerdictKeepsAContactAHumanPublished(t *testing.T) {
	e := integration.Setup(t)
	const published = "advisor@health.test"
	activityID := seedHeldThreadMail(t, e, "thread-published", published, "Privat")
	threadID := seedThreadQuestion(t, e, "thread-published", activityID)
	personID := seedMachinePromotedContact(t, e, published)
	seedHumanEdit(t, e, personID)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact a human published was retracted — a person's decision that this " +
			"record belongs to the workspace outranks a classifier's about one thread")
	}
}

// A record a human touched is theirs. A classifier's opinion about one
// conversation does not overrule somebody having decided they want it.
func TestAPersonalVerdictKeepsAContactAHumanEdited(t *testing.T) {
	e := integration.Setup(t)
	const edited = "friend@family.test"
	activityID := seedHeldThreadMail(t, e, "thread-edited", edited, "Privat")
	threadID := seedThreadQuestion(t, e, "thread-edited", activityID)
	personID := seedCapturedContact(t, e, edited)
	seedHumanEdit(t, e, personID)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact a human edited was retracted — an edit is somebody saying they want this record")
	}
}

// seedCapturedContact lands the contact shape capture creates: owner-scoped,
// captured_by a connector, with the address on it.
func seedCapturedContact(t *testing.T, e *integration.Env, email string) ids.PersonID {
	t.Helper()
	var id ids.PersonID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO person (full_name, source, captured_by, owner_id, visibility)
			VALUES ($1, 'gmail:seed', 'connector:gmail', $2, 'owner')
			RETURNING id`, email, e.Rep1).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, source, captured_by)
			VALUES ($1, $2, 'gmail:seed', 'connector:gmail')`, id.UUID, email)
		return err
	}); err != nil {
		t.Fatalf("seeding the captured contact: %v", err)
	}
	return id
}

// seedOpenThreadMail lands a message already open to the whole workspace: the
// shape a cleared sender's mail is born with, which carries no verdict row and
// no held audience.
func seedOpenThreadMail(t *testing.T, e *integration.Env, threadKey, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, thread_key, audience)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, $5, 'workspace')`,
			id, subject, "opn-"+id.String(), from, threadKey); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'shared')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding a workspace-visible message: %v", err)
	}
	return id
}

// seedMachinePromotedContact lands the contact the SENDER verdict makes: minted
// under the agent principal, published to the workspace by that engine, with
// the system audit row the promotion writes. Hand-built rather than driven
// through the verdict engine because this file's subject is the confidentiality
// pass; the shape is taken from what that engine actually writes.
func seedMachinePromotedContact(t *testing.T, e *integration.Env, email string) ids.PersonID {
	t.Helper()
	var id ids.PersonID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO person (full_name, source, captured_by, owner_id, visibility)
			VALUES ($1, 'capture_counterparty_verdict', 'agent:capture_counterparty_verdict', $2, 'workspace')
			RETURNING id`, email, e.Rep1).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, source, captured_by)
			VALUES ($1, $2, 'capture_counterparty_verdict', 'agent:capture_counterparty_verdict')`,
			id.UUID, email); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
			VALUES ('system', 'agent:capture_counterparty_verdict', 'update', 'person', $1,
			        '{"visibility":"owner"}'::jsonb, '{"visibility":"workspace"}'::jsonb)`, id.UUID)
		return err
	}); err != nil {
		t.Fatalf("seeding the promoted contact: %v", err)
	}
	return id
}

// personVisibilityIs reads who may see the record now.
func personVisibilityIs(t *testing.T, e *integration.Env, id ids.PersonID) string {
	t.Helper()
	var visibility string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT visibility FROM person WHERE id = $1`, id.UUID).Scan(&visibility)
	}); err != nil {
		t.Fatalf("reading the contact's visibility: %v", err)
	}
	return visibility
}

// seedHumanEdit lands the audit row a person's own edit leaves behind.
func seedHumanEdit(t *testing.T, e *integration.Env, id ids.PersonID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
			VALUES ('human', $1, 'update', 'person', $2)`, e.Rep1.String(), id.UUID)
		return err
	}); err != nil {
		t.Fatalf("seeding the human edit: %v", err)
	}
}

func personIsArchived(t *testing.T, e *integration.Env, id ids.PersonID) bool {
	t.Helper()
	var archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT archived_at IS NOT NULL FROM person WHERE id = $1`, id.UUID).Scan(&archived)
	}); err != nil {
		t.Fatalf("reading whether the contact was retracted: %v", err)
	}
	return archived
}
