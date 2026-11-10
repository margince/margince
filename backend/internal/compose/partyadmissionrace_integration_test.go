// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

type afterPartyAuthorityRead struct {
	capture.PartyAuthority
	after func()
	once  sync.Once
}

func (a *afterPartyAuthorityRead) EffectiveAuthority(ctx context.Context, workspace, user ids.UUID) (authz.RBAC, principal.SeatType, error) {
	rbac, seat, err := a.PartyAuthority.EffectiveAuthority(ctx, workspace, user)
	if err == nil {
		a.once.Do(a.after)
	}
	return rbac, seat, err
}

func TestConcurrentPartyAdmissionsKeepTheSharedFilingLimit(t *testing.T) {
	e := integration.Setup(t)
	var copied []string
	for index := range 30 {
		copied = append(copied, fmt.Sprintf("Buyer %d <buyer%d@buyer.test>", index, index))
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ref := capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, strings.Join(copied, ", "), "", "Could we discuss the product requirements?")
	ctx := threadConnectorCtx(e, e.AdminUser)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; results <- sink.AdmitActivityParties(ctx, ids.From[ids.ActivityKind](ref.ID)) }()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	created := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) LIKE '%@buyer.test' AND archived_at IS NULL`)
	filed := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='contact'`, ref.ID)
	if created != filed || created < 2 || created >= len(copied) {
		t.Fatalf("created=%d filed=%d recipients=%d", created, filed, len(copied)+1)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1`, ref.ID) > 25 {
		t.Fatal("concurrent admission exceeded the filing limit")
	}
}

func TestAnAdmissionTakesTheSeatBeforeItSharesTheActivity(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ref := capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Buyer <buyer@buyer.test>", "", "Could we discuss the product requirements?")
	ctx := threadConnectorCtx(e, e.AdminUser)

	ensure, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ensure.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	var ensurePID int32
	if err := ensure.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&ensurePID); err != nil {
		t.Fatal(err)
	}
	if _, err := ensure.Exec(ctx, `SELECT 1 FROM app_user WHERE id=$1 FOR NO KEY UPDATE`, e.AdminUser); err != nil {
		t.Fatal(err)
	}

	admitted, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		admitted <- sink.AdmitActivityParties(ctx, ids.From[ids.ActivityKind](ref.ID))
	}()
	observer := integration.OwnerConn(t)
	testdb.AwaitQueuedBehind(t, observer, ensurePID, 1, returned, "the admission on the held seat")

	// The next lock an ensure asks for. Waiting for it would let a retry hide the
	// cycle. NOWAIT asks whether the admission holds this row while it waits.
	if _, err := ensure.Exec(ctx, `SELECT 1 FROM activity WHERE id=$1 FOR UPDATE NOWAIT`, ref.ID); err != nil {
		t.Fatalf("the admission holds the activity while it waits for the seat: %v", err)
	}
	if err := ensure.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
}

func TestAnOlderAdmissionCannotRetireNewerSourceEvidence(t *testing.T) {
	e := integration.Setup(t)
	captured := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	rec := partiesMailRecord(t, e, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	ctx := threadConnectorCtx(e, e.AdminUser)
	ref, err := captured.Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithPartyAuthority(&afterPartyAuthorityRead{
		PartyAuthority: identity.NewServiceFor(e.DB()),
		after: func() {
			rec.Containers = append(rec.Containers, "Archive")
			if _, err := captured.Upsert(ctx, rec); err != nil {
				t.Fatal(err)
			}
		},
	})
	if err := sink.AdmitActivityParties(ctx, ids.From[ids.ActivityKind](ref.ID)); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_pending AND admission_generation>1`, ref.ID) != 1 {
		t.Fatal("an older admission discarded newer source evidence")
	}
	if err := sink.RecoverPartyAdmissions(principal.SystemActing(e.Admin(), "party-recovery-test"), 200); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_pending`, ref.ID) != 0 {
		t.Fatal("current source evidence never settled")
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 1 {
		t.Fatal("recovery duplicated or lost the recipient")
	}
}

// authorityBeforeRecords counts the record locks the first in-transaction
// authority read finds its transaction already holding.
type authorityBeforeRecords struct {
	capture.PartyAuthority
	once     sync.Once
	read     bool
	rowLocks int
	err      error
}

func (a *authorityBeforeRecords) EffectiveAuthorityTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (authz.RBAC, principal.SeatType, error) {
	a.once.Do(func() {
		a.read = true
		a.err = tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid = pg_backend_pid()
			AND locktype = 'relation' AND mode IN ('RowShareLock', 'RowExclusiveLock')`).Scan(&a.rowLocks)
	})
	return a.PartyAuthority.EffectiveAuthorityTx(ctx, tx, user)
}

// A member's removal takes the authorization lock and then locks that member's
// records. Admission reads authority before it locks a record, so the two
// cannot wait on each other in a cycle.
func TestPartyAdmissionReadsAuthorityBeforeItLocksARecord(t *testing.T) {
	e := integration.Setup(t)
	captured := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	ref := capturePartiesMailUsing(t, e, captured, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	authority := &authorityBeforeRecords{PartyAuthority: identity.NewServiceFor(e.DB())}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithPartyAuthority(authority)
	if err := sink.AdmitActivityParties(threadConnectorCtx(e, e.AdminUser), ids.From[ids.ActivityKind](ref.ID)); err != nil {
		t.Fatal(err)
	}
	if !authority.read || authority.err != nil {
		t.Fatalf("admission read authority in its transaction: %v (%v)", authority.read, authority.err)
	}
	if authority.rowLocks != 0 {
		t.Errorf("the authority read came after %d record locks, want none", authority.rowLocks)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 1 {
		t.Fatal("admission did not file the recipient")
	}
}
