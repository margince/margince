// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func validSuggestion() extension.LinkSuggestionRequest {
	return extension.LinkSuggestionRequest{
		Activity: ids.NewV7().String(),
		Target:   extension.RecordRef{Type: "deal", ID: ids.NewV7().String()},
		Reason:   "The subject names this deal.", Score: 0.8,
	}
}

func TestAJobTickCannotProposeALink(t *testing.T) {
	_, err := extensionCore{unattended: true}.LinkSuggestions().Propose(context.Background(), validSuggestion())
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestAProposalNamesAMessageAndATargetByID(t *testing.T) {
	core := extensionCore{unit: "bad-id-unit", authority: func(ctx context.Context) (context.Context, error) { return ctx, nil }}
	for name, mutate := range map[string]func(*extension.LinkSuggestionRequest){
		"message not a uuid": func(r *extension.LinkSuggestionRequest) { r.Activity = "x" },
		"target not a uuid":  func(r *extension.LinkSuggestionRequest) { r.Target.ID = "x" },
	} {
		t.Run(name, func(t *testing.T) {
			in := validSuggestion()
			mutate(&in)
			if _, err := core.LinkSuggestions().Propose(context.Background(), in); !errors.Is(err, extension.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestAUnitThatProposesTooManyLinksIsRefused(t *testing.T) {
	const unit = "flooding-unit"
	for range extensionSuggestionsPerMinute {
		if !extensionSuggestions.Allow(unit) {
			t.Fatal("refused before the limit")
		}
	}
	core := extensionCore{unit: unit, authority: func(ctx context.Context) (context.Context, error) { return ctx, nil }}
	if _, err := core.LinkSuggestions().Propose(context.Background(), validSuggestion()); !errors.Is(err, extension.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
