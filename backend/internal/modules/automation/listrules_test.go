// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func listEntry(t *testing.T, key string) CatalogEntry {
	t.Helper()
	entry, ok := CatalogEntryByKey(key)
	if !ok {
		t.Fatalf("%s is not in the catalog", key)
	}
	return entry
}

// refusedField is the params field a validation refused, or "" when it passed.
func refusedField(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var param *ParamError
	if !errors.As(err, &param) {
		t.Fatalf("a refusal that names no field: %v", err)
	}
	return param.Field
}

func TestAListRulesParamsAreCheckedBeforeAnythingIsStored(t *testing.T) {
	list := ids.NewV7().String()
	for _, tc := range []struct {
		name   string
		key    string
		params map[string]any
		field  string
	}{
		{"a rule on no list", listNotifyName, map[string]any{}, "params.list_id"},
		{"a list that is not an id", listNotifyName, map[string]any{"list_id": "buyers"}, "params.list_id"},
		{"a direction outside the three", listNotifyName, map[string]any{"list_id": list, "direction": "sideways"}, "params.direction"},
		{"a Shortlist for a rule that adds to none", listNotifyName, map[string]any{"list_id": list, "shortlist_id": list}, "params.shortlist_id"},
		{"a due date for a rule that makes no task", listNotifyName, map[string]any{"list_id": list, "due_in_days": float64(2)}, "params.due_in_days"},
		{"a knob nobody reads", listNotifyName, map[string]any{"list_id": list, "colour": "red"}, "params.colour"},
		{"a Shortlist rule with no Shortlist", listShortlistName, map[string]any{"list_id": list}, "params.shortlist_id"},
		{"a task due in a fraction of a day", listTaskName, map[string]any{"list_id": list, "due_in_days": 1.5}, "params.due_in_days"},
		{"a task due in 31 days", listTaskName, map[string]any{"list_id": list, "due_in_days": float64(31)}, "params.due_in_days"},
		{"a complete task rule", listTaskName, map[string]any{"list_id": list, "direction": "either", "due_in_days": float64(3)}, ""},
		{"a complete Shortlist rule", listShortlistName, map[string]any{"list_id": list, "direction": "left", "shortlist_id": list}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := refusedField(t, listEntry(t, tc.key).Validate(tc.params)); got != tc.field {
				t.Fatalf("refused %q, want %q", got, tc.field)
			}
		})
	}
}

func TestAListRuleNamesOnlyListsItsAuthorMayUse(t *testing.T) {
	f := newListFixture()
	archived, invalid, other, liveTarget, archivedTarget := f.live, f.live, f.short, f.live, f.short
	archived.ID, invalid.ID, other.ID, liveTarget.ID, archivedTarget.ID = ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	archived.Archived, invalid.Invalid, other.EntityType, archivedTarget.Archived = true, true, "company", true
	for _, l := range []ListRef{archived, invalid, other, liveTarget, archivedTarget} {
		f.lists.lists[l.ID] = l
	}
	denied := f.lists
	denied.shortlist = apperrors.ErrPermissionDenied
	for _, tc := range []struct {
		name      string
		lists     scriptedLists
		watched   ids.UUID
		shortlist ids.UUID
		field     string
	}{
		{"a list the author cannot find", f.lists, ids.NewV7(), ids.Nil, "params.list_id"},
		{"a Shortlist in the watched place", f.lists, f.short.ID, ids.Nil, "params.list_id"},
		{"an archived Live List", f.lists, archived.ID, ids.Nil, "params.list_id"},
		{"a Live List whose filter broke", f.lists, invalid.ID, ids.Nil, "params.list_id"},
		{"a Shortlist the author cannot find", f.lists, f.live.ID, ids.NewV7(), "params.shortlist_id"},
		{"a Live List as the target", f.lists, f.live.ID, liveTarget.ID, "params.shortlist_id"},
		{"an archived Shortlist", f.lists, f.live.ID, archivedTarget.ID, "params.shortlist_id"},
		{"a Shortlist of another record type", f.lists, f.live.ID, other.ID, "params.shortlist_id"},
		{"a Shortlist the author may not change", denied, f.live.ID, f.short.ID, "params.shortlist_id"},
		{"a Shortlist the author may change", f.lists, f.live.ID, f.short.ID, ""},
		{"a notify rule on a Live List", f.lists, f.live.ID, ids.Nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{"list_id": tc.watched.String()}
			if !tc.shortlist.IsZero() {
				params["shortlist_id"] = tc.shortlist.String()
			}
			if got := refusedField(t, validateListRuleRefs(context.Background(), tc.lists, params)); got != tc.field {
				t.Fatalf("refused %q, want %q", got, tc.field)
			}
		})
	}
}

func TestTheCatalogOffersListRulesOnlyWhereListsAreOn(t *testing.T) {
	offered := func(h Handlers) map[string]bool {
		rec := httptest.NewRecorder()
		h.ListAutomationCatalog(rec, httptest.NewRequest(http.MethodGet, "/automations/catalog", nil))
		var body struct {
			Data []struct {
				Key string `json:"key"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		keys := map[string]bool{}
		for _, e := range body.Data {
			keys[e.Key] = true
		}
		return keys
	}
	off := offered(NewHandlers(nil))
	on := offered(NewHandlers(nil).WithLists(newListFixture().lists))
	for _, key := range listRuleKeys {
		if off[key] || !on[key] {
			t.Errorf("%s offered with lists off = %v and on = %v, want only with them on", key, off[key], on[key])
		}
	}
	if !off[stageChangeNotifyName] {
		t.Error("switching lists off hid a rule that names no list")
	}
}

func TestAPausedRuleSaysWhyOnTheWire(t *testing.T) {
	reason := PausedBurst
	wire, err := wireAutomation(Automation{ID: ids.New[ids.AutomationKind](), Key: listNotifyName, Params: json.RawMessage(`{}`), PausedReason: &reason})
	if err != nil || wire.Status != "paused" || wire.PausedReason == nil || string(*wire.PausedReason) != PausedBurst {
		t.Fatalf("a rule paused for a burst reads %+v (%v), want paused with the reason", wire, err)
	}
	running, err := wireAutomation(Automation{ID: ids.New[ids.AutomationKind](), Key: listNotifyName, Enabled: true})
	if err != nil || running.PausedReason != nil {
		t.Fatalf("a running rule reads %+v (%v), want no pause reason", running, err)
	}
}
