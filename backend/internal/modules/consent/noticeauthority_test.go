// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNoticeMutationRepliesRequireReadAuthority(t *testing.T) {
	user, id := ids.NewV7(), ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"privacy_request": {Update: true}}},
	})
	store := NewStore(nil)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		run  func() (NoticeCase, error)
	}{
		{"assign duty", func() (NoticeCase, error) { return store.AssignNoticeCase(ctx, id, user) }},
		{"assign review", func() (NoticeCase, error) { return store.AssignNoticeCaseReview(ctx, id, user) }},
		{"close duty", func() (NoticeCase, error) {
			return store.ExcuseNoticeCase(ctx, id, ExcuseInput{State: NoticeProvidedElsewhere, Note: "Disclosure recorded", Now: now})
		}},
		{"assess source", func() (NoticeCase, error) {
			return store.AssessNoticeSource(ctx, id, SourceAssessment{Kind: "subject_initiated", Note: "Incoming original", Now: now})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.run(); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("update-only result=%v", err)
			}
		})
	}
}
