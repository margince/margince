// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the gone-quiet review actually asks.
//
// The reason and the proposed date are both composed from the database, so a
// unit test over the sentence proves nothing about the card a person sees.
// These read the staged approval the sweep produced.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedCorrection is the proposal as the card will read it.
func (e *closeDateEnv) stagedCorrection(t *testing.T, dealID ids.UUID) deals.CloseDateCorrection {
	t.Helper()
	var raw json.RawMessage
	if err := e.owner.QueryRow(context.Background(),
		`SELECT proposed_change FROM approval
		  WHERE kind = 'close_date_correction' AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&raw); err != nil {
		t.Fatalf("no staged correction on deal %s: %v", dealID, err)
	}
	correction, err := deals.UnmarshalCloseDateCorrection(raw)
	if err != nil {
		t.Fatalf("staged payload does not decode: %v", err)
	}
	return correction
}

// grantOwnerRealPermissions gives the deal owner an ACTUAL role row.
//
// The harness's in-memory `As(...)` permissions do not reach this path on
// purpose: the sweep resolves the owner's authority from the database through
// EffectiveAuthority, which is the whole point — a card is named only under
// grants the owner really holds, not under whatever a caller asserted. So a
// test that wants a named reason has to grant the owner the objects for real.
func (e *closeDateEnv) grantOwnerRealPermissions(t *testing.T, userID ids.UUID) {
	t.Helper()
	e.grantOwnerRole(t, userID, `{"objects":{"person":{"read":true},
		   "deal":{"read":true,"update":true},"activity":{"read":true},
		   "organization":{"read":true}},"row_scope":"all"}`)
}

// grantOwnerWithoutPeople is the same owner minus person:read — the reader the
// name gate is supposed to refuse. Everything else is identical, so a test
// using it isolates exactly the one grant.
func (e *closeDateEnv) grantOwnerWithoutPeople(t *testing.T, userID ids.UUID) {
	t.Helper()
	e.grantOwnerRole(t, userID, `{"objects":{"deal":{"read":true,"update":true},
		   "activity":{"read":true},"organization":{"read":true}},
		  "row_scope":"all"}`)
}

func (e *closeDateEnv) grantOwnerRole(t *testing.T, userID ids.UUID, document string) {
	t.Helper()
	ctx := context.Background()
	roleID := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO role (id, key, name, permissions)
		 VALUES ($1, $2, 'Quiet review rep', $3)`,
		roleID, "quiet_rep_"+roleID.String()[:8], document); err != nil {
		t.Fatalf("seeding the owner's role: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO role_assignment (id, user_id, role_id) VALUES ($1, $2, $3)`,
		ids.NewV7(), userID, roleID); err != nil {
		t.Fatalf("assigning the owner's role: %v", err)
	}
}

// seedDealEmail links one message to the deal, in a direction, from or to a
// named person — the correspondence the review reads to say what happened.
//
// Every message a test seeds must be OLDER than StalledThresholdDays. Linking
// an activity fires activity_link_last_activity, which pushes the deal's
// last_activity_at forward to the message's date; a recent message therefore
// makes the deal active, the 🔻 tier never fires, and the test fails looking
// for a card the sweep correctly declined to stage.
func (e *closeDateEnv) seedDealEmail(t *testing.T, dealID ids.UUID, direction, personName string, daysAgo int) {
	t.Helper()
	ctx := context.Background()
	personID := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO person (id, full_name, source, captured_by)
		 VALUES ($1, $2, 'manual', 'human:x')`, personID, personName); err != nil {
		t.Fatalf("seeding person %q: %v", personName, err)
	}
	activityID := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity (id, kind, direction, occurred_at, source, captured_by)
		 VALUES ($1, 'email', $2, now() - make_interval(days => $3), 'manual', 'human:x')`,
		activityID, direction, daysAgo); err != nil {
		t.Fatalf("seeding %s activity: %v", direction, err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		 VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), activityID, dealID); err != nil {
		t.Fatalf("linking activity to deal: %v", err)
	}
	// The counterparty's role differs by direction: they SEND an inbound
	// message and RECEIVE an outbound one.
	role := "to"
	if direction == "inbound" {
		role = "from"
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_participant (id, activity_id, person_id, role)
		 VALUES ($1, $2, $3, $4)`, ids.NewV7(), activityID, personID, role); err != nil {
		t.Fatalf("seeding participant: %v", err)
	}
}

// addParticipant puts a SECOND person in the same role on the deal's newest
// message, turning a one-to-one exchange into group correspondence.
func (e *closeDateEnv) addParticipant(t *testing.T, dealID ids.UUID, personName, role string) {
	t.Helper()
	ctx := context.Background()
	personID := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO person (id, full_name, source, captured_by)
		 VALUES ($1, $2, 'manual', 'human:x')`, personID, personName); err != nil {
		t.Fatalf("seeding person %q: %v", personName, err)
	}
	var activityID ids.UUID
	if err := e.owner.QueryRow(ctx,
		`SELECT a.id FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1`, dealID).Scan(&activityID); err != nil {
		t.Fatalf("finding the deal's newest activity: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_participant (id, activity_id, person_id, role)
		 VALUES ($1, $2, $3, $4)`, ids.NewV7(), activityID, personID, role); err != nil {
		t.Fatalf("seeding the second participant: %v", err)
	}
}

// addAddressParticipant puts an ADDRESS-ONLY participant on the deal's newest
// message — a real and common shape, since an address that matched nobody still
// gets a row, and privacy erasure nulls person_id on rows that once matched.
func (e *closeDateEnv) addAddressParticipant(t *testing.T, dealID ids.UUID, address, role string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_participant (id, activity_id, address, role)
		 VALUES ($1, (SELECT a.id FROM activity a
		               JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $2
		              ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1), $3, $4)`,
		ids.NewV7(), dealID, address, role); err != nil {
		t.Fatalf("seeding the address participant: %v", err)
	}
}

// The review used to say "deal has gone quiet; confirm it is still alive" on
// every quiet deal — a sentence with no fact in it. It must name who is
// waiting, and it must say the silence is ours to break.
func TestQuietReviewNamesTheContactWhoIsWaitingForAReply(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerRealPermissions(t, e.Rep1)
	id := e.seedSweepDeal(t, "Gone quiet", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "outbound", "Boris Klein", 120)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if !strings.Contains(basis, "Anna Weber") {
		t.Errorf("basis = %q, want it to name Anna Weber — she wrote last", basis)
	}
	if !strings.Contains(basis, "nobody has answered") {
		t.Errorf("basis = %q, want it to say the reply is ours to send", basis)
	}
	if strings.Contains(basis, "Boris Klein") {
		t.Errorf("basis = %q — Boris wrote earlier; naming him reports the wrong silence", basis)
	}
}

// The other direction is a different finding and a different next action: the
// prospect went cold, and nobody here dropped anything.
func TestQuietReviewSaysWeGotNoReplyWhenWeWroteLast(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerRealPermissions(t, e.Rep1)
	id := e.seedSweepDeal(t, "No reply", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 120)
	e.seedDealEmail(t, id, "outbound", "Anna Weber", 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if !strings.Contains(basis, "We wrote to Anna Weber") {
		t.Errorf("basis = %q, want it to say we wrote and got nothing back", basis)
	}
}

// A deal with no correspondence at all still gets a reason, and it says so
// rather than inventing one.
func TestQuietReviewSaysSoWhenThereIsNoCorrespondence(t *testing.T) {
	e := setupCloseDate(t)
	// The owner is granted for real: without a grant the review would refuse the
	// read and fall back, which is a DIFFERENT sentence with its own test. This
	// one is about a deal that genuinely has no correspondence.
	e.grantOwnerRealPermissions(t, e.Rep1)
	id := e.seedSweepDeal(t, "Never contacted", e.late, stringp("commit"), intp(30), 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if !strings.Contains(basis, "no correspondence") {
		t.Errorf("basis = %q, want it to say there is nothing to judge the deal by", basis)
	}
}

// The name comes from a read under the DEAL OWNER's authority, so a deal with
// no owner has no authority to read under. It is still reviewed — the date
// hygiene does not depend on who owns it — but it is reviewed unnamed rather
// than under the sweep's own unbounded system principal, which would resolve
// any name in the workspace into a payload other people later read.
func TestQuietReviewOnAnUnownedDealNamesNobody(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Orphaned", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET owner_id = NULL WHERE id = $1`, id); err != nil {
		t.Fatalf("clearing the owner: %v", err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if strings.Contains(basis, "Anna Weber") {
		t.Errorf("basis = %q — an unowned deal has no authority to read names under", basis)
	}
	if basis == "" {
		t.Error("an unowned deal still gets a reason, just an unnamed one")
	}
}

// A message to four people has no single person the silence belongs to.
// Picking one — by id order or any other arbitrary rule — prints a name the
// reader can check against the thread and find misleading, so group
// correspondence reports its dates with no name attached.
func TestQuietReviewNamesNobodyOnGroupCorrespondence(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerRealPermissions(t, e.Rep1)
	id := e.seedSweepDeal(t, "Group thread", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)
	e.addParticipant(t, id, "Boris Klein", "from")

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	for _, name := range []string{"Anna Weber", "Boris Klein"} {
		if strings.Contains(basis, name) {
			t.Errorf("basis = %q — two senders means no single one to name", basis)
		}
	}
	if !strings.Contains(basis, "The contact wrote") {
		t.Errorf("basis = %q, want the unnamed reading with the dates intact", basis)
	}
}

// An address that never resolved to a person is still somebody on the thread.
// Counting only the MATCHED participants would read "Anna plus two unknown
// addresses" as a private exchange and name her for a silence three people
// share — the same misnaming the group rule exists to prevent, reached by the
// half of the data that is easy to forget.
func TestQuietReviewCountsUnmatchedAddressesAsParticipants(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerRealPermissions(t, e.Rep1)
	id := e.seedSweepDeal(t, "Mixed thread", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)
	e.addAddressParticipant(t, id, "someone@unmatched.test", "from")

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if strings.Contains(basis, "Anna Weber") {
		t.Errorf("basis = %q — an unmatched address is still a second participant", basis)
	}
}

// The security property, stated as a test.
//
// The sweep runs as a system principal, which passes auth.Require
// unconditionally and which no row scope bounds. If the review resolved names
// under THAT, an owner who may not read people would still get the name written
// into their card — a disclosure frozen into a stored record, which no
// read-side gate can undo. Reading as the owner is what makes the refusal land.
//
// The dates survive the refusal: when the silence started is on the deal's own
// correspondence, who it was with belongs to the person record, and losing the
// second is no reason to throw away the first.
func TestQuietReviewWithoutPersonReadGivesDatesButNoName(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerWithoutPeople(t, e.Rep1)
	id := e.seedSweepDeal(t, "Gone quiet", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if strings.Contains(basis, "Anna Weber") {
		t.Errorf("basis = %q — the owner holds no person:read, so the card must not name her", basis)
	}
	if !strings.Contains(basis, "The contact wrote") {
		t.Errorf("basis = %q, want the unnamed reading with the dates still in it", basis)
	}
}

// Row scope and object grant answer different questions — WHICH activities may
// this caller see, and MAY they read activities at all. The discover clause is
// only the first, so an owner with no activity grant would otherwise still have
// their correspondence read and its dates written into the card.
func TestQuietReviewWithoutActivityReadReadsNoCorrespondence(t *testing.T) {
	e := setupCloseDate(t)
	e.grantOwnerRole(t, e.Rep1, `{"objects":{"deal":{"read":true,"update":true},
		   "person":{"read":true},"organization":{"read":true}},"row_scope":"all"}`)
	id := e.seedSweepDeal(t, "No activity grant", e.late, stringp("commit"), intp(30), 90)
	e.seedDealEmail(t, id, "inbound", "Anna Weber", 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	basis := e.stagedCorrection(t, id).Basis
	if strings.Contains(basis, "Anna Weber") || strings.Contains(basis, "5 August") {
		t.Errorf("basis = %q — an owner without activity:read gets no correspondence facts", basis)
	}
}

// The card asks a human to confirm a date, so it must PROPOSE one. It used to
// carry the deal's current date in both fields, which asked for agreement to a
// change that was not one.
func TestQuietReviewProposesADateThatDiffersFromTheCurrentOne(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Gone quiet", e.late, stringp("commit"), intp(30), 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	correction := e.stagedCorrection(t, id)
	if correction.PreviousCloseDate == nil {
		t.Fatal("the card must say what date the deal carries now")
	}
	if correction.ExpectedCloseDate == *correction.PreviousCloseDate {
		t.Errorf("proposed %s and current %s are the same date — the card asks for nothing",
			correction.ExpectedCloseDate, *correction.PreviousCloseDate)
	}
	proposed, err := time.Parse(time.DateOnly, correction.ExpectedCloseDate)
	if err != nil {
		t.Fatalf("proposed date does not parse: %v", err)
	}
	if proposed.Before(today()) {
		t.Errorf("proposed %s is in the past — the invariant forbids it", correction.ExpectedCloseDate)
	}
}

// Proposing a date on the card is not the same as writing one to the deal. The
// zombie guard still holds: only a human confirming lifts it.
func TestQuietReviewLeavesTheDealsOwnDateAlone(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Gone quiet", e.late, stringp("commit"), intp(30), 90)
	originalDate := today().AddDate(0, 0, 30)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.expectedClose == nil || !swept.expectedClose.Equal(originalDate) {
		t.Errorf("date = %v, want the original %s — proposing is not writing",
			swept.expectedClose, originalDate.Format(time.DateOnly))
	}
}
