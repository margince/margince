// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The project record over real rows: the key rules the schema enforces,
// the phase transition that must write its history in the same
// transaction, the cross-company deal pointer the constraint trigger
// refuses, the archive that ends a grouping without ending what it
// grouped — and the row-scope case that would otherwise fail silently.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedProject creates a project on a fresh company, owned by the given
// user (nil = the creating admin, since a project defaults to its creator).
type projectFixture struct {
	ID      ids.ProjectID
	Version int64
}

func seedProject(ctx context.Context, t *testing.T, e *Env, name string, org ids.UUID, owner *ids.UUID) projectFixture {
	t.Helper()
	in := projects.CreateProjectInput{
		Name:           name,
		OrganizationID: orgIDOf(org),
		OwnerID:        userIDPtr(owner),
		Source:         "manual",
	}
	p, err := e.Projects.CreateProject(ctx, in)
	if err != nil {
		t.Fatalf("create project %q: %v", name, err)
	}
	return projectFixture{ID: projectIDOf(ids.UUID(p.Id)), Version: *p.Version}
}

// mintedKey reads back the key the server chose for a project. A test that
// needs the key can no longer state one: it asks the project which key it got.
func mintedKey(ctx context.Context, t *testing.T, e *Env, id ids.ProjectID) string {
	t.Helper()
	got, err := e.Projects.GetProject(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the project back for its key: %v", err)
	}
	if got.Key == nil {
		t.Fatal("the project carries no key; the server mints one for every project")
	}
	return *got.Key
}

// A project is born at the head of the ladder with its history already
// complete: the creation row is what makes "how did it get here"
// answerable from the very first read.
func TestProjectIsBornWithItsHistoryRow(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)

	got, err := e.Projects.GetProject(e.Admin(), p.ID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase == nil || string(*got.Phase) != projects.PhaseInitiative {
		t.Errorf("phase = %v, want %s", got.Phase, projects.PhaseInitiative)
	}
	if n := e.WsCount(t,
		`SELECT count(*) FROM project_phase_history WHERE project_id = $1 AND from_phase IS NULL AND to_phase = 'initiative'`,
		p.ID); n != 1 {
		t.Errorf("creation history rows = %d, want exactly 1", n)
	}
}

// The key is MINTED, not supplied: a caller cannot name one, so a collision is
// the server's race to resolve rather than a refusal to report. Two projects
// whose names give the same stem get different numbers, and the shape the
// column demands holds for both.
func TestProjectKeysAreMintedUniquePerStem(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)

	first := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)
	second := seedProject(e.Admin(), t, e, "ERP rollout", org, nil)

	keyOf := func(p projectFixture) string {
		t.Helper()
		got, err := e.Projects.GetProject(e.Admin(), p.ID, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if got.Key == nil {
			t.Fatal("a created project carries no key; the server mints one for every project")
		}
		return *got.Key
	}
	firstKey, secondKey := keyOf(first), keyOf(second)
	if firstKey == secondKey {
		t.Fatalf("both projects were minted the key %q; the number is what separates them", firstKey)
	}
	for _, key := range []string{firstKey, secondKey} {
		if !strings.HasPrefix(key, "ER-") {
			t.Errorf("minted key %q does not carry the stem its name gives", key)
		}
	}

	// Archiving frees the key for reuse: the uniqueness index is partial on
	// live rows, so the next project with this stem may take the number back.
	if _, err := e.Projects.ArchiveProject(e.Admin(), first.ID, nil); err != nil {
		t.Fatal(err)
	}
	third := seedProject(e.Admin(), t, e, "ERP restart", org, nil)
	if k := keyOf(third); k == secondKey {
		t.Errorf("the new project took %q, which a LIVE project still holds", k)
	}
}

