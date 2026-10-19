// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAConversationNarrowedAfterLoadingNeverReachesTheModel(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	body := "I will send confidential pricing tomorrow."
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &body, Source: "manual", OccurredAt: &now, ThreadKey: "narrowed-after-loading", Direction: new("outbound"),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	thread := settledThread{Key: "narrowed-after-loading", Newest: now, Count: 1}
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		thread.Messages, err = threadMessages(reader, tx, &thread)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(thread.Messages) != 1 || thread.Messages[0].Body != body {
		t.Fatal("fixture did not load the original conversation")
	}
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(row.Id)), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}},
	}); err != nil {
		t.Fatal(err)
	}
	brain := &capturingBrain{}
	extractor := NewSignalExtractor(e.Pool, brain, func() time.Time { return now }, slog.Default())
	raised, err := extractor.readThread(reader, ids.From[ids.WorkspaceKind](e.WS), thread, now)
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("narrowed conversation was not refused: %v", err)
	}
	if raised != 0 || brain.system != "" || brain.content != "" {
		t.Fatal("cached private words reached the model or produced work")
	}
}
