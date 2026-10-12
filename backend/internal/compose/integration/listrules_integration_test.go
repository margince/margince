// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Automation rules that watch a Live List. Each test writes contacts through
// the contacts store, runs the real Live List check, and hands the event the
// check put on the outbox to the real workflow engine, the way the worker's
// consumer does.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listRuleFixture is a Live List of Buyers, Rep1 holding the rep role, and
// the engine the worker runs.
type listRuleFixture struct {
	*liveListFixture
	rules  *automation.AutomationStore
	engine *automation.WorkflowEngine
}

func newListRuleFixture(t *testing.T) *listRuleFixture {
	t.Helper()
	f := newLiveListFixture(t)
	f.e.GrantRole(t, f.e.Rep1, "rep")
	return &listRuleFixture{
		liveListFixture: f,
		rules:           automation.NewAutomationStore(f.e.DB()).WithLists(compose.NewListRules(f.e.Pool)),
		engine:          compose.NewWorkflowEngine(f.e.DB()),
	}
}

// author is Rep1 as the seat that writes rules: may author automations, and
// may read and change lists and the contacts on them.
func (f *listRuleFixture) author() context.Context {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		"automation": {Create: true, Read: true, Update: true},
		"list":       {Create: true, Read: true, Update: true},
		"contact":    {Read: true},
		"activity":   {Create: true, Read: true},
	}
	return f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, p)
}

// rule authors and enables one list rule as Rep1.
func (f *listRuleFixture) rule(t *testing.T, key string, params map[string]any) ids.AutomationID {
	t.Helper()
	params["list_id"] = f.list.String()
	created, err := f.rules.Create(f.author(), automation.CreateAutomationInput{Key: key, Name: key, Params: params})
	if err != nil {
		t.Fatalf("author %s: %v", key, err)
	}
	on := true
	if _, err := f.rules.Update(f.author(), created.ID, automation.UpdateAutomationInput{Enabled: &on}); err != nil {
		t.Fatalf("enable %s: %v", key, err)
	}
	return created.ID
}

// deliver hands the newest event of eventType about the list to the engine,
// and answers false when there is none.
func (f *listRuleFixture) deliver(t *testing.T, eventType string) bool {
	t.Helper()
	var raw []byte
	err := f.e.Pool.QueryRow(context.Background(), `SELECT coalesce((SELECT envelope::text FROM event_outbox
		WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2 ORDER BY seq DESC LIMIT 1), '')`,
		eventType, f.list.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		return false
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if err := f.engine.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("the engine refused %s: %v", eventType, err)
	}
	return true
}

func (f *listRuleFixture) runs(t *testing.T, key string) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM workflow_run WHERE handler = $1 AND status = 'applied'`, key)
}

// ruleState reads whether a rule runs and why it paused.
func (f *listRuleFixture) ruleState(t *testing.T, id ids.AutomationID) automation.Automation {
	t.Helper()
	a, err := f.rules.Get(f.author(), id)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (f *listRuleFixture) notices(t *testing.T, needle string) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND (subject || ' ' || body) LIKE $2`,
		f.e.Rep1, "%"+needle+"%")
}

func TestARuleFiresOncePerRecordThatJoinedAndActsForItsOwner(t *testing.T) {
	f := newListRuleFixture(t)
	shortlist := f.shortlist(t)
	f.rule(t, "list_membership_shortlist", map[string]any{"direction": "entered", "shortlist_id": shortlist.String()})
	f.contactTitled(t, "Before Anybody Looked", "Buyer")
	f.check(t)
	if f.deliver(t, "list.evaluated") {
		t.Fatal("the first check announced a change, so the members it found would fire as joining")
	}
	first := f.contactTitled(t, "Ann Buyer", "Buyer")
	second := f.contactTitled(t, "Ben Buyer", "Buyer")
	f.check(t)
	f.deliver(t, "list.evaluated")
	f.deliver(t, "list.evaluated")

	if n := f.runs(t, "list_membership_shortlist"); n != 2 {
		t.Fatalf("two records joined and a redelivered check fired %d runs, want 2", n)
	}
	for _, contact := range []ids.UUID{first, second} {
		if n := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE list_id = $1 AND entity_id = $2
			AND action = 'added' AND reason = 'automation' AND actor = 'system'`, shortlist, contact); n != 1 {
			t.Fatalf("contact %s has %d automation additions to the Shortlist, want 1", contact, n)
		}
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'list' AND entity_id = $1
		AND on_behalf_of = $2`, shortlist, f.e.Rep1); n != 2 {
		t.Fatalf("%d Shortlist changes were recorded on the rule owner's behalf, want 2", n)
	}
}

