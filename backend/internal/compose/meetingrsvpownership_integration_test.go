// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who may settle a colleague's own calendar rows. A tombstone naming only their
// mailbox's event id may. So may the organizer's roster once it drops them, and
// any current roster once their calendar is disconnected.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const sharedMeetingUID = "shared-review@calendar.test"

// mailboxCopy is one mailbox's copy of a meeting, as its provider lists it.
type mailboxCopy struct {
	provider, id, uid, owner, organizer string
	// guests are the listed attendees besides the organizer, with their answers.
	guests map[string]string
}

func (c mailboxCopy) record(t *testing.T) connector.NormalizedRecord {
	t.Helper()
	raw, err := json.Marshal(c.event())
	if err != nil {
		t.Fatal(err)
	}
	var rec connector.NormalizedRecord
	if c.provider == sourceGCal {
		rec, err = gcal.RecordOf(raw, c.owner)
	} else {
		rec, err = graphcal.RecordOf(raw, c.owner)
	}
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func (c mailboxCopy) event() map[string]any {
	if c.provider == sourceGCal {
		attendees := []map[string]any{{"email": c.organizer, "responseStatus": "accepted", "self": c.owner == c.organizer}}
		for address, answer := range c.guests {
			attendees = append(attendees, map[string]any{"email": address, "responseStatus": answer, "self": c.owner == address})
		}
		return map[string]any{
			"id": c.id, "iCalUID": c.uid, "status": "confirmed", "summary": "Customer review",
			"start": map[string]string{"dateTime": "2030-01-07T10:00:00Z"}, "organizer": map[string]string{"email": c.organizer},
			"attendees": attendees,
		}
	}
	graphAnswers := map[string]string{"accepted": "accepted", "declined": "declined", "needsAction": "notResponded"}
	own := "organizer"
	attendees := []map[string]any{}
	for address, answer := range c.guests {
		if address == c.owner {
			own = graphAnswers[answer]
		}
		attendees = append(attendees, map[string]any{
			"type": "required", "emailAddress": map[string]string{"address": address},
			"status": map[string]string{"response": graphAnswers[answer]},
		})
	}
	return map[string]any{
		"id": c.id, "iCalUId": c.uid, "subject": "Customer review",
		"start":          map[string]string{"dateTime": "2030-01-07T10:00:00", "timeZone": "UTC"},
		"organizer":      map[string]any{"emailAddress": map[string]string{"address": c.organizer}},
		"responseStatus": map[string]string{"response": own},
		"attendees":      attendees,
	}
}

// removeByStub runs a provider's deletion stub, which carries the event id and
// no iCal UID, through the decoder and into the sink's removal.
func removeByStub(ctx context.Context, t *testing.T, sink *capture.Sink, provider, eventID string) {
	t.Helper()
	stub := map[string]any{"id": eventID, "status": "cancelled"}
	if provider == sourceGraphCal {
		stub = map[string]any{"id": eventID, "@removed": map[string]string{"reason": "deleted"}}
	}
	raw, err := json.Marshal(stub)
	if err != nil {
		t.Fatal(err)
	}
	settle, read := gcal.SettlementOf, gcal.SeriesIdentityOf
	if provider == sourceGraphCal {
		settle, read = graphcal.SettlementOf, graphcal.SeriesIdentityOf
	}
	if settlement, err := settle(raw, ""); err != nil || settlement != meetingmap.SettleRemove {
		t.Fatalf("the stub settled as %v (%v), want a removal", settlement, err)
	}
	identity, _, err := read(raw)
	if err != nil || identity.Stated() {
		t.Fatalf("the stub states identity %+v (%v), want none", identity, err)
	}
	if err := sink.RemoveCalendarMeeting(ctx, connector.NaturalKey{SourceSystem: provider, SourceID: eventID}, identity, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

type ownershipEnv struct {
	e        *integration.Env
	provider string
	sink     *capture.Sink
	address  map[ids.UUID]string
}

func setupOwnership(t *testing.T, provider string) ownershipEnv {
	t.Helper()
	e := integration.Setup(t)
	o := ownershipEnv{e: e, provider: provider, sink: newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil), address: map[ids.UUID]string{}}
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		o.address[seat] = seatAddress(t, e, seat)
		e.GrantRole(t, seat, "admin")
		e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES($1,$2,'connected',$3)`, provider, seat, o.address[seat])
	}
	return o
}

func (o ownershipEnv) sync(t *testing.T, seat ids.UUID, c mailboxCopy) ids.UUID {
	t.Helper()
	c.provider, c.owner = o.provider, o.address[seat]
	if c.uid == "" {
		c.uid = sharedMeetingUID
	}
	ref, err := o.sink.Upsert(connectorOwnerCtx(o.e, seat, o.provider), c.record(t))
	if err != nil {
		t.Fatal(err)
	}
	return ref.ID
}

func (o ownershipEnv) remove(t *testing.T, seat ids.UUID, eventID string) {
	t.Helper()
	removeByStub(connectorOwnerCtx(o.e, seat, o.provider), t, o.sink, o.provider, eventID)
}

func (o ownershipEnv) ownRows(t *testing.T, meeting, seat ids.UUID) int {
	t.Helper()
	return o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND user_id=$2`, meeting, seat)
}

func (o ownershipEnv) answer(t *testing.T, meeting, seat ids.UUID) string {
	t.Helper()
	for _, response := range []string{connector.ResponseAccepted, connector.ResponseDeclined, connector.ResponseNeedsAction} {
		if o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND user_id=$2 AND address=$3 AND response=$4`,
			meeting, seat, o.address[seat], response) > 0 {
			return response
		}
	}
	return ""
}

func TestATombstoneNamingOnlyAMailboxEventIDRetiresOnlyThatSeatsCopy(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			o := setupOwnership(t, provider)
			e := o.e
			organizer, guest := o.address[e.Rep1], o.address[e.Rep2]
			guests := map[string]string{guest: "accepted", "buyer@customer.test": "accepted"}
			meeting := o.sync(t, e.Rep1, mailboxCopy{id: "evt-organizer", organizer: organizer, guests: guests})
			if got := o.sync(t, e.Rep2, mailboxCopy{id: "evt-guest", organizer: organizer, guests: guests}); got != meeting {
				t.Fatal("the guest's calendar filed a second meeting")
			}
			if n := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE source_system=$1 AND source_id='evt-guest'`, provider); n != 1 {
				t.Fatalf("the guest's own original was stored %d times", n)
			}
			unrelated := o.sync(t, e.Rep3, mailboxCopy{
				id: "evt-unrelated", uid: "unrelated@calendar.test",
				organizer: o.address[e.Rep3], guests: map[string]string{"buyer@customer.test": "accepted"},
			})

			o.remove(t, e.Rep3, "evt-guest")
			o.remove(t, e.Rep2, "evt-never-stored")
			if o.ownRows(t, meeting, e.Rep2) == 0 {
				t.Fatal("a tombstone the guest's records do not prove retired the guest")
			}
			for _, id := range []ids.UUID{meeting, unrelated} {
				if status, set := readMeetingStatus(t, e, id); set {
					t.Fatalf("an unproved tombstone set meeting status %s", status)
				}
			}

			o.remove(t, e.Rep2, "evt-guest")
			if n := o.ownRows(t, meeting, e.Rep2); n != 0 {
				t.Fatalf("the guest's tombstone kept %d of their rows", n)
			}
			if status, set := readMeetingStatus(t, e, meeting); set {
				t.Fatalf("a guest's tombstone set the shared status %s", status)
			}
			if o.ownRows(t, meeting, e.Rep1) == 0 {
				t.Fatal("a guest's tombstone retired the organizer")
			}
		})
	}
}

