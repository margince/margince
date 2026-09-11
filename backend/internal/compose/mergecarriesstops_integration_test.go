// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A merge carries the retiring subject's STOPS, through the real people→consent
// edge.
//
// Here rather than in either module because neither can prove it alone: people
// owns the merge and cannot import consent, consent owns
// communication_suppression and cannot import people, and the seam between
// them is wired in this package. A test on either side would have to fake the
// other, and the defect this closes was precisely that nothing connected them.
//
// The defect: a merge moved person_consent and left communication_suppression
// pointing at the retired record. The stop was not deleted — it was orphaned,
// which is worse, because an export still shows it while the send path no
// longer asks about it. Somebody objects to marketing, their contact is later
// merged during ordinary cleanup, and marketing resumes against them with
// nothing in the audit saying a stop was dropped.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// liveObjections counts one subject's live marketing objections, which is the
// question the send engine asks of them.
func liveObjections(t *testing.T, e *integration.Env, personID ids.UUID) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND kind = $2 AND revoked_at IS NULL`,
		personID, commsauthz.ReasonObjection).Scan(&n); err != nil {
		t.Fatalf("counting live stops: %v", err)
	}
	return n
}

func TestAMergeCarriesTheRetiringPersonsObjection(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	// The person who objected, and the duplicate they will be merged into.
	objector := e.SeedPerson(t, "Objector", nil)
	survivor := e.SeedPerson(t, "Survivor", nil)

	if err := consentStore.Suppress(admin, consent.SuppressInput{
		PersonID: ids.From[ids.PersonKind](objector),
		Kind:     commsauthz.ReasonObjection,
		Reason:   "asked on the phone to stop the newsletter",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
	if got := liveObjections(t, e, survivor); got != 0 {
		t.Fatalf("precondition: the survivor already holds %d objection(s)", got)
	}

	if _, err := peopleStore.MergePerson(admin, ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	// THE WHOLE POINT: the survivor is the record the engine now evaluates,
	// so the stop has to be reachable from there.
	if got := liveObjections(t, e, survivor); got != 1 {
		t.Fatalf("the survivor holds %d live objection(s) after the merge, want 1 — "+
			"marketing would resume against somebody who refused it", got)
	}
	// AND THE ORIGINAL SURVIVES, because it is evidence about the record that
	// actually made the objection. Repointing it would make the history say
	// the objection was made about a different person.
	if got := liveObjections(t, e, objector); got != 1 {
		t.Errorf("the retired record holds %d objection(s), want its own kept as evidence", got)
	}

	// The carried row says where it came from, so the two do not read as two
	// independent objections.
	var carriedFrom *ids.UUID
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT carried_from FROM communication_suppression
		 WHERE person_id = $1 AND revoked_at IS NULL`, survivor).Scan(&carriedFrom); err != nil {
		t.Fatalf("reading the carried row: %v", err)
	}
	if carriedFrom == nil {
		t.Error("the carried stop names no origin, so nothing says which merge produced it")
	}
}

