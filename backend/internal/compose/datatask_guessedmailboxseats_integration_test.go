// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seedGuessedSeat gives a bare-stamped legacy mail the two rows the participant
// backfill wrote when it read the provider's only connection as the owner: the
// seat, and the counterparty's address. No writer produces that shape any more.
func seedGuessedSeat(t *testing.T, e *integration.Env, conn *pgx.Conn, sourceID string, seat ids.UUID) ids.UUID {
	t.Helper()
	id := seedLegacyActivity(t, e, sourceID, "connector:gmail", "inbound", "pat@counterparty.test")
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'to')`, id, seat); err != nil {
		t.Fatalf("seeding the guessed owner of %s: %v", sourceID, err)
	}
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'pat@counterparty.test', 'from')`, id); err != nil {
		t.Fatalf("seeding the counterparty of %s: %v", sourceID, err)
	}
	return id
}

func ownSeatRows(t *testing.T, e *integration.Env, activity ids.UUID, seat ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id = $1 AND user_id = $2 AND address IS NULL AND contact_id IS NULL`, activity, seat)
}

func runGuessedMailboxTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func guessedMailboxPreview(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := guessedMailboxSeatsDataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatalf("previewing the repair: %v", err)
	}
	return count
}

// limitToParticipants narrows one message to its participants.
func limitToParticipants(t *testing.T, conn *pgx.Conn, id ids.UUID) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `UPDATE activity SET audience = 'participants' WHERE id = $1`, id); err != nil {
		t.Fatalf("limiting %s: %v", id, err)
	}
}

// legacyCapturedMail is mail the real sink captured for seat while stamps named
// only the connector: the sink's rows, restamped bare, with no ledger row left.
func legacyCapturedMail(t *testing.T, e *integration.Env, conn *pgx.Conn, sourceID string, seat ids.UUID) ids.UUID {
	t.Helper()
	id := captureMail(t, e, seat, sourceID, connector.DirectionInbound, "pat@counterparty.test")
	if _, err := conn.Exec(context.Background(), `UPDATE activity SET captured_by = 'connector:gmail' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `DELETE FROM capture_import WHERE activity_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// readsContent reports whether seat may read the message's content.
func readsContent(t *testing.T, e *integration.Env, seat, activity ids.UUID) bool {
	t.Helper()
	reader := e.As(seat, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	_, err := activities.NewStore(e.DB()).GetActivityContent(reader, ids.From[ids.ActivityKind](activity), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("reading %s as %s: %v", activity, seat, err)
	}
	return true
}

func hasAReader(t *testing.T, e *integration.Env, activity ids.UUID) bool {
	t.Helper()
	var reachable bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		reachable, err = auth.ActivityHasAReaderTx(e.Admin(), tx, activity)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return reachable
}

// A participant row nothing proves grants its seat nothing, before and after the
// repair runs. Proof of the seat's own mailbox, and a reader someone chose, still
// admit.
func TestAGuessedParticipantGrantsNothing(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	guessed := seedGuessedSeat(t, e, conn, "gm-read-guessed", e.Rep1)
	ledger := seedGuessedSeat(t, e, conn, "gm-read-ledger", e.Rep1)
	if _, err := conn.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, ledger, e.Rep1); err != nil {
		t.Fatal(err)
	}
	creator := legacyCapturedMail(t, e, conn, "gm-read-creator", e.Rep1)
	stamped := captureMail(t, e, e.Rep2, "gm-read-stamped", connector.DirectionInbound, "pat@counterparty.test")
	granted := seedGuessedSeat(t, e, conn, "gm-read-granted", e.Rep2)
	for _, id := range []ids.UUID{guessed, ledger, creator, stamped, granted} {
		limitToParticipants(t, conn, id)
	}
	if _, err := conn.Exec(ctx, `UPDATE activity SET audience = 'selected' WHERE id = $1`, granted); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
		VALUES ($1, 'user', $2, $3)`, granted, e.Rep2, "human:"+e.AdminUser.String()); err != nil {
		t.Fatal(err)
	}

	check := func(when string) {
		if readsContent(t, e, e.Rep1, guessed) {
			t.Errorf("%s: a participant nothing proves admits its seat", when)
		}
		if hasAReader(t, e, guessed) {
			t.Errorf("%s: a participant nothing proves counts as the message's reader", when)
		}
		for name, c := range map[string]struct{ seat, activity ids.UUID }{
			"ledger": {e.Rep1, ledger}, "creator": {e.Rep1, creator}, "seat stamp": {e.Rep2, stamped}, "grant": {e.Rep2, granted},
		} {
			if !readsContent(t, e, c.seat, c.activity) {
				t.Errorf("%s: the %s reader lost the message", when, name)
			}
		}
	}
	check("before the repair")
	runGuessedMailboxTask(ctx, t, e, guessedMailboxSeatsDataTask())
	check("after the repair")
}

