// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the confidentiality engine does to a held thread, end to end.
//
// Under the classified posture every captured message is born held, and this
// engine is what makes that livable: it opens the ordinary conversations so
// what stays private is the correspondence that had a reason to. The tests
// below are the two halves of that promise — an ordinary thread becomes
// readable, and everything else does not.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAnOrdinaryThreadIsOpenedForTheWorkspace(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-ordinary", "einkauf@kunde.example", "Nachbestellung")
	threadID := seedThreadQuestion(t, e, "thread-ordinary", activityID)

	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := threadStatus(t, e, threadID); got != capture.VerdictCleared {
		t.Fatalf("thread status = %q, want cleared", got)
	}
	if got := activityAudience(t, e, activityID); got != "workspace" {
		t.Fatalf("an ordinary thread's message is %q, want workspace — the whole point of the engine "+
			"is that a classified mailbox does not stay invisible to the team", got)
	}
}

func TestASensitiveThreadStaysHeldAndNamesWhy(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-personnel", "kanzlei@example.test", "Unterlagen")
	threadID := seedThreadQuestion(t, e, "thread-personnel", activityID)

	runConfidentiality(t, e, threadID, confidentialityPersonnel, 0.95)

	if got := threadStatus(t, e, threadID); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held", got)
	}
	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("a personnel thread's message is %q, want participants", got)
	}
	// And it says WHY. Without the kind travelling with the status, every held
	// thread reads as the generic `verdict` and a reader is told a message is
	// private without being told what kind of private.
	if got := activityAudienceReason(t, e, activityID); got != confidentialityPersonnel {
		t.Fatalf("the message's audience reason is %q, want personnel", got)
	}
}

// activityAudienceReason answers what a reader is told about why a message is
// held.
func activityAudienceReason(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var reason string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(audience_reason, '') FROM activity WHERE id = $1`, id).Scan(&reason)
	}); err != nil {
		t.Fatalf("reading the audience reason: %v", err)
	}
	return reason
}

func TestAnOpeningAnswerBelowTheFloorHoldsTheThread(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-unsure", "someone@example.test", "Frage")
	threadID := seedThreadQuestion(t, e, "thread-unsure", activityID)

	// `ordinary` is the one answer that opens, so it is the one answer that has
	// to clear a floor. A holding answer needs none: requiring confidence to
	// hold would publish exactly the threads the model found hardest.
	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.6)

	if got := threadStatus(t, e, threadID); got != capture.VerdictUnsure {
		t.Fatalf("thread status = %q, want unsure — a below-floor opening answer is not believed", got)
	}
	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("a thread the model was unsure about is %q, want participants", got)
	}
}

func TestAHoldingAnswerNeedsNoConfidence(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-lowlegal", "counsel@example.test", "Sache")
	threadID := seedThreadQuestion(t, e, "thread-lowlegal", activityID)

	runConfidentiality(t, e, threadID, confidentialityLegal, 0.3)

	if got := threadStatus(t, e, threadID); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held even at 0.3 — the floor guards opening, not holding", got)
	}
}

// runConfidentiality drives the real engine over one thread with a scripted
// answer, so what is under test is the engine's own apply path rather than a
// test's idea of it.
func runConfidentiality(t *testing.T, e *integration.Env, threadID ids.UUID, kind string, confidence float64) {
	t.Helper()
	brain := &scriptedConfidentialityBrain{kind: kind, confidence: confidence, id: threadID}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
}

func threadStatus(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM capture_thread_verdict WHERE id = $1`, id).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the thread status: %v", err)
	}
	return status
}