// A phase move writes the row change and its history row from ONE
// transaction, and announces itself as a phase change rather than a
// generic update.
func TestAdvanceProjectPhaseWritesHistoryAndTheFirstClassEvent(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)

	moved, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{ToPhase: "delivering"})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Phase == nil || string(*moved.Phase) != "delivering" {
		t.Errorf("phase = %v, want delivering", moved.Phase)
	}
	if n := e.WsCount(t,
		`SELECT count(*) FROM project_phase_history WHERE project_id = $1 AND from_phase = 'initiative' AND to_phase = 'delivering'`,
		p.ID); n != 1 {
		t.Errorf("transition history rows = %d, want exactly 1", n)
	}
	if n := e.WsCount(t,
		`SELECT count(*) FROM event_outbox
		  WHERE envelope->>'type' = 'project.phase_changed' AND envelope->'entity'->>'id' = $1::text`,
		p.ID); n != 1 {
		t.Errorf("project.phase_changed events = %d, want exactly 1", n)
	}
	if n := e.WsCount(t,
		`SELECT count(*) FROM event_outbox
		  WHERE envelope->>'type' = 'project.updated' AND envelope->'entity'->>'id' = $1::text`,
		p.ID); n != 0 {
		t.Errorf("project.updated events = %d, want 0 — a phase move is not a diff", n)
	}
}

