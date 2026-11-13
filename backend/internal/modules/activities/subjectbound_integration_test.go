// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The one typing a subject can shorten it. So a subject past the column limit
// is refused with the column and its limit, not cut.
func TestATypedSubjectPastTheLimitIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	limit := textbounds.LimitOf("public.activity.subject")
	subject := strings.Repeat("ế", limit+1)
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "note", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	_, _, err = e.store(nil).LogActivity(ctx, in)
	column, refusedAt, ok := storekit.TextBoundViolation(err)
	if !ok || column != "subject" || refusedAt != limit {
		t.Fatalf("LogActivity answered %v, want the subject column refused at %d", err, limit)
	}
}
