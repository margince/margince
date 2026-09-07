// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The exception against a real flag row: who grants it, and the one condition
// this tree cannot answer.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// allows runs the evaluator inside a transaction, the way the verdict does.
func (e *resolveEnv) allows(t *testing.T, exception *messaging.MarketingException) bool {
	t.Helper()
	var out bool
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = existingCustomerAllows(e.ctx, tx, e.person.String(), exception)
		return err
	}); err != nil {
		t.Fatalf("asking the exception: %v", err)
	}
	return out
}

// flag records a sale. person_id is the primary key, so this is the row.
func (e *resolveEnv) flag(t *testing.T, saleRef, goods string) {
	t.Helper()
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO consent_existing_customer_flag
		    (person_id, sale_reference, collected_at, similar_goods_note, optout_notice_given)
		VALUES ($1, $2, now(), $3, true)`, e.person, saleRef, goods); err != nil {
		t.Fatalf("recording the sale: %v", err)
	}
}

// A GERMAN SALE AUTHORIZES NOTHING WHERE NO PACK GRANTS THE EXCEPTION.
//
// The flag read used to be a bare EXISTS with no reference to where the
// installation is, so a Vietnamese installation — whose pack declares no
// MarketingExceptions at all — took marketing authority from a German sale.
// extensions/vn/vn.go asserts this cannot happen; until now nothing held it.
func TestNoPackNoException(t *testing.T) {
	e := setupResolve(t)
	e.flag(t, "INV-2026-114", "espresso machines")

	if e.allows(t, nil) {
		t.Fatal("a jurisdiction that grants no exception granted one: evidence of a German sale " +
			"authorized marketing where §7(3) does not apply")
	}
}

// AN EXCEPTION REQUIRING SIMILARITY IS REFUSED, because nothing on a send names
// the goods it advertises and the transmit phase carries no marketing purpose
// at all. Approximating it with the purpose key would be satisfiable by an
// operator typing that key into similar_goods_note.
func TestSimilarityCannotBeAnsweredSoTheExceptionRefuses(t *testing.T) {
	e := setupResolve(t)
	e.flag(t, "INV-2026-114", "espresso machines")

	if e.allows(t, &messaging.MarketingException{
		Kind: messaging.ExistingCustomer, RequiresSimilarity: true,
	}) {
		t.Fatal("allowed on a similarity condition this tree cannot evaluate: one purchase " +
			"would become a permanent mailing list")
	}
}

// AND A PACK THAT DOES NOT REQUIRE SIMILARITY STILL GETS ITS EXCEPTION, so the
// refusal above is about the condition and not about the exception itself.
func TestAnExceptionWithoutSimilarityStillApplies(t *testing.T) {
	e := setupResolve(t)
	e.flag(t, "INV-2026-114", "espresso machines")

	if !e.allows(t, &messaging.MarketingException{
		Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true,
	}) {
		t.Fatal("a recorded sale did not satisfy an exception asking only for one")
	}
}