// Closing is a claim that the work ended; an unexplained claim is not
// answerable later, so it is refused.
func TestClosingAProjectRequiresAReason(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)

	_, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{ToPhase: projects.PhaseClosed})
	var needsReason *projects.ClosedReasonRequiredError
	if !errors.As(err, &needsReason) {
		t.Fatalf("closing without a reason produced %v, want ClosedReasonRequiredError", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM project_phase_history WHERE project_id = $1`, p.ID); n != 1 {
		t.Errorf("history rows = %d, want only the creation row — a refused move records nothing", n)
	}

	// Re-opening clears the closed reason: a live project must not carry
	// the explanation of a close that no longer applies.
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhaseClosed, Reason: strPtr("Delivered."),
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{ToPhase: "delivering"})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ClosedReason != nil {
		t.Errorf("closed_reason = %v after re-opening, want nil", *reopened.ClosedReason)
	}
}

// A deal and the project it belongs to must name the same company. The
// rule spans two rows, so it lives in a constraint trigger — and it must
// surface as a named 422, never as an opaque server fault.
func TestADealCannotPointAtAnotherCompanysProject(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	orgA := e.SeedOrg(t, "BAER Pharma", nil)
	orgB := e.SeedOrg(t, "Kessler GmbH", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", orgA, nil)

	orgBID := orgIDOf(orgB)
	_, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Wrong company", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgBID, ProjectID: &p.ID, Source: "manual",
	})
	var mismatch *deals.DealProjectOrgMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("a cross-company pointer produced %v, want DealProjectOrgMismatchError", err)
	}

	orgAID := orgIDOf(orgA)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Right company", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgAID, ProjectID: &p.ID, Source: "manual",
	}); err != nil {
		t.Fatalf("a same-company pointer was refused: %v", err)
	}
}

// Archiving a project ends the grouping, not what it grouped: the
// conversations and the deals survive, and so does the phase history that
// explains where the work got to.
func TestArchivingAProjectKeepsWhatItGrouped(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)

	orgID := orgIDOf(org)
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Phase one", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, ProjectID: &p.ID, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: strPtr("[ERP-27] kickoff"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: p.ID.UUID}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.Projects.ArchiveProject(e.Admin(), p.ID, nil); err != nil {
		t.Fatal(err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NULL`, ids.UUID(d.Id)); n != 1 {
		t.Error("archiving the project archived its deal — the grouping dies, the deal does not")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, ids.UUID(act.Id)); n != 1 {
		t.Error("archiving the project archived its conversation")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM project_phase_history WHERE project_id = $1`, p.ID); n == 0 {
		t.Error("archiving the project erased its phase history — the history is what survives")
	}
}

// The case that fails SILENTLY if the activity link-walk has no project
// branch: an activity linked ONLY to a project is reached through that
// project, so a seat that reads the project reads its conversation too.
// Getting this half-right looks exactly like missing data.
//
// A project is workspace-readable (platform/auth tableclass.go), so the walk
// admits every seat here rather than only the owner's team. What the walk
// still narrows is a link onto a capture-private record, which
// TestAMeetingNeverDisclosesTheRecordBehindALinkTheCallerCannotSee covers.
func TestAnActivityLinkedOnlyToAProjectIsReachedThroughIt(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	// Real seeded users: owner_id is a composite FK to app_user, so a
	// synthetic uuid would be refused before the scope rule is exercised.
	owner := e.Rep1
	colleague := e.Rep3
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, &owner)

	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: strPtr("rollout schedule"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: p.ID.UUID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID := ids.From[ids.ActivityKind](ids.UUID(act.Id))

	scoped := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"project":  {Read: true},
			"activity": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	}
	// The project's owner sees the conversation about their project.
	if _, err := e.Activities.GetActivity(e.As(owner, nil, scoped), activityID, storekit.LiveOnly); err != nil {
		t.Errorf("the project's owner cannot see its activity: %v", err)
	}
	// So does a colleague who neither owns it nor was granted it: a project
	// is the workspace's to read, and its timeline travels with it.
	if _, err := e.Activities.GetActivity(e.As(colleague, nil, scoped), activityID, storekit.LiveOnly); err != nil {
		t.Errorf("a colleague on the project's timeline got %v; a project is read by every seat "+
			"holding the object grant, and the conversation about it is reached through it", err)
	}
	// A seat holding no ACTIVITY grant is refused the record type outright —
	// the link walk decides which rows, never whether the caller may read
	// activities at all.
	noActivity := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeOwn,
	}
	_, err = e.Activities.GetActivity(e.As(colleague, nil, noActivity), activityID, storekit.LiveOnly)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat with no activity grant got %v, want ErrPermissionDenied", err)
	}
}

// A rep who is neither owner nor stakeholder reads the project and its
// timeline whole, and still cannot change it. The read class and the write
// arm are separate rules (platform/auth tableclass.go vs writescope.go), and
// widening the first must not move the second: a consultant seeing the work
// is not a consultant who may rewrite it.
func TestARepReadsAProjectTheyDoNotOwnButCannotWriteIt(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	owner := e.Rep1
	// Rep3 sits in the other team, so neither own nor team scope reaches the
	// project — only the read class can.
	consultant := e.Rep3
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, &owner)

	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: strPtr("kickoff notes"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: p.ID.UUID}},
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := e.As(consultant, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"project":  {Read: true, Update: true},
			"activity": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	got, err := e.Projects.GetProject(reader, p.ID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("a rep working a project they do not own cannot read it: %v", err)
	}
	if ids.UUID(got.Id) != p.ID.UUID {
		t.Fatalf("read back project %s, want %s", ids.UUID(got.Id), p.ID.UUID)
	}

	// The timeline travels with the record it hangs off.
	entityType, entityID := "project", p.ID.UUID
	timeline, _, err := e.Activities.ListActivities(reader, activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &entityID,
	})
	if err != nil {
		t.Fatalf("reading the project's timeline: %v", err)
	}
	if len(timeline) != 1 || ids.UUID(timeline[0].Id) != ids.UUID(act.Id) {
		t.Fatalf("the project timeline returned %d activities, want the one linked to it", len(timeline))
	}

	// The object grant says update; the ROW is still not theirs to change.
	// The refusal is permission-denied rather than not-found: this caller
	// legitimately reads the row, so there is no existence left to hide and
	// saying "not there" about a record they can see would be a lie.
	renamed := "Renamed by a passer-by"
	if _, err := e.Projects.UpdateProject(reader, p.ID, projects.UpdateProjectInput{Name: &renamed}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep who does not own the project changed its name → %v, want ErrPermissionDenied — "+
			"reading a project is not permission to rewrite it", err)
	}
	// And the row really is untouched, so the refusal was not a rollback of a
	// write that had already landed.
	after, err := e.Projects.GetProject(e.Admin(), p.ID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name == renamed {
		t.Fatal("the refused update still landed on the row")
	}
}

// A119 Amendment 1.A: a project is born in `initiative`, before any deal
// exists, and the object carrying interest at that stage is the lead.
func TestALeadCanBelongToAProject(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", org, nil)

	lead, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: strPtr("Anna Weber"), Source: "manual", ProjectID: &p.ID,
	})
	if err != nil {
		t.Fatalf("create lead on a project: %v", err)
	}
	if lead.ProjectId == nil || ids.UUID(*lead.ProjectId) != p.ID.UUID {
		t.Errorf("lead project = %v, want %v", lead.ProjectId, p.ID.UUID)
	}

	// PROJ-LIFE-2: a closed project still accepts work. Nothing about the
	// phase gates an attachment — only the auto-link ladder consults it.
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhaseClosed, Reason: strPtr("Delivered."),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: strPtr("Late enquiry"), Source: "manual", ProjectID: &p.ID,
	}); err != nil {
		t.Errorf("a closed project refused a new lead: %v — phase is advisory, not a gate", err)
	}
}

// Filters are registered by the caller AFTER the shared list prelude is
// built, so every one of them must land in the same argument list the query
// is executed with. A prelude passed by value silently drops them and the
// query goes out short of placeholders — which fails as an opaque driver
// error, not as a wrong result.
func TestListProjectsAppliesFiltersRegisteredAfterThePrelude(t *testing.T) {
	e := Setup(t)
	wanted := e.SeedOrg(t, "BAER Pharma", nil)
	other := e.SeedOrg(t, "Kessler GmbH", nil)
	erp := seedProject(e.Admin(), t, e, "ERP replacement", wanted, nil)
	seedProject(e.Admin(), t, e, "Rollout A", other, nil)

	orgID := orgIDOf(wanted)
	byOrg, _, err := e.Projects.ListProjects(e.Admin(), projects.ListProjectsInput{OrganizationID: &orgID})
	if err != nil {
		t.Fatalf("list by organization: %v", err)
	}
	if len(byOrg) != 1 || byOrg[0].Name != "ERP replacement" {
		t.Errorf("organization filter returned %d rows, want only the anchored one", len(byOrg))
	}

	// Two filters plus a quick-find: three arguments registered after the
	// prelude, which is where the value-copy bug showed up.
	phase, query := projects.PhaseInitiative, "ERP"
	found, _, err := e.Projects.ListProjects(e.Admin(), projects.ListProjectsInput{
		OrganizationID: &orgID, Phase: &phase, Query: &query,
	})
	if err != nil {
		t.Fatalf("list by organization+phase+q: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("combined filters returned %d rows, want 1", len(found))
	}

	// And the key lookup, matched case-insensitively like its index. The key is
	// minted, so the filter is fed the one the server chose — lower-cased, which
	// is what proves the match ignores case.
	minted, err := e.Projects.GetProject(e.Admin(), erp.ID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if minted.Key == nil {
		t.Fatal("the project carries no key; the server mints one for every project")
	}
	key := strings.ToLower(*minted.Key)
	byKey, _, err := e.Projects.ListProjects(e.Admin(), projects.ListProjectsInput{Key: &key})
	if err != nil {
		t.Fatalf("list by key: %v", err)
	}
	if len(byKey) != 1 {
		t.Errorf("key lookup returned %d rows, want 1", len(byKey))
	}
}

// An activity's visibility DERIVES from its links, so replacing one is not a
// harmless association edit: cut the link a team sees the activity through
// and the activity leaves their world. Relink therefore replaces only what
// the caller can see.
//
// Scoping rather than refusing is deliberate — a refusal would confirm that
// an invisible link exists, which is precisely what the scope withholds.
//
// Organization links carry the test because several are allowed per activity:
// the replacement INSERT succeeds, so whatever the delete removed stays
// removed. On a one-per-activity type the insert refuses and the whole
// transaction rolls back, which would hide the difference this test exists to
// show. And an organization can be capture-private, which is what keeps a
// link invisible to a colleague — a deal is readable by every seat with the
// grant, so it can no longer stand in for a hidden link.
func TestRelinkReplacesOnlyTheLinksTheCallerCanSee(t *testing.T) {
	e := Setup(t)
	theirs := e.SeedOrg(t, "Their private account", &e.Rep1)
	mine := e.SeedOrg(t, "My account", &e.Rep3)
	person := e.SeedPerson(t, "Shared Contact", &e.Rep3)

	// One activity linked to the other rep's private account and to a person
	// the attacker owns. The person link is how they reach the activity at all.
	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "organization", EntityID: theirs},
			{EntityType: "person", EntityID: person},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Made private once the link exists: the seeding admin is not the captor
	// and could not link to a private account.
	e.MakeCapturePrivate(t, "organization", theirs, e.Rep1)

	attacker := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"activity":              {Read: true, Update: true},
			"organization":          {Read: true},
			"person":                {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	if _, err := e.Activities.RelinkActivity(attacker, ids.From[ids.ActivityKind](ids.UUID(act.Id)),
		activities.RelinkActivityInput{
			EntityType: "organization", EntityID: mine, ReplaceExistingOfType: true,
		}); err != nil {
		t.Fatalf("relinking to an account the caller owns: %v", err)
	}

	// Their own link landed — so the write really happened and the delete
	// really ran, which is what makes the next assertion mean something.
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND entity_type = 'organization' AND organization_id = $2`,
		ids.UUID(act.Id), mine); n != 1 {
		t.Fatalf("the caller's own relink did not land (%d links)", n)
	}
	// And the link they could never see is untouched.
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND entity_type = 'organization' AND organization_id = $2`,
		ids.UUID(act.Id), theirs); n != 1 {
		t.Fatalf("the private account's link was removed by a caller who could not see it (%d remain)", n)
	}
}

// Moving an activity from one project to another is a plain success now, and
// that is the point worth pinning.
//
// The scoped delete in relink only removes links the caller can SEE, and at
// most one project link may exist per activity. While a project was row-
// scoped those two rules collided: replacing a project link the caller could
// not see refused where replacing nothing succeeded, and the difference told
// them a hidden link was there. One bit escaped, and it could not be closed
// from the relink path — hiding the link and enforcing one-per-activity were
// the same question asked twice.
//
// Making a project workspace-readable (platform/auth tableclass.go) closed it
// by removing the premise: there is no project link a seat holding the object
// grant cannot see, so the delete reaches every one of them and the move
// simply works.
//
// The closure rests on BOTH halves of "a project is never invisible": no
// own/team arm, and no capture privacy — migration 1787320003 narrowed the
// visibility CHECK to 'workspace', so an owner-private project is not a state
// the database can hold. Widen either and the oracle returns, which is why
// TestEveryTableThatCanHoldAnOwnerRowIsOwnerPrivate guards the second half.
// This case is the witness that the oracle is gone.
func TestMovingAnActivityBetweenProjectsReplacesTheLinkItCannotSee(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Oracle GmbH", nil)
	theirs := seedProject(e.Admin(), t, e, "Their delivery", org, &e.Rep1)
	ours := seedProject(e.Admin(), t, e, "Our pursuit", org, &e.Rep3)
	person := e.SeedPerson(t, "Reachable Contact", &e.Rep3)

	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "project", EntityID: theirs.ID.UUID},
			{EntityType: "person", EntityID: person},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Rep3 owns neither the activity's current project nor sits in Rep1's
	// team: only the read class reaches that link.
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true},
			"project":  {Read: true},
			"person":   {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	if _, err := e.Activities.RelinkActivity(outsider, ids.From[ids.ActivityKind](ids.UUID(act.Id)),
		activities.RelinkActivityInput{
			EntityType: "project", EntityID: ours.ID.UUID, ReplaceExistingOfType: true,
		}); err != nil {
		t.Fatalf("moving the activity onto another project: %v — the replace could not reach the "+
			"link it had to delete, which is the 23505 the one-project index raises", err)
	}
	// The move really happened: the old link is gone and the new one is there.
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND entity_type = 'project' AND project_id = $2`,
		ids.UUID(act.Id), theirs.ID.UUID); n != 0 {
		t.Errorf("the displaced project link survived (%d remain)", n)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity_link
		WHERE activity_id = $1 AND entity_type = 'project' AND project_id = $2`,
		ids.UUID(act.Id), ours.ID.UUID); n != 1 {
		t.Errorf("the new project link did not land (%d rows)", n)
	}
	// And exactly one project link remains, which is what the index is for.
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'project'`,
		ids.UUID(act.Id)); n != 1 {
		t.Errorf("project links = %d, want exactly 1", n)
	}
}

