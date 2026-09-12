// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Narrowing a read to ONE body of work, on both surfaces that carry it: the
// timeline list, and the context walk every assembled picture is built from.
//
// The rule is "filed under this project, or filed under none". The NEGATIVE
// half is what these prove — a test asserting only that the wanted rows appear
// would pass against a filter that does nothing at all, which is the failure
// mode a predicate like this actually has.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// scopeFixture is one account running two engagements, plus ordinary
// correspondence belonging to neither — the shape the rule exists for.
type scopeFixture struct {
	contact ids.UUID
	company ids.UUID
	erp     ids.ProjectID
	// other is the second engagement, the one a scope to erp must drop.
	other ids.ProjectID
	// The keys the SERVER minted for the two projects. A caller no longer
	// chooses a key, so a test that reads a page's project by key has to read
	// back what the create actually produced.
	erpKey   string
	otherKey string
	// bystander is a second contact who appears ONLY in the other
	// engagement's mail — the hop-2 case: a scoped walk reaches contacts
	// through the activities it kept, so a contact reachable only through a
	// dropped activity must not appear either.
	bystander ids.UUID
	onERP     string
	onOther   string
	unfiled   string
	// One open task and one future meeting per engagement, so the next-steps,
	// next-meeting and since-last-visit sections each have a row the scope
	// must drop and a row it must keep.
	erpTask      string
	otherTask    string
	erpMeeting   string
	otherMeeting string
	// otherAt is when the other engagement's mail arrived — the NEWEST
	// exchange on the account, so an unscoped last-touch date is this one and
	// a scoped read that still reports it has leaked.
	otherAt time.Time
}

// Everything here is written by the REAL writers — CreateProject, LogActivity
// and RelinkActivity — each through its own authority check and the audit +
// outbox write shape. Hand-inserted rows would let the filter pass over a row
// shape production never produces.
func seedTwoEngagementAccount(t *testing.T, e *Env) scopeFixture {
	t.Helper()
	admin := e.Admin()
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	// Somebody at the account who was in the room. A meeting is with a contact
	// and cannot be filed against a company, so an ATTENDEE WITH A JOB THERE is
	// how a meeting reaches the account at all — and it has to be a second
	// contact, because `contact` is deliberately unemployed here (the contact
	// page's project routes are proved one at a time, seat before employer).
	attendee := e.SeedContact(t, "Ilse Teilnehmer", &e.Rep1)
	attendeeID, companyID := ContactIDOf(attendee), companyIDOf(company)
	if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &attendeeID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing the attendee: %v", err)
	}

	newProject := func(name string) (ids.ProjectID, string) {
		p, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
			Name: name, CompanyID: companyIDOf(company), Source: "manual",
		})
		if err != nil {
			t.Fatalf("create project %q: %v", name, err)
		}
		if p.Key == nil {
			t.Fatalf("the server minted no key for %q", name)
		}
		return projectIDOf(ids.UUID(p.Id)), *p.Key
	}
	erp, erpKey := newProject("ERP rollout")
	migration, migrationKey := newProject("Datacentre migration")
	bystander := e.SeedContact(t, "Rack Vendor", &e.Rep1)

	// Three exchanges with the same contact on the same account: one per
	// engagement, and one ordinary message nobody filed. Each names the contact,
	// and names the COMPANY too where the kind permits it — a meeting is
	// with a contact and reaches the account through the contact's employer
	// instead, which is the arm activities.CompanyLinkedActivityExists walks.
	log := func(in activities.LogActivityInput, subject string, within *ids.ProjectID, occurredAt time.Time, others ...ids.UUID) string {
		in.Subject, in.OccurredAt = &subject, &occurredAt
		in.Links = []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: contact},
		}
		if in.Kind == "meeting" || in.Kind == "call" {
			in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: attendee})
		} else {
			in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "company", EntityID: company})
		}
		for _, other := range others {
			in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: other})
		}
		logged, _, err := e.Activities.LogActivity(admin, in)
		if err != nil {
			t.Fatalf("log %q: %v", subject, err)
		}
		if within != nil {
			id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
			if _, err := e.Activities.RelinkActivity(admin, id, activities.RelinkActivityInput{
				EntityType: "project", EntityID: within.UUID,
			}); err != nil {
				t.Fatalf("file %q under its project: %v", subject, err)
			}
		}
		return ids.UUID(logged.Id).String()
	}

	mail := func(subject string, within *ids.ProjectID, occurredAt time.Time, others ...ids.UUID) string {
		return log(activities.LogActivityInput{Kind: "email", Direction: strPtr("inbound")}, subject, within, occurredAt, others...)
	}
	task := func(subject string, within *ids.ProjectID) string {
		return log(activities.LogActivityInput{Kind: "task"}, subject, within, roomFixedNow.AddDate(0, 0, -1))
	}
	meeting := func(subject string, within *ids.ProjectID, startsAt time.Time) string {
		return log(activities.LogActivityInput{Kind: "meeting", MeetingStatus: strPtr("booked")}, subject, within, startsAt)
	}
	otherAt := roomFixedNow.AddDate(0, 0, -1)
	return scopeFixture{
		contact: contact, company: company, erp: erp, other: migration, bystander: bystander, otherAt: otherAt,
		erpKey: erpKey, otherKey: migrationKey,
		onERP:     mail("ERP cutover plan", &erp, roomFixedNow.AddDate(0, 0, -3)),
		onOther:   mail("Rack decommissioning", &migration, otherAt, bystander),
		unfiled:   mail("Invoice question", nil, roomFixedNow.AddDate(0, 0, -2)),
		erpTask:   task("Send ERP cutover checklist", &erp),
		otherTask: task("Book the rack haulier", &migration),
		// The other engagement's meeting is the SOONER one, so an unscoped
		// next-meeting read names it and a scoped read that still does has
		// leaked the other project.
		erpMeeting:   meeting("ERP go-live rehearsal", &erp, roomFixedNow.AddDate(0, 0, 5)),
		otherMeeting: meeting("Rack move walkthrough", &migration, roomFixedNow.AddDate(0, 0, 2)),
	}
}

