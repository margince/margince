// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The audience gate is spelled twice, and this is what holds the two spellings
// together.
//
// `activityAudienceArm` asks whether ONE named caller may read a message.
// `ActivityHasAReaderTx` asks whether ANYBODY may — the question an audience
// write owes, because a write that leaves the answer false has not narrowed the
// message, it has deleted it from view with no way back. The two bind the user
// differently, so they are different SQL rather than one clause with a
// different operand (audienceorphan.go says why), and nothing in the compiler
// notices when one grows an arm the other lacks.
//
// Each case below builds a row whose ONLY admitting arm is the named one, then
// asks both functions about it. They must agree, and the agreement has to fail
// in both directions:
//
//   - an arm the read clause has and the refusal lacks → a legal write refused,
//     which is loud;
//   - an arm the refusal keeps after the read clause dropped it → an orphan
//     admitted, which is silent, and is the half no ordinary assertion notices.
//
// THE ARM SOURCES ARE SEEDED DIRECTLY, which AGENTS.md otherwise warns against.
// It is right here because the SUBJECT is the predicate, not the writer: what is
// under test is what the gate makes of a row in a given shape, and reaching each
// shape through the producer that happens to write it today would test the
// producers instead — and could not reach `no arm at all`, which is the case the
// whole invariant exists for.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// armCase is one way a human is admitted to an activity's content: the row
// shape that admits through that arm alone, and the seat it admits.
type armCase struct {
	name string
	// audience the row carries. `participants` is the value that turns the
	// workspace arm off without naming anybody, which is what makes every
	// other arm the sole one under test.
	audience string
	// arrange puts the row into the shape this arm is about. It returns the
	// user the arm should admit, or ids.Nil when the arm admits nobody.
	arrange func(t *testing.T, e *Env, tx pgx.Tx, activityID ids.UUID) ids.UUID
	// teams is what identity's loadGrants would hand that user's principal.
	//
	// It is stated per case rather than derived, because the harness's As binds
	// TeamIDs DIRECTLY and never runs loadGrants — so a case that simply passed
	// every team the user belongs to would model a principal the product never
	// builds, and would quietly assert that an ARCHIVED team still admits its
	// members. loadGrants joins `team ... archived_at IS NULL`, so a live team
	// the user is in appears here and an archived one does not. That difference
	// is the whole subject of two cases below.
	teams []ids.UUID
}

// seedBareActivity writes an activity carrying none of the arms: captured by a
// connector that names no human, imported by no mailbox, with no participant
// and no audience member. Every case below starts here and adds exactly one.
func seedBareActivity(t *testing.T, tx pgx.Tx, audience string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := tx.Exec(context.Background(), `
		INSERT INTO activity (id, kind, channel_provider, direction, occurred_at, source, captured_by, audience, body)
		VALUES ($1, 'message', 'telegram', 'inbound', now(), 'telegram', 'connector:telegram', $2, 'hello')`,
		id, audience); err != nil {
		t.Fatalf("seeding the bare activity: %v", err)
	}
	return id
}