func activityAudience(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var audience string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT audience FROM activity WHERE id = $1`, id).Scan(&audience)
	}); err != nil {
		t.Fatalf("reading the activity audience: %v", err)
	}
	return audience
}

// seedThreadQuestion opens the question the way capture opens it — through the
// store's own EnsureTx, not a hand-written INSERT, so what the engine claims is
// the row production would have written.
func seedThreadQuestion(t *testing.T, e *integration.Env, threadKey string, activityID ids.UUID) ids.UUID {
	t.Helper()
	// The classified mailbox the question belongs to. A question is claimed
	// only for a seat still asking to be classified, so a fixture without the
	// connection describes a mailbox that never had one.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection
			       (user_id, provider, status, credential_ref, mail_posture)
			VALUES ($1, 'gmail', 'connected', 'vault:test', 'classified')
			ON CONFLICT (user_id, provider)
			DO UPDATE SET mail_posture = 'classified', archived_at = NULL`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the classified mailbox the question belongs to: %v", err)
	}
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return store.EnsureTx(context.Background(), tx, threadKey, e.Rep1, activityID, time.Now().Add(-time.Minute))
	}); err != nil {
		t.Fatalf("opening the thread question: %v", err)
	}
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM capture_thread_verdict WHERE thread_key = $1`, threadKey).Scan(&id)
	}); err != nil {
		t.Fatalf("reading back the thread question: %v", err)
	}
	return id
}

// scriptedConfidentialityBrain answers with a fixed kind, for the id it finds
// IN THE PROMPT rather than the one it was constructed with.
//
// Reading the id back out of the fence is the point: production takes it from a
// ledger row no model has seen, so a brain that answered from its own field
// would pass even if the request carried the wrong thread — or no thread at
// all. The constructed id is used only to check the prompt named the one this
// test meant.
type scriptedConfidentialityBrain struct {
	kind       string
	confidence float64
	id         ids.UUID
}

func (s *scriptedConfidentialityBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	askedFor := fencedIDs(req.System, req.Messages[0].Content, "id")
	if len(askedFor) != 1 {
		return model.Response{}, fmt.Errorf(
			"confidentiality prompt declared no data boundary, or fenced %d threads rather than one", len(askedFor))
	}
	if askedFor[0] != s.id.String() {
		return model.Response{}, fmt.Errorf(
			"the prompt asked about thread %s, not the one under test", askedFor[0])
	}
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": askedFor[0], "verdict": s.kind, "confidence": s.confidence},
	}})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// seedHeldThreadMail lands one message on a thread, held to its participants,
// with the import row that makes this seat a contributor to its audience.
//
// The import row is not decoration: activity.audience is DERIVED across every
// seat that imported the message, so a message with no import row has no
// contributor asking for anything and the recompute would open it regardless of
// any verdict. Seeding one is what makes the audience assertions in this file
// measure the engine rather than the absence of a row.
func seedHeldThreadMail(t *testing.T, e *integration.Env, threadKey, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, thread_key,
			                      audience, audience_reason)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, $5, 'participants', 'pending_verdict')`,
			id, subject, "cnf-"+id.String(), from, threadKey); err != nil {
			return err
		}
		// The seat's own import row, carrying the posture the classified
		// mailbox imported under. This is the contribution the derivation reads.
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			VALUES ($1, $2, 'classified', 'pending')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding a held thread message: %v", err)
	}
	return id
}

func TestOneSeatsVerdictDoesNotPublishAColleaguesHeldMessage(t *testing.T) {
	// The per-owner model, at the point where it is easiest to break. A thread
	// reaching two mailboxes is two people's correspondence: each seat gets its
	// own ledger row, each may conclude differently, and the message ends at
	// the STRICTEST of their answers.
	//
	// The stamp that writes a verdict onto import rows is one UPDATE. Drop its
	// user_id clause and it writes every seat's contribution at once, so one
	// seat's `ordinary` publishes a message their colleague's mailbox is
	// holding — and nothing about the change looks wrong.
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-shared", "kunde@example.test", "Angebot")
	addImportRowFor(t, e, activityID, e.Rep2)
	threadID := seedThreadQuestion(t, e, "thread-shared", activityID)

	// Rep1's own verdict says ordinary. Rep2 has not been judged, so Rep2's
	// import row is still pending and still asks for the message to be held.
	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("the message is %q, want participants — one seat's ordinary verdict must not "+
			"publish a message a colleague's mailbox is still holding", got)
	}
	if got := importVerdictFor(t, e, activityID, e.Rep2); got != "pending" {
		t.Fatalf("the colleague's import row says %q, want pending — a verdict is scoped to the "+
			"seat whose thread ledger it came from", got)
	}
}

