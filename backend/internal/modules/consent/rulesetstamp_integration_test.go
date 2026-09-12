// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Recording which rules judged a decision.
//
// messaging.Rules has said since it shipped that its Version is "stamped onto
// every decision taken under it", and nothing stamped it:
// gates/messagingruleapplied_test.go carried the gap in its register with the
// reason that communication_decision had no column for it.
//
// The promise is to a SUBJECT. Somebody asking a year later which rules judged
// their message is owed an answer from the record, and re-deriving it from
// whatever the packs say today answers a different question — packs change, and
// that is the whole reason the version exists.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// inCountry binds the env's gate and store to a pack with the given version.
func inCountry(t *testing.T, e *resolveEnv, code string, version int) {
	t.Helper()
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: jurisdiction.Code(code), Version: version,
	})
	reader := InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
		return jurisdiction.Code(code), nil
	})
	e.gate = e.gate.WithInstallationCountry(reader)
	e.store = e.store.WithInstallationCountry(reader)
}

// rulesetOf reads back what a decision recorded about the rules that judged it.
func rulesetOf(t *testing.T, e *resolveEnv, delivery ids.UUID, phase string) (*int, []string) {
	t.Helper()
	var version *int
	var codes []string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT ruleset_version, ruleset_codes
		  FROM communication_decision
		 WHERE delivery_id = $1 AND phase = $2
		 LIMIT 1`, delivery, phase).Scan(&version, &codes); err != nil {
		t.Fatalf("reading the %s decision: %v", phase, err)
	}
	return version, codes
}

// TestADecisionRecordsTheRulesetThatJudgedIt is the register line this closes.
//
// BOTH PHASES, because they answer different questions. The staging row says
// what was true when the message was composed; the transmit row says what was
// true when it left, and a delivery can sit queued across a change of either.
func TestADecisionRecordsTheRulesetThatJudgedIt(t *testing.T) {
	e := setupResolve(t)
	inCountry(t, e, "ra", 4)
	delivery := e.plantDelivery(t)

	stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)

	for _, phase := range []string{"staging", "transmit"} {
		version, codes := rulesetOf(t, e, delivery, phase)
		if version == nil || *version != 4 {
			t.Errorf("the %s decision recorded version %v, want 4 — a subject asking which "+
				"rules judged their message has only this row to read", phase, version)
		}
		if len(codes) != 1 || codes[0] != "ra" {
			t.Errorf("the %s decision recorded codes %v, want [ra]", phase, codes)
		}
	}
}

// TestAnInstallationDeclaringNoCountryRecordsNoRuleset.
//
// NULL is a real answer and not a gap: no country resolves to no rules at all,
// and a zero version would read as a ruleset that exists and has none. The
// consent requirement that binds everywhere is enforced by the marketing
// verdict, which runs before any pack.
func TestAnInstallationDeclaringNoCountryRecordsNoRuleset(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)

	for _, phase := range []string{"staging", "transmit"} {
		version, codes := rulesetOf(t, e, delivery, phase)
		if version != nil {
			t.Errorf("the %s decision recorded version %d with no jurisdiction declared — "+
				"that names a ruleset which does not exist", phase, *version)
		}
		if len(codes) != 0 {
			t.Errorf("the %s decision recorded codes %v with no jurisdiction declared", phase, codes)
		}
	}
}

// TestErasureKeepsTheRulesetAndLosesTheSubject.
//
// erasure_consent.go already promised this in its own comment — "the verdict,
// the category, the reason and the ruleset survive as an unattributed statistic
// about a send that happened" — while no column carried a ruleset. Now one
// does, so the promise is testable rather than aspirational.
func TestErasureKeepsTheRulesetAndLosesTheSubject(t *testing.T) {
	e := setupResolve(t)
	inCountry(t, e, "rb", 2)
	delivery := e.plantDelivery(t)

	stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)

	// The erasure write itself, as privacy performs it: the address is
	// tombstoned and the subject link cut, in place.
	//
	// RESTATED, not called. deleteConsentCapabilities is unexported in the
	// privacy module and this suite is in consent, so what is proven here is
	// that the two new columns survive THAT shape of update — not that the
	// production eraser runs it. The statement is copied from
	// privacy/erasure_consent.go, and gates/piicolumncoverage_test.go is what
	// fails if a future column escapes the redaction's accounting.
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE delivery_id = $1`, delivery); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	version, codes := rulesetOf(t, e, delivery, "transmit")
	if version == nil || *version != 2 || len(codes) != 1 || codes[0] != "rb" {
		t.Errorf("erasure destroyed the ruleset: version %v codes %v. The controller must "+
			"still be able to answer for a message it already sent, which is why the "+
			"decision survives the subject", version, codes)
	}
}

// TestTheRecordedRulesetIsRefusedWhenItNamesNoJurisdiction.
//
// array_length answers NULL for an empty array, and a CHECK evaluating to NULL
// is ACCEPTED. Written as `= 1` alone the constraint admitted a version
// alongside zero jurisdictions — the exact pair it exists to refuse. The
// production writer cannot produce that pair, so this asks the database
// directly rather than through the gate.
func TestTheRecordedRulesetIsRefusedWhenItNamesNoJurisdiction(t *testing.T) {
	e := setupResolve(t)
	inCountry(t, e, "rc", 3)
	delivery := e.plantDelivery(t)
	stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)

	for _, bad := range []struct {
		name    string
		version any
		codes   any
	}{
		{"a version with no codes at all", 3, []string{}},
		{"codes recorded as an empty array", nil, []string{}},
	} {
		_, err := e.owner.Exec(context.Background(), `
			UPDATE communication_decision
			   SET ruleset_version = $2, ruleset_codes = $3
			 WHERE delivery_id = $1`, delivery, bad.version, bad.codes)
		if err == nil {
			t.Errorf("%s was accepted — a decision that recorded no jurisdiction must say "+
				"so with NULL, so the record has one spelling for it and not two", bad.name)
		}
	}
}

// TestTheRecordedRulesetIsTheOneThatJudgedNotTheOneLiveAtWriteTime.
//
// Codex found this: the stamp was read AFTER the decisions were taken, in the
// function that writes the rows. An installation changing its declared country
// mid-transmit would have had its frequency ceiling applied under the old pack
// while the row named the new one — a record saying Vietnam judged a message
// Germany judged.
//
// The country reads are separate unlocked SELECTs under READ COMMITTED, so
// sharing one transaction does not make two of them agree. Reading once, before
// any decision, is what does.
//
// Mutation: move the rulesetStamp call in stageDecisions back below the decide
// loop and this fails.
func TestTheRecordedRulesetIsTheOneThatJudgedNotTheOneLiveAtWriteTime(t *testing.T) {
	e := setupResolve(t)
	inCountry(t, e, "rd", 5)
	delivery := e.plantDelivery(t)

	// The country flips the moment the first decision has been taken, which is
	// exactly the window the old ordering left open.
	flipped := false
	e.gate = e.gate.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			if flipped {
				return jurisdiction.Code("re"), nil
			}
			flipped = true
			return jurisdiction.Code("rd"), nil
		}))
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: jurisdiction.Code("re"), Version: 6,
	})

	stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)

	version, codes := rulesetOf(t, e, delivery, "staging")
	if version == nil || *version != 5 || len(codes) != 1 || codes[0] != "rd" {
		t.Errorf("the staging decision recorded version %v codes %v, want 5 and [rd] — "+
			"the row names a ruleset that judged nothing", version, codes)
	}
}
