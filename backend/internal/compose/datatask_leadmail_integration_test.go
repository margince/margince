// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asLeadReadingSeat gives the capturing seat a real role that reads leads, so
// the task resolves the seat's live authority the way it does in production.
func asLeadReadingSeat(t *testing.T, l *leadMailEnv) {
	t.Helper()
	l.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'rep'`, l.owner)
}

// leadFilingPass runs every batch at limit 1 on one transaction, rolled back
// when rollback is set, and answers the activities changed.
func leadFilingPass(ctx context.Context, t *testing.T, l *leadMailEnv, rollback bool) (changed int) {
	t.Helper()
	undo := errors.New("rolled back")
	step := leadMailDataTask(l.e.Pool).Steps[0]
	err := database.WithWorkspaceTx(ctx, l.e.Pool, func(tx pgx.Tx) error {
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		for {
			out, err := step.Batch(ctx, tx, in)
			if err != nil {
				return err
			}
			changed += out.Changed
			if out.Done {
				break
			}
			in.Cursor = out.Cursor
		}
		if rollback {
			return undo
		}
		return nil
	})
	if err != nil && (!rollback || !errors.Is(err, undo)) {
		t.Fatalf("running the lead filing: %v", err)
	}
	return changed
}

func runLeadMailBatch(ctx context.Context, l *leadMailEnv, in datatask.BatchIn) (datatask.BatchOut, error) {
	var out datatask.BatchOut
	err := database.WithWorkspaceTx(ctx, l.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = leadMailDataTask(l.e.Pool).Steps[0].Batch(ctx, tx, in)
		return err
	})
	return out, err
}

func estimateLeadFiling(ctx context.Context, t *testing.T, l *leadMailEnv, maximum int64) (estimated int64, more bool) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, l.e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, more, err = capture.EstimateLeadFiling(ctx, tx, maximum)
		return err
	}); err != nil {
		t.Fatalf("estimating the lead filing: %v", err)
	}
	return estimated, more
}

func TestTheLeadFilingTaskFilesEarlierMailOnceAndReplaysSafely(t *testing.T) {
	l := setupLeadMail(t)
	ctx := principal.SystemActing(l.e.Admin(), "system:datatask-test")
	// Captured while no lead held the address, so capture filed it nowhere.
	earlier := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Our requirements"}, true)
	meeting := l.meeting(t, "lena@prospect.example")
	notice := l.mail(t, owedMail{from: "dse@eu.docusign.net", to: l.seat, subject: "Please sign"}, true)
	// The same judgement, each time with one of its two records gone: the
	// ladder's trace swept, or the hold replaced by a later audience choice.
	swept := l.mail(t, owedMail{from: "dse@eu.docusign.net", to: l.seat, subject: "Reminder", messageID: "swept@docusign.net"}, true)
	l.e.WsExec(t, `DELETE FROM capture_trace WHERE source_id = 'swept@docusign.net'`)
	reopened := l.mail(t, owedMail{from: "dse@eu.docusign.net", to: l.seat, subject: "Completed"}, true)
	l.e.WsExec(t, `UPDATE activity SET audience = 'workspace', audience_reason = 'manual' WHERE id = $1`, reopened)
	history := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Long ago", at: l.at.AddDate(0, 0, -30)}, true)
	roleMail := l.mail(t, owedMail{from: "info@prospect.example", to: l.seat, subject: "Enquiry", at: l.at.AddDate(0, 0, -30)}, true)
	l.moveAddress(t, "info@prospect.example", "bob@prospect.example")
	carol := l.lead(t, "info@prospect.example")
	lead := l.lead(t, "lena@prospect.example")
	infrastructure := l.lead(t, "dse@eu.docusign.net")
	asLeadReadingSeat(t, l)

	if _, err := runLeadMailBatch(ctx, l, datatask.BatchIn{Cursor: json.RawMessage(`{`), Limit: 10, RunID: ids.NewV7()}); err == nil {
		t.Fatal("a malformed cursor was accepted")
	}
	if estimated, more := estimateLeadFiling(ctx, t, l, 1); estimated != 1 || !more {
		t.Fatalf("the estimate under a cap of 1 = %d (more %t); want 1 and more to come", estimated, more)
	}
	if estimated, more := estimateLeadFiling(ctx, t, l, 1000); estimated != 3 || more {
		t.Fatalf("the estimate = %d (more %t); want the earlier mail, the meeting and the lead's history", estimated, more)
	}
	if changed := leadFilingPass(ctx, t, l, true); changed != 3 || l.filedUnderLead(t, earlier, lead) {
		t.Fatalf("a rolled-back pass changed %d activities and left the link: %t", changed, l.filedUnderLead(t, earlier, lead))
	}

	task := leadMailDataTask(l.e.Pool)
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, l.e, task)

	for name, activity := range map[string]ids.UUID{"the earlier mail": earlier, "the earlier meeting": meeting, "mail from before the lead": history} {
		if !l.filedUnderLead(t, activity, lead) {
			t.Errorf("the task did not file %s under the lead", name)
		}
	}
	for name, activity := range map[string]ids.UUID{"held": notice, "with its trace swept": swept, "reopened by hand": reopened} {
		if l.filedUnderLead(t, activity, infrastructure) {
			t.Errorf("the task filed a suppressed message, %s, under a lead holding its sender's address", name)
		}
	}
	if l.filedUnderLead(t, roleMail, carol) {
		t.Error("the task filed mail to an address another lead held then under the lead holding it now")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = $1
		AND after->>'entity_id' = $2::text`, earlier, lead.UUID); n != 1 {
		t.Errorf("the filing wrote %d relink audit rows, want one", n)
	}
	if estimated, _ := estimateLeadFiling(ctx, t, l, 1000); estimated != 0 {
		t.Errorf("the estimate after the task = %d, want nothing left to file", estimated)
	}
	if changed := leadFilingPass(ctx, t, l, false); changed != 0 {
		t.Fatalf("a second pass changed %d activities, want none", changed)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE lead_id = $1`, lead); n != 3 {
		t.Errorf("the lead holds %d links after the replay, want each of its three activities once", n)
	}
}