// addImportRowFor makes a second seat a contributor to one message's audience.
func addImportRowFor(t *testing.T, e *integration.Env, activityID, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			VALUES ($1, $2, 'classified', 'pending')`, activityID, user)
		return err
	}); err != nil {
		t.Fatalf("adding a second importer: %v", err)
	}
}

// importVerdictFor reads one seat's contribution to a message's audience.
func importVerdictFor(t *testing.T, e *integration.Env, activityID, user ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(verdict_status, '') FROM capture_import
			 WHERE activity_id = $1 AND user_id = $2`, activityID, user).Scan(&status)
	}); err != nil {
		t.Fatalf("reading a seat's import verdict: %v", err)
	}
	return status
}

func TestAHeldMailboxIsNeverAskedAndNeverOpened(t *testing.T) {
	// The strongest privacy setting the product offers. `held` means "hold this
	// whatever a classifier concludes", and the audience derivation evaluates a
	// verdict BEFORE a posture — so a cleared answer about a held mailbox's mail
	// would widen the row and overrule the owner.
	//
	// The defence is that the question is never opened. Both `held` and
	// `classified` produce the same audience, so the enqueue has to read the
	// posture itself; a check on the derived audience cannot tell them apart.
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-heldbox", "kunde@example.test", "Angebot")
	setImportPosture(t, e, activityID, e.Rep1, "held")

	// No ledger row exists for this thread, so a pass finds nothing to judge.
	// Running one anyway is the assertion: an engine that opened the question
	// would clear the message here.
	engine := NewConfidentialityVerdictEngine(e.Pool, &scriptedConfidentialityBrain{
		kind: confidentialityOrdinary, confidence: 0.99,
	}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}

	if n := countThreadQuestions(t, e, "thread-heldbox"); n != 0 {
		t.Fatalf("%d confidentiality questions for a held mailbox, want 0 — its owner already answered", n)
	}
	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("a held mailbox's message is %q, want participants", got)
	}
}

// setImportPosture rewrites the posture a message was imported under.
func setImportPosture(t *testing.T, e *integration.Env, activityID, user ids.UUID, posture string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_import SET posture_at_import = $3
			 WHERE activity_id = $1 AND user_id = $2`, activityID, user, posture)
		return err
	}); err != nil {
		t.Fatalf("setting the import posture: %v", err)
	}
}

func countThreadQuestions(t *testing.T, e *integration.Env, threadKey string) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_thread_verdict WHERE thread_key = $1`, threadKey).Scan(&n)
	}); err != nil {
		t.Fatalf("counting thread questions: %v", err)
	}
	return n
}

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
func TestAPersonalVerdictKeepsAContactWhoAlsoWritesAboutBusiness(t *testing.T) {
	e := integration.Setup(t)
	const both = "cousin@family.test"
	privateActivity := seedHeldThreadMail(t, e, "thread-private", both, "Familie")
	threadID := seedThreadQuestion(t, e, "thread-private", privateActivity)
	// A second conversation with the same person that nothing judged personal.
	seedHeldThreadMail(t, e, "thread-business", both, "Angebot")
	personID := seedCapturedContact(t, e, both)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if personIsArchived(t, e, personID) {
		t.Fatal("a contact who also writes about business was retracted — one private " +
			"conversation does not make somebody's whole correspondence private")
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

// TestAThreadWhoseMessageWasErasedRetiresRatherThanBeingJudgedOnNothing pins the
// end of a question nothing can answer.
//
// first_activity_id is nullable and its foreign key is ON DELETE SET NULL, so
// erasing the message a question was raised about leaves the question standing
// with nothing to ask about. The claim used to take that row anyway: its
// subject and body lookups return empty for a missing activity, so the model
// was asked to judge a blank prompt and its answer was recorded as if it had
// read the conversation.
func TestAThreadWhoseMessageWasErasedRetiresRatherThanBeingJudgedOnNothing(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-erased", "kunde@example.test", "Angebot")
	threadID := seedThreadQuestion(t, e, "thread-erased", activityID)

	// The erasure the retention path performs, which SET NULLs the pointer.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`DELETE FROM activity WHERE id = $1`, activityID); err != nil {
			return err
		}
		// Past the grace a live capture is given to supply its own pointer.
		// The erasure is minutes old by the time a sweep sees it; a row this
		// second old belongs to a transaction still in flight.
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_thread_verdict SET updated_at = now() - interval '1 hour' WHERE id = $1`,
			threadID)
		return err
	}); err != nil {
		t.Fatalf("erasing the message the question was about: %v", err)
	}

	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming due threads: %v", err)
	}
	for _, c := range claimed {
		if c.ID == threadID {
			t.Fatal("a thread whose message is gone was claimed, and would be judged on an empty prompt")
		}
	}

	if _, err := store.RetireExhausted(e.Admin(), "unreadable"); err != nil {
		t.Fatalf("retiring: %v", err)
	}
	if got := threadStatus(t, e, threadID); got != "unsure" {
		t.Fatalf("thread status = %q, want unsure: a question nothing can answer must reach a terminal state", got)
	}
}

// TestAThreadPointedAtRestrictedMailIsNotJudgedOnAnEmptyPrompt is the same
// failure reached by the other route.
//
// The claim's own subject and body lookups exclude a restricted row, so a
// message put under a statutory hold after its question opened leaves the
// classifier reading nothing while the row still looks answerable.
func TestAThreadPointedAtRestrictedMailIsNotJudgedOnAnEmptyPrompt(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-restricted", "kunde@example.test", "Angebot")
	threadID := seedThreadQuestion(t, e, "thread-restricted", activityID)

	// A hold needs the evidence that qualified it: the database refuses a
	// restriction that records no basis, which is the point of the trigger.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			       (activity_id, basis, qualified_at, decided_by_name, reason)
			VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`,
			activityID); err != nil {
			return err
		}
		// The whole shape a restriction takes: archived, with a reason and an
		// end date. The schema refuses any partial version of it.
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET restricted_at = now(), archived_at = now(),
			       restricted_reason = 'litigation hold',
			       restricted_until = now() + interval '1 year',
			       retention_class = 'commercial_correspondence', retention_class_at = now()
			 WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("placing the message under a hold: %v", err)
	}

	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming due threads: %v", err)
	}
	for _, c := range claimed {
		if c.ID == threadID {
			t.Fatal("a thread pointed at restricted mail was claimed, and its prompt would carry no text")
		}
	}
}

