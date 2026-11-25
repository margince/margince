// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const visitKind = "ext:alpha:visit"

// registerVisit declares the alpha unit's visit kind until the test ends.
func registerVisit(t *testing.T) {
	t.Helper()
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: "ext_alpha_visit",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
}

func readerOf(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func TestAnExtensionKindIsUnavailableWhenRemovedOrNotReadable(t *testing.T) {
	registerVisit(t)
	cases := []struct {
		name string
		ctx  context.Context
		kind string
		want bool
	}{
		{"declared and readable", readerOf("ext_alpha_visit"), visitKind, false},
		{"declared, no read right", readerOf("contact"), visitKind, true},
		{"no unit declares it", readerOf("ext_alpha_visit"), "ext:gone:visit", true},
		{"a core type is never unavailable", readerOf(), typeContact, false},
	}
	for _, c := range cases {
		if got := kindUnavailable(c.ctx, c.kind); got != c.want {
			t.Errorf("%s: kindUnavailable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOnlyAKindNoUnitDeclaresIsRemoved(t *testing.T) {
	registerVisit(t)
	if kindRemoved(visitKind) || kindRemoved(typeContact) || !kindRemoved("ext:gone:visit") {
		t.Error("kindRemoved must hold only for an extension kind no unit declares")
	}
}

func TestAListOverARemovedKindIsMarkedAndAnUnreadableOneStillRefused(t *testing.T) {
	registerVisit(t)
	lists := []listRow{
		{EntityType: "ext:gone:visit", ListType: listTypeDynamic},
		{EntityType: typeContact, ListType: listTypeStatic},
	}
	got, err := gatedSummaries(readerOf(typeContact), lists)
	if err != nil {
		t.Fatalf("gatedSummaries = %v, want the page to keep loading", err)
	}
	if got[0].Health != healthUnavailable || got[1].Health != healthOK {
		t.Errorf("health = %s, %s, want unavailable, ok", got[0].Health, got[1].Health)
	}
	_, err = gatedSummaries(readerOf(typeContact), []listRow{{EntityType: visitKind, ListType: listTypeStatic}})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a declared kind the reader may not read = %v, want permission denied", err)
	}
}

func TestTheWireNeverCarriesARemovedKindsFilter(t *testing.T) {
	definition := map[string]any{"field": "site", "op": "eq", "value": "x"}
	removed := wireList(listSummary{listRow: listRow{Definition: definition}, Health: healthUnavailable})
	kept := wireList(listSummary{listRow: listRow{Definition: definition}, Health: healthOK})
	if removed.Definition != nil || kept.Definition == nil {
		t.Errorf("definitions = %v, %v; want none for the removed kind and the filter otherwise", removed.Definition, kept.Definition)
	}
}

func TestTheLiveListCheckStoresARemovedKindAsInvalid(t *testing.T) {
	if !invalidFilter(errKindRemoved) {
		t.Error("a removed kind must checkpoint the list invalid, not fail every pass")
	}
}

func TestALiveListOverAnUnreadableKindIsRefusedAndOverARemovedOneEmpty(t *testing.T) {
	registerVisit(t)
	var store Store
	ctx := readerOf(typeContact)
	if _, _, err := store.liveFilter(ctx, listRow{EntityType: visitKind, ListType: listTypeDynamic}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("liveFilter over an unreadable kind = %v, want permission denied", err)
	}
	if _, _, err := store.liveFilter(ctx, listRow{EntityType: "ext:gone:visit", ListType: listTypeDynamic}); !errors.Is(err, errKindRemoved) {
		t.Errorf("liveFilter over a removed kind = %v, want errKindRemoved", err)
	}
	if clause := extensionMemberClause(ctx, visitKind, "ev.entity_id"); clause != "FALSE" {
		t.Errorf("member clause = %q, want FALSE", clause)
	}
	visible, err := observedVisibleClause(readerOf("ext_alpha_visit"), visitKind, func(any) int { return 1 })
	if err != nil || visible != `EXISTS (SELECT 1 FROM "ext"."ext_alpha_visit" e WHERE e.id = ev.entity_id AND e.archived_at IS NULL)` {
		t.Errorf("observed clause = %q, %v, want the kind's own table in the ext schema", visible, err)
	}
}

func TestASavedViewOverAnUnavailableKindIsRefusedAsBadInput(t *testing.T) {
	registerVisit(t)
	var store Store
	err := store.validateViewFilter(readerOf(typeContact), visitKind, map[string]any{viewFilterKey: map[string]any{}})
	if _, ok := errors.AsType[*BadInputError](err); !ok {
		t.Errorf("validateViewFilter = %v, want a BadInputError the transport answers 422", err)
	}
}
