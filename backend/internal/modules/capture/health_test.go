// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The classification in isolation: each connection carries its WORST
// condition and no other, a parked mailbox raises nothing even with stale
// sidecar facts on it, and a healthy one stays silent.
func TestConnectionConcernCarriesTheWorstConditionOnly(t *testing.T) {
	auth := "auth"
	failed := &BackfillRun{Status: backfillStatusError}
	for _, tc := range []struct {
		name     string
		view     ConnectionView
		wantKind string
	}{
		{"healthy connected", ConnectionView{Status: "connected"}, ""},
		{"reauth outranks everything", ConnectionView{Status: statusReauthRequired, LastErrorClass: &auth, Backfill: failed}, ConcernReauthRequired},
		{"error state", ConnectionView{Status: statusError, LastErrorClass: &auth}, ConcernConnectionError},
		{"connected but failing", ConnectionView{Status: "connected", LastErrorClass: &auth}, ConcernSyncFailing},
		{"failed history import", ConnectionView{Status: "connected", Backfill: failed}, ConcernBackfillFailed},
		{"running import is not a failure", ConnectionView{Status: "connected", Backfill: &BackfillRun{Status: "running"}}, ""},
		{"parked mailbox stays quiet", ConnectionView{Status: statusDisconnected, LastErrorClass: &auth, Backfill: failed}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if kind := connectionConcern(tc.view); kind != tc.wantKind {
				t.Fatalf("connectionConcern = %q, want %q", kind, tc.wantKind)
			}
		})
	}
}

// The lane's withheld arm is the permission sentinel from the read itself: a
// principal with no human behind it is refused BEFORE any query — the
// registry here holds no database at all, so a refusal that did not precede
// the read would panic rather than pass.
func TestHealthConcernsRefusesAPrincipalWithNoHumanBehindIt(t *testing.T) {
	registry := NewRegistry(nil, nil, nil, nil)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:test",
	})
	if _, err := registry.HealthConcerns(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("HealthConcerns for a system principal = %v, want ErrPermissionDenied", err)
	}
	// The refusal also answers a request with no principal bound at all.
	if _, err := registry.HealthConcerns(context.Background()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("HealthConcerns with no actor = %v, want ErrPermissionDenied", err)
	}
}