// TestAReadableThreadIsStillClaimed is the admit case for the two refusals
// above: a filter that refused everything would pass both of them.
func TestAReadableThreadIsStillClaimed(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-readable", "kunde@example.test", "Angebot")
	threadID := seedThreadQuestion(t, e, "thread-readable", activityID)

	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming due threads: %v", err)
	}
	for _, c := range claimed {
		if c.ID == threadID {
			if c.Subject != "Angebot" {
				t.Fatalf("claimed subject = %q, want the message's own", c.Subject)
			}
			return
		}
	}
	t.Fatal("a thread whose message is readable was not claimed")
}

// TestAnOrdinaryVerdictOpensTheSiblingsTheClassifierDidNotRead is the ordering
// accident this pass exists to correct.
//
// The classifier reads one message and answers about the conversation. Messages
// of that conversation already imported when the answer came back used to keep
// their import posture for good: the thread's question is answered, and the
// unique ledger row stops a second one being opened. In the demo database that
// is 230 messages held on threads their own verdict called ordinary.
func TestAnOrdinaryVerdictOpensTheSiblingsTheClassifierDidNotRead(t *testing.T) {
	e := integration.Setup(t)
	const customer = "einkauf@kunde.example"
	const lawyer = "anwalt@kanzlei.example"

	judged := seedHeldThreadMail(t, e, "thread-siblings", customer, "Nachbestellung")
	alsoCustomer := seedHeldThreadMail(t, e, "thread-siblings", customer, "Nachtrag")
	fromLawyer := seedHeldThreadMail(t, e, "thread-siblings", lawyer, "Aufhebungsvertrag")
	threadID := seedThreadQuestion(t, e, "thread-siblings", judged)

	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := activityAudience(t, e, judged); got != "workspace" {
		t.Fatalf("the judged message is %q, want workspace", got)
	}
	if got := activityAudience(t, e, alsoCustomer); got != "workspace" {
		t.Fatalf("a second message from the sender the verdict READ is %q, want workspace: "+
			"it is the same correspondence, and only import order kept it held", got)
	}
	if got := importVerdictFor(t, e, alsoCustomer, e.Rep1); got != capture.VerdictCleared {
		t.Fatalf("that message's import row says %q, want cleared", got)
	}

	// The message from a sender the verdict never read is NOT published on an
	// answer about somebody else's mail.
	if got := activityAudience(t, e, fromLawyer); got != "participants" {
		t.Fatalf("a message from a sender the verdict never read is %q, want participants", got)
	}
	if got := threadStatus(t, e, threadID); got != capture.VerdictPending {
		t.Fatalf("thread status = %q, want pending: the unread sender must be asked about", got)
	}
	var pointer ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT first_activity_id FROM capture_thread_verdict WHERE id = $1`, threadID).Scan(&pointer)
	}); err != nil {
		t.Fatalf("reading the re-opened pointer: %v", err)
	}
	if pointer != fromLawyer {
		t.Fatalf("the re-opened question points at %s, want the lawyer's message %s", pointer, fromLawyer)
	}
	// The senders this verdict cleared are remembered, so the next message from
	// one of them inherits instead of buying another model call.
	var seen []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT seen_addresses FROM capture_thread_verdict WHERE id = $1`, threadID).Scan(&seen)
	}); err != nil {
		t.Fatalf("reading seen addresses: %v", err)
	}
	if len(seen) == 0 || seen[0] != customer {
		t.Fatalf("seen addresses = %v, want the customer kept across the re-open", seen)
	}
}

