// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The commitments lane against a real database.
//
// The unit lane cannot see any of what these pin. Whose promises a rep is shown
// is a WHERE clause on person.owner_id; which claims are settled is a filter on
// two lifecycle columns nothing in Go touches; and the join that keeps a claim
// from outliving the message it quotes is SQL. Each of those reads as a working
// lane in a stub and can be wrong here.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// laneClock pins the read instant so "due by end of today" is a fixed boundary
// rather than whatever the suite happened to run at.
var laneClock = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

// seedPromise records one claim through the REAL writer, against a message
// that really exists, and returns the claim id.
//
// Through RecordConversationClaim rather than an INSERT on purpose: a lane
// filled by rows the production writer never produces would prove nothing
// about the production writer, and this writer is the one that takes the
// person and activity gates.
func seedPromise(
	t *testing.T, e *integration.Env, personID ids.UUID, body string, due *time.Time,
) ids.UUID {
	t.Helper()
	subject := "Rückfragen zum Angebot"
	occurred := laneClock.AddDate(0, 0, -7)
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, OccurredAt: &occurred, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: personID}},
	})
	if err != nil {
		t.Fatalf("logging the message the promise was made in: %v", err)
	}
	claim, err := people.NewStore(e.DB()).RecordConversationClaim(e.Admin(), people.ClaimInput{
		PersonID: ids.From[ids.PersonKind](personID), Kind: "commitment_ours",
		Body: body, ActivityID: ids.UUID(message.Id), Quote: body, DueAt: due,
		Source: "manual",
	})
	if err != nil {
		t.Fatalf("recording the promise: %v", err)
	}
	return ids.UUID(claim.Id)
}

func bodiesOf(rows []people.CommitmentDue) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Body)
	}
	return out
}

// A rep is shown the promises made to the people THEY own, and nobody else's.
// Ownership is the only thing standing between one rep's lane and another's,
// and it is a WHERE clause — so it is checked against a database that has both
// reps' promises in it.
func TestACommitmentReachesOnlyTheRepWhoOwnsTheRelationship(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	mine := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	theirs := e.SeedPerson(t, "Frau Keller", &e.Rep2)
	seedPromise(t, e, mine, "Referenzliste schicken", &due)
	seedPromise(t, e, theirs, "Angebot überarbeiten", &due)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading rep1's promises: %v", err)
	}
	if got := bodiesOf(rows); len(got) != 1 || got[0] != "Referenzliste schicken" {
		t.Fatalf("rep1's lane = %v, want only their own promise", got)
	}
}

// A promise on a person nobody owns reaches nobody. It is a real promise and
// there is no rep it is honestly on the hook for, so a lane that showed it to
// everyone would put one person's work on every screen.
func TestAPromiseOnAnUnownedPersonReachesNobodysLane(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	orphan := e.SeedPerson(t, "Niemands Kontakt", nil)
	seedPromise(t, e, orphan, "Unterlagen nachreichen", &due)
	// Genuinely ownerless. A create that names no owner is stamped with the
	// ACTING user (storekit.OwnerOrActor), so seeding with nil produces an
	// admin-owned person rather than an unowned one — and a person nobody owns
	// is what this test is about.
	e.WsExec(t, `UPDATE person SET owner_id = NULL WHERE id = $1`, orphan)

	store := people.NewStore(e.DB())
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3, e.AdminUser} {
		rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](rep), laneClock.Add(24*time.Hour), 20)
		if err != nil {
			t.Fatalf("reading a rep's promises: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("an unowned person's promise reached a lane: %v", bodiesOf(rows))
		}
	}
}

