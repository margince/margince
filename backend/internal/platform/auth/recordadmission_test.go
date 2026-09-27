// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The refusals the whole-record admissions make before any row is read. Each
// case passes a nil transaction, so an admission that forgot a check and went
// on to query would panic rather than pass. The row halves and the seat
// ceiling, which is asked after the row, are the integration suite's
// (compose/integration/recordaccess_integration_test.go), against real rows.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func withGrant(scope principal.RowScope, grant principal.ObjectGrant) principal.Principal {
	p := human(scope)
	p.SeatType = principal.SeatFull
	p.Permissions.Objects = map[string]principal.ObjectGrant{"contact": grant}
	return p
}

func TestChangingARecordNeedsTheUpdateGrant(t *testing.T) {
	p := withGrant(principal.RowScopeAll, principal.ObjectGrant{Read: true})
	err := EnsureChangeable(principal.WithActor(context.Background(), p), nil, "contact", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied: owning a row is not the verb to change it", err)
	}
}

func TestReadingARecordNeedsTheReadGrant(t *testing.T) {
	p := withGrant(principal.RowScopeAll, principal.ObjectGrant{Update: true})
	err := EnsureReadable(principal.WithActor(context.Background(), p), nil, "contact", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied: row scope never answers whether a seat reads the table", err)
	}
}
