// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func scopedCtx(scope principal.Scope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(),
		PassportID: ids.NewV7(), Scopes: principal.NewScopeSet(scope),
	})
}

// Each target refuses a stray argument itself, which proves the call reached
// it with the route field gone and the rest intact.
func TestFieldRoutersRouteEveryChoice(t *testing.T) {
	r := fullRegistry(t)
	for _, tc := range []struct {
		router string
		scope  principal.Scope
		field  string
		table  map[string]string
	}{
		{"read_forecast", principal.ScopeRead, "view", (readForecast{}).routeTargets()},
		{"read_workspace", principal.ScopeRead, "topic", (readWorkspace{}).routeTargets()},
		{"find_records", principal.ScopeRead, "engine", (findRecords{}).routeTargets()},
		{"read_context", principal.ScopeRead, "about", (readContext{}).routeTargets()},
		{"read_attention", principal.ScopeRead, "view", (readAttention{}).routeTargets()},
		{"read_reports", principal.ScopeRead, "kind", (readReports{}).routeTargets()},
		{"change_reports", principal.ScopeWrite, "action", (changeReports{}).routeTargets()},
		{"read_analytics", principal.ScopeRead, "step", (readAnalytics{}).routeTargets()},
		{"change_state", principal.ScopeWrite, "action", (changeState{}).routeTargets()},
		{"change_activities", principal.ScopeWrite, "action", (changeActivities{}).routeTargets()},
		{"change_tags", principal.ScopeWrite, "action", (changeTags{}).routeTargets()},
		{"schedule_meeting", principal.ScopeSend, "action", (scheduleMeeting{}).routeTargets()},
		{"read_project", principal.ScopeRead, "view", (readProject{}).routeTargets()},
		{"send_outbound", principal.ScopeSend, "as", (sendOutbound{}).routeTargets()},
		{"draft_outbound", principal.ScopeDraft, "for", (draftOutbound{}).routeTargets()},
	} {
		for choice, target := range tc.table {
			served, err := r.InvokeServed(scopedCtx(tc.scope), tc.router,
				json.RawMessage(`{"`+tc.field+`":"`+choice+`","stray":1}`))
			if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != target {
				t.Errorf("%s %s: served %q with %v, want %s's own refusal", tc.router, choice, served.Tool, err, target)
			}
		}
	}
}

// A router whose targets come from seams a composition may leave out is absent
// with them, rather than refusing the boot.
func TestARouterIsAbsentWhereATargetsSeamIs(t *testing.T) {
	r := NewRegistry(nil, nil)
	RegisterColleaguesTool(r, func(context.Context, ColleagueQuery) ([]Colleague, bool, error) { return nil, false, nil })
	registerWhereServed(r, readWorkspace{})
	if _, served := r.Spec("read_workspace"); served {
		t.Error("read_workspace registered without list_pipelines or list_channel_providers behind it")
	}
}

func TestChoosingRoutersPickTheTargetByTheArgumentsNamed(t *testing.T) {
	r := fullRegistry(t)
	for _, tc := range []struct {
		router, args, target string
		scope                principal.Scope
	}{
		{"read_relationships", `{"contact_id":"not-a-uuid"}`, "who_knows", principal.ScopeRead},
		{"read_relationships", `{"company_id":"not-a-uuid","deal_id":null}`, "intro_path_to", principal.ScopeRead},
		{"read_relationships", `{"deal_id":"not-a-uuid"}`, "company_coverage", principal.ScopeRead},
		{"resolve_duplicates", `{"candidate_id":"not-a-uuid","decision":"not_the_same"}`, "decide_duplicate", principal.ScopeWrite},
		{"resolve_duplicates", `{"record_type":"contact","source_id":"not-a-uuid","target_id":"not-a-uuid"}`, "merge_records", principal.ScopeWrite},
		{"resolve_duplicates", `{"candidate_id":null,"source_id":"not-a-uuid","target_id":"not-a-uuid","record_type":"contact"}`, "merge_records", principal.ScopeWrite},
	} {
		served, err := r.InvokeServed(scopedCtx(tc.scope), tc.router, json.RawMessage(tc.args))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != tc.target {
			t.Errorf("%s %s: served %q with %v, want %s's own refusal", tc.router, tc.args, served.Tool, err, tc.target)
		}
	}
	for _, tc := range []struct {
		router, args string
		scope        principal.Scope
	}{
		{"resolve_duplicates", `{"decision":"not_the_same"}`, principal.ScopeWrite},
		{"resolve_duplicates", `{"candidate_id":"a","source_id":"b","target_id":"c","record_type":"contact"}`, principal.ScopeWrite},
		{"read_relationships", `{}`, principal.ScopeRead},
		{"read_relationships", `{"contact_id":"a","deal_id":"b"}`, principal.ScopeRead},
	} {
		served, err := r.InvokeServed(scopedCtx(tc.scope), tc.router, json.RawMessage(tc.args))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != tc.router {
			t.Errorf("%s %s: served %q with %v, want the router's own refusal", tc.router, tc.args, served.Tool, err)
		}
	}
}
