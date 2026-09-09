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

// liveStops counts one subject's live stops of a given kind, which is the
// question the send engine asks.
func liveStops(t *testing.T, e *integration.Env, personID ids.UUID, kind string) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND kind = $2 AND revoked_at IS NULL`,
		personID, kind).Scan(&n); err != nil {
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
	if got := liveStops(t, e, survivor, commsauthz.ReasonObjection); got != 0 {
		t.Fatalf("precondition: the survivor already holds %d objection(s)", got)
	}

	if _, err := peopleStore.MergePerson(admin, ids.From[ids.PersonKind](objector), ids.From[ids.PersonKind](survivor)); err != nil {
		t.Fatalf("merging: %v", err)
	}

	// THE WHOLE POINT: the survivor is the record the engine now evaluates,
	// so the stop has to be reachable from there.
	if got := liveStops(t, e, survivor, commsauthz.ReasonObjection); got != 1 {
		t.Fatalf("the survivor holds %d live objection(s) after the merge, want 1 — "+
			"marketing would resume against somebody who refused it", got)
	}
	// AND THE ORIGINAL SURVIVES, because it is evidence about the record that
	// actually made the objection. Repointing it would make the history say
	// the objection was made about a different person.
	if got := liveStops(t, e, objector, commsauthz.ReasonObjection); got != 1 {
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

	if got := liveStops(t, e, survivor, commsauthz.ReasonObjection); got != 1 {
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
	if got := liveStops(t, e, stoppedSrc, commsauthz.ReasonObjection); got != 1 {
		t.Errorf("the refused merge left %d stop(s) on the source, want its own intact", got)
	}
}