// TestAHoldingVerdictHoldsEverySibling is the other half of the admission rule.
//
// "This conversation is private" is true of the whole conversation whoever
// wrote which part, so a holding verdict takes every message — including the
// one from a sender it never read, which an opening verdict must not touch.
func TestAHoldingVerdictHoldsEverySibling(t *testing.T) {
	e := integration.Setup(t)
	judged := seedHeldThreadMail(t, e, "thread-personnel", "hr@kunde.example", "Kündigung")
	fromLawyer := seedHeldThreadMail(t, e, "thread-personnel", "anwalt@kanzlei.example", "Aufhebung")
	threadID := seedThreadQuestion(t, e, "thread-personnel", judged)

	runConfidentiality(t, e, threadID, confidentialityPersonnel, 0.95)

	for _, id := range []ids.UUID{judged, fromLawyer} {
		if got := activityAudience(t, e, id); got != "participants" {
			t.Fatalf("a message on a personnel thread is %q, want participants", got)
		}
		if got := importVerdictFor(t, e, id, e.Rep1); got != capture.VerdictHeld {
			t.Fatalf("its import row says %q, want held — including the message the "+
				"classifier never read, because the conversation is what was judged", got)
		}
	}
	if got := threadStatus(t, e, threadID); got == capture.VerdictPending {
		t.Fatal("a held thread was re-opened; new information that it involves somebody " +
			"unexpected is never a reason to re-ask whether it may be published")
	}
}

// TestOneSeatsSiblingStampDoesNotPublishAColleaguesHeldMessage is the per-owner
// model at the point the wider stamp could break it.
func TestOneSeatsSiblingStampDoesNotPublishAColleaguesHeldMessage(t *testing.T) {
	e := integration.Setup(t)
	const customer = "einkauf@kunde.example"
	judged := seedHeldThreadMail(t, e, "thread-two-seats", customer, "Nachbestellung")
	shared := seedHeldThreadMail(t, e, "thread-two-seats", customer, "Nachtrag")
	addImportRowFor(t, e, shared, e.Rep2)
	threadID := seedThreadQuestion(t, e, "thread-two-seats", judged)

	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := activityAudience(t, e, shared); got != "participants" {
		t.Fatalf("a message a colleague's mailbox still holds is %q, want participants: "+
			"one seat's answer is not the other's", got)
	}
	if got := importVerdictFor(t, e, shared, e.Rep1); got != capture.VerdictCleared {
		t.Fatalf("the answering seat's own contribution says %q, want cleared", got)
	}
}

