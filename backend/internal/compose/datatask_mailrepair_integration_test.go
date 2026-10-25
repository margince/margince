// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail stored by an older parser keeps its mistakes until the data task reads
// the stored original again. One mistake is a classifier label read from a body
// of only the From/To lines. The other is a subject or body stored before
// legacy charsets were decoded.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (o *owedEnv) label(t *testing.T, message ids.UUID) string {
	t.Helper()
	return o.e.WsScalar(t, `SELECT coalesce(capture_label, '') FROM activity WHERE id = $1`, message)
}

// labelled puts the label on the mail through the classifier's own writer, then
// stamps it as of the given time.
func (o *owedEnv) labelled(t *testing.T, message ids.UUID, label string, at time.Time) {
	t.Helper()
	if applied, err := o.e.Activities.SetCaptureLabel(o.judge(), message, label); err != nil || !applied {
		t.Fatalf("labelling %v: applied=%v err=%v", message, applied, err)
	}
	o.e.WsExec(t, `UPDATE activity SET capture_labeled_at = $2 WHERE id = $1`, message, at)
}

func (o *owedEnv) inClassifyBacklog(t *testing.T, message ids.UUID) bool {
	t.Helper()
	backlog, err := o.e.Activities.UnlabeledCaptureEmails(o.judge(), "old-rows", 100, 400)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range backlog {
		if candidate.ID == message {
			return true
		}
	}
	return false
}

// stepWalk runs one step of the registered task to its end at one row per
// batch. It answers the estimate taken first and the rows scanned and changed.
func stepWalk(ctx context.Context, t *testing.T, o *owedEnv, step datatask.Step) (estimate int64, scanned, changed int) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		if estimate, _, err = step.Estimate(ctx, tx, 1000); err != nil {
			return err
		}
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		for {
			out, err := step.Batch(ctx, tx, in)
			if err != nil {
				return err
			}
			scanned += out.Scanned
			changed += out.Changed
			if out.Done {
				return nil
			}
			in.Cursor = out.Cursor
		}
	}); err != nil {
		t.Fatalf("walking %s: %v", step.Name, err)
	}
	return estimate, scanned, changed
}

func stepNamed(t *testing.T, task datatask.Task, name string) datatask.Step {
	t.Helper()
	for _, step := range task.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("the task has no step %q", name)
	return datatask.Step{}
}