func TestALeaveRuleFiresOnlyForRecordsThatLeft(t *testing.T) {
	f := newListRuleFixture(t)
	stays := f.contactTitled(t, "Stays", "Buyer")
	goes := f.contactTitled(t, "Goes", "Buyer")
	f.rule(t, "list_membership_task", map[string]any{"direction": "left", "due_in_days": float64(3)})
	f.check(t)
	f.retitle(t, goes, "Partner")
	f.contactTitled(t, "Arrives", "Buyer")
	f.check(t)
	f.deliver(t, "list.evaluated")

	if n := f.runs(t, "list_membership_task"); n != 1 {
		t.Fatalf("one record left and one joined, and a leave rule fired %d runs, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE a.subject = 'Left a Live List' AND l.contact_id = $1`, goes); n != 1 {
		t.Fatalf("the contact that left has %d follow-up tasks, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, stays); n != 0 {
		t.Fatalf("the contact that stayed got %d tasks", n)
	}
}

func TestAChangedFilterFiresNoRule(t *testing.T) {
	f := newListRuleFixture(t)
	f.contactTitled(t, "Old Match", "Buyer")
	f.contactTitled(t, "New Match", "Seller")
	f.rule(t, "list_membership_notify", map[string]any{"direction": "either"})
	f.check(t)
	before, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpdateList(f.e.Admin(), f.list, collections.UpdateListInput{
		Definition: titleIs("Seller"), IfVersion: &before.Version,
	}); err != nil {
		t.Fatal(err)
	}
	f.check(t)
	if !f.deliver(t, "list.evaluated") {
		t.Fatal("the check after the filter changed announced nothing, so this proves nothing")
	}
	if n := f.runs(t, "list_membership_notify"); n != 0 {
		t.Fatalf("the new filter's difference fired %d runs, want none", n)
	}
}

func TestAnEventFromAnOlderFilterFiresNothing(t *testing.T) {
	f := newListRuleFixture(t)
	f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.check(t)
	f.contactTitled(t, "Joined Under The Old Filter", "Buyer")
	f.check(t)
	before, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpdateList(f.e.Admin(), f.list, collections.UpdateListInput{
		Definition: titleIs("Buyer "), IfVersion: &before.Version,
	}); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, "list.evaluated")
	if n := f.runs(t, "list_membership_notify"); n != 0 {
		t.Fatalf("a change seen under the list's previous filter fired %d runs, want none", n)
	}
}

func TestMoreThanAHundredChangesPauseTheRuleAndTellItsOwner(t *testing.T) {
	f := newListRuleFixture(t)
	id := f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.check(t)
	for i := range 101 {
		f.contactTitled(t, fmt.Sprintf("Buyer %03d", i), "Buyer")
	}
	f.check(t)
	f.deliver(t, "list.evaluated")

	if n := f.runs(t, "list_membership_notify"); n != 0 {
		t.Fatalf("a burst of 101 fired %d runs, want none", n)
	}
	state := f.ruleState(t, id)
	if state.Enabled || state.PausedReason == nil || *state.PausedReason != automation.PausedBurst {
		t.Fatalf("after the burst the rule is %+v, want paused for the burst", state)
	}
	if n := f.notices(t, "moved 101 records"); n != 1 {
		t.Fatalf("the owner got %d notices naming the burst, want 1", n)
	}
	on := true
	resumed, err := f.rules.Update(f.author(), id, automation.UpdateAutomationInput{Enabled: &on})
	if err != nil || !resumed.Enabled || resumed.PausedReason != nil {
		t.Fatalf("resuming = %+v (%v), want running with the reason cleared", resumed, err)
	}
}

func TestExactlyAHundredChangesStillFire(t *testing.T) {
	f := newListRuleFixture(t)
	f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.check(t)
	for i := range 100 {
		f.contactTitled(t, fmt.Sprintf("Buyer %03d", i), "Buyer")
	}
	f.check(t)
	f.deliver(t, "list.evaluated")
	if n := f.runs(t, "list_membership_notify"); n != 100 {
		t.Fatalf("a check that moved exactly 100 records fired %d runs, want 100", n)
	}
}

func TestArchivingTheWatchedListPausesTheRuleAndRestoringDoesNotResumeIt(t *testing.T) {
	f := newListRuleFixture(t)
	id := f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	if _, err := f.store.SetArchivedView(f.e.Admin(), f.list, true); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, "list.archived")
	if state := f.ruleState(t, id); state.Enabled || state.PausedReason == nil || *state.PausedReason != automation.PausedListArchived {
		t.Fatalf("after the archive the rule is %+v, want paused because its list was archived", state)
	}
	if n := f.notices(t, "was archived"); n != 1 {
		t.Fatalf("the owner got %d notices about the archive, want 1", n)
	}
	on := true
	if _, err := f.rules.Update(f.author(), id, automation.UpdateAutomationInput{Enabled: &on}); err == nil {
		t.Fatal("the rule resumed onto an archived list")
	}
	if _, err := f.store.SetArchivedView(f.e.Admin(), f.list, false); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, "list.restored")
	if state := f.ruleState(t, id); state.Enabled {
		t.Fatal("restoring the list resumed the rule")
	}
}