// A settled or disputed claim is not on the lane. `done` and `dismissed` are
// finished, and `needs_review` means the extractor found contradicting
// evidence — presenting that as "you promised this" states a contested thing
// as a fact.
func TestASettledOrDisputedPromiseLeavesTheLane(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	open := seedPromise(t, e, person, "Bleibt offen", &due)
	done := seedPromise(t, e, person, "Schon erledigt", &due)
	dismissed := seedPromise(t, e, person, "War nie eine Zusage", &due)
	disputed := seedPromise(t, e, person, "Widersprüchlich belegt", &due)

	// These are the extractor's own lifecycle columns and have no writer on
	// this path, so they are set directly — the filter is exercised against
	// the states it exists to reject.
	e.WsExec(t, `UPDATE conversation_claim SET status = 'done' WHERE id = $1`, done)
	e.WsExec(t, `UPDATE conversation_claim SET status = 'dismissed' WHERE id = $1`, dismissed)
	e.WsExec(t, `UPDATE conversation_claim SET needs_review = true WHERE id = $1`, disputed)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	if got := bodiesOf(rows); len(got) != 1 || got[0] != "Bleibt offen" {
		t.Fatalf("the lane = %v, want only the open, undisputed promise", got)
	}
	if rows[0].ID != open {
		t.Errorf("the surviving row is %s, want the open claim %s", rows[0].ID, open)
	}
}

// An undated promise is not today's work, and a promise due after today is not
// either. The window is the lane's whole claim to be a DAY's page.
func TestOnlyAPromiseDueByTheEndOfTodayIsOnTodaysLane(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	overdue := laneClock.AddDate(0, 0, -3)
	todayLater := laneClock.Add(6 * time.Hour)
	nextWeek := laneClock.AddDate(0, 0, 7)
	seedPromise(t, e, person, "Längst überfällig", &overdue)
	seedPromise(t, e, person, "Heute Nachmittag", &todayLater)
	seedPromise(t, e, person, "Nächste Woche", &nextWeek)
	seedPromise(t, e, person, "Ohne Datum", nil)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	got := bodiesOf(rows)
	if len(got) != 2 {
		t.Fatalf("the lane = %v, want the overdue one and this afternoon's", got)
	}
	// Soonest due first, so the most overdue promise leads the lane.
	if got[0] != "Längst überfällig" || got[1] != "Heute Nachmittag" {
		t.Errorf("the lane = %v, want the overdue promise first", got)
	}
}

// The lane carries the evidence, not just the paraphrase: the message's own
// subject and when it was said. Without them a card can say a promise exists
// and not where it came from, which is the half a reader needs to check it.
func TestTheLaneNamesTheConversationThePromiseWasMadeIn(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	seedPromise(t, e, person, "Referenzliste schicken", &due)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the lane carries %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.SourceLabel != "Rückfragen zum Angebot" {
		t.Errorf("source label = %q, want the message subject", row.SourceLabel)
	}
	if row.PersonName != "Herr Vogt" {
		t.Errorf("person name = %q, want the person it was promised to", row.PersonName)
	}
	if !row.OccurredAt.Equal(laneClock.AddDate(0, 0, -7)) {
		t.Errorf("occurred at %s, want when the message was sent", row.OccurredAt)
	}
	if row.SourceQuote == "" {
		t.Error("the verbatim quote is empty, so the promise cannot be checked")
	}
}

// A claim whose evidence has been archived leaves the lane with it. The claim
// contract's rule is that a claim is checkable against what was written, and a
// promise citing a message nobody can open any more is not.
func TestAPromiseWhoseEvidenceWasArchivedLeavesTheLane(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	claim := seedPromise(t, e, person, "Referenzliste schicken", &due)

	e.WsExec(t, `UPDATE activity SET archived_at = now()
		WHERE id = (SELECT source_activity_id FROM conversation_claim WHERE id = $1)`, claim)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a promise outlived its evidence: %v", bodiesOf(rows))
	}
}

