// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Project attribution through the real capture path and the real activities
// filing write that compose injects. A failed attribution is retried by the next
// capture, a success is announced once, and the ladder never files what the
// write refuses. Two captures naming one project must not deadlock.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// failingOnceMatcher answers with an error on its first call and with the
// project after that, the transient fault a retry exists for. A matcher built
// with calls: 1 starts past the fault.
type failingOnceMatcher struct {
	project ids.UUID
	calls   int
}

func (m *failingOnceMatcher) MatchProjectKey(context.Context, pgx.Tx, []string) (ids.UUID, error) {
	m.calls++
	if m.calls == 1 {
		return ids.Nil, errors.New("a transient matcher fault")
	}
	return m.project, nil
}

// meetingMatcher holds its first two callers until both have arrived, so two
// captures reach the project write together. Later calls pass at once.
type meetingMatcher struct {
	project ids.UUID
	calls   atomic.Int32
	arrived chan struct{}
	both    chan struct{}
	once    sync.Once
	ctx     context.Context
}

func (m *meetingMatcher) MatchProjectKey(ctx context.Context, tx pgx.Tx, _ []string) (ids.UUID, error) {
	// A workspace-readable mail moves the project's clock, which locks the
	// project row; a held one does not. This stands in for a shared mailbox.
	if _, err := tx.Exec(ctx, `UPDATE activity SET audience = 'workspace', audience_reason = NULL
		WHERE source_id LIKE 'lock-%' AND audience <> 'workspace'`); err != nil {
		return ids.Nil, err
	}
	if m.calls.Add(1) > 2 {
		return m.project, nil
	}
	m.arrived <- struct{}{}
	if len(m.arrived) == 2 {
		m.once.Do(func() { close(m.both) })
	}
	select {
	case <-m.both:
		return m.project, nil
	case <-m.ctx.Done():
		return ids.Nil, m.ctx.Err()
	}
}

// cycleOnceMatcher answers a broken deadlock first, as Postgres does, and the
// project after that.
type cycleOnceMatcher struct {
	project ids.UUID
	calls   int
}

func (m *cycleOnceMatcher) MatchProjectKey(context.Context, pgx.Tx, []string) (ids.UUID, error) {
	m.calls++
	if m.calls == 1 {
		return ids.Nil, &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
	}
	return m.project, nil
}

func attributionSinkOn(e *Env, keys capture.ProjectKeyMatcher) *capture.Sink {
	return capture.NewSink(e.DB()).WithProjectAttribution(capture.ProjectAttribution{
		Keys:  keys,
		Stamp: activities.StampCorrespondenceForProject,
		File:  activities.FileOnProjectTx,
	})
}

func seedAttributionProject(t *testing.T, e *Env, key string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO project (id, name, key, source, captured_by)
		VALUES ($1, 'Retry Rollout', $2, 'manual', 'connector:gmail')`, id, key)
	return id
}

// mailboxContext is the connector principal a mail sync captures under.
func mailboxContext(e *Env, scope principal.RowScope) context.Context {
	ctx := context.Background()
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"}, RowScope: scope,
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true},
				"project":  {Read: true},
			},
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func projectMail(sourceID, subject string) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: subject, Body: "as discussed", Direction: connector.DirectionInbound,
			OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		},
		Source: "gmail:" + sourceID, CapturedBy: "connector:gmail",
		Counterparty: connector.Counterparty{Direction: connector.DirectionInbound, Email: sourceID + "@retry.test"},
		ThreadKey:    "gmail:" + sourceID,
	}
}

func projectLinksOf(t *testing.T, e *Env, sources ...string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.entity_type = 'project' AND a.source_id = ANY($1)`, sources)
}

// filingAnnouncements counts the relink events and relink audit rows of the
// named mails.
func filingAnnouncements(t *testing.T, e *Env, sources ...string) (events, audits int) {
	t.Helper()
	events = e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'activity.updated'
		AND envelope->'entity'->>'id' IN (SELECT id::text FROM activity WHERE source_id = ANY($1))`, sources)
	audits = e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		AND entity_id IN (SELECT id FROM activity WHERE source_id = ANY($1))`, sources)
	return events, audits
}

