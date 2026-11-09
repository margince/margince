// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Their mail and our reply logged by hand at one stored instant. A day on either side makes ours the
// answer, and two instants stay owed. Every surface reads the tie alike.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// handLogged posts one mail through POST /activities, filed under the contact on a thread both mails
// share. The log form sends no thread, and no answer arm can then match the pair.
func (o *owedEnv) handLogged(t *testing.T, contact ids.UUID, direction crmcontracts.CreateActivityRequestDirection,
	thread string, at time.Time, precision crmcontracts.OccurredPrecision,
) ids.UUID {
	t.Helper()
	subject := "Samples for the trial"
	links := []struct {
		EntityId   openapi_types.UUID                                `json:"entity_id"` //nolint:staticcheck // mirrors the generated inline struct, whose field is spelled EntityId
		EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: openapi_types.UUID(contact), EntityType: crmcontracts.CreateActivityRequestLinksEntityTypeContact}}
	status, row := postActivity(o.reader(), t, o.e, crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail, Source: "manual", Direction: &direction,
		Subject: &subject, OccurredAt: &at, OccurredPrecision: &precision, ThreadKey: &thread, Links: &links,
	})
	if status != http.StatusCreated {
		t.Fatalf("logging the %s mail answered %d", direction, status)
	}
	return ids.UUID(row.Id)
}

