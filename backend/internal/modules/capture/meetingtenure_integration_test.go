// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// An address that changed hands carries none of its former holder's meetings.
//
// activity_participant.address records what the INVITATION said, and nothing
// rewrites it when a contact_email is archived — so the bare-address arm of the
// met-in-person question would go on reading a meeting the previous holder
// attended as evidence about whoever holds the address now. The consequence is
// a contact minted for a stranger, and — since a meeting is a qualifying event
// on the consent side — a lawful basis to mail them.
//
// Against real Postgres because the whole of the rule is one SQL predicate over
// three tables, and because it has to hold under the same row-scope clause the
// production query carries.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recycledAddress is one an employer reassigns — the shape this is about.
const recycledAddress = "first.last@company.example"

// meetingTenureEnv is one workspace and the context capture's own reads run
// under: a connector principal, because that is who is syncing when the ladder
// asks this question.
func meetingTenureEnv(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if appDSN == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` " +
			"(integration tests fail loudly, they never skip)")
	}
	owner := ownerConn(t)
	ctx := context.Background()
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	wsCtx := principal.WithWorkspaceID(ctx, ws)
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   "connector:imap",
		Permissions: principal.Permissions{
			RoleKeys: []string{"capture"},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(wsCtx, ids.NewV7()), pool
}

// seedMeetingWith records one connector-captured meeting the address was
// invited to, at a given age, with no contact behind the participant row —
// which is the state the bare-address arm exists to read.
func seedMeetingWith(ctx context.Context, tx pgx.Tx, address string, ago time.Duration) error {
	var activityID ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO activity (kind, occurred_at, direction, source, captured_by, counterparty_email)
		VALUES ('meeting', now() - $2::interval, 'inbound', 'imap', 'connector:imap', $1)
		RETURNING id`, address, ago.String()).Scan(&activityID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, address, role)
		VALUES ($1, $2, 'attendee')`, activityID, address)
	return err
}

// archiveAddressAt retires a contact_email for the address, which is the only
// mark a hand-over leaves.
func archiveAddressAt(ctx context.Context, tx pgx.Tx, address string, ago time.Duration) error {
	var contactID ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO contact (full_name, source, captured_by)
		VALUES ('The Former Holder', 'manual', 'human:test') RETURNING id`).Scan(&contactID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO contact_email (contact_id, email, archived_at, source, captured_by)
		VALUES ($1, $2, now() - $3::interval, 'manual', 'human:test')`,
		contactID, address, ago.String())
	return err
}

func TestAMeetingBeforeAnAddressChangedHandsIsNotEvidenceAboutItsNewHolder(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)

	var met bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		// The former holder's meeting, then the hand-over: the address is
		// retired AFTER that meeting, so the meeting is not the new holder's.
		if err := seedMeetingWith(ctx, tx, recycledAddress, 60*24*time.Hour); err != nil {
			return err
		}
		if err := archiveAddressAt(ctx, tx, recycledAddress, 30*24*time.Hour); err != nil {
			return err
		}
		var err error
		met, _, err = metInPersonTx(ctx, tx, recycledAddress, ids.NewV7())
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace has met the address: %v", err)
	}
	if met {
		t.Error("a meeting the FORMER holder attended counted as having met the address's " +
			"new holder — the invitation row is never rewritten when an address is " +
			"retired, so it would mint a contact for a stranger and a basis to mail them")
	}
}

func TestAMeetingAfterAnAddressChangedHandsIsEvidence(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)

	var met bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := archiveAddressAt(ctx, tx, recycledAddress, 60*24*time.Hour); err != nil {
			return err
		}
		if err := seedMeetingWith(ctx, tx, recycledAddress, 30*24*time.Hour); err != nil {
			return err
		}
		var err error
		met, _, err = metInPersonTx(ctx, tx, recycledAddress, ids.NewV7())
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace has met the address: %v", err)
	}
	if !met {
		t.Error("a meeting held AFTER the address changed hands was refused as evidence — " +
			"the bound is the hand-over, not the existence of one")
	}
}

// The control. The bound must narrow the arm for a recycled address and for
// nothing else: without this, dropping the bare-address arm altogether would
// pass the case above and silently end the only path by which a guest with no
// record yet becomes a contact.
func TestAMeetingWithAnAddressThatNeverChangedHandsIsStillEvidence(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)

	var met bool
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := seedMeetingWith(ctx, tx, "steady@company.example", 30*24*time.Hour); err != nil {
			return err
		}
		var err error
		met, _, err = metInPersonTx(ctx, tx, "steady@company.example", ids.NewV7())
		return err
	}); err != nil {
		t.Fatalf("reading whether the workspace has met the address: %v", err)
	}
	if !met {
		t.Error("a meeting with an address nobody ever retired was refused as evidence — " +
			"the tenure bound has narrowed the arm to nothing")
	}
}
