// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// scriptedLists answers Find with one live list and ObservedChanges with a
// fixed refusal.
type scriptedLists struct {
	list     ListRef
	observed error
}

func (s scriptedLists) Find(context.Context, ids.UUID) (ListRef, error) { return s.list, nil }

func (s scriptedLists) ObservedChanges(context.Context, ids.UUID, int64, time.Time, []string, int) ([]ListChange, int, error) {
	return nil, 0, s.observed
}

func (s scriptedLists) CheckShortlist(context.Context, ids.UUID, string) error { return nil }

func (s scriptedLists) AddMember(context.Context, context.Context, ids.UUID, datasource.EntityRef) (bool, error) {
	return false, nil
}

func TestAnOwnerWhoMayNoLongerReadTheListPausesTheRule(t *testing.T) {
	list := ListRef{ID: ids.NewV7(), Name: "Buyers", EntityType: "contact", Live: true, Version: 2}
	rule := listRule{name: listNotifyName, action: ActionTypeNotify, ex: Executors{
		Lists: scriptedLists{list: list, observed: apperrors.ErrPermissionDenied},
	}}
	params, err := json.Marshal(map[string]string{paramListID: list.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	check, err := json.Marshal(crmcontracts.PublicEventListEvaluated{DefinitionVersion: 2, EvaluatedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	ev := workflow.Event{
		ID: ids.NewV7(), WorkspaceID: ids.NewV7(), OwnerID: ids.NewV7(), Params: params, Payload: check,
		Entity: datasource.EntityRef{Type: rbacObjList, ID: list.ID},
	}
	firings, pause, err := rule.expand(context.Background(), &fakeAuthzResolver{}, ev)
	if err != nil || len(firings) != 0 || pause == nil || pause.reason != PausedListUnavailable {
		t.Fatalf("expand = %v, %+v, %v; want no firing and a pause because the list is unavailable", firings, pause, err)
	}
}