// contactWriting is the one contact accountOf filed at the company.
func (o *owedEnv) contactWriting(t *testing.T, address string) ids.UUID {
	t.Helper()
	var contact ids.UUID
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT contact_id FROM contact_email WHERE email = $1 AND archived_at IS NULL`, address).Scan(&contact)
	}); err != nil {
		t.Fatalf("finding the contact writing from %s: %v", address, err)
	}
	return contact
}

// handTie is one hand-logged tie: what it is logged as, in which order, and
// what every surface must then read.
type handTie struct {
	name      string
	precision crmcontracts.OccurredPrecision
	replyLast bool
	owed      bool
}

func handTies() []handTie {
	day, instant := crmcontracts.OccurredPrecisionOccurredPrecisionDay, crmcontracts.OccurredPrecisionOccurredPrecisionInstant
	return []handTie{
		{"by their date, the reply logged last", day, true, false},
		{"by their date, the reply logged first", day, false, false},
		{"at one stored instant, the reply logged last", instant, true, true},
		{"at one stored instant, the reply logged first", instant, false, true},
	}
}

// whoseMove is the move every surface must read off the tie.
func (c handTie) whoseMove() crmcontracts.LastExchangeWhoseMove {
	if c.owed {
		return crmcontracts.LastExchangeWhoseMoveUs
	}
	return crmcontracts.LastExchangeWhoseMoveThem
}

// logTie files the tie at the stored instant of at under a new account, and
// returns the account, its contact and their mail.
func (o *owedEnv) logTie(t *testing.T, c handTie, at time.Time) (ids.CompanyID, ids.UUID, ids.UUID) {
	t.Helper()
	address := "pat-" + ids.NewV7().String() + "@tie.example"
	company := o.accountOf(t, "Tie "+c.name, address)
	contact := o.contactWriting(t, address)
	at = at.Truncate(time.Second)
	if c.precision == crmcontracts.OccurredPrecisionOccurredPrecisionDay {
		at = occurredday.Anchor(at)
	}
	thread := "tie-" + ids.NewV7().String()
	reply := func() {
		o.handLogged(t, contact, crmcontracts.CreateActivityRequestDirectionOutbound, thread, at, c.precision)
	}
	if !c.replyLast {
		reply()
	}
	asked := o.handLogged(t, contact, crmcontracts.CreateActivityRequestDirectionInbound, thread, at, c.precision)
	if c.replyLast {
		reply()
	}
	return company, contact, asked
}

// movedBy is whose move the record's last exchange leaves, empty when it names none.
func movedBy(latest crmcontracts.LatestInboundList) crmcontracts.LastExchangeWhoseMove {
	if latest.LastExchange == nil {
		return ""
	}
	return latest.LastExchange.WhoseMove
}

func TestAHandLoggedTieIsAnsweredOnlyWhenADayIsLogged(t *testing.T) {
	for _, c := range handTies() {
		t.Run(c.name, func(t *testing.T) {
			o := setupOwed(t)
			company, contact, asked := o.logTie(t, c, o.now.Add(-26*time.Hour))
			if o.badged(t, asked) != c.owed {
				t.Errorf("the History badge reads needs-reply = %v, want %v", !c.owed, c.owed)
			}
			latest, err := activities.NewStore(o.e.DB()).LatestInbound(o.reader(), "contact", contact)
			if err != nil {
				t.Fatalf("reading the Next bar's list: %v", err)
			}
			listed := false
			for _, item := range latest.Data {
				listed = listed || ids.UUID(item.Activity.Id) == asked
			}
			if listed != c.owed {
				t.Errorf("the Next bar lists their mail = %v, want %v", listed, c.owed)
			}
			if _, _, engagement := o.whoseMove(t, company); (engagement == crmcontracts.ContactEngagementWaiting) != c.owed {
				t.Errorf("the Contacts tab reads %q, want waiting = %v", engagement, c.owed)
			}
			if got := movedBy(latest); got != c.whoseMove() {
				t.Errorf("the last exchange leaves the move %q, want %q", got, c.whoseMove())
			}
		})
	}
}

// Past the no-reply wait the account's strip says whose move the tie leaves,
// by the same rule as the contact's last exchange.
func TestAHandLoggedTieLeavesTheStripTheSameMove(t *testing.T) {
	for _, c := range handTies() {
		t.Run(c.name, func(t *testing.T) {
			o := setupOwed(t)
			company, contact, _ := o.logTie(t, c, o.now.Add(-14*24*time.Hour))
			want := map[bool]string{true: "waiting_on_us", false: "waiting_on_them"}[c.owed]
			if state, _, _ := o.whoseMove(t, company); state != want {
				t.Errorf("the strip reads %q, want %q", state, want)
			}
			latest, err := activities.NewStore(o.e.DB()).LatestInbound(o.reader(), "contact", contact)
			if err != nil {
				t.Fatalf("reading the last exchange: %v", err)
			}
			if got := movedBy(latest); got != c.whoseMove() {
				t.Errorf("the last exchange leaves the move %q, want %q", got, c.whoseMove())
			}
		})
	}
}

// tieRow is one row of a tie, in the order it is logged.
type tieRow struct {
	direction crmcontracts.CreateActivityRequestDirection
	precision crmcontracts.OccurredPrecision
}

// At one stored instant our day entry answers their instant, and our instant does not. So a page reads our side
// of a tie as its day entry and theirs as its instant, whichever was logged last.
func TestATieOfThreeReadsAsTheRowsThatDecideIt(t *testing.T) {
	day, instant := crmcontracts.OccurredPrecisionOccurredPrecisionDay, crmcontracts.OccurredPrecisionOccurredPrecisionInstant
	in, out := crmcontracts.CreateActivityRequestDirectionInbound, crmcontracts.CreateActivityRequestDirectionOutbound
	for _, c := range []struct {
		name              string
		rows              []tieRow
		inbound, outbound crmcontracts.OccurredPrecision
		answered          bool
	}{
		{"our day entry beside our instant", []tieRow{{in, instant}, {out, day}, {out, instant}}, instant, day, true},
		{"their instant beside their day entry", []tieRow{{in, instant}, {out, instant}, {in, day}}, instant, instant, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := setupOwed(t)
			address := "pat-" + ids.NewV7().String() + "@tie.example"
			company := o.accountOf(t, "Tie of three", address)
			contact := o.contactWriting(t, address)
			at, thread := occurredday.Anchor(o.now.Add(-26*time.Hour)), "tie-"+ids.NewV7().String()
			for _, row := range c.rows {
				o.handLogged(t, contact, row.direction, thread, at, row.precision)
			}
			view := contactView(t, o.e, contact, func() time.Time { return o.now })
			if got := view.LastInboundPrecision; got == nil || *got != c.inbound {
				t.Errorf("the contact page reads their side as %s, want %s", stated(got), c.inbound)
			}
			if got := view.LastOutboundPrecision; got == nil || *got != c.outbound {
				t.Errorf("the contact page reads our side as %s, want %s", stated(got), c.outbound)
			}
			awaiting := view.Moment != nil && view.Moment.Rule == crmcontracts.ContactMomentRuleAwaitingReply
			if awaiting != c.answered {
				t.Errorf("the contact page waits on their reply = %v, want %v", awaiting, c.answered)
			}
			if !c.answered {
				return
			}
			if _, _, engagement := o.whoseMove(t, company); engagement != crmcontracts.ContactEngagementAnswered {
				t.Errorf("the Contacts tab reads %q, want answered", engagement)
			}
		})
	}
}

// stated is a precision the page may leave out, "none" when it does.
func stated(precision *crmcontracts.OccurredPrecision) string {
	if precision == nil {
		return "none"
	}
	return string(*precision)
}

// A deal's quiet sentence reads each side of a tie as the row that decides it: ours as its day entry, theirs as
// its instant.
func TestADealsQuietSidesReadATieAsTheRowsThatDecideIt(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	dealUUID := e.SeedDeal(t, "Tie quiet deal", pipeline, open, nil)
	at := occurredday.Anchor(clocktest.Now(t).AddDate(0, 0, -6))
	for _, row := range []struct{ direction, precision string }{
		{"inbound", activities.PrecisionInstant},
		{"outbound", activities.PrecisionDay},
		{"inbound", activities.PrecisionDay},
		{"outbound", activities.PrecisionInstant},
	} {
		subject := "Terms"
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "email", Subject: &subject, Direction: &row.direction, OccurredAt: &at,
			OccurredPrecision: row.precision, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealUUID}},
		}); err != nil {
			t.Fatalf("logging the %s %s mail: %v", row.precision, row.direction, err)
		}
	}
	var facts deals.QuietFacts
	inWorkspaceTx(t, e, func(tx pgx.Tx) error {
		var err error
		facts, err = deals.ReadQuietFacts(e.Admin(), tx, ids.From[ids.DealKind](dealUUID))
		return err
	})
	if facts.LastInbound == nil || facts.LastOutbound == nil {
		t.Fatalf("the deal reads no side of the tie: %+v", facts)
	}
	if facts.LastInbound.DateOnly || !facts.LastOutbound.DateOnly {
		t.Errorf("the deal reads their side dated = %v and ours = %v, want their instant and our day entry",
			facts.LastInbound.DateOnly, facts.LastOutbound.DateOnly)
	}
}