func TestTimelineScopedToOneProjectDropsTheOtherEngagement(t *testing.T) {
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)

	contact := string(datasource.RecordContact)
	got, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{
		EntityType: &contact, EntityID: &f.contact, WithinProjectID: &f.erp,
	})
	if err != nil {
		t.Fatalf("list within project: %v", err)
	}
	seen := map[string]bool{}
	for _, a := range got {
		seen[a.Id.String()] = true
	}

	if seen[f.onOther] {
		t.Error("the other engagement's mail survived a scoped read — the scope filtered nothing")
	}
	if !seen[f.onERP] {
		t.Error("the scoped project's own mail is missing")
	}
	// Attribution is optional, so unfiled mail is the account's general
	// history. Dropping it would leave a brief reading as though the
	// relationship had no past.
	if !seen[f.unfiled] {
		t.Error("mail filed under NO project was dropped; the rule keeps it")
	}
}

func TestTimelineWithoutAScopeStillSeesEveryEngagement(t *testing.T) {
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)

	contact := string(datasource.RecordContact)
	got, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{
		EntityType: &contact, EntityID: &f.contact,
	})
	if err != nil {
		t.Fatalf("list unscoped: %v", err)
	}
	if len(got) != 7 {
		t.Fatalf("unscoped timeline = %d rows, want all 7 — an absent scope narrows nothing", len(got))
	}
}

