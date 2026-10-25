// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A bought address can make a contact the same human as another one. The
// applier then runs the duplicate check a create runs and files the pair.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// boughtEmailSetup is an incumbent "Ab" on ab@example.com and an empty contact
// a purchase is about to fill.
func boughtEmailSetup(t *testing.T, e *Env) (incumbent, subject ids.UUID) {
	t.Helper()
	store := contacts.NewStore(e.DB())
	created, err := store.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Ab", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "ab@example.com", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	subject = ids.NewV7()
	execAsOwner(t, e, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Anna Beispiel', 'manual', 'human:x')`, subject)
	return ids.UUID(created.Id), subject
}

func boughtEmail(address string) []provider.Claim {
	return []provider.Claim{{Key: provider.ClaimProfessionalEmails, Value: []byte(`[{"value":"` + address + `"}]`)}}
}

func openContactPairsOf(t *testing.T, e *Env, subject ids.UUID) (count int, other ids.UUID) {
	t.Helper()
	queryAsOwner(t, e, `
		SELECT count(*) FROM dedupe_candidate
		 WHERE entity_type = 'contact' AND disposition = 'open'
		   AND $1 IN (left_contact_id, right_contact_id)`, &count, subject)
	if count == 1 {
		queryAsOwner(t, e, `
			SELECT CASE WHEN left_contact_id = $1 THEN right_contact_id ELSE left_contact_id END
			  FROM dedupe_candidate WHERE $1 IN (left_contact_id, right_contact_id)`, &other, subject)
	}
	return count, other
}

func TestABoughtTaggedAddressFilesOnePairWithItsBase(t *testing.T) {
	e := Setup(t)
	incumbent, subject := boughtEmailSetup(t, e)
	applyPurchase(t, e, subject, "surfe", boughtEmail("ab+accounting@example.com"))

	count, other := openContactPairsOf(t, e, subject)
	if count != 1 || other != incumbent {
		t.Fatalf("got %d pairs (other %s), want one pair with %s", count, other, incumbent)
	}
}

func TestABoughtUnrelatedAddressFilesNothing(t *testing.T) {
	e := Setup(t)
	_, subject := boughtEmailSetup(t, e)
	applyPurchase(t, e, subject, "surfe", boughtEmail("anna.beispiel@elsewhere.example"))

	if count, _ := openContactPairsOf(t, e, subject); count != 0 {
		t.Fatalf("got %d pairs, want none", count)
	}
}

// holdBoughtCreate runs a real contact create and holds its transaction open,
// with the lane locks the create took, until release is closed.
func holdBoughtCreate(t *testing.T, e *Env, store *contacts.Store, email string, release <-chan struct{}) (pid int, done <-chan error) {
	t.Helper()
	ready, finished := make(chan int, 1), make(chan error, 1)
	go func() {
		finished <- database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var pid int
			if err := tx.QueryRow(e.Admin(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			if _, err := store.CreateContactTx(e.Admin(), tx, contacts.CreateContactInput{
				FullName: "Probe Holder", Source: "manual",
				Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
			}, contacts.CustomColumns{}); err != nil {
				return err
			}
			ready <- pid
			<-release
			return nil
		})
	}()
	select {
	case pid = <-ready:
	case err := <-finished:
		t.Fatalf("the held create failed: %v", err)
	}
	return pid, finished
}

// waitForBoughtWaiters returns once n backends wait behind pid.
func waitForBoughtWaiters(t *testing.T, e *Env, pid, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		waiting := countBoughtWaiters(ctx, t, e, pid)
		if waiting >= n {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("only %d of %d writers waited behind the held create", waiting, n)
		case <-tick.C:
		}
	}
}

// countBoughtWaiters counts the backends in this test database, other than
// pid, that wait on a lock. It holds a connection only for the probe, and
// clears the statistics snapshot on that connection first.
func countBoughtWaiters(ctx context.Context, t *testing.T, e *Env, pid int) int {
	t.Helper()
	conn, err := e.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquiring the probe connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		t.Fatalf("clearing the statistics snapshot: %v", err)
	}
	var waiting int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND cardinality(pg_blocking_pids(pid)) > 0
		   AND pid <> $1`, pid).Scan(&waiting); err != nil {
		t.Fatalf("probing for waiters: %v", err)
	}
	return waiting
}

func isDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40P01"
}

// A create of an address waits first, then a purchase buying that address for
// another contact. Neither is aborted by a deadlock: the purchase takes the
// address-base lock before it writes the address.
func TestABoughtAddressAndACreateOfItNeverDeadlock(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := ids.NewV7()
	execAsOwner(t, e, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Anna Beispiel', 'manual', 'human:x')`, subject)
	run := seedRun(t, e, subject, "completed", ids.NewV7().String(), false)
	claims := boughtEmail("ab+x@example.com")

	release := make(chan struct{})
	pid, held := holdBoughtCreate(t, e, store, "ab+zz@example.com", release)
	create := make(chan error, 1)
	go func() {
		_, err := store.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: "Bea Create", Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: "ab+x@example.com", EmailType: "work", IsPrimary: true}},
		})
		create <- err
	}()
	waitForBoughtWaiters(t, e, pid, 1)
	bought := make(chan error, 1)
	go func() {
		bought <- database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if err := contacts.WriteProviderClaims(e.Admin(), tx, run, subject.String(), "surfe", claims,
				time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)); err != nil {
				return err
			}
			return contacts.ApplyProviderClaims(e.Admin(), tx, run, subject.String(), "surfe", claims)
		})
	}()
	waitForBoughtWaiters(t, e, pid, 2)
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("the held create: %v", err)
	}

	createErr, boughtErr := <-create, <-bought
	if isDeadlock(createErr) || isDeadlock(boughtErr) {
		t.Fatalf("a writer was aborted by a deadlock (create %v, purchase %v)", createErr, boughtErr)
	}
	if createErr != nil || boughtErr != nil {
		t.Fatalf("create %v, purchase %v: both must complete", createErr, boughtErr)
	}
}
