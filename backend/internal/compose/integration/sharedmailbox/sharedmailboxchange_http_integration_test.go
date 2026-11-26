// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestMovingASharedMailboxToOtherReadersNeedsTheAdminsConfirm(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connect(t, e.Team1)
	mail := s.deliverToBoth(t, ids.UUID(box.Id))
	path := "/shared-mailboxes/" + box.Id.String()
	move := map[string]any{"reader_team_id": e.Team2}

	if rec := s.call(s.admin(), t, http.MethodPatch, path, move); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "confirm_required") {
		t.Fatalf("an unconfirmed move answered %d %s, want 422 confirm_required", rec.Code, rec.Body)
	}
	move["confirm"] = true
	if rec := s.call(e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms), t, http.MethodPatch, path, move); rec.Code != http.StatusForbidden {
		t.Fatalf("the admin's agent moving the readers got %d, want 403", rec.Code)
	}
	for _, visibility := range []string{"classified", "shared"} {
		rec := s.call(s.admin(), t, http.MethodPatch, path, map[string]any{"mail_visibility": visibility, "confirm": true})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving the visibility to %s without the opt-in answered %d, want 422", visibility, rec.Code)
		}
	}
	if timeline, _ := s.sees(t, e.Rep2, []ids.UUID{e.Team1}, mail); !timeline {
		t.Fatal("a refused move must leave the readers as they were")
	}

	if rec := s.call(s.admin(), t, http.MethodPatch, path, move); rec.Code != http.StatusOK {
		t.Fatalf("the confirmed move answered %d: %s", rec.Code, rec.Body)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->>'type' = 'shared_mailbox.changed' AND o.envelope->'payload'->>'change' = 'readers_changed'`, box.Id); got != 1 {
		t.Fatalf("the move wrote %d audit rows with their event, want 1", got)
	}
	if timeline, _ := s.sees(t, e.Rep2, []ids.UUID{e.Team1}, mail); timeline {
		t.Fatal("the old reader team still reads the moved mailbox")
	}
	// Rep3 also received the mail; the new team's member reads it through the mailbox.
	other := ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'teammate@authz.test', 'Teammate')`, other)
	e.GrantRole(t, other, "rep")
	service := compose.TeamServiceForTest(e.Pool)
	if err := service.SetTeamMember(e.Admin(), adminIdentity(e), e.Team2, other, true); err != nil {
		t.Fatalf("adding the new team's member: %v", err)
	}
	if timeline, agent := s.sees(t, other, []ids.UUID{e.Team2}, mail); !timeline || !agent {
		t.Fatalf("the new reader team's member reads timeline=%v agent=%v, want both", timeline, agent)
	}
}

func TestOnlyAnAdminStartsASharedMailboxsHistoryImport(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := ids.UUID(s.connect(t, e.Team1).Id)
	var queued []ids.UUID
	enqueue := func(ctx context.Context, _ pgx.Tx, run ids.UUID) error {
		queued = append(queued, run)
		return ctx.Err()
	}
	start := func(ctx context.Context) (capture.BackfillRun, error) {
		return s.registry.StartSharedBackfill(ctx, box, 3, connector.BackfillEstimate{}, enqueue)
	}
	for who, ctx := range map[string]context.Context{
		"a rep":             e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms),
		"the admin's agent": e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms),
	} {
		if _, err := start(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s starting the import got %v, want a refusal", who, err)
		}
	}
	run, err := start(s.admin())
	if err != nil {
		t.Fatalf("the admin's start: %v", err)
	}
	if run.ConnectionID != box || run.Status != "queued" || len(queued) != 1 || queued[0] != run.ID {
		t.Fatalf("the start answered %+v and scheduled %v, want one queued run on %s", run, queued, box)
	}
	if _, err := start(s.admin()); !errors.Is(err, capture.ErrBackfillRunning) {
		t.Fatalf("a second start got %v, want the one live run to refuse it", err)
	}
	if got := historyImportAnnouncements(t, s, box); got != 1 {
		t.Fatalf("the start wrote %d audit rows with their event, want 1", got)
	}
	// A run that stopped on an error is continued by the next start, which
	// announces itself the same way.
	e.WsExec(t, `UPDATE capture_backfill SET status = 'error', completed_at = now(), cursor = '{"page":"2"}' WHERE id = $1`, run.ID)
	if resumed, err := start(s.admin()); err != nil || resumed.ID != run.ID {
		t.Fatalf("the second start got %+v, %v, want run %s continued", resumed, err, run.ID)
	}
	if got := historyImportAnnouncements(t, s, box); got != 2 {
		t.Fatalf("the continued run wrote %d audit rows with their event in all, want 2", got)
	}
	if _, err := s.registry.StartSharedBackfill(s.admin(), ids.NewV7(), 3, connector.BackfillEstimate{}, enqueue); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unknown mailbox got %v, want not found", err)
	}
}

// historyImportAnnouncements counts the mailbox's audit rows whose event says a
// history import started or continued.
func historyImportAnnouncements(t *testing.T, s *sharedMailboxSuite, box ids.UUID) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->>'type' = 'shared_mailbox.changed'
		  AND o.envelope->'payload'->>'change' = 'history_import_started'`, box)
}

func TestApplyingAHeldVisibilityToHistoryNarrowsEvenWhenTheMailboxIsAlreadyHeld(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connect(t, e.Team1)
	id := ids.UUID(box.Id)
	s.shared.raws = [][]byte{orderMail("history-1@acme.example", salesMailbox)}
	s.sync(t, id)
	// The row as an import before the mailbox was held left it.
	e.WsExec(t, `UPDATE capture_import SET posture_at_import = 'shared' WHERE connection_id = $1`, id)

	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+id.String(),
		map[string]any{"mail_visibility": "held", "apply_to_history": true, "confirm": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("the narrowing answered %d: %s", rec.Code, rec.Body)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE connection_id = $1 AND posture_at_import = 'held'`, id); got != 1 {
		t.Fatal("apply_to_history on an already-held mailbox must still narrow its history")
	}
}