// The context walk carries its OWN copy of the predicate, in another module a
// module may not import (ADR-0054). A test of the timeline list alone would
// pass with this half absent — and this is the half every assembled picture
// reads, so catch_me_up_on and prep_for_meeting hang off it.
func TestAssembledContextScopedToOneProjectDropsTheOtherEngagement(t *testing.T) {
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)
	// AssembleContext embeds nothing — only Search does — so the walk needs no
	// embedder to answer.
	retriever := search.NewRetriever(search.NewStore(harnessDB(e.Pool, e.WS)), nil)
	anchor := datasource.EntityRef{Type: datasource.EntityContact, ID: f.contact}

	idsIn := func(opts retrieval.AssembleOptions) map[string]bool {
		t.Helper()
		return walkIDs(e.Admin(), t, retriever, anchor, opts)
	}

	scoped := idsIn(retrieval.AssembleOptions{MaxItems: 25, ProjectID: f.erp.String()})
	if scoped[f.onOther] {
		t.Error("the context walk carried the other engagement into a scoped picture")
	}
	if !scoped[f.onERP] {
		t.Error("the scoped project's own mail is missing from the walk")
	}
	if !scoped[f.unfiled] {
		t.Error("the walk dropped mail filed under no project; the rule keeps it")
	}

	// The same anchor unscoped still sees everything, so the narrowing above is
	// the scope's doing rather than something else in the walk quietly losing a
	// row — which would make the assertions pass for the wrong reason.
	wide := idsIn(retrieval.AssembleOptions{MaxItems: 25})
	if !wide[f.onOther] {
		t.Error("an unscoped walk lost the other engagement, so the scoped one proves nothing")
	}
}

// walkIDs flattens one context walk to the ids it carried, across every
// section.
func walkIDs(ctx context.Context, t *testing.T, retriever *search.Retriever, anchor datasource.EntityRef, opts retrieval.AssembleOptions) map[string]bool {
	t.Helper()
	got, err := retriever.AssembleContext(ctx, anchor, opts)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	out := map[string]bool{}
	for _, section := range got.Sections {
		for _, item := range section.Items {
			out[item.Ref.ID.String()] = true
		}
	}
	return out
}

// Hop 2 follows the activities hop 1 KEPT. A contact reachable only through
// the dropped mail is outside the scope too; had hop 2 walked the unscoped
// timeline, the other engagement's contact would still be in the picture
// under a heading the scope claims to have narrowed.
//
// Anchored on the ACCOUNT: a contact anchor never walks contact neighbours (the
// anchor is not its own neighbour), so related_contacts only exists from here.
func TestAssembledContextScopedToOneProjectDropsContactsReachedOnlyThroughTheOtherEngagement(t *testing.T) {
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)
	retriever := search.NewRetriever(search.NewStore(harnessDB(e.Pool, e.WS)), nil)
	anchor := datasource.EntityRef{Type: datasource.EntityCompany, ID: f.company}

	scoped := walkIDs(e.Admin(), t, retriever, anchor, retrieval.AssembleOptions{MaxItems: 25, ProjectID: f.erp.String()})
	if scoped[f.bystander.String()] {
		t.Error("a contact linked only through the other engagement's mail reached the scoped walk's related_contacts")
	}
	if !scoped[f.contact.String()] {
		t.Error("the contact on the scoped engagement's own mail is missing from related_contacts")
	}
	wide := walkIDs(e.Admin(), t, retriever, anchor, retrieval.AssembleOptions{MaxItems: 25})
	if !wide[f.bystander.String()] {
		t.Error("an unscoped walk lost the other engagement's contact, so the scoped absence proves nothing")
	}
}