// legacyPair is legacy mail between seat and contact under one stamp, with the
// two participant rows the participant backfill wrote.
func legacyPair(t *testing.T, e *integration.Env, conn *pgx.Conn, sourceID, stamp string, seat, contact ids.UUID) ids.UUID {
	t.Helper()
	id := seedLegacyActivity(t, e, sourceID, stamp, "inbound", "pat@counterparty.test")
	if _, err := conn.Exec(context.Background(), `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'to')`, id, seat); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'from')`, id, contact); err != nil {
		t.Fatal(err)
	}
	return id
}

// The readers that ask whether a seat was on a message agree with the content
// gate: a participant nothing proves is not presence.
func TestAGuessedParticipantIsNoEvidenceOfPresence(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	contact := e.SeedContact(t, "Pat Counterparty", &e.Rep1)
	if _, err := conn.Exec(context.Background(), `UPDATE contact SET visibility = 'owner' WHERE id = $1`, contact); err != nil {
		t.Fatal(err)
	}
	guessed := legacyPair(t, e, conn, "gm-presence-guessed", "connector:gmail", e.Rep1, contact)
	stamped := legacyPair(t, e, conn, "gm-presence-stamped", "connector:gmail:"+e.Rep1.String(), e.Rep1, contact)
	// The seat's mailbox delivering it lets the seat read it. It does not make an
	// addressless seat row real participation, so it adds no interaction.
	ledger := legacyPair(t, e, conn, "gm-presence-ledger", "connector:gmail", e.Rep1, contact)
	if _, err := conn.Exec(context.Background(), `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, ledger, e.Rep1); err != nil {
		t.Fatal(err)
	}
	header := legacyPair(t, e, conn, "gm-presence-header", "connector:gmail", e.Rep1, contact)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capture.StampFurtherParticipants(e.Admin(), tx, ids.From[ids.ActivityKind](header), "email", "", true,
			[]connector.MessageParticipant{{Email: "a@authz.test", Role: connector.ParticipantRoleTo}})
	}); err != nil {
		t.Fatal(err)
	}
	seat := mailboxOwnerCtx(e, e.Rep1)

	for name, c := range map[string]struct {
		activity ids.UUID
		want     bool
	}{"guessed": {guessed, false}, "stamped": {stamped, true}, "ledger": {ledger, true}, "header": {header, true}} {
		limitToParticipants(t, conn, c.activity)
		if got := readsContent(t, e, e.Rep1, c.activity); got != c.want {
			t.Errorf("%s mail: content readable = %v, want %v", name, got, c.want)
		}
		var holds, minable bool
		if err := database.WithWorkspaceTx(seat, e.Pool, func(tx pgx.Tx) error {
			var err error
			if holds, err = capture.SeatHoldsActivityTx(seat, tx, ids.From[ids.ActivityKind](c.activity)); err != nil {
				return err
			}
			return tx.QueryRow(context.Background(), `SELECT `+contacts.SignatureSourceEligible("p", "a")+`
				FROM contact p, activity a WHERE p.id = $1 AND a.id = $2`, contact, c.activity).Scan(&minable)
		}); err != nil {
			t.Fatal(err)
		}
		if holds != c.want || minable != c.want {
			t.Errorf("%s mail: seat holds it = %v, signature may be read = %v, want %v", name, holds, minable, c.want)
		}
		if _, err := conn.Exec(context.Background(), `UPDATE activity SET audience = 'workspace' WHERE id = $1`, c.activity); err != nil {
			t.Fatal(err)
		}
	}
	for name, fold := range map[string]func(context.Context, pgx.Tx) error{
		"incremental": func(ctx context.Context, tx pgx.Tx) error {
			return search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{guessed, stamped, ledger, header})
		},
		"rebuild": search.RebuildEdges,
	} {
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return fold(e.Admin(), tx) }); err != nil {
			t.Fatal(err)
		}
		if got := e.WsCount(t, `SELECT count_total FROM graph_interaction_edge WHERE user_id = $1 AND contact_id = $2`, e.Rep1, contact); got != 2 {
			t.Errorf("%s: the interaction edge counts %d messages, want only the stamped and header-proven ones", name, got)
		}
	}
}