// The timeline filter and the timeline write must speak one vocabulary. They
// did not: writes accepted every link target while the filter knew only
// three, so an activity could be linked to a lead or a project and then be
// unfindable by the very link that had just been written.
func TestTheTimelineFilterKnowsEveryLinkTargetTheWriteAccepts(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Vocabulary GmbH", nil)
	project := seedProject(e.Admin(), t, e, "Findable work", org, nil)
	person := e.SeedPerson(t, "Findable Contact", nil)

	act, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "project", EntityID: project.ID.UUID},
			{EntityType: "person", EntityID: person},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Every kind the write took must answer a filter on that same kind.
	for kind, id := range map[string]ids.UUID{
		"project": project.ID.UUID,
		"person":  person,
	} {
		t.Run(kind, func(t *testing.T) {
			entityType := kind
			entityID := id
			found, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{
				EntityType: &entityType, EntityID: &entityID,
			})
			if err != nil {
				t.Fatalf("filtering the timeline by %s: %v", kind, err)
			}
			if len(found) != 1 || ids.UUID(found[0].Id) != ids.UUID(act.Id) {
				t.Fatalf("filtering by %s returned %d activities, want the one linked to it", kind, len(found))
			}
		})
	}
}

// The minted number is the LOWEST free one for its stem, and "free" is decided
// the way the uniqueness index decides it: case-insensitively, over live rows
// only. Both halves have bitten before — a case-sensitive read hands back a
// number the index then refuses, and max+1 leaves a permanent hole where an
// archived project gave its number back.
func TestTheMintedNumberIsTheLowestFreeOneForItsStem(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Stemmed GmbH", nil)

	first := seedProject(e.Admin(), t, e, "Warehouse rollout", org, nil)
	second := seedProject(e.Admin(), t, e, "Warehouse refresh", org, nil)
	if k := mintedKey(e.Admin(), t, e, first.ID); k != "WR-1" {
		t.Fatalf("first minted key = %q, want WR-1", k)
	}
	if k := mintedKey(e.Admin(), t, e, second.ID); k != "WR-2" {
		t.Fatalf("second minted key = %q, want WR-2", k)
	}

	// Archiving frees WR-1, so the next project with this stem takes it back
	// rather than starting above the highest number ever used.
	if _, err := e.Projects.ArchiveProject(e.Admin(), first.ID, nil); err != nil {
		t.Fatal(err)
	}
	third := seedProject(e.Admin(), t, e, "Warehouse rebuild", org, nil)
	if k := mintedKey(e.Admin(), t, e, third.ID); k != "WR-1" {
		t.Errorf("after archiving WR-1 the next project minted %q, want WR-1 reused", k)
	}

	// A key spelled in lower case still blocks its number: uq_project_key
	// indexes lower(key), so a case-sensitive read would mint a key the index
	// refuses and turn a create into a 500.
	e.WsExec(t, `UPDATE project SET key = 'wr-9' WHERE id = $1`, second.ID)
	fourth := seedProject(e.Admin(), t, e, "Warehouse revamp", org, nil)
	if k := mintedKey(e.Admin(), t, e, fourth.ID); strings.EqualFold(k, "wr-9") {
		t.Errorf("minted %q over a live lower-cased key; the index would refuse it", k)
	}
}

