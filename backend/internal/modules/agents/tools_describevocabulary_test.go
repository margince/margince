// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readerCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(),
		PassportID: ids.NewV7(), Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
}

// Every target takes no arguments, so a stray one is refused by the target
// itself, which proves the call reached it without the topic.
func TestDescribeVocabularyRoutesEveryTopic(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	r.Register(describeQueryVocabulary{})
	r.Register(describeAnalyticsVocabulary{})
	r.Register(describeRecordFields{})
	r.Register(describeListFilters{})
	r.Register(describeReportBlocks{})
	r.Register(describeReportVocabulary{})
	r.Register(describeVocabulary{})
	for topic, target := range (describeVocabulary{}).routeTargets() {
		served, err := r.InvokeServed(readerCtx(), "describe_vocabulary",
			json.RawMessage(`{"topic":"`+topic+`","stray":1}`))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != target {
			t.Errorf("%s: served %q with %v, want %s's own refusal of the stray argument", topic, served.Tool, err, target)
		}
	}
}