// A caller that asks for no sensible bound gets the default rather than a
// broken statement. The limit is formatted into the SQL, so zero and negative
// are a syntax error at the database rather than an empty page — the failure
// mode a bound exists to remove.
func TestASweepWithNoSensibleBoundStillAnswers(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	seedPromise(t, e, person, "Referenzliste schicken", &due)

	store := people.NewStore(e.DB())
	for _, asked := range []int{0, -1} {
		rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), asked)
		if err != nil {
			t.Fatalf("a limit of %d failed the read: %v", asked, err)
		}
		if len(rows) != 1 {
			t.Errorf("a limit of %d returned %d rows, want the one open promise", asked, len(rows))
		}
	}
}

// A reader with no activity grant is REFUSED, not handed an empty lane. Every
// row this read returns quotes a message, so a caller who may not read messages
// may not read promises either — and the feed turns that refusal into a named
// omitted lane rather than a clear day.
//
// The admit case runs beside it deliberately: a refusal test alone passes just
// as well against a read that refuses everybody, which is how three security
// tests in this tree once went green against an authority that admitted nobody.
func TestAReaderWithoutTheActivityGrantIsRefusedRatherThanShownNothing(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	seedPromise(t, e, person, "Referenzliste schicken", &due)
	store := people.NewStore(e.DB())
	by := laneClock.Add(24 * time.Hour)
	owner := ids.From[ids.UserKind](e.Rep1)

	// REFUSE: read-only holds person but not activity.
	readOnly := e.As(e.Rep1, nil, integration.ReadOnlyPerms)
	if _, err := store.OpenCommitmentsDue(readOnly, owner, by, 20); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader with no activity grant got %v, want permission denied", err)
	}

	// ADMIT: the same seat with the activity grant reads its own promise. Without
	// this the test above would pass against a read that refused everyone.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	rows, err := store.OpenCommitmentsDue(rep, owner, by, 20)
	if err != nil {
		t.Fatalf("a rep with the activity grant was refused: %v", err)
	}
	if got := bodiesOf(rows); len(got) != 1 || got[0] != "Referenzliste schicken" {
		t.Fatalf("the rep's lane = %v, want their own promise", got)
	}
}

// A PROMISE DUE EXACTLY AT THE BOUND BELONGS TO TOMORROW.
//
// The caller passes the END of the day, so an inclusive test put a promise due
// at exactly tomorrow's midnight on today's list — reported late a day early,
// and again tomorrow when it actually is. The task lane beside this one already
// read it exclusively, so the two disagreed about the same afternoon.
func TestAPromiseDueExactlyAtTheBoundBelongsToTomorrow(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Frau Vogt", &e.Rep1)
	bound := laneClock.Add(24 * time.Hour)
	justInside := bound.Add(-time.Second)
	seedPromise(t, e, person, "Kurz vor Mitternacht", &justInside)
	seedPromise(t, e, person, "Punkt Mitternacht", &bound)

	store := people.NewStore(e.DB())
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), bound, 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	got := bodiesOf(rows)
	if len(got) != 1 || got[0] != "Kurz vor Mitternacht" {
		t.Fatalf("the lane = %v, want only the promise due before the day ends", got)
	}
}

// THE COUNT BESIDE THE PAGE IS THE SAME QUESTION, asked without a bound.
//
// The lane shows a dozen and the badge says how many there are, so the two must
// agree about WHICH promises those are: same owner, same window, same scopes. A
// count assembled from its own copy of the arms would drift from the cards under
// it one arm at a time, and nothing would fail to say so.
func TestTheCommitmentCountAnswersTheSameQuestionAsThePage(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	other := e.SeedPerson(t, "Frau Nachbar", &e.Rep2)
	todayLater := laneClock.Add(6 * time.Hour)
	nextWeek := laneClock.AddDate(0, 0, 7)
	for range 3 {
		seedPromise(t, e, person, "Heute Nachmittag", &todayLater)
	}
	// Neither of these is on this rep's lane: one is due next week, one belongs
	// to a colleague. A count that included either would be counting a lane the
	// reader is not looking at.
	seedPromise(t, e, person, "Nächste Woche", &nextWeek)
	seedPromise(t, e, other, "Nicht meins", &todayLater)

	store := people.NewStore(e.DB())
	bound := laneClock.Add(24 * time.Hour)
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), bound, 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	total, err := store.CountOpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), bound)
	if err != nil {
		t.Fatalf("counting the lane: %v", err)
	}
	if len(rows) != 3 || total != 3 {
		t.Fatalf("the page carries %d and the count says %d, want three each — the badge and the "+
			"cards must be answering the same question", len(rows), total)
	}
}

