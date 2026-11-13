// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A sender chooses the subject, so a capture cuts one past the column limit
// instead of losing the whole mail to the guard.
func TestACapturedSubjectPastTheLimitLandsCut(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))
	limit := textbounds.LimitOf("public.activity.subject")
	record := aProjectSubjectRecord()
	fields, ok := record.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatalf("the record carries %T, want activity fields", record.Fields)
	}
	fields.Subject = strings.Repeat("ế", limit+50)
	record.Fields = fields

	if _, err := sink.Upsert(captureSinkContextFor(ctx, ws), record); err != nil {
		t.Fatalf("a long subject failed the capture: %v", err)
	}
	var stored int
	if err := owner.QueryRow(ctx, `SELECT char_length(subject) FROM activity`).Scan(&stored); err != nil {
		t.Fatalf("reading the subject: %v", err)
	}
	if stored != limit {
		t.Fatalf("stored a subject of %d characters, want the limit of %d", stored, limit)
	}
}