// THE AUTHORITY TRAVELS, or the merge is a laundering path: merge the record,
// then lift the stop as an ordinary admin row.
func TestACarriedObjectionKeepsTheSubjectsAuthority(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	objector := e.SeedPerson(t, "Authority Source", nil)
	survivor := e.SeedPerson(t, "Authority Survivor", nil)

	if err := consentStore.Suppress(admin, consent.SuppressInput{
		PersonID: ids.From[ids.PersonKind](objector), Kind: commsauthz.ReasonObjection, Reason: "stop the newsletter",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
	if _, err := peopleStore.MergePerson(admin, ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	var level string
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT decided_by_level FROM communication_suppression
		 WHERE person_id = $1 AND revoked_at IS NULL`, survivor).Scan(&level); err != nil {
		t.Fatalf("reading the carried row: %v", err)
	}
	if level != string(commsauthz.LevelSubject) {
		t.Fatalf("the carried objection landed at %q, want %q — an admin-level copy is one "+
			"any admin can lift, so merging would launder away the subject's own act",
			level, commsauthz.LevelSubject)
	}
}

// A survivor who already holds a live stop of the same kind keeps their own.
// Two live rows of one kind would mean the second lift silently re-enables
// mail the first was still refusing.
func TestACarryDoesNotDuplicateAStopTheSurvivorAlreadyHolds(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	objector := e.SeedPerson(t, "Duplicate Source", nil)
	survivor := e.SeedPerson(t, "Duplicate Survivor", nil)

	for _, id := range []ids.UUID{objector, survivor} {
		if err := consentStore.Suppress(admin, consent.SuppressInput{
			PersonID: ids.From[ids.PersonKind](id), Kind: commsauthz.ReasonObjection, Reason: "no marketing",
		}); err != nil {
			t.Fatalf("recording the objection: %v", err)
		}
	}

	if _, err := peopleStore.MergePerson(admin, ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	if got := liveObjections(t, e, survivor); got != 1 {
		t.Errorf("the survivor holds %d live objections after the merge, want exactly 1", got)
	}
}

// AN UNWIRED SEAM REFUSES A MERGE THAT WOULD DROP A STOP, and only that one.
//
// The refusal is conditioned on the DATA, not the wiring. A subject with no
// stop has none to lose, so an unwired installation merges them exactly as it
// did before this seam existed — refusing there would break every caller that
// builds a people store for a purpose with nothing to do with consent, and
// protect nobody.
func TestAnUnwiredMergeRefusesOnlyWhenAStopWouldBeLost(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	// Deliberately NOT .WithStopCarrier(...).
	unwired := people.NewStore(e.DB())

	// A pair with nothing recorded: merging them loses nothing.
	plainSrc := e.SeedPerson(t, "Plain Source", nil)
	plainDst := e.SeedPerson(t, "Plain Survivor", nil)
	if _, err := unwired.MergePerson(admin,
		ids.From[ids.PersonKind](plainSrc), ids.From[ids.PersonKind](plainDst)); err != nil {
		t.Fatalf("an ordinary merge was refused for want of a seam it did not need: %v", err)
	}

	// A pair where the retiring record objected: merging them WOULD lose it.
	stoppedSrc := e.SeedPerson(t, "Stopped Source", nil)
	stoppedDst := e.SeedPerson(t, "Stopped Survivor", nil)
	if err := consentStore.Suppress(admin, consent.SuppressInput{
		PersonID: ids.From[ids.PersonKind](stoppedSrc),
		Kind:     commsauthz.ReasonObjection,
		Reason:   "no marketing",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}

	_, err := unwired.MergePerson(admin,
		ids.From[ids.PersonKind](stoppedSrc), ids.From[ids.PersonKind](stoppedDst))
	if err == nil {
		t.Fatal("a merge that would have dropped a recorded stop went through unnoticed")
	}
	var notWired *people.StopCarrierNotWiredError
	if !errors.As(err, &notWired) {
		t.Fatalf("the merge failed with %v, want StopCarrierNotWiredError", err)
	}
	// And the stop is still where it was: the refusal rolled the merge back
	// rather than half-applying it.
	if got := liveObjections(t, e, stoppedSrc); got != 1 {
		t.Errorf("the refused merge left %d stop(s) on the source, want its own intact", got)
	}
}

// A WEAKER ROW ON THE SURVIVOR MUST NOT BLOCK A STRONGER OBJECTION.
//
// The first spelling of the carry compared kind and liveness only. So a
// survivor holding a user-level subject_request kept a subject-level one out —
// and an admin could then lift the weaker row, which is exactly the laundering
// path carrying the authority was meant to close: merge the record, then lift.
//
// Found by review, not by the tests above, because all of them started from a
// survivor with nothing recorded.
func TestAStrongerStopCarriesOverAWeakerOneTheSurvivorHolds(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	objector := e.SeedPerson(t, "Stronger Source", nil)
	survivor := e.SeedPerson(t, "Weaker Survivor", nil)

	// BOTH ROWS ARE THE SAME KIND, or the guard never fires and this test
	// proves nothing — found by mutation: with two different kinds it passed
	// with the authority comparison reverted.
	//
	// The survivor's, written at the SEAT's level: an admin recorded it, so it
	// lands at admin and any admin can lift it.
	if err := consentStore.Suppress(admin, consent.SuppressInput{
		PersonID: ids.From[ids.PersonKind](survivor),
		Kind:     commsauthz.ReasonObjection,
		Reason:   "recorded by an admin",
	}); err != nil {
		t.Fatalf("recording the survivor's stop: %v", err)
	}
	// Then force it down to a weaker authority than the source will carry. A
	// marketing_objection is always written at subject level by design, so the
	// only way to build the weaker-survivor case this guard exists for is to
	// plant it — which is honest here: a legacy row predating that rule is
	// exactly the shape an installation holds.
	if _, err := e.Pool.Exec(context.Background(), `
		UPDATE communication_suppression SET decided_by_level = 'user'
		 WHERE person_id = $1 AND revoked_at IS NULL`, survivor); err != nil {
		t.Fatalf("planting the weaker survivor row: %v", err)
	}
	// The retiring record's, at the SUBJECT's level.
	if err := consentStore.Suppress(admin, consent.SuppressInput{
		PersonID: ids.From[ids.PersonKind](objector),
		Kind:     commsauthz.ReasonObjection,
		Reason:   "objected to marketing",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}

	if _, err := peopleStore.MergePerson(admin,
		ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	// The objection reached the survivor at its own authority, so no seat can
	// lift it.
	// Asked as EXISTS rather than max(): max() on text sorts alphabetically,
	// where "user" comes after "subject" — so the first spelling of this
	// assertion reported the weaker row as the strongest one and failed a
	// working carry. The question is simply whether a subject-level objection
	// reached the survivor at all.
	var reached bool
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM communication_suppression
			 WHERE person_id = $1 AND kind = $2 AND revoked_at IS NULL
			   AND decided_by_level = $3)`,
		survivor, commsauthz.ReasonObjection, string(commsauthz.LevelSubject)).Scan(&reached); err != nil {
		t.Fatalf("reading the survivor's stops: %v", err)
	}
	if !reached {
		t.Error("no subject-level objection reached the survivor: a weaker row kept the subject's " +
			"own act out, and an admin can lift what remains")
	}
}

// TWO LIVE STOPS OF ONE KIND ON THE SOURCE CARRY AS ONE.
//
// A subquery in an INSERT ... SELECT does not see the rows that statement is
// inserting — Postgres evaluates it against the statement's snapshot — so the
// NOT EXISTS could not deduplicate within one carry. Direct suppression
// deliberately allows repeated requests to stack up, so a source holding two
// is an ordinary state, not a corrupt one.
func TestASubjectHoldingTwoStopsOfOneKindCarriesOne(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	objector := e.SeedPerson(t, "Twice Asked", nil)
	survivor := e.SeedPerson(t, "Twice Survivor", nil)

	for _, reason := range []string{"asked in January", "asked again in March"} {
		if err := consentStore.Suppress(admin, consent.SuppressInput{
			PersonID: ids.From[ids.PersonKind](objector),
			Kind:     "subject_request",
			Reason:   reason,
		}); err != nil {
			t.Fatalf("recording %q: %v", reason, err)
		}
	}

	if _, err := peopleStore.MergePerson(admin,
		ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	var n int
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND kind = 'subject_request' AND revoked_at IS NULL`,
		survivor).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("the survivor holds %d live subject_requests, want 1 — a second row is one more "+
			"lift somebody has to remember to make", n)
	}
}

// A STOP RECORDED WHILE THE MERGE RUNS must still reach the survivor.
//
// The two writers touch different rows — Suppress inserts against the retiring
// person, the carry reads them and inserts against the survivor — so nothing in
// Postgres makes them queue on their own. Before suppressionlock.go they did
// not, and the loser was the stop:
//
//	Suppress(objector)              MergePerson(objector -> survivor)
//	   BEGIN                            BEGIN
//	                                    reads the objector's live rows: none
//	   INSERT the objection
//	   COMMIT                           COMMIT, having carried nothing
//
// The objection then exists only on the record the merge retired, which is the
// orphaned-stop defect this whole file exists to close, reintroduced through
// the carry's own concurrency rather than through the merge forgetting.
//
// The test drives that exact order: the merge is held open until the concurrent
// Suppress has been ISSUED, so with no lock the carry reads before the insert
// lands. With the lock, one of the two waits for the other and the stop is
// carried either by the merge or by being already present when it looks.
func TestAStopRecordedDuringAMergeStillReachesTheSurvivor(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	consentStore := consent.NewStore(e.DB())
	peopleStore := people.NewStore(e.DB()).WithStopCarrier(consentStore)

	objector := e.SeedPerson(t, "Late Objector", nil)
	survivor := e.SeedPerson(t, "Survivor", nil)

	// The concurrent writer, started first so its transaction is genuinely
	// open while the merge runs. It reports back rather than failing from
	// another goroutine.
	suppressed := make(chan error, 1)
	go func() {
		suppressed <- consentStore.Suppress(admin, consent.SuppressInput{
			PersonID: ids.From[ids.PersonKind](objector),
			Kind:     commsauthz.ReasonObjection,
			Reason:   "called while their duplicate was being merged",
		})
	}()

	// The merge races it. Whichever order the two land in, the survivor must
	// end up holding the objection: either the carry saw it, or the carry ran
	// first and the stop landed on a record the merge had already retired —
	// which is the case the lock exists to make impossible.
	if _, err := peopleStore.MergePerson(admin,
		ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}
	if err := <-suppressed; err != nil {
		t.Fatalf("recording the concurrent objection: %v", err)
	}

	if got := liveObjections(t, e, survivor); got != 1 {
		t.Fatalf("the survivor holds %d live objection(s), want 1 — a stop recorded "+
			"during the merge landed only on the record the merge retired, so the "+
			"send path will never see it", got)
	}
}
