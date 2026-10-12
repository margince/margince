// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// scriptedLists answers from fixed lists and fixed refusals, and records the
// Shortlist additions it was asked for.
type scriptedLists struct {
	lists     map[ids.UUID]ListRef
	changes   []ListChange
	total     int
	observed  error
	shortlist error
	already   bool
	added     *[]datasource.EntityRef
}

func (s scriptedLists) Find(_ context.Context, id ids.UUID) (ListRef, error) {
	l, ok := s.lists[id]
	if !ok {
		return ListRef{}, apperrors.ErrNotFound
	}
	return l, nil
}

func (s scriptedLists) ObservedChanges(context.Context, ids.UUID, int64, time.Time, []string, int) ([]ListChange, int, error) {
	return s.changes, s.total, s.observed
}

func (s scriptedLists) CheckShortlist(context.Context, ids.UUID, string) error { return s.shortlist }

func (s scriptedLists) AddMember(_ context.Context, _ context.Context, _ ids.UUID, record datasource.EntityRef) (bool, error) {
	if s.added != nil {
		*s.added = append(*s.added, record)
	}
	return !s.already, nil
}

// listFixture is one Live List and one Shortlist of the same record type.
type listFixture struct {
	live, short ListRef
	lists       scriptedLists
}

func newListFixture() *listFixture {
	live := ListRef{ID: ids.NewV7(), Name: "Buyers", EntityType: "contact", Live: true, Version: 2}
	short := ListRef{ID: ids.NewV7(), Name: "Follow-ups", EntityType: "contact"}
	return &listFixture{live: live, short: short, lists: scriptedLists{lists: map[ids.UUID]ListRef{live.ID: live, short.ID: short}}}
}

// event is a check of the live list, as the rule with params sees it.
func (f *listFixture) event(t *testing.T, params map[string]any, check crmcontracts.PublicEventListEvaluated) workflow.Event {
	t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	params[paramListID] = f.live.ID.String()
	rawParams, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(check)
	if err != nil {
		t.Fatal(err)
	}
	return workflow.Event{
		ID: ids.NewV7(), WorkspaceID: ids.NewV7(), OwnerID: ids.NewV7(), Params: rawParams, Payload: payload,
		Entity: datasource.EntityRef{Type: rbacObjList, ID: f.live.ID},
	}
}

func (f *listFixture) rule(action ActionType, lists scriptedLists) listRule {
	return listRule{name: listNotifyName, action: action, ex: Executors{Lists: lists}}
}

func currentCheck() crmcontracts.PublicEventListEvaluated {
	return crmcontracts.PublicEventListEvaluated{DefinitionVersion: 2, EvaluatedAt: time.Unix(0, 0).UTC()}
}

func TestAnOwnerWhoMayNoLongerReadTheListPausesTheRule(t *testing.T) {
	f := newListFixture()
	lists := f.lists
	lists.observed = apperrors.ErrPermissionDenied
	firings, pause, err := f.rule(ActionTypeNotify, lists).expand(context.Background(), &fakeAuthzResolver{}, f.event(t, nil, currentCheck()))
	if err != nil || len(firings) != 0 || pause == nil || pause.reason != PausedListUnavailable {
		t.Fatalf("expand = %v, %+v, %v; want no firing and a pause because the list is unavailable", firings, pause, err)
	}
}

// expandCase is one check a rule is handed, and what it must do with it.
type expandCase struct {
	name  string
	lists func(scriptedLists) scriptedLists
	check func(*crmcontracts.PublicEventListEvaluated)
	pause string
	fires int
}

func expandCases(f *listFixture) []expandCase {
	archived, invalid := f.live, f.live
	archived.Archived, invalid.Invalid = true, true
	only := func(l ListRef) func(scriptedLists) scriptedLists {
		return func(s scriptedLists) scriptedLists { s.lists = map[ids.UUID]ListRef{f.live.ID: l}; return s }
	}
	return []expandCase{
		{name: "the first check after a filter change", check: func(c *crmcontracts.PublicEventListEvaluated) { c.FilterChanged = true }},
		{name: "a check under an older filter", check: func(c *crmcontracts.PublicEventListEvaluated) { c.DefinitionVersion = 1 }},
		{name: "an archived list", lists: only(archived), pause: PausedListArchived},
		{name: "a broken filter", lists: only(invalid), pause: PausedListInvalid},
		{name: "a list nobody can find", lists: func(s scriptedLists) scriptedLists { s.lists = nil; return s }, pause: PausedListUnavailable},
		{name: "a burst", lists: func(s scriptedLists) scriptedLists { s.total = burstCap + 1; return s }, pause: PausedBurst},
		{name: "one joined record", fires: 1, lists: func(s scriptedLists) scriptedLists {
			s.total = 1
			s.changes = []ListChange{{
				EventID: ids.NewV7(), Action: directionEntered,
				Record: datasource.EntityRef{Type: "contact", ID: f.short.ID},
			}}
			return s
		}},
	}
}

func TestACheckFiresOnlyWhatItMayFire(t *testing.T) {
	f := newListFixture()
	for _, tc := range expandCases(f) {
		t.Run(tc.name, func(t *testing.T) {
			lists, check := f.lists, currentCheck()
			if tc.lists != nil {
				lists = tc.lists(lists)
			}
			if tc.check != nil {
				tc.check(&check)
			}
			firings, pause, err := f.rule(ActionTypeNotify, lists).expand(context.Background(), &fakeAuthzResolver{}, f.event(t, nil, check))
			if err != nil {
				t.Fatal(err)
			}
			gotPause := ""
			if pause != nil {
				gotPause = pause.reason
			}
			if gotPause != tc.pause || len(firings) != tc.fires {
				t.Fatalf("expand paused %q and fired %d, want %q and %d", gotPause, len(firings), tc.pause, tc.fires)
			}
			if tc.fires == 1 && (firings[0].Entity.ID != f.short.ID || !strings.Contains(string(firings[0].Payload), "Buyers")) {
				t.Fatalf("the firing is about %+v with %s, want the record that joined", firings[0].Entity, firings[0].Payload)
			}
		})
	}
}

