// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// labelEditor is a member holding the update grant on one label's owner.
func labelEditor(object string) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:editor", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{object: {Read: true, Update: true}}},
	})
}

func labelService(t *testing.T, lane *recordingLane) *Service {
	now := clocktest.Now(t)
	svc := NewService(nil, nil, func() time.Time { return now })
	if lane != nil {
		svc = svc.WithLane(lane)
	}
	return svc
}

func TestAStageNameIsSuggestedInTheLanguageAsked(t *testing.T) {
	lane := &recordingLane{reply: `{"already_in_target": false, "subject": "Đã gửi báo giá", "body": ""}`}
	got, err := labelService(t, lane).SuggestLabel(labelEditor("pipeline"), "Proposal sent", textlang.Vietnamese)
	if err != nil {
		t.Fatalf("SuggestLabel: %v", err)
	}
	if got.Text != "Đã gửi báo giá" || got.Unchanged || got.ModelUsed == nil || len(lane.sent) != 1 {
		t.Fatalf("got %+v after %d model calls, want the Vietnamese name from one call", got, len(lane.sent))
	}
}

func TestOnlyALabelEditorMaySuggest(t *testing.T) {
	lane := &recordingLane{reply: `{"already_in_target": false, "subject": "x", "body": ""}`}
	for name, ctx := range map[string]context.Context{
		"a member who edits neither": labelEditor("deal"),
		"an agent":                   principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test"}),
	} {
		if _, err := labelService(t, lane).SuggestLabel(ctx, "Proposal sent", textlang.German); err == nil {
			t.Errorf("%s was given a suggestion", name)
		}
	}
	if _, err := labelService(t, lane).SuggestLabel(labelEditor("deal"), "Proposal sent", textlang.German); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member without the grant: %v, want permission denied", err)
	}
	if len(lane.sent) != 0 {
		t.Errorf("a refused caller reached the model %d times", len(lane.sent))
	}
	if _, err := labelService(t, lane).SuggestLabel(labelEditor("custom_field"), "Budget holder", textlang.German); err != nil {
		t.Errorf("a custom field editor was refused: %v", err)
	}
}

func TestAModelThatChangedTheLabelsFiguresIsRefused(t *testing.T) {
	lane := &recordingLane{reply: `{"already_in_target": false, "subject": "Phase", "body": ""}`}
	if _, err := labelService(t, lane).SuggestLabel(labelEditor("pipeline"), "Phase 2", textlang.German); err == nil {
		t.Fatal("a suggestion that dropped the 2 was accepted")
	}
}

func TestASuggestionWithNoModelSaysSo(t *testing.T) {
	if _, err := labelService(t, nil).SuggestLabel(labelEditor("pipeline"), "Proposal sent", textlang.German); !errors.Is(err, ErrNoModel) {
		t.Fatalf("got %v, want ErrNoModel", err)
	}
}

func TestALabelPastTheStoredLimitAsksNoModel(t *testing.T) {
	lane := &recordingLane{reply: `{"already_in_target": false, "subject": "x", "body": ""}`}
	long := strings.Repeat("a", MaxLabelRunes+1)
	if _, err := labelService(t, lane).SuggestLabel(labelEditor("pipeline"), long, textlang.German); !errors.Is(err, ErrTooLong) {
		t.Fatalf("got %v, want ErrTooLong", err)
	}
	if len(lane.sent) != 0 {
		t.Fatalf("a label past the limit reached the model")
	}
}
