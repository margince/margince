// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A merge carries the introduction asks that name the retired contact, on both
// columns that can name one, and closes an ask the merge made a duplicate the
// way a requester closes one.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asker is the admin seat with the introduction grant the admin fixture does
// not carry.
func (c *carryEnv) asker() context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["introduction"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return c.e.As(c.e.AdminUser, nil, perms)
}

// ask records one open ask through the real writer, of colleague, about
// contact, optionally through another contact.
func (c *carryEnv) ask(t *testing.T, contact ids.ContactID, colleague ids.UUID, through *ids.ContactID) ids.UUID {
	t.Helper()
	req := introductionsRequest(contact, colleague)
	if through != nil {
		via := through.UUID
		req.RouteType, req.ThroughContactID = "through_contact", &via
	}
	id, err := c.intros.Create(c.asker(), req)
	if err != nil {
		t.Fatalf("asking for the introduction: %v", err)
	}
	return id
}

// introductionsRequest is a direct ask of colleague about contact.
func introductionsRequest(contact ids.ContactID, colleague ids.UUID) introductions.NewRequest {
	return introductions.NewRequest{
		ContactID: contact.UUID, IntroducerUser: colleague, RouteType: "direct",
		InternalReason: "they run procurement", DueAt: time.Now().Add(72 * time.Hour),
	}
}

func (c *carryEnv) askStatus(t *testing.T, id ids.UUID) (status string, contact ids.UUID, through *ids.UUID) {
	t.Helper()
	if err := c.e.Pool.QueryRow(context.Background(),
		`SELECT status, contact_id, through_contact_id FROM intro_request WHERE id = $1`, id).
		Scan(&status, &contact, &through); err != nil {
		t.Fatalf("reading ask %s: %v", id, err)
	}
	return status, contact, through
}