func TestARestoredMailIsClassifiedAgainFromItsFullText(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := "Can we meet on Thursday?"
	headerOnly := "From: pat@customer.example\nTo: " + o.seat
	mail := func(id string, hours time.Duration) ids.UUID {
		return o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat, subject: "Meeting", messageID: id + "@customer.example",
			body: readInFull(ask), encoding: "base64", authenticated: true, at: o.now.Add(-hours * time.Hour),
		})
	}
	lost, restoredEarlier, relabelled, whole := mail("relabel-lost", 50), mail("relabel-old", 49), mail("relabel-new", 48), mail("relabel-whole", 47)
	o.e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'real' WHERE email = 'pat@customer.example'`)
	for _, id := range []ids.UUID{lost, restoredEarlier, relabelled} {
		o.asLegacy(t, id, headerOnly)
	}
	o.labelled(t, lost, "noise", o.now.Add(-time.Hour))
	o.labelled(t, whole, "noise", o.now.Add(-time.Hour))
	o.e.WsExec(t, `UPDATE activity SET capture_label_declined_at = now(), capture_label_declined_ruleset = 'old-rows' WHERE id = $1`, lost)
	if o.inClassifyBacklog(t, lost) {
		t.Fatal("the labelled mail is already in the classify backlog")
	}

	// restoredEarlier and relabelled were restored by the task as it was
	// before it reset labels, which left the label the empty body earned.
	task := o.runMailBodyTask(t)
	o.labelled(t, restoredEarlier, "noise", o.now)
	o.labelled(t, relabelled, "meeting", o.now)
	o.e.WsExec(t, `UPDATE activity SET capture_labeled_at = now() - interval '1 day' WHERE id = $1`, restoredEarlier)
	o.e.WsExec(t, `UPDATE activity SET capture_labeled_at = now() + interval '1 hour' WHERE id = $1`, relabelled)

	if got := o.label(t, lost); got != "" {
		t.Errorf("a restored mail still carries the label %q", got)
	}
	if !o.inClassifyBacklog(t, lost) {
		t.Error("a restored mail was not offered to the next classify pass")
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND capture_label_declined_at IS NULL AND capture_labeled_at IS NULL`, lost); n != 1 {
		t.Error("the restoration kept a label stamp or a decline read from the empty body")
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND before->>'capture_label' = 'noise' AND after ? 'capture_label'`, lost); n != 1 {
		t.Errorf("the restoration audit rows naming the dropped label = %d, want 1", n)
	}

	step := stepNamed(t, task, "reset_labels_read_from_short_bodies")
	ctx := principal.SystemActing(o.e.Admin(), "system:datatask-test")
	if estimate, scanned, changed := stepWalk(ctx, t, o, step); estimate != 1 || scanned != 1 || changed != 1 {
		t.Fatalf("estimate %d, scanned %d, reset %d; want the 1 earlier restoration each", estimate, scanned, changed)
	}
	if got := o.label(t, restoredEarlier); got != "" {
		t.Errorf("a mail restored before the reset still carries the label %q", got)
	}
	if got := o.label(t, relabelled); got != "meeting" {
		t.Errorf("a label set after the restoration became %q; it read the full text", got)
	}
	if got := o.label(t, whole); got != "noise" {
		t.Errorf("a mail that was never restored became %q", got)
	}
	if estimate, _, changed := stepWalk(ctx, t, o, step); estimate != 0 || changed != 0 {
		t.Errorf("a second run estimated %d and reset %d", estimate, changed)
	}
	assertMailBodyReplayChangesNothing(t, o, task)
}

func (o *owedEnv) damaged(t *testing.T, message ids.UUID, subject, body string) {
	t.Helper()
	o.e.WsExec(t, `UPDATE activity SET subject = $2, body = $3, body_completeness = 'complete' WHERE id = $1`, message, subject, body)
}

func TestMailStoredBeforeTheCharsetFixIsReadAgainFromItsOriginal(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	const encoded = "=?Windows-1252?Q?Angebot_f=FCr_Sie?="
	prefix := "From: pat@customer.example\nTo: " + o.seat + "\n\n"
	mail := func(id, subject, charset string) ids.UUID {
		return o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat, subject: subject, messageID: id + "@customer.example",
			body: "Mit freundlichen Gr=FC=DFen", encoding: "quoted-printable", charset: charset, at: o.now.Add(-2 * time.Hour),
		})
	}
	damaged, edited, current := mail("cs-damaged", encoded, "windows-1252"), mail("cs-edited", encoded, "windows-1252"), mail("cs-current", "Hallo", "windows-1252")
	o.damaged(t, damaged, encoded, prefix+"Mit freundlichen Gr??en")
	o.damaged(t, edited, encoded, prefix+"Mit freundlichen Gr??en")
	if _, err := o.e.Activities.UpdateActivity(o.e.Admin(), ids.From[ids.ActivityKind](edited),
		activities.UpdateActivityInput{Body: new(prefix + "Mit freundlichen Gr??en, Pat")}); err != nil {
		t.Fatal(err)
	}
	o.damaged(t, edited, encoded, prefix+"Mit freundlichen Gr??en")
	var noOriginal ids.UUID
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `INSERT INTO activity (kind, subject, body, direction, occurred_at,
			source_system, source_id, source, captured_by, body_completeness)
			VALUES ('email', $1, $2, 'inbound', $3, 'email', 'cs-none@customer.example', 'gmail:seed', 'connector:gmail', 'complete')
			RETURNING id`, encoded, prefix+"Gr??en", o.now).Scan(&noOriginal)
	}); err != nil {
		t.Fatal(err)
	}
	o.labelled(t, damaged, "noise", o.now.Add(-time.Hour))
	if declined, err := o.e.Activities.MarkOwedVerdictDeclined(o.judge(), damaged, "old-rows"); err != nil || !declined {
		t.Fatalf("declining the verdict: declined=%v err=%v", declined, err)
	}
	subjectless := mail("cs-nosubject", "", "windows-1252")
	o.damaged(t, subjectless, "", prefix+"Mit freundlichen Gr??en")
	o.e.WsExec(t, `UPDATE activity SET subject = NULL WHERE id = $1`, subjectless)
	before := o.stored(t, current)
	currentVersion := o.e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, current)

	task := o.runMailBodyTask(t)

	if subject := o.e.WsScalar(t, `SELECT subject FROM activity WHERE id = $1`, damaged); subject != "Angebot für Sie" {
		t.Errorf("subject = %q, want it decoded", subject)
	}
	if got := o.stored(t, damaged); got.body != prefix+"Mit freundlichen Grüßen" || got.completeness != "complete" {
		t.Errorf("body = %q (%q), want the decoded text", got.body, got.completeness)
	}
	if got := o.label(t, damaged); got != "" {
		t.Errorf("the label %q was read from the damaged text", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND owed_verdict_declined_at IS NULL AND owed_verdict_declined_ruleset IS NULL`, damaged); n != 1 {
		t.Error("a decline read from the damaged body still keeps the mail out of the verdict pass")
	}
	if got := o.stored(t, subjectless); got.body != prefix+"Mit freundlichen Grüßen" {
		t.Errorf("body of a mail with no subject = %q, want it decoded", got.body)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND subject IS NULL`, subjectless); n != 1 {
		t.Error("the repair gave a mail with no subject one")
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after @> '{"body_restored": true, "subject_restored": true}'`, damaged); n != 1 {
		t.Errorf("repairs audited = %d, want 1", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = $1 AND a.after @> '{"subject_restored": true}'`, damaged); n != 1 {
		t.Errorf("repair events = %d, want 1", n)
	}
	if subject := o.e.WsScalar(t, `SELECT subject FROM activity WHERE id = $1`, edited); subject != encoded {
		t.Errorf("the subject of a mail whose body a member edited became %q", subject)
	}
	if got := o.stored(t, edited); got.body != prefix+"Mit freundlichen Gr??en" {
		t.Errorf("a body a member edited was rewritten to %q", got.body)
	}
	if subject := o.e.WsScalar(t, `SELECT subject FROM activity WHERE id = $1`, noOriginal); subject != encoded {
		t.Errorf("a mail with no stored original became %q", subject)
	}
	if o.e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, current) != currentVersion || o.stored(t, current).body != before.body {
		t.Error("a mail the parser had read correctly was written")
	}

	step := stepNamed(t, task, "repair_mail_charsets")
	ctx := principal.SystemActing(o.e.Admin(), "system:datatask-test")
	estimate, scanned, changed := stepWalk(ctx, t, o, step)
	if int(estimate) != scanned || changed != 0 {
		t.Errorf("a second run estimated %d, scanned %d and changed %d; want the estimate to match the walk and no writes", estimate, scanned, changed)
	}
	assertMailBodyReplayChangesNothing(t, o, task)
}
