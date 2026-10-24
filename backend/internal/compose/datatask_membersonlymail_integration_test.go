// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleagueMailFixture is mail captured before capture held mail between
// colleagues: each row is put back to the open state that capture wrote then.
type colleagueMailFixture struct {
	e                                    *integration.Env
	colleagues, unstored, shared, manual ids.UUID
	outside                              ids.UUID
}

func colleagueMessage(key, from, to string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to, "Subject: " + key,
		"Date: " + time.Now().UTC().AddDate(0, 0, -3).Format(time.RFC1123Z),
		"Message-ID: <" + key + "@members-task.test>",
		"Content-Type: text/plain; charset=utf-8", "", "About " + key + ".", "",
	}, "\r\n"))
}

func newColleagueMailFixture(t *testing.T) colleagueMailFixture {
	t.Helper()
	e := integration.Setup(t)
	f := colleagueMailFixture{e: e}
	self := sharedMailbox(t, e, e.Rep1)
	colleague := "b@authz.test"
	for key, to := range map[string]string{
		"colleagues": colleague, "unstored": colleague, "shared": colleague, "manual": colleague,
		"outside": "dana@client.test",
	} {
		captureMailVia(t, e, e.Rep1, "gmail", colleagueMessage(key, self, to))
	}
	f.colleagues, f.unstored = activityBySubject(t, e, "colleagues"), activityBySubject(t, e, "unstored")
	f.shared, f.manual = activityBySubject(t, e, "shared"), activityBySubject(t, e, "manual")
	f.outside = activityBySubject(t, e, "outside")
	for _, id := range []ids.UUID{f.colleagues, f.unstored, f.shared, f.manual} {
		e.WsExec(t, `UPDATE activity SET audience = 'workspace', audience_reason = NULL WHERE id = $1`, id)
		e.WsExec(t, `UPDATE capture_import SET posture_at_import = 'shared', verdict_reason = NULL, verdict_reasons = NULL
		              WHERE activity_id = $1`, id)
	}
	e.WsExec(t, `UPDATE activity SET audience_reason = 'manual' WHERE id = $1`, f.manual)
	e.WsExec(t, `UPDATE capture_import SET verdict_status = 'shared_by_owner' WHERE activity_id = $1`, f.shared)
	// Retention erased this original, so the stamped participants stand in.
	e.WsExec(t, `WITH gone AS (UPDATE activity SET raw_capture_id = NULL WHERE id = $1
	              RETURNING source_system, source_id)
	              DELETE FROM raw_capture rc USING gone
	               WHERE rc.source_system = gone.source_system AND rc.source_id = gone.source_id`, f.unstored)
	return f
}

func (f colleagueMailFixture) audience(t *testing.T, id ids.UUID) (string, string) {
	t.Helper()
	var audience string
	var reason *string
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT audience, audience_reason FROM activity WHERE id = $1`, id).Scan(&audience, &reason)
	}); err != nil {
		t.Fatal(err)
	}
	if reason == nil {
		return audience, ""
	}
	return audience, *reason
}

func (f colleagueMailFixture) audienceWrites(t *testing.T, id ids.UUID) (audits, events int) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
			 WHERE entity_type = 'activity' AND entity_id = $1 AND after ? 'audience'`, id).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox
			 WHERE envelope #>> '{entity,id}' = $1::text AND envelope #>> '{type}' = 'activity.updated'`, id).Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

func TestTheMembersOnlyTaskHoldsColleaguesMailOnceAndLeavesPeoplesDecisions(t *testing.T) {
	f := newColleagueMailFixture(t)
	outsideBefore, _ := f.audience(t, f.outside)
	type writes struct{ audits, events int }
	before := map[ids.UUID]writes{}
	for _, id := range []ids.UUID{f.colleagues, f.unstored} {
		audits, events := f.audienceWrites(t, id)
		before[id] = writes{audits, events}
	}
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, f.e, membersOnlyMailDataTask())

	for name, id := range map[string]ids.UUID{"from its original": f.colleagues, "from its stamped parties": f.unstored} {
		if got, reason := f.audience(t, id); got != "participants" || reason != activities.ReasonMembersOnly {
			t.Errorf("colleagues' mail judged %s is %q / %q, want participants / members_only", name, got, reason)
		}
		audits, events := f.audienceWrites(t, id)
		if audits-before[id].audits != 1 || events-before[id].events != 1 {
			t.Errorf("holding colleagues' mail judged %s wrote %d audit row(s) and %d event(s), want 1 and 1",
				name, audits-before[id].audits, events-before[id].events)
		}
	}
	if got, reason := f.audience(t, f.manual); got != "workspace" || reason != activities.ReasonManual {
		t.Errorf("a human's decision moved to %q / %q", got, reason)
	}
	if got, _ := f.audience(t, f.shared); got != "workspace" {
		t.Errorf("mail its owner shared moved to %q", got)
	}
	if got, _ := f.audience(t, f.outside); got != outsideBefore {
		t.Errorf("mail to an outside party moved from %q to %q", outsideBefore, got)
	}

	step := membersOnlyMailDataTask().Steps[0]
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a second run changed %d row(s)", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheMembersOnlyTaskRollsBackWithItsBatch(t *testing.T) {
	f := newColleagueMailFixture(t)
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	step := membersOnlyMailDataTask().Steps[0]
	rollback := errors.New("roll back after the write")
	err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 2 {
			t.Errorf("the batch held %d row(s), want the two colleagues' mails", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got, _ := f.audience(t, f.colleagues); got != "workspace" {
		t.Errorf("a rolled-back batch left the mail %q", got)
	}
	count, more, err := step.Estimate(ctx, f.e.Pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !more {
		t.Errorf("the estimate capped at 1 reads (%d, more=%v), want (1, true)", count, more)
	}
}
