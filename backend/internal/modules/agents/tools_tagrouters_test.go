// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// Every target refuses a malformed tag id itself, which proves the call reached
// it with the routing field gone and the rest intact.
func TestTagRoutersRouteEveryChoice(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterTagTools(r, stubTags{})
	for _, tc := range []struct {
		router string
		ctx    context.Context
		table  map[string]string
		field  string
	}{
		{"read_tags", readerCtx(), (readTags{}).routeTargets(), "mode"},
		{"change_tags", agentCtx(), (changeTags{}).routeTargets(), "action"},
	} {
		for choice, target := range tc.table {
			served, err := r.InvokeServed(tc.ctx, tc.router,
				json.RawMessage(`{"`+tc.field+`":"`+choice+`","tag_id":"not-a-uuid"}`))
			if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != target {
				t.Errorf("%s %s: served %q with %v, want %s's own refusal", tc.router, choice, served.Tool, err, target)
			}
		}
	}
}