func TestABrokenFilterPausesTheRule(t *testing.T) {
	f := newListRuleFixture(t)
	id := f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.e.WsExec(t, `UPDATE list SET definition = '{"field": "cf_gone", "op": "eq", "value": "x"}' WHERE id = $1`, f.list)
	f.check(t)
	if state := f.ruleState(t, id); state.Enabled || state.PausedReason == nil || *state.PausedReason != automation.PausedListInvalid {
		t.Fatalf("after the broken check the rule is %+v, want paused because its filter broke", state)
	}
}

func TestARecordOutsideTheOwnersScopeIsSkipped(t *testing.T) {
	f := newListRuleFixture(t)
	f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.check(t)
	seen := f.contactTitled(t, "Open Buyer", "Buyer")
	hidden := f.contactTitled(t, "Private Buyer", "Buyer")
	f.e.MakeCapturePrivate(t, "contact", hidden, f.e.Rep3)
	f.check(t)
	f.deliver(t, "list.evaluated")

	if n := f.runs(t, "list_membership_notify"); n != 1 {
		t.Fatalf("one visible and one hidden record joined, and the rule fired %d runs, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND target_id = $2`, f.e.Rep1, seen); n != 1 {
		t.Fatalf("the owner got %d notices about the record they can see, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM notice WHERE target_id = $1`, hidden); n != 0 {
		t.Fatalf("the owner was told about a record outside their scope %d times", n)
	}
}

// shortlist is a workspace Shortlist of contacts Rep1 looks after.
func (f *listRuleFixture) shortlist(t *testing.T) ids.ListID {
	t.Helper()
	l, err := f.store.CreateList(f.author(), collections.CreateListInput{
		Name: "Follow-ups", EntityType: "contact", ListType: "static", Sharing: "workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	return l.ID
}

func TestAddingARecordAlreadyOnTheShortlistChangesNothing(t *testing.T) {
	f := newListRuleFixture(t)
	shortlist := f.shortlist(t)
	f.rule(t, "list_membership_shortlist", map[string]any{"direction": "entered", "shortlist_id": shortlist.String()})
	f.check(t)
	contact := f.contactTitled(t, "Already Chosen", "Buyer")
	if _, err := f.store.AddMember(f.author(), shortlist, collections.MemberChange{
		EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen,
	}); err != nil {
		t.Fatal(err)
	}
	f.check(t)
	f.deliver(t, "list.evaluated")
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE list_id = $1 AND entity_id = $2`, shortlist, contact); n != 1 {
		t.Fatalf("the Shortlist records %d changes for a record already on it, want only the first", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM workflow_run WHERE handler = 'list_membership_shortlist'
		AND applied::text LIKE '%deduplicated%'`); n != 1 {
		t.Fatalf("%d runs say they found the record already there, want 1", n)
	}
}

func TestTheListNamesTheRulesThatDependOnIt(t *testing.T) {
	f := newListRuleFixture(t)
	shortlist := f.shortlist(t)
	f.rule(t, "list_membership_shortlist", map[string]any{"direction": "entered", "shortlist_id": shortlist.String()})
	watched, err := f.store.ListView(f.author(), f.list)
	if err != nil || watched.Dependencies == nil {
		t.Fatalf("the watched list's dependencies = %v (%v)", watched.Dependencies, err)
	}
	deps := *watched.Dependencies
	if len(deps) != 1 || deps[0].Kind != "automation" || deps[0].Role == nil || *deps[0].Role != "watches" ||
		deps[0].AutomationName == nil || *deps[0].AutomationName != "list_membership_shortlist" {
		t.Fatalf("the watched list's dependencies = %+v, want the rule watching it", deps)
	}
	written, err := f.store.ListView(f.author(), shortlist)
	if err != nil || written.Dependencies == nil || len(*written.Dependencies) != 1 || *(*written.Dependencies)[0].Role != "writes" {
		t.Fatalf("the Shortlist's dependencies = %+v (%v), want the rule adding to it", written.Dependencies, err)
	}
	unnamed, err := f.store.ListView(f.rep1(), f.list)
	if err != nil || unnamed.Dependencies == nil || len(*unnamed.Dependencies) != 1 || (*unnamed.Dependencies)[0].AutomationName != nil {
		t.Fatalf("a reader who may not read automations saw %+v (%v), want the dependency without its name", unnamed.Dependencies, err)
	}
}

func TestTheWhatChangedSummaryNamesOnlyRecordsTheReaderCanSee(t *testing.T) {
	f := newListRuleFixture(t)
	leaves := f.contactTitled(t, "Leaving Buyer", "Buyer")
	f.check(t)
	f.visitedAnHourAgo(f.outsider(), t, f.e.Rep3)
	joins := f.contactTitled(t, "Visible Buyer", "Buyer")
	private := f.contactTitled(t, "Private Buyer", "Buyer")
	f.e.MakeCapturePrivate(t, "contact", private, f.e.Rep1)
	f.retitle(t, leaves, "Seller")
	f.check(t)

	view, err := f.store.ListView(f.outsider(), f.list)
	if err != nil || view.ChangesSinceVisit == nil {
		t.Fatalf("the outsider's summary = %+v (%v), want one", view.ChangesSinceVisit, err)
	}
	summary := view.ChangesSinceVisit
	if summary.Joined.Count != 1 || len(summary.Joined.Records) != 1 || ids.UUID(summary.Joined.Records[0].EntityId) != joins ||
		summary.Joined.Records[0].Name == nil || *summary.Joined.Records[0].Name != "Visible Buyer" {
		t.Fatalf("joined = %+v, want only the record the reader can see, by name", summary.Joined)
	}
	if summary.Left.Count != 1 || ids.UUID(summary.Left.Records[0].EntityId) != leaves || summary.FilterChanges != 0 {
		t.Fatalf("left = %+v, filter changes = %d, want the one that left and no filter change", summary.Left, summary.FilterChanges)
	}
}

func TestAFollowUpTaskNamesNoPrivateList(t *testing.T) {
	f := newListRuleFixture(t)
	private, err := f.store.CreateList(f.author(), collections.CreateListInput{
		Name: "Rep1 secret targets", EntityType: "contact", ListType: "dynamic", Sharing: "private",
		Definition: titleIs("Buyer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.list = private.ID
	f.rule(t, "list_membership_task", map[string]any{"direction": "entered"})
	f.check(t)
	joins := f.contactTitled(t, "Quiet Buyer", "Buyer")
	f.check(t)
	f.deliver(t, "list.evaluated")

	if n := f.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.contact_id = $1 AND a.subject = 'Joined a Live List'`, joins); n != 1 {
		t.Fatalf("the contact that joined has %d neutral follow-up tasks, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE subject LIKE '%secret targets%' OR body LIKE '%secret targets%'`); n != 0 {
		t.Fatalf("%d activities name the private list, which anybody who can read the contact would see", n)
	}
}

func TestAnArchiveNewsOlderThanARestoreAndResumePausesNothing(t *testing.T) {
	f := newListRuleFixture(t)
	id := f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	if _, err := f.store.SetArchivedView(f.e.Admin(), f.list, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SetArchivedView(f.e.Admin(), f.list, false); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, "list.archived")
	if state := f.ruleState(t, id); !state.Enabled {
		t.Fatalf("a late archive event paused a rule whose list is live again: %+v", state)
	}
}

func TestAReaderWithoutTheListGrantReadsNoObservedChange(t *testing.T) {
	f := newListRuleFixture(t)
	f.check(t)
	f.contactTitled(t, "Seen Buyer", "Buyer")
	f.check(t)
	view, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	actions := []string{"entered"}
	if changes, _, err := f.store.ObservedChanges(f.rep1(), f.list, view.Version, f.clock, actions, 10); err != nil || len(changes) != 1 {
		t.Fatalf("a reader holding list read got %v (%v), want the one change", changes, err)
	}
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}}
	noList := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, p)
	if changes, _, err := f.store.ObservedChanges(noList, f.list, view.Version, f.clock, actions, 10); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without list read got %v (%v), want a refusal", changes, err)
	}
}

func TestTheSummaryWaitsForAVisitAndCountsFilterChanges(t *testing.T) {
	f := newListRuleFixture(t)
	f.check(t)
	summary := func() *crmcontracts.ListChangeSummary {
		t.Helper()
		view, err := f.store.ListView(f.outsider(), f.list)
		if err != nil {
			t.Fatal(err)
		}
		return view.ChangesSinceVisit
	}
	if got := summary(); got != nil {
		t.Fatalf("a reader who never visited got a summary %+v", got)
	}
	f.visitedAnHourAgo(f.outsider(), t, f.e.Rep3)
	if got := summary(); got == nil || got.Joined.Count+got.Left.Count+got.FilterChanges != 0 {
		t.Fatalf("with nothing changed since the visit the summary is %+v, want one saying nothing moved", got)
	}
	before, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpdateList(f.e.Admin(), f.list, collections.UpdateListInput{
		Definition: titleIs("Seller"), IfVersion: &before.Version,
	}); err != nil {
		t.Fatal(err)
	}
	renamed := "Buyers (renamed)"
	after, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpdateList(f.e.Admin(), f.list, collections.UpdateListInput{Name: &renamed, IfVersion: &after.Version}); err != nil {
		t.Fatal(err)
	}
	if got := summary(); got == nil || got.FilterChanges != 1 {
		t.Fatalf("a filter change and a rename since the visit read as %+v, want exactly one filter change", got)
	}
	shortlist, err := f.store.ListView(f.author(), f.shortlist(t))
	if err != nil || shortlist.ChangesSinceVisit != nil {
		t.Fatalf("a Shortlist read carries a summary %+v (%v)", shortlist.ChangesSinceVisit, err)
	}
}

func TestANoticeRuleTellsItsOwnerWhichListARecordJoined(t *testing.T) {
	f := newListRuleFixture(t)
	f.rule(t, "list_membership_notify", map[string]any{"direction": "entered"})
	f.check(t)
	joins := f.contactTitled(t, "Noted Buyer", "Buyer")
	f.check(t)
	f.deliver(t, "list.evaluated")
	if n := f.e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND target_id = $2
		AND subject = 'Buyers' AND body = 'A record joined the list.'`, f.e.Rep1, joins); n != 1 {
		t.Fatalf("the owner has %d notices naming the list the record joined, want 1", n)
	}
}