// The DERIVED sections narrow with the timeline they sit under.
// The scope reached the timeline, next steps, next meeting, last touch and
// since-last-visit. It did not reach the AGGREGATES: relationship strength and
// account health folded every project's activity, so a page showing one
// engagement's mail reported a surface counted over both.
//
// Asserted BOTH ways round against the two-engagement fixture. A test that only
// checked the scoped number would pass against a read that lost the row for any
// other reason, so the unscoped read of the same account has to still carry it.
func TestAScopedAccountPageDerivesItsHealthFromOneEngagement(t *testing.T) {
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)
	employAtAccount(t, e, f)
	// The bystander is the whole lever here: a contact at the account who has
	// spoken on the OTHER engagement and nowhere else. Employing them is what
	// puts them in the account's contact set at all, and the fixture leaves it
	// to the tests that want it — the sections proved elsewhere count contacts
	// and would each have to be retaught a third one.
	bystanderID, companyID := ContactIDOf(f.bystander), companyIDOf(f.company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &bystanderID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing the bystander: %v", err)
	}
	// A meeting that has already happened on each engagement, the other
	// engagement's the more recent. The fixture's own meetings are both booked
	// for the future, which is the question "when do we next see them" rather
	// than "when did we last".
	held := func(subject string, within ids.ProjectID, daysAgo int) time.Time {
		at := roomFixedNow.AddDate(0, 0, -daysAgo)
		logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", MeetingStatus: strPtr("booked"), Subject: &subject, OccurredAt: &at,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.contact}},
		})
		if err != nil {
			t.Fatalf("log %q: %v", subject, err)
		}
		id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
		if _, err := e.Activities.RelinkActivity(e.Admin(), id, activities.RelinkActivityInput{
			EntityType: "project", EntityID: within.UUID,
		}); err != nil {
			t.Fatalf("file %q under its project: %v", subject, err)
		}
		return at
	}
	erpMet := held("ERP discovery workshop", f.erp, 6)
	held("Rack survey", f.other, 2)

	svc := companySurfaceService(e)

	scoped, err := svc.AssembleScoped(e.Admin(), companyID, company360.AssembleOptions{ProjectID: &f.erp})
	if err != nil {
		t.Fatalf("assemble scoped: %v", err)
	}
	wide, err := svc.AssembleScoped(e.Admin(), companyID, company360.AssembleOptions{})
	if err != nil {
		t.Fatalf("assemble unscoped: %v", err)
	}
	if scoped.Health == nil || wide.Health == nil {
		t.Fatal("the health block is missing from one of the two reads")
	}

	// Three contacts have spoken to us across the account — the contact, the
	// meeting attendee and the bystander — but only two of them within the ERP
	// rollout. "How many ways in do we have here" has to answer for the page
	// the reader is on, or the count contradicts the timeline under it.
	if got := deref(t, scoped.Health.ActiveContacts, "scoped active contacts"); got != 2 {
		t.Errorf("the ERP page reports %d active contacts, want 2 (the contact and the meeting attendee) "+
			"— the bystander has only ever spoken on the datacentre migration", got)
	}
	if got := deref(t, wide.Health.ActiveContacts, "unscoped active contacts"); got != 3 {
		t.Errorf("the unscoped page reports %d active contacts, want 3 — if the bystander is missing "+
			"here too then the scoped count above proves nothing about the scope", got)
	}

	// The account's newest exchange is the other engagement's, so an unscoped
	// health block dates itself from a message the scoped page does not show.
	scopedAge := deref(t, scoped.Health.DaysSinceLastInbound, "scoped last-inbound age")
	wideAge := deref(t, wide.Health.DaysSinceLastInbound, "unscoped last-inbound age")
	// The last meeting is the same question one more time, on the read that
	// answers "when did we last sit down with them".
	if scoped.Health.LastMeetingAt == nil || wide.Health.LastMeetingAt == nil {
		t.Fatal("one of the two pages reports no last meeting, though each engagement has held one")
	}
	if !scoped.Health.LastMeetingAt.Equal(erpMet) {
		t.Errorf("the ERP page dates its last meeting to %s, want the ERP discovery workshop at %s "+
			"— the rack survey belongs to the engagement this page is not showing",
			scoped.Health.LastMeetingAt.Format(time.RFC3339), erpMet.Format(time.RFC3339))
	}
	if wide.Health.LastMeetingAt.Equal(erpMet) {
		t.Errorf("the unscoped page also stops at the ERP workshop, so the assertion above proves " +
			"nothing about the scope")
	}

	if scopedAge == wideAge {
		t.Errorf("both pages date their health from a message %d days old — the other engagement's mail "+
			"is the account's newest, so a scoped read that agrees with the unscoped one is still folding it",
			scopedAge)
	}
}