func TestATombstoneFromTheOrganizersOwnMailboxCancelsTheMeeting(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			o := setupOwnership(t, provider)
			e := o.e
			organizer := o.address[e.Rep2]
			guests := map[string]string{o.address[e.Rep1]: "accepted", "buyer@customer.test": "accepted"}
			meeting := o.sync(t, e.Rep1, mailboxCopy{id: "evt-guest", organizer: organizer, guests: guests})
			if got := o.sync(t, e.Rep2, mailboxCopy{id: "evt-organizer", organizer: organizer, guests: guests}); got != meeting {
				t.Fatal("the organizer's calendar filed a second meeting")
			}
			o.remove(t, e.Rep2, "evt-organizer")
			if status, _ := readMeetingStatus(t, e, meeting); status != "canceled" {
				t.Fatalf("the organizer's tombstone left the status %q", status)
			}
		})
	}
}

func TestTheOrganizersCompleteRosterRetiresAColleagueItRemoved(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			o := setupOwnership(t, provider)
			e := o.e
			organizer, removed, colleague := o.address[e.Rep1], o.address[e.Rep2], o.address[e.Rep3]
			everyone := map[string]string{removed: "accepted", colleague: "accepted", "buyer@customer.test": "accepted"}
			without := map[string]string{colleague: "accepted", "buyer@customer.test": "accepted"}
			meeting := o.sync(t, e.Rep1, mailboxCopy{id: "evt-1", organizer: organizer, guests: everyone})
			o.sync(t, e.Rep2, mailboxCopy{id: "evt-2", organizer: organizer, guests: everyone})
			o.sync(t, e.Rep3, mailboxCopy{id: "evt-3", organizer: organizer, guests: everyone})

			o.sync(t, e.Rep3, mailboxCopy{id: "evt-3", organizer: organizer, guests: without})
			if o.ownRows(t, meeting, e.Rep2) == 0 {
				t.Fatal("an attendee's roster retired a colleague's own rows")
			}
			o.sync(t, e.Rep1, mailboxCopy{id: "evt-1", organizer: organizer, guests: without})
			if n := o.ownRows(t, meeting, e.Rep2); n != 0 {
				t.Fatalf("the organizer's roster kept %d rows of the colleague it removed", n)
			}
			if o.ownRows(t, meeting, e.Rep3) == 0 || e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND address='buyer@customer.test'`, meeting) == 0 {
				t.Fatal("the organizer's roster retired a guest it still lists")
			}
			if status, set := readMeetingStatus(t, e, meeting); set {
				t.Fatalf("removing a guest set the shared status %s", status)
			}
		})
	}
}

func TestADisconnectedCalendarsAnswerFollowsTheOrganizersRosterUntilItReconnects(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			o := setupOwnership(t, provider)
			e := o.e
			organizer, guest := o.address[e.Rep1], o.address[e.Rep2]
			listed := func(answer string) mailboxCopy {
				return mailboxCopy{id: "evt-1", organizer: organizer, guests: map[string]string{guest: answer, "buyer@customer.test": "accepted"}}
			}
			own := func(answer string) mailboxCopy {
				c := listed(answer)
				c.id = "evt-2"
				return c
			}
			meeting := o.sync(t, e.Rep1, listed("needsAction"))
			o.sync(t, e.Rep2, own("declined"))
			o.sync(t, e.Rep1, listed("accepted"))
			if got := o.answer(t, meeting, e.Rep2); got != "declined" {
				t.Fatalf("a connected guest's own answer became %q", got)
			}
			rows := o.ownRows(t, meeting, e.Rep2)

			registry := NewCaptureRegistry(e.Pool, keyvault.NewMemory(), CaptureConfig{})
			if err := registry.Disconnect(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), provider); err != nil {
				t.Fatal(err)
			}
			o.sync(t, e.Rep1, listed("accepted"))
			if got := o.answer(t, meeting, e.Rep2); got != "accepted" {
				t.Fatalf("a disconnected guest's answer stayed %q against the organizer's roster", got)
			}
			if n := o.ownRows(t, meeting, e.Rep2); n != rows {
				t.Fatalf("the organizer's roster changed the guest's rows from %d to %d", rows, n)
			}
			if status, set := readMeetingStatus(t, e, meeting); set {
				t.Fatalf("taking over an answer set the shared status %s", status)
			}

			reconnect(t, e, registry, e.Rep2, provider, guest)
			o.sync(t, e.Rep2, own("declined"))
			o.sync(t, e.Rep1, listed("accepted"))
			if got := o.answer(t, meeting, e.Rep2); got != "declined" {
				t.Fatalf("a reconnected guest's own answer became %q", got)
			}
		})
	}
}

// reconnect grants the seat's calendar again, as its consent callback does.
func reconnect(t *testing.T, e *integration.Env, registry *capture.Registry, seat ids.UUID, provider, address string) {
	t.Helper()
	auth, err := json.Marshal(map[string]any{"refresh_token": "refresh", "owner_email": address, "access_token": "access"})
	if err != nil {
		t.Fatal(err)
	}
	if provider == sourceGraphCal {
		registry.Register(graphcal.New(nil, graphcal.NewAPI(nil, "")))
	} else {
		registry.Register(gcal.New(nil, gcal.NewAPI(nil, "")))
	}
	human := e.As(seat, []ids.UUID{e.Team1}, integration.AdminPerms)
	actor, ok := principal.Actor(human)
	if !ok {
		t.Fatal("actor not bound")
	}
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead)
	if _, err := registry.Connect(principal.WithActor(human, actor), provider, connector.Auth(auth)); err != nil {
		t.Fatal(err)
	}
}
