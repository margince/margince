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
	req := introductions.NewRequest{
		ContactID: contact.UUID, IntroducerUser: colleague, RouteType: "direct",
		InternalReason: "they run procurement", DueAt: time.Now().Add(72 * time.Hour),
	}
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
	retireds := c.ask(t, retired, c.e.Rep1, nil)

	c.merge(t, retired, survivor)

	if status, _, _ := c.askStatus(t, survivors); status != "requested" {
		t.Errorf("the survivor's own ask is %s, want it to stand as requested", status)
	}
	status, contact, _ := c.askStatus(t, retireds)
	if status != "cancelled" || contact != survivor.UUID {
		t.Errorf("the duplicate ask is %s on %s, want cancelled and carried to the survivor", status, contact)
	}
	if n := c.count(t, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'intro_request.closed'
		   AND envelope->'payload'->>'intro_request_id' = $1
		   AND envelope->'payload'->>'reason' = 'cancelled'`, retireds.String()); n != 1 {
		t.Errorf("%d intro_request.closed event(s) for the closed duplicate, want 1", n)
	}
}

// Two of the retired contact's own asks that were distinct routes — about it
// through the survivor, and about the survivor through it — are the same
// route once both columns point at the survivor. The older stands.
func TestTwoAsksThatCollideOnlyOnceBothColumnsMapKeepTheOlder(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Arm Retired", "arm@carry.test")
	survivor := c.contactAt(t, "Arm Survivor", "arm-survivor@carry.test")
	older := c.ask(t, retired, c.e.Rep2, &survivor)
	newer := c.ask(t, survivor, c.e.Rep2, &retired)

	c.merge(t, retired, survivor)

	if status, _, _ := c.askStatus(t, older); status != "requested" {
		t.Errorf("the older ask is %s, want it to stand", status)
	}
	if status, _, _ := c.askStatus(t, newer); status != "cancelled" {
		t.Errorf("the newer ask is %s, want cancelled as the duplicate", status)
	}
}

func TestAnUnwiredMergeRefusesWhenAnAskWouldBeStranded(t *testing.T) {
	c := setupCarry(t)
	unwired := contacts.NewStore(c.e.DB()).WithStopCarrier(c.consent).WithSatelliteCarriers(c.consent, nil)
	retired := c.contactAt(t, "Unwired Ask", "unwired-ask@carry.test")
	survivor := c.contactAt(t, "Unwired Ask Survivor", "unwired-ask-survivor@carry.test")
	target := c.contactAt(t, "Unwired Ask Target", "unwired-ask-target@carry.test")
	c.ask(t, target, c.e.Rep1, &retired)

	_, err := unwired.MergeContact(c.admin, retired, survivor)
	var notWired *contacts.SatelliteCarrierNotWiredError
	if !errors.As(err, &notWired) {
		t.Fatalf("the merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
}
