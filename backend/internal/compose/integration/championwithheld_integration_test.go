// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A champion the reader may not see is still a champion.
//
// The worklist's `no_champion` reason says nobody inside the account is arguing
// for a drifting deal. It is only safe to say when the committee could be read
// in full: a seat refused by the person row scope is absent from every read, so
// a lane that treated "I saw no champion" as "there is no champion" would tell
// a rep to go and recruit one into a committee that already has one.
//
// ChampionCoverFor answers that with a tri-state, and only a seeded, row-scoped
// reader can prove it. The unit tests hand-build `deals.ChampionCover` values
// and hold the FOLD; the SQL that SETS `Withheld` is invisible to them, so a
// version whose protective arm can never fire passes every one of them.
//
// The fixtures hide a seat through TWO different endpoints on purpose. An edge
// is admitted by a conjunction over all six, so a probe reading a single arm
// answers a narrower question that looks identical until the seat is refused by
// one of the others. Varying only the person arm cannot tell the two apart.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// championSeatReader can read deals and people, and is bounded to its OWN rows.
//
// The bound is the whole fixture: a rep who may see every row cannot be refused
// a seat, so an unbounded reader proves nothing about the withheld arm. The
// edge grant is present, because a caller refused edges outright is a different
// case with its own test in the deals module.
func championSeatReader() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"deal":         {Read: true},
			"person":       {Read: true},
			"organization": {Read: true},
			"relationship": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	}
}

// A deal whose champion belongs to somebody else reports its committee as
// WITHHELD, and therefore makes no claim about who is carrying it.
//
// This is the case the whole tri-state exists for: the reader sees the deal,
// cannot see the seat, and must not be told nobody is carrying it.
func TestAChampionTheReaderMayNotSeeIsReportedWithheldRatherThanAbsent(t *testing.T) {
	e := Setup(t)
	rep := e.Rep1
	org := e.SeedOrg(t, "Kessler Systems", &rep)

	// CAPTURE-PRIVATE, not merely owned by somebody else. `person` is an
	// identity table (auth/tableclass.go), so customer identity is
	// workspace-readable and the owner arm of the row predicate is TRUE for
	// every reader — ownership alone hides nobody. `visibility = 'owner'` is
	// the arm that actually refuses, and it is the state an unpromoted
	// captured contact sits in.
	hidden := e.SeedPerson(t, "Somebody else's contact", &e.AdminUser)
	hiddenID := ids.From[ids.PersonKind](hidden)
	makeCapturePrivate(t, e, hidden, e.AdminUser)

	orgID := ids.From[ids.OrganizationKind](org)
	dealID := seedRepDeal(t, e, orgID, rep, "Fleet retrofit")

	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &hiddenID, DealID: &dealID,
		Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion: %v", err)
	}

	cover := championCoverFor(t, e, championSeatReader(), dealID.UUID)

	answer, found := cover[dealID.UUID]
	if !found {
		t.Fatal("the deal is absent from the answer, which reads as \"no committee\" — " +
			"it has one, and the reader simply may not see it")
	}
	if !answer.Withheld {
		t.Error("the committee is reported readable when a seat on it was refused; " +
			"the queue will tell this rep nobody is carrying a deal that has a champion")
	}
	if answer.Covered {
		t.Error("the answer claims a visible engaged champion, which this reader has none of")
	}
}

// The reader who CAN see the seat gets an answer, not a refusal.
//
// Without this arm the test above passes against a version that reports every
// committee as withheld — a gate refusing everybody looks exactly like a gate
// working, and the queue would silently stop saying `no_champion` at all.
func TestAVisibleCommitteeIsAnsweredRatherThanWithheld(t *testing.T) {
	e := Setup(t)
	rep := e.Rep1
	org := e.SeedOrg(t, "Turbinenbau", &rep)
	orgID := ids.From[ids.OrganizationKind](org)

	// Seeded by the same admin as the withheld case, and NOT made capture
	// private. Capture privacy is the single varied factor, deliberately:
	// `person` is an identity table, so the owner arm is true for every reader
	// and swapping the owner would vary something that changes no answer. This
	// arm has to differ in the thing that actually refuses, or it does not
	// control for anything.
	seen := e.SeedPerson(t, "A contact nobody made private", &e.AdminUser)
	seenID := ids.From[ids.PersonKind](seen)
	dealID := seedRepDeal(t, e, orgID, rep, "Quarterly renewal")

	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &seenID, DealID: &dealID,
		Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion: %v", err)
	}

	cover := championCoverFor(t, e, championSeatReader(), dealID.UUID)

	answer, found := cover[dealID.UUID]
	if !found {
		t.Fatal("the deal is absent from the answer, though its committee is fully readable")
	}
	if answer.Withheld {
		t.Error("a committee this reader can read in full is reported as withheld — " +
			"the queue then never says no_champion, however many deals lack one")
	}
	// Not engaged: the seat exists and has had no two-way exchange, so the
	// honest answer is uncovered rather than covered. That is what makes the
	// row able to say `no_champion` at all, and it is the arm the withheld
	// case above must not be confused with.
	if answer.Covered {
		t.Error("the answer claims an ENGAGED champion on a seat with no exchange behind it")
	}
}

