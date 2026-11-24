// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The region limit holds for an extension's call as it does for a core task.
// Under the sovereign profile a cloud-only binding serves neither task.
// The cloud client is never called.
func TestAnExtensionTaskObeysTheRegionLimitLikeACoreTask(t *testing.T) {
	for name, task := range map[string]Task{"core task": TaskSummarize, "extension task": TaskExtensionTask} {
		t.Run(name, func(t *testing.T) {
			cloud := NewFakeClient().Script("must not be served")
			r := testRouter(map[Tier]model.Client{TierCheapCloud: cloud, TierPremium: cloud}, &memMeter{}, DefaultMonthlyTokens, ProfileSovereign)
			ctx := provenance.WithExtension(wsContext(t), provenance.Extension{Unit: "books"})
			req := model.Request{Messages: []model.Message{{Role: "user", Content: "hello"}}, Subjects: model.AboutNoContact("a test")}

			_, _, err := r.Complete(ctx, task, req)
			if !errors.Is(err, ErrAllTiersFailed) {
				t.Fatalf("err = %v, want ErrAllTiersFailed", err)
			}
			if n := len(cloud.Calls()); n != 0 {
				t.Fatalf("the cloud model was called %d times under the sovereign profile", n)
			}
		})
	}
}

func TestExtensionOfNamesTheUnitAndTheTaskFromTheInvocation(t *testing.T) {
	unit := provenance.WithExtension(context.Background(), provenance.Extension{Unit: "books"})
	for name, tc := range map[string]struct {
		ctx  context.Context
		want string
	}{
		"no extension":             {context.Background(), ""},
		"a task without a unit":    {WithExtensionTask(context.Background(), "summarise"), ""},
		"a unit without a task":    {unit, "books"},
		"a unit and its task":      {WithExtensionTask(unit, "summarise"), "books/summarise"},
		"a unit with an empty one": {WithExtensionTask(unit, ""), "books"},
	} {
		if got := extensionOf(tc.ctx); got != tc.want {
			t.Errorf("%s: extensionOf = %q, want %q", name, got, tc.want)
		}
	}
}
