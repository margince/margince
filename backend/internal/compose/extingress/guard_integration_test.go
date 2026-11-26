// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package extingress_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/extingress"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The guard re-reads the member and the credential inside the capture
// transaction. A removed credential, or a member who is not there, refuses.
func TestTheMemberGuardRefusesAWithdrawnCredentialAndAMissingMember(t *testing.T) {
	e := integration.Setup(t)
	answer := func(consented bool, member ids.UUID) error {
		guard := extingress.MemberGuard("unit", func(context.Context, pgx.Tx, string, ids.UUID) (bool, error) { return consented, nil })
		return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return guard(e.Admin(), tx, member) })
	}
	if err := answer(true, e.Rep1); err != nil {
		t.Fatalf("a live member with a credential was refused: %v", err)
	}
	if err := answer(false, e.Rep1); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a withdrawn credential = %v, want a permission refusal", err)
	}
	if err := answer(true, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member who is not there = %v, want a permission refusal", err)
	}
}