// A committee this reader can see NO part of is withheld too, not absent.
//
// It reaches the answer by a different route from the partial case: with every
// seat refused, the seat query returns no row for the deal at all, so it is the
// withheld read alone that puts it in the map. Without it the deal falls
// through to "no committee" and the lane returns nil — the right answer for the
// wrong reason, which breaks the day somebody makes the grouped query emit a
// row for an empty group.
func TestACommitteeWhollyOutOfSightIsWithheldRatherThanMissing(t *testing.T) {
	e := Setup(t)
	rep := e.Rep1
	org := e.SeedOrg(t, "Nordwerk", &rep)
	orgID := ids.From[ids.OrganizationKind](org)
	dealID := seedRepDeal(t, e, orgID, rep, "Renewal nobody can see into")

	// Two seats, both capture-private to the admin: a champion and a plain
	// stakeholder, so the deal genuinely has a committee and this reader can
	// see no part of it.
	champion := "champion"
	for _, seat := range []struct {
		name string
		role *string
	}{{"Hidden champion", &champion}, {"Hidden stakeholder", nil}} {
		person := e.SeedPerson(t, seat.name, &e.AdminUser)
		makeCapturePrivate(t, e, person, e.AdminUser)
		personID := ids.From[ids.PersonKind](person)
		if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
			Kind: "deal_stakeholder", PersonID: &personID, DealID: &dealID,
			Role: seat.role, Source: "manual",
		}); err != nil {
			t.Fatalf("seating %s: %v", seat.name, err)
		}
	}

	cover := championCoverFor(t, e, championSeatReader(), dealID.UUID)

	answer, found := cover[dealID.UUID]
	if !found {
		t.Fatal("a deal whose whole committee is out of sight is absent from the answer, " +
			"which the caller reads as \"this deal has no committee\" — it has two seats")
	}
	if !answer.Withheld {
		t.Error("the committee is reported readable when no seat on it could be read")
	}
}

// A champion refused by an endpoint OTHER than the person one is withheld too.
//
// THE ARM THIS FIXTURE EXISTS FOR. An edge is admitted by a conjunction over
// every endpoint it carries, and the three tests above all hide the seat behind
// the same one — the person. This one hides it behind `counterparty_org_id`, so
// a probe reading only the person arm reports the committee fully readable and
// the deal says "no champion" over a champion that is sitting in it.
//
// It is reachable rather than theoretical. `rel_stakeholder_shape` pins
// organization_id, project_id and counterparty_person_id to NULL on a
// deal_stakeholder and says nothing about counterparty_org_id, and
// CreateRelationshipInput accepts it. `organization` is capture-private on the
// same terms `person` is, so an unpromoted company is a seat's hidden endpoint.
//
// The CHAMPION here is fully readable. That is the point: the person arm admits
// this seat, so only a probe reading the whole conjunction can find it refused.
func TestAChampionRefusedByANonPersonEndpointIsWithheldRatherThanAbsent(t *testing.T) {
	e := Setup(t)
	rep := e.Rep1
	org := e.SeedOrg(t, "Halden Werke", &rep)
	orgID := ids.From[ids.OrganizationKind](org)
	dealID := seedRepDeal(t, e, orgID, rep, "Line upgrade")

	// Readable by this rep: not capture-private, so the person arm of the
	// conjunction admits the seat and cannot be what refuses it.
	visible := e.SeedPerson(t, "A champion in plain sight", &e.AdminUser)
	visibleID := ids.From[ids.PersonKind](visible)

	// The endpoint that refuses. A company the admin captured and nobody
	// promoted, so `organization`'s capture-privacy arm hides it from this rep
	// exactly as it hides an unpromoted contact.
	partner := e.SeedOrg(t, "A partner nobody promoted", &e.AdminUser)
	makeOrgCapturePrivate(t, e, partner, e.AdminUser)
	partnerID := ids.From[ids.OrganizationKind](partner)

	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &visibleID, DealID: &dealID,
		CounterpartyOrgID: &partnerID, Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion behind a hidden counterparty org: %v", err)
	}

	cover := championCoverFor(t, e, championSeatReader(), dealID.UUID)

	answer, found := cover[dealID.UUID]
	if !found {
		t.Fatal("a deal whose only seat is refused by its counterparty org is absent " +
			"from the answer, which the caller reads as \"no committee\"")
	}
	if !answer.Withheld {
		t.Error("a seat refused by the counterparty_org_id arm is reported readable; " +
			"the probe is reading one arm of the conjunction rather than its complement, " +
			"and this rep will be told nobody is carrying a deal that has a champion")
	}
	if answer.Covered {
		t.Error("the answer claims a visible engaged champion on a seat this reader cannot read")
	}
}