// The repair takes off only the owner nothing proves. A seat whose mailbox the
// capture ledger or the capturing connector ties to the mail keeps it, as does
// mail stamped with a seat. A colleague's unrelated work on the message is no
// proof, and stays on record.
func TestTheGuessedMailboxRepairRemovesOnlyUnprovenOwners(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	guessed := seedGuessedSeat(t, e, conn, "gm-guessed", e.Rep1)
	// A colleague bound from the message's own To line is header evidence, not
	// the guessed owner, and stays.
	if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, address, role)
		VALUES ($1, $2, 'rep2@myco.example', 'to')`, guessed, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ('gmail', 'gm-guessed', '{"headers":"…"}'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	ledger := seedGuessedSeat(t, e, conn, "gm-ledger", e.Rep1)
	if _, err := conn.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, ledger, e.Rep1); err != nil {
		t.Fatal(err)
	}
	creator := legacyCapturedMail(t, e, conn, "gm-creator", e.Rep1)
	worked := seedGuessedSeat(t, e, conn, "gm-worked", e.Rep2)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditEvent(e.Admin(), tx, "update", "activity", worked, map[string]any{"note": "kept"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stamped := captureMail(t, e, e.Rep2, "gm-stamped", connector.DirectionInbound, "pat@counterparty.test")

	if got := guessedMailboxPreview(ctx, t, e); got != 2 {
		t.Errorf("the preview counts %d messages, want the two with an unproven owner", got)
	}
	task := guessedMailboxSeatsDataTask()
	task.Steps[0].Limit = 1
	runGuessedMailboxTask(ctx, t, e, task)

	for name, c := range map[string]struct {
		activity, seat ids.UUID
		want           int
	}{
		"guessed": {guessed, e.Rep1, 0}, "worked": {worked, e.Rep2, 0},
		"ledger": {ledger, e.Rep1, 1}, "creator": {creator, e.Rep1, 1}, "seat stamp": {stamped, e.Rep2, 1},
	} {
		if got := ownSeatRows(t, e, c.activity, c.seat); got != c.want {
			t.Errorf("%s owner rows = %d, want %d", name, got, c.want)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND actor_type = 'human'`, worked); got != 1 {
		t.Errorf("the colleague's own work on the message = %d audit rows, want it kept", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address IS NOT NULL`, guessed); got != 2 {
		t.Errorf("the message's header rows went with the guessed owner: %d rows, want the counterparty and the colleague", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN raw_capture rc ON rc.source_id = a.source_id WHERE a.id = $1 AND a.archived_at IS NULL`, guessed); got != 1 {
		t.Errorf("the repaired message or its stored original is gone")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		AND before ? 'mailbox_seats' AND evidence ? 'data_task_run'`, guessed); got != 1 {
		t.Errorf("repair audit rows = %d, want one naming the run and the removed seat", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1`, guessed.String()); got != 1 {
		t.Errorf("activity.updated events for the repaired mail = %d, want 1", got)
	}

	if got := guessedMailboxPreview(ctx, t, e); got != 0 {
		t.Errorf("after the repair the preview still counts %d messages", got)
	}
	step := task.Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err })
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay removed %d more owners", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

func TestTheGuessedMailboxRepairRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	guessed := seedGuessedSeat(t, e, conn, "gm-rollback", e.Rep1)

	task := guessedMailboxSeatsDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runGuessedMailboxTask(ctx, t, e, task)

	if got := ownSeatRows(t, e, guessed, e.Rep1); got != 1 {
		t.Errorf("the owner removal survived a rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, guessed); got != 0 {
		t.Errorf("repair audit rows escaped the rollback: %d", got)
	}
}
