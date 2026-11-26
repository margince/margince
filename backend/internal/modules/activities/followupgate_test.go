// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The follow-up statements read their row, a later contact and a reply that
// lifts a snooze. Postgres plans every copy of the content gate a statement
// carries, and a pooled connection keeps that memory, so each statement gates
// those activities once. These tests pin it.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// statementTx keeps the statement a read sends and fails it, so the read is
// rendered without a database.
type statementTx struct {
	pgx.Tx
	sql string
}

var errRendered = errors.New("rendered")

func (tx *statementTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	tx.sql = sql
	return nil, errRendered
}

func TestTheAwaitingRepliesStatementCarriesOneContentGate(t *testing.T) {
	tx := &statementTx{}
	if _, err := (&Store{}).queryAwaitingReplies(teamScopedCtx(), tx, clocktest.Now(t), 3, nil); !errors.Is(err, errRendered) {
		t.Fatalf("rendering the awaiting-replies read: %v", err)
	}
	if got := auth.ContentGateCopies(tx.sql); got != 1 {
		t.Fatalf("the awaiting-replies read carries %d content gates, want 1", got)
	}
}

func TestTheMeetingFollowUpStatementCarriesOneContentGate(t *testing.T) {
	everyMeeting := everyMeetingFollowUp
	sql, _, err := meetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting, AwaitingReplyScanCap)
	if err != nil {
		t.Fatalf("rendering the meeting follow-up read: %v", err)
	}
	if got := auth.ContentGateCopies(sql); got != 1 {
		t.Fatalf("the meeting follow-up read carries %d content gates, want 1", got)
	}
}

func TestTheMeetingFollowUpMaterializesRecipientsBeforeCheckingLaterActivities(t *testing.T) {
	everyMeeting := everyMeetingFollowUp
	query, _, err := meetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting, AwaitingReplyScanCap)
	if err != nil {
		t.Fatal(err)
	}
	normalized := strings.Join(strings.Fields(query), " ")
	// The fence prevents recipient links from being scanned once per later
	// activity, and each later activity is read by its key alone.
	for _, part := range []string{
		"NOT EXISTS (WITH touched AS MATERIALIZED (",
		"SELECT 1 FROM touched CROSS JOIN LATERAL (SELECT keyed.id,",
		"FROM activity keyed WHERE keyed.id = touched.activity_id LIMIT 1) later",
	} {
		if !strings.Contains(normalized, part) {
			t.Fatalf("meeting recipients are not materialized before checking later activities: missing %q", part)
		}
	}
}

func TestAMeetingFollowUpPageOfZeroHoldsNone(t *testing.T) {
	everyMeeting := everyMeetingFollowUp
	const newestFirst = "ORDER BY a.occurred_at DESC, a.id DESC"
	paged, _, err := meetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.Join(strings.Fields(paged), " "), newestFirst+" LIMIT 0") {
		t.Error("a page of zero meetings is not LIMIT 0")
	}
	if _, _, err := meetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting, -1); err == nil {
		t.Error("a negative page was rendered instead of refused")
	}
	unpaged, _, err := unpagedMeetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.Join(strings.Fields(unpaged), " "), newestFirst) {
		t.Error("the unpaged read carries a page")
	}
}

// The deal read's per-meeting work runs only on the asked deals' meetings.
func TestTheDealFollowUpReadNarrowsItsCandidatesToTheAskedDeals(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "deal": {Read: true},
		}, RowScope: principal.RowScopeAll},
	})
	asked := []ids.UUID{ids.NewV7()}
	sql, args, err := meetingFollowUpsByDealStatement(ctx, clocktest.Now(t), 3, asked)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(sql, followUpCandidates+" AS MATERIALIZED (")
	end := strings.Index(sql, followUpReadable+" AS MATERIALIZED (")
	if start < 0 || end < start {
		t.Fatalf("the statement has no candidate set ahead of the readable set:\n%.300s", sql)
	}
	position := slices.IndexFunc(args, func(v any) bool {
		deals, ok := v.([]ids.UUID)
		return ok && slices.Equal(deals, asked)
	})
	want := fmt.Sprintf("asked.deal_id = ANY($%d)", position+1)
	if position < 0 || !strings.Contains(sql[start:end], want) {
		t.Errorf("the candidate set does not hold %q, so every meeting the reader held is judged:\n%s", want, sql[start:end])
	}
}

// A caller refused either object learns nothing and costs no SQL.
func TestTheDealFollowUpBatchRefusesBeforeReadingAnything(t *testing.T) {
	grants := func(objects ...string) context.Context {
		held := map[string]principal.ObjectGrant{}
		for _, object := range objects {
			held[object] = principal.ObjectGrant{Read: true}
		}
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
			Permissions: principal.Permissions{RoleKeys: []string{"rep"}, Objects: held, RowScope: principal.RowScopeAll},
		})
	}
	for name, ctx := range map[string]context.Context{"no deal read": grants("activity"), "no activity read": grants("deal")} {
		tx := &statementTx{}
		_, err := (&Store{}).NextMeetingFollowUpsByDealTx(ctx, tx, clocktest.Now(t), []ids.UUID{ids.NewV7()})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: answered %v, want permission denied", name, err)
		}
		if tx.sql != "" {
			t.Errorf("%s: sent SQL before refusing: %.80s", name, tx.sql)
		}
	}
}
