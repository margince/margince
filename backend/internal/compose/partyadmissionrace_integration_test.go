// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
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
