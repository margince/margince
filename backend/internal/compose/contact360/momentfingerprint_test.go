// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// A contact's promise card set aside while far off must come back once the
// promise is moved to today.

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAContactPromiseMovedToTodayComesBackFromASetAside(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	subject := "Send the signed contract"
	later, today := now.AddDate(0, 1, 0), now
	task := crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Subject: &subject,
		OccurredAt: now.AddDate(0, 0, -10), DueAt: &later,
	}
	moved := task
	moved.DueAt = &today

	before := openPromiseFrom(context.Background(), now, task)
	after := openPromiseFrom(context.Background(), now, moved)
	if before.EvidenceFingerprint == after.EvidenceFingerprint {
		t.Error("a task moved from next month to today keeps its fingerprint, so it stays set aside")
	}
}
