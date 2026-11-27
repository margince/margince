// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The refusals that hold when the schema's own guarantees slip. The dedupe
// index makes a live address unique, and the instruction lock keeps a decision
// live until it is spent. Each reader below refuses anyway rather than trusting
// that. The address cases lift the index inside a transaction they roll back,
// so no other test sees the gap.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withAddressOnTwoContacts runs check on an owner transaction in which the
// env's address is also live on a second contact.
func (e *resolveEnv) withAddressOnTwoContacts(t *testing.T, check func(tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the lifted index: %v", err)
		}
	}()
	second := ids.New[ids.ContactKind]()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`DROP INDEX uq_contact_email_dedupe`, nil},
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Dana Twin', 'manual', 'human:x')`, []any{second}},
		{`INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		  VALUES ($1, $2, true, 'manual', 'human:x')`, []any{second, e.address}},
	} {
		if _, err := tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("%s: %v", stmt.sql, err)
		}
	}
	check(tx)
}

func TestTheSendGateRefusesAnAddressLiveOnTwoContacts(t *testing.T) {
	t.Parallel()
	e := setupResolve(t)
	e.withAddressOnTwoContacts(t, func(tx pgx.Tx) {
		_, found, err := resolveContact(e.ctx, tx, connector.Recipient{Email: e.address})
		if found || !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Errorf("an address on two live contacts resolved found=%v err=%v, want ErrConsentNotGranted", found, err)
		}
	})
}

func TestAnUnsubscribeLinkIsNotMintedForAnAddressLiveOnTwoContacts(t *testing.T) {
	t.Parallel()
	e := setupResolve(t)
	e.withAddressOnTwoContacts(t, func(tx pgx.Tx) {
		_, found, err := liveAddressHolderTx(e.ctx, tx, e.address)
		if found || !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("an address on two live contacts resolved found=%v err=%v, want ErrConflict", found, err)
		}
	})
}

// A decision that is no longer directed cannot be spent, so the message it
// was about is not staged under it.
func TestADecisionNoLongerLiveIsNotSpent(t *testing.T) {
	t.Parallel()
	e := setupResolve(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rollback: %v", err)
		}
	}()

	err = consumeInstructionTx(e.ctx, tx, LiveInstruction{ID: ids.NewV7()}, ids.NewV7(), [32]byte{})

	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("spending a decision that is not live answered %v, want ErrConflict", err)
	}
}
