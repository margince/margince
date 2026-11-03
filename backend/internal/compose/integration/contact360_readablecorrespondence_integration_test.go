// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contact's last-message dates and follow-up card count the messages the
// reader can open. Alex and Blair read the same contact, Casey. Alex wrote to
// Casey 30 days before the clock, on a workspace-visible mail. Casey replied
// two days before the clock, on a mail limited to its participants: Alex can
// open it, Blair cannot. Alex must see the reply and no "no reply" card;
// Blair's page must not change at all because of a reply Blair cannot open.

type correspondenceFixture struct {
	e                *Env
	alex, blair      context.Context
	casey            ids.UUID
	outbound, inward ids.UUID
}

func newCorrespondenceFixture(t *testing.T) correspondenceFixture {
	t.Helper()
	e := Setup(t)
	f := correspondenceFixture{
		e:     e,
		alex:  e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms),
		blair: e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms),
	}
	f.casey = e.SeedContact(t, "Casey Example", &e.Rep1)
	f.outbound = f.logMail(t, "Renewal proposal", "outbound", 30*24*time.Hour)
	return f
}

// logMail writes one mail through the activities writer as Alex.
func (f correspondenceFixture) logMail(t *testing.T, subject, direction string, ago time.Duration) ids.UUID {
	t.Helper()
	at := roomAgo(ago)
	body := "Message: " + subject
	logged, _, err := f.e.Activities.LogActivity(f.alex, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.casey}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return ids.UUID(logged.Id)
}

// caseyReplies records Casey's reply and limits it to its participants
// through the audience control a human uses.
func (f *correspondenceFixture) caseyReplies(t *testing.T) {
	t.Helper()
	f.inward = f.logMail(t, "Re: Renewal proposal", "inbound", 2*24*time.Hour)
	// The writer stamps the linked contact as the sender of a logged inbound
	// mail, which is what the inbound clock asks (relstrength.SenderSQL).
	f.setAudience(t, "participants")
}

func (f correspondenceFixture) setAudience(t *testing.T, audience string) {
	t.Helper()
	if _, err := f.e.Activities.SetAudience(f.alex, ids.From[ids.ActivityKind](f.inward),
		activities.SetAudienceInput{Audience: audience}); err != nil {
		t.Fatalf("setting the reply's audience to %s: %v", audience, err)
	}
}

func (f correspondenceFixture) page(t *testing.T, reader context.Context) crmcontracts.Contact360 {
	t.Helper()
	page, err := contactRoomService(f.e).Assemble(reader, ids.From[ids.ContactKind](f.casey))
	if err != nil {
		t.Fatalf("assembling Casey's page: %v", err)
	}
	return page
}

// worklistTouch is the reading the Worklist rows and the quiet-relationship
// lane ask for (attentiontouchseam.go, attentionlanesseam.go).
func (f correspondenceFixture) worklistTouch(t *testing.T, reader context.Context) contact360.LastTouch {
	t.Helper()
	var touch contact360.LastTouch
	if err := database.WithWorkspaceTx(reader, f.e.Pool, func(tx pgx.Tx) error {
		touched, err := contact360.LastTouchFor(reader, tx, []ids.ContactID{ids.From[ids.ContactKind](f.casey)}, contact360.AssembleOptions{AsOf: roomFixedNow})
		touch = touched[ids.From[ids.ContactKind](f.casey)]
		return err
	}); err != nil {
		t.Fatalf("reading the Worklist touch: %v", err)
	}
	return touch
}

func TestAReplyTheReaderCanOpenAnswersTheirFollowUpCard(t *testing.T) {
	f := newCorrespondenceFixture(t)
	f.caseyReplies(t)
	replyAt := roomAgo(2 * 24 * time.Hour)

	alex := f.page(t, f.alex)
	if alex.LastTouchBasis == nil || *alex.LastTouchBasis != crmcontracts.Contact360LastTouchBasisReadableByCaller {
		t.Errorf("last_touch_basis = %v, want readable_by_caller", alex.LastTouchBasis)
	}
	if alex.LastInboundAt == nil || !alex.LastInboundAt.Equal(replyAt) {
		t.Fatalf("Alex's last_inbound_at = %v, want Casey's reply at %v", alex.LastInboundAt, replyAt)
	}
	if !sameID(alex.LastInboundActivityId, f.inward) {
		t.Errorf("Alex's last_inbound_activity_id = %v, want the reply %v", alex.LastInboundActivityId, f.inward)
	}
	if alex.RelationshipDirection == nil || *alex.RelationshipDirection != crmcontracts.RelationshipDirectionTwoWay {
		t.Errorf("Alex's relationship_direction = %v, want two_way", alex.RelationshipDirection)
	}
	if alex.Moment != nil && alex.Moment.Rule == crmcontracts.ContactMomentRuleGoneQuiet {
		t.Errorf("Alex was told the outbound is unanswered although Casey's reply is readable: %+v", alex.Moment)
	}
	if touch := f.worklistTouch(t, f.alex); touch.InboundAt == nil || !touch.InboundAt.Equal(replyAt) {
		t.Errorf("Alex's Worklist inbound = %v, want the reply the contact page shows", touch.InboundAt)
	}
}