// makeOrgCapturePrivate is makeCapturePrivate's twin for a company.
//
// `organization` sits beside `person` in ownerPrivateTables, so the state and
// the columns are the same and only the table differs. Two named helpers rather
// than one taking a table, because the call sites read as what they hide — a
// contact or a company — and that is the fact each fixture is varying.
func makeOrgCapturePrivate(t *testing.T, e *Env, org ids.UUID, owner ids.UUID) {
	t.Helper()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.Admin(),
			`UPDATE organization SET visibility = 'owner', owner_id = $2 WHERE id = $1`,
			org, owner)
		return execErr
	})
	if err != nil {
		t.Fatalf("making the company capture private: %v", err)
	}
}

// An edge left pointing at an ARCHIVED person claims no withheld seat.
//
// The two statements must agree about what a live seat is. championSeats joins
// `person ... archived_at IS NULL`, so an edge whose person is archived is no
// seat to it; if the withheld probe counted that same edge, the deal would be
// reported withheld forever and `no_champion` would silently stop firing — the
// always-off mirror of the bug this file exists for.
//
// The row is seeded by archiving the person WITHOUT the edge, which is not what
// the product does: people/personarchive.go cascades archived_at onto the edge
// in the same transaction. That cascade is a sibling module's invariant, and
// this test is what keeps the disagreement from mattering if a new archive path
// or a backfill ever misses it.
func TestAnEdgeOnAnArchivedPersonIsNoWithheldSeat(t *testing.T) {
	e := Setup(t)
	rep := e.Rep1
	org := e.SeedOrg(t, "Stillgelegt AG", &rep)
	orgID := ids.From[ids.OrganizationKind](org)
	dealID := seedRepDeal(t, e, orgID, rep, "Deal whose only seat is gone")

	// Capture-private, so the person arm WOULD refuse this seat and the deal
	// would read as withheld — were the person still live.
	gone := e.SeedPerson(t, "A contact who left", &e.AdminUser)
	goneID := ids.From[ids.PersonKind](gone)
	makeCapturePrivate(t, e, gone, e.AdminUser)

	champion := "champion"
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "deal_stakeholder", PersonID: &goneID, DealID: &dealID,
		Role: &champion, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the champion: %v", err)
	}
	archivePersonLeavingTheEdge(t, e, gone)

	// The fixture's central claim, asserted rather than described: the edge is
	// STILL LIVE and points at an archived person. Without this the test would
	// go vacuously green the day an archive path learns to cascade onto the
	// edge, and would then prove nothing about the liveness arm it exists for.
	if live := liveSeatCount(t, e, dealID); live != 1 {
		t.Fatalf("the fixture holds %d live seats on the deal, want 1 — "+
			"the edge was archived too, so nothing here reaches the liveness arm", live)
	}

	cover := championCoverFor(t, e, championSeatReader(), dealID.UUID)

	if answer, found := cover[dealID.UUID]; found && answer.Withheld {
		t.Error("an edge pointing at an archived person is counted as a withheld seat; " +
			"the deal reports a committee it no longer has and never says no_champion again")
	}
}

// liveSeatCount is how many un-archived stakeholder edges the deal carries,
// read as the admin so the count is the DATA's rather than a reader's.
func liveSeatCount(t *testing.T, e *Env, dealID ids.DealID) int {
	t.Helper()
	var live int
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(),
			`SELECT count(*) FROM relationship
			  WHERE kind = 'deal_stakeholder' AND deal_id = $1 AND archived_at IS NULL`,
			dealID).Scan(&live)
	})
	if err != nil {
		t.Fatalf("counting the live seats: %v", err)
	}
	return live
}

// archivePersonLeavingTheEdge archives the person and DELIBERATELY leaves the
// edge live, which the product's own archive path never does. It is the state a
// missed cascade would leave behind, and the only way to reach the liveness arm.
func archivePersonLeavingTheEdge(t *testing.T, e *Env, person ids.UUID) {
	t.Helper()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.Admin(),
			`UPDATE person SET archived_at = now() WHERE id = $1`, person)
		return execErr
	})
	if err != nil {
		t.Fatalf("archiving the person: %v", err)
	}
}

func seedRepDeal(
	t *testing.T, e *Env, orgID ids.OrganizationID, owner ids.UUID, name string,
) ids.DealID {
	t.Helper()
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	holder := ids.From[ids.UserKind](owner)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, OwnerID: &holder, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	return ids.From[ids.DealKind](ids.UUID(deal.Id))
}

func championCoverFor(
	t *testing.T, e *Env, perms principal.Permissions, dealID ids.UUID,
) map[ids.UUID]deals.ChampionCover {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	var out map[ids.UUID]deals.ChampionCover
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		cover, err := deals.ChampionCoverFor(ctx, tx, []ids.UUID{dealID},
			time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
		out = cover
		return err
	})
	if err != nil {
		t.Fatalf("reading the champion cover: %v", err)
	}
	return out
}