// A project created without a requested owner belongs to its creator — the
// same birth default person, organization and deal already apply. The
// stake is the New-deal form's "New project…" flow: write authority reads an
// unowned row as nobody's to change, so an ownerless project could never be
// attached to a deal by the very rep who had just created it.
func TestAProjectCreatedWithoutAnOwnerBelongsToItsCreator(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	org := e.SeedOrg(t, "BAER Pharma", &e.Rep1)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"project":      {Create: true, Read: true, Update: true},
			"deal":         {Create: true, Read: true, Update: true},
			"organization": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})

	p, err := e.Projects.CreateProject(rep, projects.CreateProjectInput{
		Name: "ERP rollout", OrganizationID: orgIDOf(org), Source: "manual",
	})
	if err != nil {
		t.Fatalf("a rep creating a project on their own company: %v", err)
	}
	if p.OwnerId == nil || ids.UUID(*p.OwnerId) != e.Rep1 {
		t.Fatalf("owner = %v, want the creating rep %s", p.OwnerId, e.Rep1)
	}

	// The flow that exposed the gap: the same rep immediately binds a new
	// deal to the project they just created.
	orgID := orgIDOf(org)
	projID := projectIDOf(ids.UUID(p.Id))
	if _, err := e.Deals.CreateDeal(rep, deals.CreateDealInput{
		Name: "ERP rollout deal", PipelineID: pipeline, StageID: open,
		OrganizationID: &orgID, ProjectID: &projID, Source: "manual",
	}); err != nil {
		t.Fatalf("the creating rep attaching their new project to a new deal: %v", err)
	}
}