func TestAFailedAttributionIsRetriedByTheNextCapture(t *testing.T) {
	e := Setup(t)
	matcher := &failingOnceMatcher{project: seedAttributionProject(t, e, "retry")}
	sink := attributionSinkOn(e, matcher)
	record, ctx := projectMail("retry-1", "[retry] the rollout plan"), mailboxContext(e, principal.RowScopeAll)

	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatalf("the first capture failed, and the attribution ladder may never fail one: %v", err)
	}
	if linked := projectLinksOf(t, e, "retry-1"); linked != 0 {
		t.Fatalf("the faulted attribution filed %d link(s), want none", linked)
	}
	if faults := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'capture_project_attribution_fault'`); faults != 1 {
		t.Errorf("the faulted attribution recorded %d fault(s), want 1: an operator must see the unfiled message", faults)
	}
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if linked := projectLinksOf(t, e, "retry-1"); linked != 1 {
		t.Errorf("after the replay the message is filed under %d project(s), want 1", linked)
	}
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatalf("the third capture failed: %v", err)
	}
	if matcher.calls != 2 {
		t.Errorf("the matcher was asked %d time(s), want 2: a filed activity stops on the guard", matcher.calls)
	}
}

func TestACaptureOfAnAlreadyFiledActivityStillStamps(t *testing.T) {
	e := Setup(t)
	project := seedAttributionProject(t, e, "retry")
	matcher := &failingOnceMatcher{project: project, calls: 1}
	var stamped []ids.UUID
	sink := capture.NewSink(e.DB()).WithProjectAttribution(capture.ProjectAttribution{
		Keys: matcher,
		Stamp: func(_ context.Context, _ pgx.Tx, _ ids.ActivityID, on ids.UUID) error {
			stamped = append(stamped, on)
			return nil
		},
		File: activities.FileOnProjectTx,
	})
	record, ctx := projectMail("retry-1", "[retry] the rollout plan"), mailboxContext(e, principal.RowScopeAll)
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatalf("the first capture failed: %v", err)
	}
	if linked := projectLinksOf(t, e, "retry-1"); linked != 1 {
		t.Fatalf("the first capture filed %d link(s), want 1", linked)
	}
	asked := matcher.calls
	stamped = nil
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if matcher.calls != asked {
		t.Errorf("the matcher was asked again (%d to %d), want the guard to stop the ladder", asked, matcher.calls)
	}
	if len(stamped) != 1 || stamped[0] != project {
		t.Fatalf("the replay stamped %v, want one stamp on the project the activity is filed under (%s)", stamped, project)
	}
}

func TestARetriedAttributionAnnouncesItsLinkOnce(t *testing.T) {
	e := Setup(t)
	project := seedAttributionProject(t, e, "retry")
	sink := attributionSinkOn(e, &failingOnceMatcher{project: project})
	record, ctx := projectMail("retry-1", "[retry] the rollout plan"), mailboxContext(e, principal.RowScopeAll)
	for pass, want := range []int{0, 1, 1} {
		if _, err := sink.Upsert(ctx, record); err != nil {
			t.Fatalf("capture pass %d failed: %v", pass+1, err)
		}
		if events, audits := filingAnnouncements(t, e, "retry-1"); events != want || audits != want {
			t.Fatalf("after capture pass %d: %d relink event(s) and %d audit row(s), want %d of each",
				pass+1, events, audits, want)
		}
	}
	if stamped := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence
		WHERE basis = 'project_linked' AND project_id = $1`, project); stamped != 1 {
		t.Errorf("the filing left %d correspondence stamp(s), want 1", stamped)
	}
}

func TestAnActivityAlreadyFiledIsNotRefiledByTheLadder(t *testing.T) {
	e := Setup(t)
	mine, other := seedAttributionProject(t, e, "mine"), seedAttributionProject(t, e, "other")
	sink := attributionSinkOn(e, &failingOnceMatcher{project: other, calls: 1})
	ctx := mailboxContext(e, principal.RowScopeAll)
	if _, err := sink.Upsert(ctx, projectMail("retry-1", "the rollout plan")); err != nil {
		t.Fatalf("the first capture failed: %v", err)
	}
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, project_id)
		SELECT id, 'project', $1 FROM activity WHERE source_id = 'retry-1'`, mine)
	if _, err := sink.Upsert(ctx, projectMail("retry-1", "[other] the rollout plan")); err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if onMine, total := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE project_id = $1`, mine),
		projectLinksOf(t, e, "retry-1"); onMine != 1 || total != 1 {
		t.Errorf("the activity carries %d project link(s), %d on its first project, want 1 and 1", total, onMine)
	}
	if events, audits := filingAnnouncements(t, e, "retry-1"); events != 0 || audits != 0 {
		t.Errorf("a filed activity produced %d event(s) and %d audit row(s), want none", events, audits)
	}
}

