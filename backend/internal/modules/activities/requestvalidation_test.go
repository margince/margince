// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyTaskCreationCanAcceptARequest(t *testing.T) {
	id := openapi_types.UUID(ids.NewV7())
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "note", RequestActivityId: &id, Source: "manual"})
	var fault *RequestAcceptanceFieldError
	if !errors.As(err, &fault) {
		t.Fatalf("note acceptance = %v, want a correctable field refusal", err)
	}
	field, _, _ := fault.FieldFault()
	if field != "request_activity_id" {
		t.Fatal("refusal does not name the invalid field")
	}
}

func TestARecencyCursorCannotResumeRequestPriorityOrder(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	cursor := "anything"
	_, _, _, _, err := listActivitiesFilter(unscopedCtx(), ListActivitiesInput{RequestReviewAsOf: &at, Cursor: &cursor})
	if !errors.Is(err, errRequestReviewWithCursor) {
		t.Fatalf("request cursor = %v", err)
	}
}