func audienceArmCases(teamOne ids.UUID) []armCase {
	return []armCase{
		{
			name:     "the workspace audience admits everyone",
			audience: "workspace",
			arrange:  func(_ *testing.T, e *Env, _ pgx.Tx, _ ids.UUID) ids.UUID { return e.Rep3 },
		},
		{
			name:     "captured_by ending in a live user id admits that user",
			audience: "participants",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(),
					`UPDATE activity SET captured_by = 'connector:gmail:' || $2 WHERE id = $1`,
					id, e.Rep1); err != nil {
					t.Fatalf("stamping the provenance: %v", err)
				}
				return e.Rep1
			},
		},
		{
			name:     "an import row admits the mailbox that brought it in",
			audience: "participants",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(),
					`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`,
					id, e.Rep1); err != nil {
					t.Fatalf("seeding the import row: %v", err)
				}
				return e.Rep1
			},
		},
		{
			name:     "a participant row admits the person on the conversation",
			audience: "participants",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(),
					`INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'to', $2)`,
					id, e.Rep1); err != nil {
					t.Fatalf("seeding the participant row: %v", err)
				}
				return e.Rep1
			},
		},
		{
			name:     "a selected membership admits the named user",
			audience: "selected",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(), `
					INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
					VALUES ($1, 'user', $2, $3)`, id, e.Rep1, e.AdminUser); err != nil {
					t.Fatalf("seeding the audience member: %v", err)
				}
				return e.Rep1
			},
		},
		{
			name:     "a selected team with a member admits that member",
			audience: "selected",
			// Live, and Rep1 is in it, so loadGrants would carry it.
			teams: []ids.UUID{teamOne},
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(), `
					INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
					VALUES ($1, 'team', $2, $3)`, id, e.Team1, e.AdminUser); err != nil {
					t.Fatalf("seeding the team audience member: %v", err)
				}
				return e.Rep1
			},
		},
		{
			// An ARCHIVED team resolves no share. Its membership rows survive so
			// a restore brings them back, but identity's loadGrants joins
			// `team ... archived_at IS NULL` before a team id reaches a
			// principal, so the read arm can never match one. Counting the
			// membership rows alone would answer "somebody can read it" about a
			// team nobody is in — an orphan admitted rather than refused, which
			// is the direction nothing downstream notices.
			name:     "a selected team that has been archived admits nobody",
			audience: "selected",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(), `
					INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
					VALUES ($1, 'team', $2, $3)`, id, e.Team1, e.AdminUser); err != nil {
					t.Fatalf("seeding the team audience member: %v", err)
				}
				if _, err := tx.Exec(context.Background(),
					`UPDATE team SET archived_at = now() WHERE id = $1`, e.Team1); err != nil {
					t.Fatalf("archiving the team: %v", err)
				}
				return ids.Nil
			},
		},
		{
			name:     "an empty selected team admits nobody",
			audience: "selected",
			arrange: func(t *testing.T, e *Env, tx pgx.Tx, id ids.UUID) ids.UUID {
				t.Helper()
				if _, err := tx.Exec(context.Background(), `
					INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
					VALUES ($1, 'team', $2, $3)`, id, e.Team2, e.AdminUser); err != nil {
					t.Fatalf("seeding the team audience member: %v", err)
				}
				if _, err := tx.Exec(context.Background(),
					`DELETE FROM team_membership WHERE team_id = $1`, e.Team2); err != nil {
					t.Fatalf("emptying the team: %v", err)
				}
				return ids.Nil
			},
		},
		{
			name:     "no arm at all admits nobody",
			audience: "participants",
			arrange:  func(_ *testing.T, _ *Env, _ pgx.Tx, _ ids.UUID) ids.UUID { return ids.Nil },
		},
		{
			name:     "a selected audience naming nobody admits nobody",
			audience: "selected",
			arrange:  func(_ *testing.T, _ *Env, _ pgx.Tx, _ ids.UUID) ids.UUID { return ids.Nil },
		},
	}
}

// readsContent answers whether this caller may read the row's CONTENT, through
// the real read path rather than by re-composing the clause here — a test that
// spelled the arm again would be a third writer of the thing under test.
func readsContent(ctx context.Context, t *testing.T, e *Env, id ids.UUID) bool {
	t.Helper()
	got, err := e.Activities.GetActivity(ctx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		// Not discoverable at all is not readable either, which is the answer
		// this function is about.
		return false
	}
	if err != nil {
		// Anything else is the read path being broken, and swallowing it would
		// let a wrong DSN or a dropped table read as "nobody can see this row" —
		// which is what half these cases EXPECT, so the suite would go green on
		// a harness that is not exercising the gate at all.
		t.Fatalf("reading the activity back: %v", err)
	}
	return got.ContentState != nil && *got.ContentState == crmcontracts.ActivityContentStateAvailable
}

// TestTheOrphanRefusalAgreesWithTheAudienceGateArmForArm is the parity claim.
func TestTheOrphanRefusalAgreesWithTheAudienceGateArmForArm(t *testing.T) {
	e := Setup(t)
	for _, tc := range audienceArmCases(e.Team1) {
		t.Run(tc.name, func(t *testing.T) {
			var admits ids.UUID
			var id ids.UUID
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				id = seedBareActivity(t, tx, tc.audience)
				admits = tc.arrange(t, e, tx, id)
				return nil
			}); err != nil {
				t.Fatalf("arranging the row: %v", err)
			}

			// What the REFUSAL believes.
			var hasReader bool
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				var err error
				hasReader, err = auth.ActivityHasAReaderTx(context.Background(), tx, id)
				return err
			}); err != nil {
				t.Fatalf("asking whether the row has a reader: %v", err)
			}

			wantReader := admits != ids.Nil
			if hasReader != wantReader {
				t.Fatalf("ActivityHasAReaderTx = %v, want %v — the refusal and the audience gate "+
					"disagree about this arm, so one of them has an arm the other does not", hasReader, wantReader)
			}

			// And what the READ CLAUSE believes, over the same row: the parity
			// is only worth something if the human the arm names really does
			// get in, and if nobody at all gets into the arm-less row.
			if wantReader && !readsContent(e.As(admits, tc.teams, activityLifecyclePerms), t, e, id) {
				t.Fatal("the arm's own user cannot read the row it admits them to — " +
					"ActivityHasAReaderTx said somebody could, and the read clause disagrees")
			}
			// Rep3 carries the case's own teams too: a row that admits NOBODY
			// must admit nobody even to a reader holding every grant the
			// admitted case would have had.
			if !wantReader && readsContent(e.As(e.Rep3, tc.teams, activityLifecyclePerms), t, e, id) {
				t.Fatal("a row with no arm was readable anyway — the read clause is not filtering, " +
					"so every agreement above is vacuous")
			}
		})
	}
}
