// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubTagSuggestions struct {
	rows []crmcontracts.TagSuggestion
	err  error
}

func (s *stubTagSuggestions) OpenTagSuggestions(_ context.Context, limit int) ([]crmcontracts.TagSuggestion, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rows[:min(limit, len(s.rows))], nil
}

func (s *stubTagSuggestions) CountOpenTagSuggestions(context.Context) (int, error) {
	return len(s.rows), s.err
}

func tagSuggestionRow(entityType crmcontracts.TagSuggestionEntityType) crmcontracts.TagSuggestion {
	return crmcontracts.TagSuggestion{
		Id: openapi_types.UUID(ids.NewV7()), State: "open", EntityType: entityType,
		EntityId: openapi_types.UUID(ids.NewV7()), EntityName: "Acme", CreatedAt: readInstant,
		Tag: crmcontracts.RowTag{TagId: openapi_types.UUID(ids.NewV7()), Name: "Product X"},
	}
}

func TestATagSuggestionWaitsInNeedsYouOnTheRecordItWouldTag(t *testing.T) {
	onCompany := tagSuggestionRow(crmcontracts.TagSuggestionEntityTypeCompany)
	onContact := tagSuggestionRow(crmcontracts.TagSuggestionEntityTypeContact)
	svc := decisionsService(stubApprovals{}, stubDuplicates{}).
		WithTagSuggestions(&stubTagSuggestions{rows: []crmcontracts.TagSuggestion{onCompany, onContact}})

	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(day.NeedsYou) != 2 || day.Counts.NeedsYou != 2 ||
		day.Counts.TagSuggestionsOpen == nil || *day.Counts.TagSuggestionsOpen != 2 {
		t.Fatalf("needs_you = %d rows, %d counted, %v tag suggestions; want 2, 2, 2",
			len(day.NeedsYou), day.Counts.NeedsYou, day.Counts.TagSuggestionsOpen)
	}
	subjects := map[string]crmcontracts.AttentionSubjectType{}
	for _, item := range day.NeedsYou {
		if item.Source != sourceTagSuggestion || item.Subject == nil {
			t.Fatalf("row %+v is not a tag suggestion naming its record", item)
		}
		subjects[item.Id] = item.Subject.Type
	}
	if subjects[onCompany.Id.String()] != subjectCompany || subjects[onContact.Id.String()] != subjectContact {
		t.Fatalf("subjects = %v, want the company row on the company and the contact row on the contact", subjects)
	}
}

func TestAFailedTagSuggestionReadCostsOnlyItsOwnRows(t *testing.T) {
	svc := decisionsService(stubApprovals{rows: []crmcontracts.Approval{approval("a staged send")}}, stubDuplicates{}).
		WithTagSuggestions(&stubTagSuggestions{err: errors.New("statement timeout")})

	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(day.NeedsYou) != 1 || day.Counts.TagSuggestionsOpen != nil {
		t.Fatalf("needs_you = %d rows, tag count %v; want the approval alone and no count", len(day.NeedsYou), day.Counts.TagSuggestionsOpen)
	}
}

func TestATagSuggestionReaderWithoutTheGrantSeesNoneAndNoCount(t *testing.T) {
	svc := decisionsService(stubApprovals{}, stubDuplicates{}).
		WithTagSuggestions(&stubTagSuggestions{err: apperrors.ErrPermissionDenied})

	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(day.NeedsYou) != 0 || day.Counts.TagSuggestionsOpen != nil {
		t.Fatalf("needs_you = %d rows, tag count %v; want none", len(day.NeedsYou), day.Counts.TagSuggestionsOpen)
	}
}
