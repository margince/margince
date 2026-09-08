// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// An attribution that fails is retried by the next capture of the same message.
//
// The ladder used to run only when the capture had just CREATED the activity. A
// transient fault — a SQL error, a matcher failure — logged for a reconcile that
// has no caller, and every later replay of that mailbox found the activity
// already present and skipped the ladder. The message stayed unfiled forever,
// and the breadcrumb was the whole record that the question went unanswered
// (#2108).
//
// Driven through the sink twice with the same record, which is what a mailbox
// replay is: the first pass fails inside the matcher, the second succeeds.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// failingOnceMatcher answers with an error the first time and with the project
// the second, which is the transient fault this retry exists for.
type failingOnceMatcher struct {
	project ids.UUID
	calls   int
}

func (m *failingOnceMatcher) MatchProjectKey(_ context.Context, _ pgx.Tx, _ []string) (ids.UUID, error) {
	m.calls++
	if m.calls == 1 {
		return ids.Nil, errors.New("a transient matcher fault")
	}
	return m.project, nil
}

func TestAFailedAttributionIsRetriedByTheNextCapture(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	project := seedProjectForCapture(t, owner, ws)

	matcher := &failingOnceMatcher{project: project}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))).
		WithProjectAttribution(capture.ProjectAttribution{
			Keys: matcher,
			Stamp: func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error {
				return nil
			},
		})

	record := aProjectSubjectRecord()
	sinkCtx := captureSinkContextFor(ctx, ws)

	// First capture: the activity lands, the matcher faults, nothing is filed.
	// The capture itself must NOT fail — the message is on the timeline and the
	// ladder never takes it off.
	if _, err := sink.Upsert(sinkCtx, record); err != nil {
		t.Fatalf("the first capture failed, and the attribution ladder may never fail one: %v", err)
	}
	if linked := projectLinkCount(t, owner, ws); linked != 0 {
		t.Fatalf("the faulted attribution filed %d link(s), want none", linked)
	}

	// The replay. Before this change it found the activity present and skipped
	// the ladder, leaving the message unfiled forever.
	if _, err := sink.Upsert(sinkCtx, record); err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if linked := projectLinkCount(t, owner, ws); linked != 1 {
		t.Errorf("after the replay the message is filed under %d project(s), want 1 — a transient "+
			"fault must not leave a message unfiled forever, and no reconcile pass exists to find it",
			linked)
	}
	if matcher.calls != 2 {
		t.Errorf("the matcher was asked %d time(s), want 2 — the replay must actually re-run the "+
			"ladder rather than the link arriving some other way", matcher.calls)
	}

	// A THIRD capture stops before the rungs: an activity already filed under a
	// project is not asked again, which is what keeps the retry cheap.
	if _, err := sink.Upsert(sinkCtx, record); err != nil {
		t.Fatalf("the third capture failed: %v", err)
	}
	if matcher.calls != 2 {
		t.Errorf("the matcher was asked %d time(s) after a third capture, want 2 — an activity "+
			"already filed must stop on the guard rather than re-run the ladder every replay",
			matcher.calls)
	}
}

// seedProjectForCapture writes one live project the matcher can answer with.
func seedProjectForCapture(t *testing.T, owner *pgx.Conn, _ ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO project (id, name, key, source, captured_by)
		 VALUES ($1, 'Retry Rollout', 'retry', 'ui', 'connector:gmail')`, id); err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	return id
}

// aProjectSubjectRecord is one inbound email whose subject carries a bracketed
// key. The same record every time, which is what a mailbox replay delivers.
func aProjectSubjectRecord() connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "retry-1"},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "[retry] the rollout plan", Body: "as discussed",
			Direction:  connector.DirectionInbound,
			OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		},
		Source:     "gmail:retry-1",
		CapturedBy: "connector:gmail",
		Counterparty: connector.Counterparty{
			Direction: connector.DirectionInbound,
			Email:     "sender@retry.test",
		},
		ThreadKey: "gmail:retry-thread",
	}
}

// captureSinkContextFor is the connector principal a mail sync captures under.
func captureSinkContextFor(ctx context.Context, ws ids.UUID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   "connector:gmail",
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"person":   {Create: true, Read: true},
				"project":  {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// projectLinkCount is how many activities are filed under a project.
func projectLinkCount(t *testing.T, owner *pgx.Conn, _ ids.UUID) int {
	t.Helper()
	var n int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity_link WHERE entity_type = 'project'`).Scan(&n); err != nil {
		t.Fatalf("counting project links: %v", err)
	}
	return n
}