func TestACheckOfAnotherListFiresNothing(t *testing.T) {
	f := newListFixture()
	ev := f.event(t, nil, currentCheck())
	ev.Entity.ID = ids.NewV7()
	firings, pause, err := f.rule(ActionTypeNotify, f.lists).expand(context.Background(), &fakeAuthzResolver{}, ev)
	if err != nil || firings != nil || pause != nil {
		t.Fatalf("a rule answered for a list it does not watch: %v, %+v, %v", firings, pause, err)
	}
}

func TestEachPauseSaysWhyAndWhatBringsTheRuleBack(t *testing.T) {
	for reason, want := range map[string]string{
		PausedListArchived:    "was archived",
		PausedListInvalid:     "no longer works",
		PausedBurst:           "moved 150 records",
		PausedListUnavailable: "can no longer find",
	} {
		if got := pauseSentence(rulePause{reason: reason, count: 150}); !strings.Contains(got, want) {
			t.Errorf("the %s pause says %q, want it to say %q", reason, got, want)
		}
	}
}

func TestEachListRuleAnswersWithItsOwnEffect(t *testing.T) {
	f := newListFixture()
	firing, err := json.Marshal(listFiring{ListID: f.live.ID, ListName: "Buyers", Action: directionLeft, MemberEventID: ids.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	for action, want := range map[ActionType]workflow.ActionKind{
		ActionTypeCreateTask:     workflow.ActionCreateTask,
		ActionTypeNotify:         workflow.ActionNotify,
		ActionTypeAddToShortlist: workflow.ActionAddListMember,
	} {
		ev := f.event(t, map[string]any{paramShortlistID: f.short.ID.String()}, currentCheck())
		ev.Payload, ev.Entity = firing, datasource.EntityRef{Type: "contact", ID: ids.NewV7()}
		effect, err := f.rule(action, f.lists).Plan(context.Background(), ev)
		if err != nil || len(effect.Actions) != 1 || effect.Actions[0].Kind != want {
			t.Fatalf("%s planned %+v (%v), want one %s", action, effect.Actions, err, want)
		}
		if action == ActionTypeCreateTask && strings.Contains(string(effect.Actions[0].Args), "Buyers") {
			t.Fatalf("the follow-up task names the list: %s", effect.Actions[0].Args)
		}
	}
}

func TestAListRuleActsOnARecordNeverOnTheCheck(t *testing.T) {
	f := newListFixture()
	ev := f.event(t, nil, currentCheck())
	rule := f.rule(ActionTypeNotify, f.lists)
	if ok, err := rule.Match(context.Background(), ev); err != nil || ok {
		t.Fatalf("the check's own event matched (%v, %v): the rule would act on the list", ok, err)
	}
	if _, err := rule.Plan(context.Background(), ev); err == nil {
		t.Fatal("a plan without a record was accepted")
	}
	if !strings.HasSuffix(rule.IdempotencyKey(ev), ev.ID.String()) {
		t.Fatalf("the check's key %q does not fall back to the event", rule.IdempotencyKey(ev))
	}
}

func TestAddingToAShortlistActsOnlyForTheRuleOwner(t *testing.T) {
	f := newListFixture()
	var added []datasource.EntityRef
	lists := f.lists
	lists.added, lists.already = &added, true
	args, err := json.Marshal(addListMemberArgs{ListID: f.short.ID})
	if err != nil {
		t.Fatal(err)
	}
	action := workflow.Action{Kind: workflow.ActionAddListMember, Target: datasource.EntityRef{Type: "contact", ID: ids.NewV7()}, Args: args}
	if _, err := applyAddListMember(context.Background(), Executors{}, action); err == nil {
		t.Fatal("a Shortlist was changed with no lists seam")
	}
	ex := Executors{Lists: lists, Authority: &fakeAuthzResolver{}}
	engineCtx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), ids.NewV7()), systemActor)
	if _, err := applyAddListMember(engineCtx, ex, action); err == nil || len(added) != 0 {
		t.Fatalf("a Shortlist was changed with no owner to act for (%v, %v)", err, added)
	}
	recorded, err := applyAddListMember(withSendingOwner(engineCtx, workflow.Event{OwnerID: ids.NewV7()}), ex, action)
	if err != nil || len(added) != 1 || !recorded.Deduplicated {
		t.Fatalf("adding a record already there = %+v, %v, %v; want one call marked deduplicated", recorded, err, added)
	}
}

func TestAListRuleNeedsListsTheInstallationOffers(t *testing.T) {
	entry, ok := CatalogEntryByKey(listNotifyName)
	if !ok {
		t.Fatal("the notify list rule is not in the catalog")
	}
	var param *ParamError
	if err := (&AutomationStore{}).validateRefs(context.Background(), entry, map[string]any{}); !errors.As(err, &param) || param.Field != "key" {
		t.Fatalf("a list rule with lists switched off answered %v, want a refusal on the key", err)
	}
	_, _, err := resolvePreviewRecipe(context.Background(), nil, Automation{Key: listNotifyName}, AutomationPreviewInput{}, time.Now())
	if !errors.As(err, &param) || !strings.Contains(param.Reason, "check runs") {
		t.Fatalf("previewing a list rule answered %v, want the documented gap", err)
	}
}
