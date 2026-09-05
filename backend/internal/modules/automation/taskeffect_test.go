// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestATaskForADisappearedTargetIsSkipped(t *testing.T) {
	ex := Executors{Provider: &fakeReadProvider{err: apperrors.ErrNotFound}}
	effect, err := ownedTaskEffect(context.Background(), ex, workflow.Event{}, "Follow up", time.Time{})
	var declined declinedFiring
	if !errors.As(err, &declined) || len(effect.Actions) != 0 {
		t.Fatalf("stale target planned work: %+v, %v", effect, err)
	}
}

func TestAnUnreadableTaskOwnerIsNotTreatedAsUnassigned(t *testing.T) {
	for name, provider := range map[string]*fakeReadProvider{
		"permission denied": {err: apperrors.ErrPermissionDenied},
		"malformed record":  {record: datasource.Record{Fields: []byte("not json")}},
	} {
		t.Run(name, func(t *testing.T) {
			effect, err := ownedTaskEffect(context.Background(), Executors{Provider: provider}, workflow.Event{}, "Follow up", time.Time{})
			if err == nil || len(effect.Actions) != 0 {
				t.Fatalf("unreadable owner planned an unassigned task: %+v, %v", effect, err)
			}
		})
	}
}