// deref reads a health figure the page is expected to carry, failing loudly
// rather than comparing against a zero the reader would never have seen.
func deref(t *testing.T, got *int, what string) int {
	t.Helper()
	if got == nil {
		t.Fatalf("the page reported no %s, so this proves nothing either way", what)
	}
	return *got
}

// The suggestion rules narrow too. This needs a different account from the
// fixture above: the no-reply rule fires on the newest exchange being something
// WE sent and nobody answered, and the two-engagement fixture's newest exchange
// is a booked meeting, which is the rule declining to chase someone we are
// about to see.
//
// So: one contact, two engagements, an unanswered message on each, and the
// other engagement's the more recent. A scoped page must chase the message it
// is showing and never the one it is not — advice a reader cannot check against
// anything on the page is worse than no advice.
func TestAScopedAccountPageChasesOnlyTheEngagementItShows(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contactID, companyID := ContactIDOf(contact), companyIDOf(company)
	if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}

	project := func(name string) ids.ProjectID {
		p, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
			Name: name, CompanyID: companyID, Source: "manual",
		})
		if err != nil {
			t.Fatalf("create project %q: %v", name, err)
		}
		return projectIDOf(ids.UUID(p.Id))
	}
	// Both are past noReplyDays, so the rule has a candidate either way and the
	// only thing deciding which it names is the scope.
	unanswered := func(subject string, within ids.ProjectID, daysAgo int) string {
		at := roomFixedNow.AddDate(0, 0, -daysAgo)
		logged, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "email", Direction: strPtr("outbound"), Subject: &subject, OccurredAt: &at,
			Links: []activities.ActivityLinkInput{
				{EntityType: "contact", EntityID: contact},
				{EntityType: "company", EntityID: company},
			},
		})
		if err != nil {
			t.Fatalf("log %q: %v", subject, err)
		}
		id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
		if _, err := e.Activities.RelinkActivity(admin, id, activities.RelinkActivityInput{
			EntityType: "project", EntityID: within.UUID,
		}); err != nil {
			t.Fatalf("file %q under its project: %v", subject, err)
		}
		return ids.UUID(logged.Id).String()
	}
	erp, migration := project("ERP rollout"), project("Datacentre migration")
	onERP := unanswered("ERP cutover plan", erp, 20)
	onOther := unanswered("Rack decommissioning", migration, 8)

	svc := companySurfaceService(e)
	scoped, err := svc.AssembleScoped(admin, companyID, company360.AssembleOptions{ProjectID: &erp})
	if err != nil {
		t.Fatalf("assemble scoped: %v", err)
	}
	wide, err := svc.AssembleScoped(admin, companyID, company360.AssembleOptions{})
	if err != nil {
		t.Fatalf("assemble unscoped: %v", err)
	}

	// The unscoped half is not decoration: if no rule fires on the account at
	// all, the scoped assertion below is satisfied by silence.
	if !citesActivity(wide, onOther) {
		t.Fatalf("the unscoped page chases nothing about the datacentre migration's unanswered mail (%s), "+
			"so it cannot show that the scoped page dropped it", onOther)
	}
	if citesActivity(scoped, onOther) {
		t.Errorf("a suggestion on the page scoped to the ERP rollout cites the datacentre migration's "+
			"mail (%s) — the reader cannot see it here, so the advice cannot be checked", onOther)
	}
	if !citesActivity(scoped, onERP) {
		t.Errorf("the ERP page chases nothing, though its own mail (%s) has gone unanswered for longer "+
			"— a scope that silences the rule is dropping the section, not narrowing it", onERP)
	}
}

// citesActivity reports whether any suggestion on the page rests on the given
// activity — as the evidence a reader checks, or as the message the card's
// button would open.
func citesActivity(page crmcontracts.Company360, activityID string) bool {
	if page.Suggestions == nil {
		return false
	}
	for _, s := range *page.Suggestions {
		if s.Action != nil && s.Action.ActivityId != nil && s.Action.ActivityId.String() == activityID {
			return true
		}
		for _, ev := range s.Evidence {
			if ev.EntityId.String() == activityID {
				return true
			}
		}
	}
	return false
}