// TestAnOutboundSiblingIsJudgedByItsOwnPartyNotOpenedOnATrade is the direction
// case, and the one a `counterparty_email` comparison gets wrong on its own.
//
// That column holds the OTHER party, so it is the author of an inbound message
// and the recipient of an outbound one. An ordinary verdict about mail from a
// customer must not open, on that evidence alone, an unread message the seat
// WROTE to somebody else on the same thread.
func TestAnOutboundSiblingIsJudgedByItsOwnPartyNotOpenedOnATrade(t *testing.T) {
	e := integration.Setup(t)
	const customer = "einkauf@kunde.example"
	const lawyer = "anwalt@kanzlei.example"

	judged := seedHeldThreadMail(t, e, "thread-outbound", customer, "Nachbestellung")
	// The seat's own reply, addressed to the SAME customer. Its counterparty is
	// therefore an address the verdict read — while its author is the seat, and
	// its text is one nothing has judged.
	ownReply := seedHeldThreadMail(t, e, "thread-outbound", customer, "Re: Nachbestellung")
	toLawyer := seedHeldThreadMail(t, e, "thread-outbound", lawyer, "Aufhebungsvertrag")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET direction = 'outbound' WHERE id = ANY($1)`,
			[]ids.UUID{ownReply, toLawyer})
		return err
	}); err != nil {
		t.Fatalf("making the siblings outbound: %v", err)
	}
	threadID := seedThreadQuestion(t, e, "thread-outbound", judged)

	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	// The seat's own reply to the party the verdict read is part of that same
	// exchange, and opens with it.
	if got := activityAudience(t, e, ownReply); got != "workspace" {
		t.Fatalf("the seat's own reply to the party the verdict read is %q, want workspace", got)
	}
	if got := activityAudience(t, e, toLawyer); got != "participants" {
		t.Fatalf("a message the seat WROTE to a party the verdict never read is %q, want "+
			"participants: the answer was about somebody else's mail", got)
	}
}

// TestAThirdSenderCohortIsHeldRatherThanLeftUndecided is the end of the budget.
//
// A claim charges an attempt and the ceiling is two, so a thread carrying three
// parties the classifier has to be asked about separately runs out. The thread
// then retires to `unsure`, which HOLDS — and the messages of the cohort nobody
// reached must end held too, rather than sitting undecided forever with the
// ledger saying the question is over.
func TestAThirdSenderCohortIsHeldRatherThanLeftUndecided(t *testing.T) {
	e := integration.Setup(t)
	judged := seedHeldThreadMail(t, e, "thread-three", "a@kunde.example", "Erste")
	second := seedHeldThreadMail(t, e, "thread-three", "b@kunde.example", "Zweite")
	third := seedHeldThreadMail(t, e, "thread-three", "c@kunde.example", "Dritte")
	threadID := seedThreadQuestion(t, e, "thread-three", judged)

	// Two passes: each answers one cohort and re-opens for the next. The
	// re-open is due a minute out, so the row is aged between them the way a
	// later tick would find it.
	for i := 0; i < 2; i++ {
		runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), `
				UPDATE capture_thread_verdict
				   SET next_attempt_at = now() - interval '1 minute',
				       updated_at = now() - interval '1 hour'
				 WHERE id = $1`, threadID)
			return err
		}); err != nil {
			t.Fatalf("ageing the re-opened thread: %v", err)
		}
	}

	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	if _, err := store.RetireExhausted(e.Admin(), "attempts"); err != nil {
		t.Fatalf("retiring: %v", err)
	}

	if got := threadStatus(t, e, threadID); got != "unsure" {
		t.Fatalf("thread status = %q, want unsure: a thread nobody could finish must reach a "+
			"terminal state rather than staying claimable forever", got)
	}
	// The two cohorts that WERE answered are open; the one nobody reached is
	// held, which is the safe direction for a message nothing judged.
	for _, id := range []ids.UUID{judged, second} {
		if got := activityAudience(t, e, id); got != "workspace" {
			t.Fatalf("an answered cohort's message is %q, want workspace", got)
		}
	}
	if got := activityAudience(t, e, third); got != "participants" {
		t.Fatalf("the cohort nobody reached is %q, want participants", got)
	}
}
