// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// rawFloorPack keeps commercial correspondence six years; rawNoFloorPack
// declares no floor. Reserved codes, registered for this binary only.
type rawFloorPack struct{ code jurisdiction.Code }

func (p rawFloorPack) Code() jurisdiction.Code { return p.code }

func (p rawFloorPack) Retention() jurisdiction.Retention {
	if p.code == "zv" {
		return nil
	}
	return rawFloorClasses{}
}

type rawFloorClasses struct{}

func (rawFloorClasses) Classes() []jurisdiction.RetentionClass {
	return []jurisdiction.RetentionClass{{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}, Anchor: jurisdiction.AnchorCalendarYearEnd}}
}

func init() {
	jurisdiction.Register(rawFloorPack{code: "zu"})
	jurisdiction.Register(rawFloorPack{code: "zv"})
}

// The raw-capture retention executor asks the floor in force inside the
// destroying transaction. The same original is kept under a jurisdiction
// whose floor shields its message, and destroyed under one with no floor.
func TestTheRawCaptureEraseAsksTheFloorInForce(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		country jurisdiction.Code
		kept    bool
	}{{"zu", true}, {"zv", false}} {
		t.Run(string(c.country), func(t *testing.T) {
			tx, err := owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rolling back: %v", err)
				}
			}()
			sourceID := "raw-floor-" + ids.NewV7().String()
			mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, source_system, source_id, captured_by, retention_class, retention_class_at)
				VALUES ($1, 'email', 'Order', 'commercial content', now() - interval '400 days', 'capture_email', 'email', $2, 'connector:t', 'commercial_correspondence', now())`,
				ids.NewV7(), sourceID)
			var rawID ids.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('email', $1, '{}'::jsonb) RETURNING id`,
				sourceID).Scan(&rawID); err != nil {
				t.Fatal(err)
			}
			country := c.country
			svc := &RetentionService{eraser: NewEraser(nil).WithCountry(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return country, nil })}
			err = svc.eraseRawCapture(ctx, tx, rawID)
			var left int
			if scanErr := tx.QueryRow(ctx, `SELECT count(*) FROM raw_capture WHERE id = $1`, rawID).Scan(&left); scanErr != nil {
				t.Fatal(scanErr)
			}
			if c.kept && (!errors.Is(err, errKeptByFloor) || left != 1) {
				t.Fatalf("under %s: err=%v, %d original(s) left; want it kept by the floor", c.country, err, left)
			}
			if !c.kept && (err != nil || left != 0) {
				t.Fatalf("under %s: err=%v, %d original(s) left; want it destroyed", c.country, err, left)
			}
		})
	}
}

// A hold on any one of the activities an original was read into keeps the
// original. Another activity it reaches being free to go changes nothing.
//
// The hold is asked of every matching activity for that reason. Asked of one,
// it answers about whichever row the join happened to offer.
func TestARawCaptureHeldThroughOneOfItsActivitiesSurvives(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	sourceID := "raw-hold-" + ids.NewV7().String()
	var rawID ids.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ('email', $1, '{}'::jsonb) RETURNING id`, sourceID).Scan(&rawID); err != nil {
		t.Fatal(err)
	}
	// Two activities read from the one original, named through raw_capture_id.
	// `uq_activity_source` keeps them from sharing a source id, and that column
	// is the first arm of `rawCaptureLink`.
	//
	// Ordinary mail, so no floor shields either, and only the link below keeps
	// the original.
	held, free := ids.NewV7(), ids.NewV7()
	for _, id := range []ids.UUID{held, free} {
		mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, subject, body, occurred_at, source,
			source_system, source_id, captured_by, raw_capture_id)
			VALUES ($1, 'email', 'Hello', 'ordinary content', now() - interval '400 days',
			        'capture_email', 'email', $2, 'connector:test', $3)`,
			id, sourceID+"-"+id.String(), rawID)
	}
	company := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO company (id, display_name, source, captured_by, legal_hold)
		VALUES ($1, 'Held Ltd', 'manual', 'human:x', true)`, company)
	mustExec(ctx, t, tx, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, held, company)

	svc := &RetentionService{eraser: NewEraser(nil)}
	err = svc.eraseRawCapture(ctx, tx, rawID)
	var left int
	if scanErr := tx.QueryRow(ctx,
		`SELECT count(*) FROM raw_capture WHERE id = $1`, rawID).Scan(&left); scanErr != nil {
		t.Fatal(scanErr)
	}
	if left != 1 {
		t.Fatalf("the original is gone (err=%v) though %s is held, while %s is not", err, held, free)
	}
}

// A restricted sibling takes the original out of the sweep's reach.
//
// The data was never reachable: `raw_capture_id` is `ON DELETE SET NULL`, so
// deleting the original updates every linked activity. The restriction trigger
// refuses that update on a still-restricted row.
//
// What the shield changes is which answer comes back.
//
// Unshielded, the refusal aborts the transaction. One refused row then fails
// the whole policy, and every later scope stops with it, which is the cost
// `correspondenceFloorPredicate` records.
//
// Shielded, the delete matches nothing and the stage reports a row kept by
// the floor.
func TestARawCaptureRestrictedThroughOneOfItsActivitiesSurvives(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	})
	sourceID := "raw-restricted-" + ids.NewV7().String()
	var rawID ids.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ('email', $1, '{}'::jsonb) RETURNING id`, sourceID).Scan(&rawID); err != nil {
		t.Fatal(err)
	}
	// Old enough that the statutory floor has closed, so the floor arm does not
	// shield it and the restriction is what has to. A restricted row always
	// carries a retention class (`activity_restriction_needs_class`), so inside
	// its floor window the floor arm would answer and this would prove nothing.
	restricted, free := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, subject, body, occurred_at, source,
		source_system, source_id, captured_by, raw_capture_id, retention_class,
		retention_class_at, restricted_at, restricted_reason, restricted_until, archived_at)
		VALUES ($1, 'email', 'Hello', 'ordinary content', now() - interval '30 years',
		        'capture_email', 'email', $2, 'connector:test', $3,
		        'commercial_correspondence', now() - interval '30 years', now(),
		        'the subject asked', now() + interval '1 year', now())`,
		restricted, sourceID+"-"+restricted.String(), rawID)
	mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, subject, body, occurred_at, source,
		source_system, source_id, captured_by, raw_capture_id)
		VALUES ($1, 'email', 'Hello', 'ordinary content', now() - interval '30 years',
		        'capture_email', 'email', $2, 'connector:test', $3)`,
		free, sourceID+"-"+free.String(), rawID)

	svc := &RetentionService{eraser: NewEraser(nil)}
	if err := svc.eraseRawCapture(ctx, tx, rawID); !errors.Is(err, errKeptByFloor) {
		t.Fatalf("the sweep answered %v for an original a restricted activity was read from, "+
			"want it kept by the floor", err)
	}
	var left int
	if scanErr := tx.QueryRow(ctx,
		`SELECT count(*) FROM raw_capture WHERE id = $1`, rawID).Scan(&left); scanErr != nil {
		t.Fatal(scanErr)
	}
	if left != 1 {
		t.Fatalf("the original is gone though %s is restricted", restricted)
	}
}
