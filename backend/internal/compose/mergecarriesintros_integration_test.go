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
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/introduction"
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

	preview := c.introPreview(t, retired, survivor, about, via)
	c.merge(t, retired, survivor)
	c.assertIntroPreview(t, preview)

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

	preview := c.introPreview(t, retired, survivor, retiredAsk)
	c.merge(t, retired, survivor)
	c.assertIntroPreview(t, preview)

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

	preview := c.introPreview(t, retired, survivor, aboutRetired, throughRetired)
	c.merge(t, retired, survivor)
	c.assertIntroPreview(t, preview)

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

	preview := c.introPreview(t, retired, survivor, closed)
	c.merge(t, retired, survivor)
	c.assertIntroPreview(t, preview)

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

	preview, previewErr := unwired.PreviewMergeSurvivorship(c.asker(), "contact", retired.UUID, survivor.UUID, nil, nil)
	if _, ok := errors.AsType[*contacts.SatelliteCarrierNotWiredError](previewErr); !ok || len(preview.Introductions) != 0 {
		t.Fatalf("unwired preview = %+v, %v", preview, previewErr)
	}
	_, err := unwired.MergeContact(c.admin, retired, survivor, nil)
	if _, ok := errors.AsType[*contacts.SatelliteCarrierNotWiredError](err); !ok {
		t.Fatalf("the merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
}

func (c *carryEnv) introPreview(t *testing.T, source, target ids.ContactID, expected ...ids.UUID) []introduction.CarriedRequest {
	t.Helper()
	preview, err := c.contacts.PreviewMergeSurvivorship(c.asker(), "contact", source.UUID, target.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Introductions) != len(expected) {
		t.Fatalf("introduction effects = %d, want %d", len(preview.Introductions), len(expected))
	}
	for _, id := range expected {
		if !slices.ContainsFunc(preview.Introductions, func(change introduction.CarriedRequest) bool { return change.ID == id }) {
			t.Fatalf("missing introduction effect for %s", id)
		}
	}
	for _, change := range preview.Introductions {
		status, contact, through := c.askStatus(t, change.ID)
		if change.Version < 1 || status != change.StatusBefore || contact != change.ContactBefore || !reflect.DeepEqual(through, change.ThroughBefore) {
			t.Fatalf("preview changed or misstated original ask: %+v", change)
		}
	}
	return preview.Introductions
}

func (c *carryEnv) assertIntroPreview(t *testing.T, preview []introduction.CarriedRequest) {
	t.Helper()
	for _, change := range preview {
		if change.StatusAfter == "archived" {
			var archived bool
			if err := c.e.Pool.QueryRow(context.Background(),
				"SELECT archived_at IS NOT NULL FROM intro_request WHERE id=$1", change.ID).Scan(&archived); err != nil {
				t.Fatal(err)
			}
			if !archived || change.CancellationReason == "" {
				t.Fatalf("merge disagrees with preview %+v: archived=%v", change, archived)
			}
			continue
		}
		status, contact, through := c.askStatus(t, change.ID)
		if status != change.StatusAfter || contact != change.ContactAfter || !reflect.DeepEqual(through, change.ThroughAfter) {
			t.Fatalf("merge disagrees with preview %+v: %s %s %v", change, status, contact, through)
		}
		var reason *string
		if err := c.e.Pool.QueryRow(context.Background(), "SELECT decision_reason FROM intro_request WHERE id=$1", change.ID).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if change.CancellationReason != "" && (reason == nil || *reason != change.CancellationReason) {
			t.Fatalf("cancellation reason = %v, want %q", reason, change.CancellationReason)
		}
	}
}

func TestMergePreviewDoesNotDisclosePrivateIntroductionDependencies(t *testing.T) {
	for _, privateWitness := range []bool{false, true} {
		t.Run(map[bool]string{false: "source ask", true: "collision witness"}[privateWitness], func(t *testing.T) {
			c := setupCarry(t)
			source := c.contactAt(t, "Private source", "private-source@carry.test")
			target := c.contactAt(t, "Private target", "private-target@carry.test")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["introduction"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
			other := c.e.As(c.e.Rep2, nil, perms)
			privateContact := source
			if privateWitness {
				c.ask(t, source, c.e.Rep1, nil)
				privateContact = target
			}
			if _, err := c.intros.Create(other, introductionsRequest(privateContact, c.e.Rep1)); err != nil {
				t.Fatal(err)
			}
			preview, err := c.contacts.PreviewMergeSurvivorship(c.asker(), "contact", source.UUID, target.UUID, nil, nil)
			if !errors.Is(err, apperrors.ErrNotFound) || !reflect.DeepEqual(preview, contacts.MergeSurvivorship{}) {
				t.Fatalf("private request exposed: %+v, %v", preview, err)
			}
		})
	}
}

func TestMergePreviewCarriesAClosedIntroductionWithoutCancellingItAgain(t *testing.T) {
	c := setupCarry(t)
	source := c.contactAt(t, "Closed source", "closed-source@carry.test")
	target := c.contactAt(t, "Closed target", "closed-target@carry.test")
	ask := c.ask(t, source, c.e.Rep1, nil)
	if err := c.intros.Cancel(c.asker(), ask, "No longer needed", 1); err != nil {
		t.Fatal(err)
	}
	preview := c.introPreview(t, source, target, ask)
	if len(preview) != 1 || preview[0].ID != ask || preview[0].StatusBefore != "cancelled" || preview[0].StatusAfter != "cancelled" || preview[0].CancellationReason != "" {
		t.Fatalf("closed request would be cancelled again: %+v", preview)
	}
	c.merge(t, source, target)
	c.assertIntroPreview(t, preview)
	var reason string
	if err := c.e.Pool.QueryRow(context.Background(), "SELECT decision_reason FROM intro_request WHERE id=$1", ask).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "No longer needed" {
		t.Fatalf("prior cancellation reason changed: %q", reason)
	}
}

func TestIntroductionPreviewChecksReadPermissionBeforeLookingForRequests(t *testing.T) {
	c := setupCarry(t)
	source := c.contactAt(t, "Read source", "read-source@carry.test")
	target := c.contactAt(t, "Read target", "read-target@carry.test")
	for _, hasAsk := range []bool{false, true} {
		if hasAsk {
			c.ask(t, source, c.e.Rep1, nil)
		}
		preview, err := c.contacts.PreviewMergeSurvivorship(c.e.Admin(), "contact", source.UUID, target.UUID, nil, nil)
		if !errors.Is(err, apperrors.ErrPermissionDenied) || !reflect.DeepEqual(preview, contacts.MergeSurvivorship{}) {
			t.Fatalf("without introduction read, hasAsk=%v: %+v, %v", hasAsk, preview, err)
		}
	}
}

func TestMergePreviewRefusesAnArchivedIntroductionItWouldCarry(t *testing.T) {
	c := setupCarry(t)
	source := c.contactAt(t, "Archived source", "archived-source@carry.test")
	target := c.contactAt(t, "Archived target", "archived-target@carry.test")
	ask := c.ask(t, source, c.e.Rep1, nil)
	// No introduction API archives requests; this exercises a persisted archived row.
	if _, err := c.e.Pool.Exec(context.Background(), "UPDATE intro_request SET archived_at=requested_at WHERE id=$1", ask); err != nil {
		t.Fatal(err)
	}
	c.ask(t, target, c.e.Rep1, nil)
	preview, err := c.contacts.PreviewMergeSurvivorship(c.asker(), "contact", source.UUID, target.UUID, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) || !reflect.DeepEqual(preview, contacts.MergeSurvivorship{}) {
		t.Fatalf("archived request omitted: %+v, %v", preview, err)
	}
	c.merge(t, source, target)
	if status, contact, _ := c.askStatus(t, ask); status != "requested" || contact != target.UUID {
		t.Fatalf("archived ask was not carried: %s %s", status, contact)
	}
}

func TestMergeIntroductionPreviewRefusesToTruncateItsScope(t *testing.T) {
	c := setupCarry(t)
	source := c.contactAt(t, "Bounded source", "bounded-source@carry.test")
	target := c.contactAt(t, "Bounded target", "bounded-target@carry.test")
	for range 500 {
		ask := c.ask(t, source, c.e.Rep1, nil)
		if err := c.intros.Cancel(c.asker(), ask, "No longer needed", 1); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := c.contacts.PreviewMergeSurvivorship(c.asker(), "contact", source.UUID, target.UUID, nil, nil)
	if err != nil || len(preview.Introductions) != 500 {
		t.Fatalf("complete bounded scope: %d, %v", len(preview.Introductions), err)
	}
	c.ask(t, source, c.e.Rep1, nil)
	preview, err = c.contacts.PreviewMergeSurvivorship(c.asker(), "contact", source.UUID, target.UUID, nil, nil)
	if !errors.Is(err, apperrors.ErrInvalidArgument) || !reflect.DeepEqual(preview, contacts.MergeSurvivorship{}) {
		t.Fatalf("truncated scope returned: %+v, %v", preview, err)
	}
}

// record writes one recorded introduction through the real writer, as the
// colleague who made it.
func (c *carryEnv) record(t *testing.T, contact ids.ContactID, introducer, introduced ids.UUID, daysAgo int) ids.UUID {
	t.Helper()
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["introduction"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	id, err := c.intros.Record(c.e.As(introducer, nil, perms), introductions.NewRecorded{
		ContactID: contact.UUID, IntroducerUser: introducer, IntroducedUser: introduced,
		On: clocktest.Now(t).AddDate(0, 0, -daysAgo), Channel: "call",
	})
	if err != nil {
		t.Fatalf("recording the introduction: %v", err)
	}
	return id
}

func (c *carryEnv) archivedAt(t *testing.T, id ids.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := c.e.Pool.QueryRow(context.Background(),
		`SELECT archived_at FROM intro_request WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

// Two recorded introductions of one pair on the two halves are one after the
// merge: the unique index on the pair would refuse the re-home, so the older
// stands on the survivor and the newer is archived, in the preview and in the
// merge alike. A recorded introduction of another pair is carried untouched.
func TestAMergeKeepsTheOlderRecordedIntroductionOfAPair(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Rec Retired", "rec@carry.test")
	survivor := c.contactAt(t, "Rec Survivor", "rec-survivor@carry.test")
	older := c.record(t, retired, c.e.Rep1, c.e.Rep2, 5)
	newer := c.record(t, survivor, c.e.Rep1, c.e.Rep2, 1)
	otherPair := c.record(t, retired, c.e.Rep2, c.e.Rep1, 1)

	preview := c.introPreview(t, retired, survivor, older, otherPair, newer)
	c.merge(t, retired, survivor)
	c.assertIntroPreview(t, preview)

	if c.archivedAt(t, older) != nil {
		t.Error("the older recorded introduction was archived")
	}
	if c.archivedAt(t, newer) == nil {
		t.Error("the newer recorded introduction of the same pair stands beside the older")
	}
	for _, id := range []ids.UUID{older, otherPair} {
		if _, contact, _ := c.askStatus(t, id); contact != survivor.UUID {
			t.Errorf("recorded introduction %s names %s, want the survivor", id, contact)
		}
	}
	if c.archivedAt(t, otherPair) != nil {
		t.Error("a recorded introduction of another pair was archived")
	}
}