// AND IT COUNTS PAST THE PAGE'S BOUND, which is the whole point: a reader with
// more than fits is told how many, on a lane with no second page.
func TestTheCommitmentCountSeesPastTheLanesCap(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	todayLater := laneClock.Add(6 * time.Hour)
	for range 5 {
		seedPromise(t, e, person, "Heute Nachmittag", &todayLater)
	}

	store := people.NewStore(e.DB())
	bound := laneClock.Add(24 * time.Hour)
	rows, err := store.OpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), bound, 2)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	total, err := store.CountOpenCommitmentsDue(e.Admin(), ids.From[ids.UserKind](e.Rep1), bound)
	if err != nil {
		t.Fatalf("counting the lane: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("the page carries %d, want the bound of two", len(rows))
	}
	if total != 5 {
		t.Errorf("the count says %d, want five — a badge that stops at the page tells a rep with "+
			"five that they have two", total)
	}
}

// The board's column and the rep's own lane answer the SAME question.
//
// They are two readers of one thing: a lead scans a table, then opens the day
// behind a number that worried them. If the two disagree the lead is sent to a
// desk where the work is not, and neither screen says which is right.
//
// The multi-owner count exists because a board reaches a hundred people and a
// hundred sequential queries is a slow morning. What this holds is that making
// it one query did not make it a different question.
func TestTheBoardsPromiseColumnAgreesWithEachRepsOwnLane(t *testing.T) {
	e := integration.Setup(t)
	mine := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	theirs := e.SeedPerson(t, "Frau Keller", &e.Rep2)
	orphan := e.SeedPerson(t, "Niemands Kontakt", nil)
	todayLater := laneClock.Add(6 * time.Hour)
	nextWeek := laneClock.AddDate(0, 0, 7)

	for range 3 {
		seedPromise(t, e, mine, "Heute Nachmittag", &todayLater)
	}
	seedPromise(t, e, theirs, "Auch heute", &todayLater)
	// Neither of these belongs on today's board: one is due next week, one is
	// on a person nobody owns.
	seedPromise(t, e, mine, "Nächste Woche", &nextWeek)
	seedPromise(t, e, orphan, "Unterlagen nachreichen", &todayLater)

	store := people.NewStore(e.DB())
	bound := laneClock.Add(24 * time.Hour)
	owners := []ids.UserID{
		ids.From[ids.UserKind](e.Rep1),
		ids.From[ids.UserKind](e.Rep2),
	}
	board, err := store.CountOpenCommitmentsDueByOwner(e.Admin(), owners, bound)
	if err != nil {
		t.Fatalf("counting the team's promises: %v", err)
	}
	for _, owner := range owners {
		alone, err := store.CountOpenCommitmentsDue(e.Admin(), owner, bound)
		if err != nil {
			t.Fatalf("counting one rep's promises: %v", err)
		}
		if board[owner.UUID] != alone {
			t.Fatalf("the board says %d for %v and their own lane says %d — "+
				"a lead sent to that desk finds different work than the number promised",
				board[owner.UUID], owner, alone)
		}
	}
	// And the figures are the ones the fixture describes, so an agreement of
	// two zeros could not pass for agreement.
	if board[e.Rep1] != 3 || board[e.Rep2] != 1 {
		t.Fatalf("the board read %d and %d, wanted 3 and 1", board[e.Rep1], board[e.Rep2])
	}
	// The unowned promise is on nobody's column, exactly as it is on nobody's
	// lane.
	if len(board) != 2 {
		t.Fatalf("the board named %d owners over a roster of two", len(board))
	}
}
