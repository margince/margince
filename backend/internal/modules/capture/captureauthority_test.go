// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func grantsOn(objects ...string) principal.Permissions {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Create: true, Read: true}
	}
	return principal.Permissions{Objects: grants}
}

// The decider and the sink's own gates must answer alike. A member the decider
// admits passes the creates the sink asks for; one it refuses fails one.
func TestCaptureRefusalAgreesWithTheSinksOwnGates(t *testing.T) {
	cases := []struct {
		name  string
		perms principal.Permissions
		seat  principal.SeatType
	}{
		{"every writer grant on a full seat", grantsOn("activity", "contact", "lead"), principal.SeatFull},
		{"no activity create", grantsOn("contact", "lead"), principal.SeatFull},
		{"no contact create", grantsOn("activity", "lead"), principal.SeatFull},
		{"no lead create", grantsOn("activity", "contact"), principal.SeatFull},
		{"a read seat holding every grant", grantsOn("activity", "contact", "lead"), principal.SeatRead},
		{"nothing at all", principal.Permissions{}, principal.SeatFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{
				Type: principal.PrincipalConnector, Permissions: tc.perms, SeatType: tc.seat,
			})
			sinkAdmits := tc.seat.CanMutate()
			for _, object := range []string{capturedActivityObject, capturedContactObject, capturedLeadObject} {
				sinkAdmits = sinkAdmits && auth.Require(ctx, object, principal.ActionCreate) == nil
			}
			refusal := CaptureRefusal(tc.perms, tc.seat)
			if (refusal == nil) != sinkAdmits {
				t.Fatalf("CaptureRefusal = %v, but the sink's gates admit = %v", refusal, sinkAdmits)
			}
		})
	}
}

// The refusal is a 403 with its own code. It names the grant the member
// lacks, in the sentence and as `missing` for a client's own wording.
func TestRoleCannotCaptureNamesTheMissingGrant(t *testing.T) {
	cases := []struct {
		perms   principal.Permissions
		seat    principal.SeatType
		missing string
		says    string
	}{
		{grantsOn("contact", "lead"), principal.SeatFull, "activity", "cannot create activities"},
		{grantsOn("activity", "lead"), principal.SeatFull, "contact", "cannot create contacts"},
		{grantsOn("activity", "contact"), principal.SeatFull, "lead", "cannot create leads"},
		{grantsOn("activity", "contact", "lead"), principal.SeatRead, "seat", "seat is read-only"},
	}
	for _, tc := range cases {
		t.Run(tc.missing, func(t *testing.T) {
			err := CaptureRefusal(tc.perms, tc.seat)
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("refusal %v does not unwrap to ErrPermissionDenied", err)
			}
			fault, ok := httperr.Classify(err)
			if !ok || fault.Status != http.StatusForbidden || fault.Code != CodeRoleCannotCapture {
				t.Fatalf("classified as %+v (%v), want 403 %s", fault, ok, CodeRoleCannotCapture)
			}
			if fault.Details["missing"] != tc.missing || !strings.Contains(fault.Detail, tc.says) {
				t.Fatalf("details %v and detail %q, want missing=%s and %q", fault.Details, fault.Detail, tc.missing, tc.says)
			}
		})
	}
}

// A sync or backfill page asks before it reads the credential: a member who may
// no longer capture never reaches the vault or the provider.
func TestSyncCredentialRefusesBeforeTheCredentialIsRead(t *testing.T) {
	ref := "never-resolved"
	r := &Registry{} // no vault: resolving the ref would fail with a different error
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector, Permissions: grantsOn("contact", "lead"), SeatType: principal.SeatFull,
	})
	_, err := r.syncCredential(ctx, &ref, nil)
	if _, refused := errors.AsType[*RoleCannotCaptureError](err); !refused {
		t.Fatalf("syncCredential = %v, want the role refusal", err)
	}
}
