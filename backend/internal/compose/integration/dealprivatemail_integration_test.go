// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A mail only its participants may read still shows its deal was worked. Filed
// on the deal, it moves the deal's clock by its date alone. The last-email chip
// and the stakeholder's engagement ignore it, and no contact or company date
// moves. A private mail filed only on a contact counts for no deal.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealprivatemail"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type privateMailWorld struct {
	e       *Env
	now     time.Time
	company ids.UUID
	deal    ids.DealID
	contact ids.UUID
}

func newPrivateMailWorld(t *testing.T) privateMailWorld {
	t.Helper()
	e := Setup(t)
	w := privateMailWorld{e: e, now: clocktest.Now(t)}
	w.company = e.SeedCompany(t, "Kessler Systems", nil)
	w.deal = seedRepDeal(t, e, ids.From[ids.CompanyKind](w.company), e.Rep1, "Fleet retrofit")
	w.contact = e.SeedContact(t, "Dana Buyer", nil)
	contact := ids.From[ids.ContactKind](w.contact)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &w.deal, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the stakeholder: %v", err)
	}
	return w
}

// privateMail logs one email to the contact, optionally on the deal, and limits
// it to its participants.
func (w privateMailWorld) privateMail(t *testing.T, direction string, daysAgo int, onDeal bool) {
	t.Helper()
	when := w.now.AddDate(0, 0, -daysAgo)
	subject := "private subject"
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact}}
	if onDeal {
		links = append(links, activities.ActivityLinkInput{EntityType: "deal", EntityID: w.deal.UUID})
	}
	logged, _, err := w.e.Activities.LogActivity(w.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, OccurredAt: &when, Source: "manual", Direction: &direction, Links: links,
	})
	if err != nil {
		t.Fatalf("logging a %s mail: %v", direction, err)
	}
	if _, err := w.e.Activities.SetAudience(w.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail to its participants: %v", err)
	}
}

func (w privateMailWorld) engaged(t *testing.T) []ids.UUID {
	t.Helper()
	ctx := w.e.Admin()
	var out []ids.UUID
	err := database.WithWorkspaceTx(ctx, w.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = deals.EngagedStakeholders(ctx, tx, w.deal, w.now)
		return err
	})
	if err != nil {
		t.Fatalf("reading the engaged stakeholders: %v", err)
	}
	return out
}

func TestAPrivateMailOnTheDealMovesOnlyItsClock(t *testing.T) {
	t.Parallel()
	w := newPrivateMailWorld(t)
	w.privateMail(t, "inbound", 1, true)
	w.privateMail(t, "outbound", 3, true)

	deal, err := w.e.Deals.GetDeal(w.e.Admin(), w.deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	want := w.now.AddDate(0, 0, -1)
	if deal.LastActivityAt == nil || !deal.LastActivityAt.Equal(want) {
		t.Errorf("last_activity_at = %v, want the private mail's date %v", deal.LastActivityAt, want)
	}
	if deal.LastEmail != nil {
		t.Errorf("last_email = %+v, want none: it would show a private mail's direction", deal.LastEmail)
	}
	if got := w.engaged(t); len(got) != 0 {
		t.Errorf("engaged stakeholders = %v, want none: engagement would reveal private mail", got)
	}
	if n := w.e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND last_activity_at IS NOT NULL`, w.contact); n != 0 {
		t.Error("a private mail moved the contact's last_activity_at")
	}
	if n := w.e.WsCount(t, `SELECT count(*) FROM company WHERE id = $1 AND last_activity_at IS NOT NULL`, w.company); n != 0 {
		t.Error("a private mail moved the company's last_activity_at")
	}
}

func TestAPrivateMailOnlyOnTheContactCountsForNoDeal(t *testing.T) {
	t.Parallel()
	w := newPrivateMailWorld(t)
	w.privateMail(t, "inbound", 1, false)
	w.privateMail(t, "outbound", 2, false)

	deal, err := w.e.Deals.GetDeal(w.e.Admin(), w.deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if deal.LastActivityAt != nil || deal.LastEmail != nil {
		t.Errorf("last_activity_at = %v, last_email = %+v, want neither for mail the deal does not hold",
			deal.LastActivityAt, deal.LastEmail)
	}
	if got := w.engaged(t); len(got) != 0 {
		t.Errorf("engaged stakeholders = %v, want none from mail filed only on the contact", got)
	}
}

func TestAnOlderDealClockIsRefoldedByTheDataTaskQuery(t *testing.T) {
	t.Parallel()
	w := newPrivateMailWorld(t)
	w.privateMail(t, "inbound", 4, true)
	w.e.WsExec(t, `UPDATE deal SET last_activity_at = NULL WHERE id = $1`, w.deal.UUID)
	w.e.WsExec(t, `SELECT move_last_activity('deal'::regclass, $1)`, w.deal.UUID)
	if n := w.e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND last_activity_at IS NOT NULL`, w.deal.UUID); n != 1 {
		t.Error("refolding the deal clock ignored the private mail filed on it")
	}
}

// The deal's timeline already lists a withheld private mail with its time to a
// reader outside its audience, and blanks its subject. The deal clock therefore
// says nothing about the mail that the timeline does not.
func TestTheTimelineAlreadyShowsAPrivateMailsTimeToANonParticipant(t *testing.T) {
	t.Parallel()
	w := newPrivateMailWorld(t)
	w.privateMail(t, "inbound", 2, true)

	entityType, entityID := "deal", ids.UUID(w.deal.UUID)
	rows, _, err := w.e.Activities.ListActivities(w.e.As(w.e.Rep3, []ids.UUID{w.e.Team2}, AccountRepPerms), activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &entityID,
	})
	if err != nil {
		t.Fatalf("listing the deal's timeline: %v", err)
	}
	want := w.now.AddDate(0, 0, -2)
	if len(rows) != 1 || !rows[0].OccurredAt.Equal(want) {
		t.Fatalf("the timeline lists %d rows, want the withheld mail at %v", len(rows), want)
	}
	if rows[0].Subject != nil && *rows[0].Subject != "" {
		t.Errorf("the withheld row carries the subject %q", *rows[0].Subject)
	}
}

// The verdict reads the newest private mails. A change to any of them changes
// the checksum that makes the deal due again, not only a change to the newest.
func TestTheVerdictChecksumMovesWhenAnOlderPrivateMailChanges(t *testing.T) {
	t.Parallel()
	w := newPrivateMailWorld(t)
	w.privateMail(t, "inbound", 5, true)
	w.privateMail(t, "outbound", 1, true)
	sum := func() string {
		return w.e.WsScalar(t, `SELECT `+dealprivatemail.ChecksumSQL(20)+` FROM deal d WHERE d.id = $1`, w.deal.UUID)
	}
	before := sum()
	if before == "" {
		t.Fatal("two private mails give the deal no checksum part")
	}
	w.e.WsExec(t, `UPDATE activity SET occurred_at = occurred_at - interval '1 day' WHERE id = (
		SELECT a.id FROM activity a JOIN activity_link l ON l.activity_id = a.id
		 WHERE l.deal_id = $1 ORDER BY a.occurred_at ASC LIMIT 1)`, w.deal.UUID)
	if after := sum(); after == before {
		t.Error("changing the date of a mail that is not the newest left the checksum as it was")
	}
}
