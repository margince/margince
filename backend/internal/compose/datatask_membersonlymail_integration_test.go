// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
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
	outside, crowd, crowdUnstored        ids.UUID
	crowdOfOutsiders, corrupt            ids.UUID
}

func colleagueMessage(at time.Time, key, from, to, cc string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to, "Cc: " + cc, "Subject: " + key,
		"Date: " + at.AddDate(0, 0, -3).Format(time.RFC1123Z),
		"Message-ID: <" + key + "@members-task.test>",
		"Content-Type: text/plain; charset=utf-8", "", "About " + key + ".", "",
	}, "\r\n"))
}

// addresses names count parties at a domain, seating each as a member when
// seated is set.
func addresses(t *testing.T, e *integration.Env, prefix, domain string, count int, seated bool) string {
	t.Helper()
	out := make([]string, 0, count)
	for i := range count {
		address := fmt.Sprintf("%s%02d@%s", prefix, i, domain)
		if seated {
			e.WsExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`, ids.NewV7(), address)
		}
		out = append(out, address)
	}
	return strings.Join(out, ", ")
}

func newColleagueMailFixture(t *testing.T) colleagueMailFixture {
	t.Helper()
	e := integration.Setup(t)
	f := colleagueMailFixture{e: e}
	self := sharedMailbox(t, e, e.Rep1)
	colleague := "b@authz.test"
	// 48 more seats beside Rep2: 49 colleagues, under the participant cap.
	team := addresses(t, e, "team", "authz.test", 48, true)
	// Past the cap capture stamps none of these, so the stored rows name
	// only the two ends while outsiders were on the mail.
	crowd := addresses(t, e, "crowd", "authz.test", 30, false) + ", " + addresses(t, e, "guest", "client.test", 25, false)
	for key, to := range map[string][2]string{
		"colleagues": {colleague, ""}, "unstored": {colleague, ""}, "shared": {colleague, ""},
		"manual": {colleague, ""}, "outside": {"dana@client.test", ""},
		"crowd": {colleague, team}, "corrupt": {colleague, ""}, "crowd-unstored": {colleague, team}, "crowd-of-outsiders": {colleague, crowd},
	} {
		captureMailVia(t, e, e.Rep1, "gmail", colleagueMessage(clocktest.Now(t), key, self, to[0], to[1]))
	}
	f.colleagues, f.unstored = activityBySubject(t, e, "colleagues"), activityBySubject(t, e, "unstored")
	f.shared, f.manual = activityBySubject(t, e, "shared"), activityBySubject(t, e, "manual")
	f.outside, f.crowd = activityBySubject(t, e, "outside"), activityBySubject(t, e, "crowd")
	f.crowdUnstored, f.crowdOfOutsiders = activityBySubject(t, e, "crowd-unstored"), activityBySubject(t, e, "crowd-of-outsiders")
	f.corrupt = activityBySubject(t, e, "corrupt")
	for _, id := range []ids.UUID{f.corrupt, f.colleagues, f.unstored, f.shared, f.manual, f.crowd, f.crowdUnstored, f.crowdOfOutsiders} {
		e.WsExec(t, `UPDATE activity SET audience = 'workspace', audience_reason = NULL WHERE id = $1`, id)
		e.WsExec(t, `UPDATE capture_import SET posture_at_import = 'shared', verdict_reason = NULL, verdict_reasons = NULL
		              WHERE activity_id = $1`, id)
	}
	e.WsExec(t, `UPDATE activity SET audience_reason = 'manual' WHERE id = $1`, f.manual)
	e.WsExec(t, `UPDATE capture_import SET verdict_status = 'shared_by_owner' WHERE activity_id = $1`, f.shared)
	// Retention erased these originals; only the stamped rows are left.
	for _, id := range []ids.UUID{f.unstored, f.crowdUnstored, f.crowdOfOutsiders} {
		e.WsExec(t, `WITH gone AS (UPDATE activity SET raw_capture_id = NULL WHERE id = $1
		              RETURNING source_system, source_id)
		              DELETE FROM raw_capture rc USING gone
		               WHERE rc.source_system = gone.source_system AND rc.source_id = gone.source_id`, id)
	}
	e.WsExec(t, `UPDATE raw_capture rc SET payload = to_jsonb('not a message'::text)
	              FROM activity a WHERE a.id = $1 AND `+storedMailOriginal, f.corrupt)
	return f
}

// stepStatus is where one of the task's steps stands.
func (f colleagueMailFixture) stepStatus(t *testing.T, step string) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status FROM data_task_run WHERE task_id = $1 AND step = $2`,
			membersOnlyMailDataTask().ID, step).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	return status
}

// unknownParties is what the task reported as mail it could not judge.
func (f colleagueMailFixture) unknownParties(t *testing.T) int64 {
	t.Helper()
	var scanned int64
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT scanned FROM data_task_run
			 WHERE task_id = $1 AND step = 'count_mail_with_unknown_parties'`,
			membersOnlyMailDataTask().ID).Scan(&scanned)
	}); err != nil {
		t.Fatal(err)
	}
	return scanned
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

func TestTheMembersOnlyTaskHoldsColleaguesMailOnceAndLeavesHumanDecisions(t *testing.T) {
	f := newColleagueMailFixture(t)
	outsideBefore, _ := f.audience(t, f.outside)
	type writes struct{ audits, events int }
	before := map[ids.UUID]writes{}
	for _, id := range []ids.UUID{f.colleagues, f.crowd} {
		audits, events := f.audienceWrites(t, id)
		before[id] = writes{audits, events}
	}
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, f.e, membersOnlyMailDataTask())

	for name, id := range map[string]ids.UUID{"two colleagues": f.colleagues, "49 colleagues": f.crowd} {
		if got, reason := f.audience(t, id); got != "participants" || reason != activities.ReasonMembersOnly {
			t.Errorf("mail among %s is %q / %q, want participants / members_only", name, got, reason)
		}
		audits, events := f.audienceWrites(t, id)
		if audits-before[id].audits != 1 || events-before[id].events != 1 {
			t.Errorf("holding mail among %s wrote %d audit row(s) and %d event(s), want 1 and 1",
				name, audits-before[id].audits, events-before[id].events)
		}
	}
	for name, id := range map[string]ids.UUID{
		"colleagues' mail without its original":                    f.unstored,
		"49 colleagues' mail without its original":                 f.crowdUnstored,
		"mail whose crowded list capture dropped, outsiders on it": f.crowdOfOutsiders,
	} {
		if got, _ := f.audience(t, id); got != "workspace" {
			t.Errorf("%s was judged on an incomplete party list: %q", name, got)
		}
	}
	if got, _ := f.audience(t, f.corrupt); got != "workspace" {
		t.Errorf("mail whose original does not parse was judged anyway: %q", got)
	}
	if got := f.stepStatus(t, "hold_members_only_mail"); got != datatask.Done {
		t.Errorf("one unreadable original left the hold step %q, want it done", got)
	}
	if got := f.unknownParties(t); got != 3 {
		t.Errorf("the task reports %d mail(s) with unknown parties, want the 3 without an original", got)
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
			t.Errorf("the batch held %d row(s), want the two colleagues' mails with an original", out.Changed)
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