// Both arms follow: an ask ABOUT the retired contact and an ask routed THROUGH
// them now name the survivor, and the survivor's page lists the first.
func TestAMergeCarriesAsksAboutAndThroughTheRetiredContact(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Intro Retired", "intro@carry.test")
	survivor := c.contactAt(t, "Intro Survivor", "intro-survivor@carry.test")
	target := c.contactAt(t, "Intro Target", "intro-target@carry.test")
	about := c.ask(t, retired, c.e.Rep1, nil)
	via := c.ask(t, target, c.e.Rep2, &retired)

	c.merge(t, retired, survivor)

	if _, contact, _ := c.askStatus(t, about); contact != survivor.UUID {
		t.Errorf("the ask about the retired contact names %s, want the survivor", contact)
	}
	if _, _, through := c.askStatus(t, via); through == nil || *through != survivor.UUID {
		t.Errorf("the ask routed through the retired contact runs through %v, want the survivor", through)
	}
	listed, err := c.intros.ForContact(c.asker(), survivor.UUID, 10)
	if err != nil {
		t.Fatalf("reading the survivor's asks: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != about {
		t.Errorf("the survivor's page lists %d ask(s), want the carried one", len(listed))
	}
}

// Two open asks of one colleague, one about each half, are one ask after the
// merge. The survivor's stands; the retired contact's closes as cancelled and
// says so in intro_request.closed.
func TestAnAskTheSurvivorAlreadyHasOpenIsClosed(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Dup Retired", "dup@carry.test")
	survivor := c.contactAt(t, "Dup Survivor", "dup-survivor@carry.test")
	survivors := c.ask(t, survivor, c.e.Rep1, nil)
	retiredAsk := c.ask(t, retired, c.e.Rep1, nil)

	c.merge(t, retired, survivor)

	if status, _, _ := c.askStatus(t, survivors); status != "requested" {
		t.Errorf("the survivor's own ask is %s, want it to stand as requested", status)
	}
	status, contact, _ := c.askStatus(t, retiredAsk)
	if status != "cancelled" || contact != survivor.UUID {
		t.Errorf("the duplicate ask is %s on %s, want cancelled and carried to the survivor", status, contact)
	}
	if n := c.count(t, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'intro_request.closed'
		   AND envelope->'payload'->>'intro_request_id' = $1
		   AND envelope->'payload'->>'reason' = 'cancelled'`, retiredAsk.String()); n != 1 {
		t.Errorf("%d intro_request.closed event(s) for the closed duplicate, want 1", n)
	}
}

// An ask about the retired contact routed through the survivor, or the other
// way round, becomes an ask about the survivor routed through the survivor. That
// is no way in, so it closes the way a requester closes one.
func TestAnAskTheMergeRoutesThroughItsOwnContactIsClosed(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Self Retired", "self@carry.test")
	survivor := c.contactAt(t, "Self Survivor", "self-survivor@carry.test")
	aboutRetired := c.ask(t, retired, c.e.Rep2, &survivor)
	throughRetired := c.ask(t, survivor, c.e.Rep1, &retired)

	c.merge(t, retired, survivor)

	for _, id := range []ids.UUID{aboutRetired, throughRetired} {
		status, contact, through := c.askStatus(t, id)
		if status != "cancelled" || contact != survivor.UUID || through == nil || *through != survivor.UUID {
			t.Errorf("ask %s is %s about %s through %v, want cancelled and re-homed onto the survivor", id, status, contact, through)
		}
		if n := c.closedEventsAbout(t, id, survivor.UUID); n != 1 {
			t.Errorf("%d intro_request.closed event(s) about the survivor for ask %s, want 1", n, id)
		}
	}
}

// Two asks about a third contact, one routed through each half, are one route
// after the merge. The retired half's closes, and its event names the contact
// the ask is ABOUT, not the survivor it was routed through.
func TestAThroughOnlyCollisionClosesAboutTheContactTheAskIsAbout(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Via Retired", "via@carry.test")
	survivor := c.contactAt(t, "Via Survivor", "via-survivor@carry.test")
	target := c.contactAt(t, "Via Target", "via-target@carry.test")
	kept := c.ask(t, target, c.e.Rep1, &survivor)
	closed := c.ask(t, target, c.e.Rep1, &retired)

	c.merge(t, retired, survivor)

	if status, _, _ := c.askStatus(t, kept); status != "requested" {
		t.Errorf("the survivor-side ask is %s, want it to stand", status)
	}
	if status, _, _ := c.askStatus(t, closed); status != "cancelled" {
		t.Errorf("the retired-side ask is %s, want cancelled as the duplicate", status)
	}
	if n := c.closedEventsAbout(t, closed, target.UUID); n != 1 {
		t.Errorf("%d intro_request.closed event(s) name the target, want 1 — the event named the wrong contact", n)
	}
}

// closedEventsAbout counts intro_request.closed envelopes for one ask whose
// entity and payload both name about.
func (c *carryEnv) closedEventsAbout(t *testing.T, ask, about ids.UUID) int {
	t.Helper()
	return c.count(t, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'intro_request.closed'
		   AND envelope->'payload'->>'intro_request_id' = $1
		   AND envelope->'payload'->>'contact_id' = $2
		   AND envelope->'entity'->>'id' = $2`, ask.String(), about.String())
}

func TestAnUnwiredMergeRefusesWhenAnAskWouldBeStranded(t *testing.T) {
	c := setupCarry(t)
	unwired := contacts.NewStore(c.e.DB()).WithStopCarrier(c.consent).WithSatelliteCarriers(c.consent, nil)
	retired := c.contactAt(t, "Unwired Ask", "unwired-ask@carry.test")
	survivor := c.contactAt(t, "Unwired Ask Survivor", "unwired-ask-survivor@carry.test")
	target := c.contactAt(t, "Unwired Ask Target", "unwired-ask-target@carry.test")
	c.ask(t, target, c.e.Rep1, &retired)

	_, err := unwired.MergeContact(c.admin, retired, survivor, nil)
	if _, ok := errors.AsType[*contacts.SatelliteCarrierNotWiredError](err); !ok {
		t.Fatalf("the merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
}
