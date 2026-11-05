// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAnIdenticalCompletedAbsoluteMoveReplacesItsOlderFailure(t *testing.T) {
	for _, tc := range []struct {
		name, kind, replacementHash, replacementBody string
		completed, hidden, otherDecider              bool
	}{
		{"identical completed move", "progress_deal", "same", `{"to_stage_id":"won"}`, true, true, false},
		{"different decider", "progress_deal", "same", `{"to_stage_id":"won"}`, true, false, true},
		{"unspent approval", "progress_deal", "same", `{"to_stage_id":"won"}`, false, false, false},
		{"different intent hash", "progress_deal", "different", `{"to_stage_id":"won"}`, true, false, false},
		{"different proposed values", "progress_deal", "same", `{"to_stage_id":"lost"}`, true, false, false},
		{"relative advance", "advance_deal", "same", `{"to_stage_id":"won"}`, true, false, false},
		{"repeatable send", "send_email", "same", `{"to_stage_id":"won"}`, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupStaging(t)
			target := newDealSeeder(t, e).deal(t)
			ctx := e.asHumanWith(grantsFor(map[string]principal.ObjectGrant{
				tableDeal: {Read: true, Update: true}, objectActivity: {Read: true, Create: true},
			}))
			stage := func(hash, body string) ids.ApprovalID {
				t.Helper()
				id, err := e.svc.Stage(ctx, StageInput{
					Kind: tc.kind, TargetType: tableDeal, TargetID: target,
					ProposedChange: json.RawMessage(body), DiffHash: hash, Summary: "Move the deal to Won",
				})
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			failure := errors.New("synthetic effect failure")
			e.svc.WithEffect(tc.kind, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
				return failure
			})
			first := stage("same", `{"to_stage_id":"won"}`)
			if _, err := e.svc.Decide(ctx, first, true, nil); !errors.Is(err, failure) {
				t.Fatalf("expected effect failure, got %v", err)
			}
			if got := e.ownFailures(ctx, t); len(got) != 1 {
				t.Fatalf("failure missing before replacement: %d", len(got))
			}
			e.svc.WithEffect(tc.kind, func(effectCtx context.Context, id ids.ApprovalID, _ json.RawMessage, hash string) error {
				if !tc.completed {
					return nil
				}
				_, _, err := e.svc.Redeem(effectCtx, id, tc.kind, hash)
				return err
			})
			second := stage(tc.replacementHash, tc.replacementBody)
			decider := ctx
			if tc.otherDecider {
				actor, ok := principal.Actor(ctx)
				if !ok {
					t.Fatal("missing test principal")
				}
				actor.UserID = e.secondRep(t)
				actor.ID = "human:" + actor.UserID.String()
				decider = principal.WithActor(ctx, actor)
			}
			if _, err := e.svc.Decide(decider, second, true, nil); err != nil {
				t.Fatal(err)
			}
			got := e.ownFailures(ctx, t)
			if (len(got) == 0) != tc.hidden {
				t.Fatalf("remaining failures=%d, hidden=%v", len(got), tc.hidden)
			}
			if at, _ := e.effectFailureOf(t, first); at == nil {
				t.Fatal("read erased the historical failure")
			}
		})
	}
}
