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