func TestAReplyTheReaderCannotOpenChangesNothingTheySee(t *testing.T) {
	f := newCorrespondenceFixture(t)
	before := f.page(t, f.blair)
	f.caseyReplies(t)
	after := f.page(t, f.blair)

	if after.LastInboundAt != nil || after.LastInboundActivityId != nil || after.LastLiveInboundAt != nil {
		t.Errorf("Blair's page carries the private reply: inbound %v (%v), live %v",
			after.LastInboundAt, after.LastInboundActivityId, after.LastLiveInboundAt)
	}
	if after.RelationshipDirection == nil || *after.RelationshipDirection != *before.RelationshipDirection {
		t.Errorf("Blair's relationship_direction moved from %v to %v", before.RelationshipDirection, after.RelationshipDirection)
	}
	if before.Moment == nil || after.Moment == nil {
		t.Fatal("Blair's page has no moment")
	}
	if after.Moment.Rule != crmcontracts.ContactMomentRuleGoneQuiet {
		t.Fatalf("Blair's moment = %s, want gone_quiet: Blair can open only the outbound", after.Moment.Rule)
	}
	if after.Moment.EvidenceFingerprint != before.Moment.EvidenceFingerprint ||
		after.Moment.Headline != before.Moment.Headline || after.Moment.WhyNow != before.Moment.WhyNow {
		t.Errorf("Blair's card changed when a reply Blair cannot open arrived:\nbefore %+v\nafter  %+v", *before.Moment, *after.Moment)
	}
	if before.Strength == nil || after.Strength == nil || before.Strength.Score != after.Strength.Score ||
		countOf(before.Strength.Inbound90d) != countOf(after.Strength.Inbound90d) {
		t.Errorf("Blair's shared strength moved: before %+v after %+v", before.Strength, after.Strength)
	}
	alex := f.page(t, f.alex)
	if alex.Strength == nil || alex.Strength.Score != after.Strength.Score {
		t.Errorf("shared strength differs by reader: Alex %+v, Blair %+v", alex.Strength, after.Strength)
	}
	if touch := f.worklistTouch(t, f.blair); touch.InboundAt != nil {
		t.Errorf("Blair's Worklist row carries the private reply at %v", touch.InboundAt)
	}

	// Opening the reply to the workspace lets Blair count it; limiting it again
	// takes it away.
	f.setAudience(t, "workspace")
	if opened := f.page(t, f.blair); opened.LastInboundAt == nil {
		t.Error("Blair's page does not count a reply opened to the workspace")
	}
	f.setAudience(t, "participants")
	if revoked := f.page(t, f.blair); revoked.LastInboundAt != nil {
		t.Errorf("Blair's page still counts the reply after its audience was limited again: %v", revoked.LastInboundAt)
	}
}

func TestTheFollowUpCardCitesItsOwnRowWithOneDateAnchor(t *testing.T) {
	f := newCorrespondenceFixture(t)
	// A second outbound at the very same instant, and a full timeline page of
	// newer notes that push both mails off the first page.
	twin := f.logMail(t, "Renewal proposal (copy)", "outbound", 30*24*time.Hour)
	for range 26 {
		note := "call notes"
		if _, _, err := f.e.Activities.LogActivity(f.alex, activities.LogActivityInput{
			Kind: "note", Body: &note, OccurredAt: new(roomAgo(24 * time.Hour)),
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.casey}},
		}); err != nil {
			t.Fatalf("logging a note: %v", err)
		}
	}

	page := f.page(t, f.blair)
	if page.Moment == nil || page.Moment.Rule != crmcontracts.ContactMomentRuleGoneQuiet {
		t.Fatalf("moment = %+v, want gone_quiet", page.Moment)
	}
	if page.LastOutboundActivityId == nil {
		t.Fatal("last_outbound_activity_id is empty")
	}
	picked := ids.UUID(*page.LastOutboundActivityId)
	if picked != f.outbound && picked != twin {
		t.Fatalf("last_outbound_activity_id = %v, not one of the two outbound mails", picked)
	}
	evidence := page.Moment.Evidence
	if len(evidence) != 1 || evidence[0].Id == nil || ids.UUID(*evidence[0].Id) != picked {
		t.Errorf("evidence = %+v, want the row the query picked (%v) although it is past the first timeline page", evidence, picked)
	}
	again := f.page(t, f.blair)
	if again.Moment == nil || again.Moment.EvidenceFingerprint != page.Moment.EvidenceFingerprint {
		t.Error("the fingerprint changed between two reads of the same evidence")
	}
	// Both sentences count from the same message: 30 days.
	for _, said := range []string{page.Moment.Headline, page.Moment.WhyNow} {
		if !strings.Contains(said, "30 days") {
			t.Errorf("%q does not count from the last outbound (30 days)", said)
		}
	}
}

func countOf(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func sameID(wire *openapi_types.UUID, want ids.UUID) bool {
	return wire != nil && ids.UUID(*wire) == want
}
