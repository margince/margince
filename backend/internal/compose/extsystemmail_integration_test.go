// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The mail read over real Postgres, from a subscription delivery.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// mail files one email on the deal and answers its id.
func (s *systemReadEnv) mail(t *testing.T, direction, audience string, machine bool, at time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, direction, audience, audience_reason, sender_is_machine)
		VALUES ($1, 'email', 'Quote', $2, 'system', 'system', $3, $4, CASE WHEN $4 = 'workspace' THEN NULL ELSE 'manual' END, $5)`,
		id, at, direction, audience, machine)
	s.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, s.deal)
	return id
}

func (s *systemReadEnv) dealMail(t *testing.T, q extension.MailQuery) extension.Page[extension.MailSummary] {
	t.Helper()
	var got extension.Page[extension.MailSummary]
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Activities().Mail(ctx, extension.UserID(s.Rep1.String()), s.deal.String(), q)
		return err
	})
	return got
}

func TestTheMailReadHoldsOnlyWhatTheWholeTeamMaySee(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadActivities)
	now := clocktest.Now(t).UTC().Truncate(time.Microsecond)
	sent := s.mail(t, "outbound", "workspace", false, now.Add(-3*time.Hour))
	s.mail(t, "inbound", "participants", false, now.Add(-2*time.Hour))
	automatic := s.mail(t, "inbound", "workspace", true, now.Add(-time.Hour))
	archived := s.mail(t, "outbound", "workspace", false, now.Add(-4*time.Hour))
	s.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)

	got := s.dealMail(t, extension.MailQuery{})
	if len(got.Items) != 2 || got.Items[0].ID != automatic.String() || got.Items[1].ID != sent.String() {
		t.Fatalf("mail = %+v, want the automatic one then the sent one, and not the private or the archived", got.Items)
	}
	if got.Items[0].AutomaticKind == "" || got.Items[1].AutomaticKind != "" {
		t.Errorf("automatic kinds = %q and %q, want the first marked and the second not", got.Items[0].AutomaticKind, got.Items[1].AutomaticKind)
	}
	if got.Items[1].Direction != "outbound" || got.Items[1].DealID != s.deal.String() || !got.Items[1].OccurredAt.Equal(now.Add(-3*time.Hour)) {
		t.Errorf("the sent mail = %+v, want outbound, at its time, on the deal", got.Items[1])
	}
	if withArchived := s.dealMail(t, extension.MailQuery{IncludeArchived: true}); len(withArchived.Items) != 3 {
		t.Errorf("asking for the archived gave %d emails, want 3", len(withArchived.Items))
	}
}

func TestADealWithMoreMailThanAPageIsReadOverSeveralPages(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadActivities)
	now := clocktest.Now(t).UTC().Truncate(time.Microsecond)
	want := map[string]bool{}
	for i := range 5 {
		want[s.mail(t, "outbound", "workspace", false, now.Add(-time.Duration(i)*time.Minute)).String()] = true
	}
	q := extension.MailQuery{Limit: 2}
	for pages := 0; ; pages++ {
		if pages > 3 {
			t.Fatal("the read did not stop paging")
		}
		got := s.dealMail(t, q)
		for _, item := range got.Items {
			if !want[item.ID] {
				t.Errorf("email %s came back twice or is not the deal's", item.ID)
			}
			delete(want, item.ID)
		}
		if got.NextCursor == "" {
			break
		}
		q.Cursor = got.NextCursor
	}
	if len(want) != 0 {
		t.Errorf("%d emails never came back over the pages", len(want))
	}
}

func TestTheMailReadIsAnsweredUnderTheNamedMembersOwnAccessToTheDeal(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadActivities)
	s.mail(t, "outbound", "workspace", false, clocktest.Now(t).UTC().Add(-time.Hour))
	s.WsExec(t, `UPDATE role SET permissions = '{"objects":{"company":{"read":true}},"row_scope":"all"}' WHERE id IN (SELECT role_id FROM role_assignment WHERE user_id = $1)`, s.Rep3)
	var err error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, err = rt.Activities().Mail(ctx, extension.UserID(s.Rep3.String()), s.deal.String(), extension.MailQuery{})
		return nil
	})
	if !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("Mail for a member who cannot open the deal = %v, want ErrNotFound", err)
	}
}

func TestEverySystemReadLeavesOneTraceRow(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadActivities)
	s.dealMail(t, extension.MailQuery{})
	var rows int
	s.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM system_log WHERE action = 'extension_system_read'
			AND detail->>'extension' = 'dealmirror' AND detail->>'read' = 'activities' AND detail->>'record_id' = $1`, s.deal.String()).Scan(&rows)
	})
	if rows != 1 {
		t.Errorf("a read left %d trace rows, want 1", rows)
	}
}