// A principal that may update activities but has no authority over this one
// files nothing: the write refuses it, and that is no fault. The activity is
// linked to a contact another seat owns, so no arm of the authority check
// admits the mailbox's own-scope principal.
func TestAMailboxWithoutAuthorityOverTheActivityFilesNothing(t *testing.T) {
	e := Setup(t)
	project := seedAttributionProject(t, e, "retry")
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, audience, source, captured_by, source_id)
		VALUES ($1, 'email', 'the rollout plan', 'workspace', 'manual', 'human:colleague', 'colleague-1')`, activity)
	e.WsExec(t, `WITH c AS (INSERT INTO contact (full_name, visibility, source, captured_by)
		VALUES ('Alice Demo', 'workspace', 'manual', 'human:colleague') RETURNING id)
		INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT $1, 'contact', c.id FROM c`, activity)
	mailbox := mailboxContext(e, principal.RowScopeOwn)

	var landed bool
	err := database.WithTx(mailbox, e.Pool, func(tx pgx.Tx) error {
		var err error
		landed, err = activities.FileOnProjectTx(mailbox, tx, ids.From[ids.ActivityKind](activity), project, nil)
		return err
	})
	if err != nil || landed {
		t.Fatalf("the filing answered landed=%v, err=%v, want a refusal that is no fault", landed, err)
	}
	if linked := projectLinksOf(t, e, "colleague-1"); linked != 0 {
		t.Errorf("a mailbox without authority filed %d project link(s), want none", linked)
	}
	if events, audits := filingAnnouncements(t, e, "colleague-1"); events != 0 || audits != 0 {
		t.Errorf("a refused filing produced %d event(s) and %d audit row(s), want none", events, audits)
	}
}

// A project the mailbox owner can no longer reach is not filed, even when the
// ladder names it.
func TestAProjectTheMailboxCannotReachIsNotFiled(t *testing.T) {
	e := Setup(t)
	project := seedAttributionProject(t, e, "retry")
	e.WsExec(t, `UPDATE project SET archived_at = now() WHERE id = $1`, project)
	sink := attributionSinkOn(e, &failingOnceMatcher{project: project, calls: 1})
	if _, err := sink.Upsert(mailboxContext(e, principal.RowScopeAll), projectMail("retry-1", "[retry] plan")); err != nil {
		t.Fatalf("the capture failed: %v", err)
	}
	if linked := projectLinksOf(t, e, "retry-1"); linked != 0 {
		t.Errorf("a project outside the mailbox's reach was filed %d time(s), want none", linked)
	}
	if events, audits := filingAnnouncements(t, e, "retry-1"); events != 0 || audits != 0 {
		t.Errorf("a refused filing produced %d event(s) and %d audit row(s), want none", events, audits)
	}
}

func TestTwoCapturesNamingOneProjectAtOnceBothFileWithoutALockCycle(t *testing.T) {
	e := Setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	matcher := &meetingMatcher{arrived: make(chan struct{}, 2), both: make(chan struct{}), ctx: ctx}
	matcher.project = seedAttributionProject(t, e, "lock")
	sink := attributionSinkOn(e, matcher)
	mailbox := mailboxContext(e, principal.RowScopeAll)

	sources := []string{"lock-a", "lock-b"}
	errs := make([]error, len(sources))
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Go(func() { _, errs[i] = sink.Upsert(mailbox, projectMail(source, "[lock] plan")) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("a capture failed: %v", err)
	}
	if n := projectLinksOf(t, e, sources...); n != 2 {
		t.Fatalf("%d of the two mails are filed under the project, want both", n)
	}
	if calls := matcher.calls.Load(); calls != 2 {
		t.Errorf("the ladder ran %d times for two mails, want 2: a capture met a lock cycle and ran again", calls)
	}
}

func TestALockCycleInTheLadderRerunsTheCapture(t *testing.T) {
	e := Setup(t)
	matcher := &cycleOnceMatcher{project: seedAttributionProject(t, e, "cycle")}
	sink := attributionSinkOn(e, matcher)
	if _, err := sink.Upsert(mailboxContext(e, principal.RowScopeAll), projectMail("cycle-1", "[cycle] plan")); err != nil {
		t.Fatalf("the capture failed: %v", err)
	}
	if n := projectLinksOf(t, e, "cycle-1"); n != 1 {
		t.Fatalf("the mail is filed under %d projects, want 1: the cycle was kept as a fault, not rerun", n)
	}
	if faults := e.WsCount(t, `SELECT count(*) FROM system_log
		WHERE action = 'capture_project_attribution_fault' AND detail->>'source_id' = 'cycle-1'`); faults != 0 {
		t.Errorf("a lock cycle was recorded as %d fault(s), want none: the capture reruns instead", faults)
	}
}
