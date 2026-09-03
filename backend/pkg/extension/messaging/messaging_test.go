// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"strings"
	"testing"
	"time"
)

// The zero value permits nothing extra.
//
// This is the direction a missing declaration must fail in: a pack that forgets
// to declare an exception requires consent, and one that forgets a cap is
// uncapped by ITS OWN rules rather than by the engine's. A zero value that
// silently granted an exception would make an incomplete pack more permissive
// than a complete one.
func TestTheZeroRuleSetGrantsNothing(t *testing.T) {
	var r Rules
	if len(r.MarketingExceptions) != 0 {
		t.Error("the zero rule set grants a marketing exception")
	}
	if r.FrequencyCap != nil {
		t.Error("the zero rule set carries a cap, which would bound advertising nobody declared a bound for")
	}
	if r.OptOutAcknowledgement {
		t.Error("the zero rule set promises an acknowledgement nothing was asked to send")
	}
}

// An exception the engine would apply while checking nothing is refused.
//
// An unguarded exception is worse than no exception: it looks lawful, it
// appears in the manifest as a declared rule, and every advertising message it
// touches goes out having satisfied a condition set that is empty.
func TestAnExceptionMustNameAtLeastOneCondition(t *testing.T) {
	err := MarketingException{Kind: ExistingCustomer}.Validate()
	if err == nil {
		t.Fatal("an exception with no conditions was accepted")
	}
	if !strings.Contains(err.Error(), "unguarded") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}
}

// And one that names a condition is accepted, or the refusal above would be
// indistinguishable from a validator that rejects every exception.
func TestAnExceptionWithAConditionIsAccepted(t *testing.T) {
	if err := (MarketingException{Kind: ExistingCustomer, RequiresSaleEvidence: true}).Validate(); err != nil {
		t.Errorf("a guarded exception was refused: %v", err)
	}
}

// A kind the engine does not implement is refused rather than stored. A pack
// naming one would declare a rule that looks registered and never applies.
func TestAnUnimplementedExceptionKindIsRefused(t *testing.T) {
	if err := (MarketingException{Kind: "soft_opt_in", RequiresSaleEvidence: true}).Validate(); err == nil {
		t.Error("an exception kind nothing implements was accepted")
	}
	if err := DisclosureKind("bank_details").Validate(); err == nil {
		t.Error("a disclosure kind nothing renders was accepted")
	}
}

// A cap of zero is a ban stated as a cap, and a window of zero silences an
// address forever. Both are refused, because both are a pack meaning one thing
// and the engine reading another.
func TestACapMustBeAbleToPermitSomething(t *testing.T) {
	if err := (FrequencyCap{Messages: 0, Window: time.Hour}).Validate(); err == nil {
		t.Error("a cap permitting zero messages was accepted")
	}
	if err := (FrequencyCap{Messages: 3, Window: 0}).Validate(); err == nil {
		t.Error("a cap with no window was accepted")
	}
	if err := (FrequencyCap{Messages: 3, Window: 24 * time.Hour}).Validate(); err != nil {
		t.Errorf("an ordinary cap was refused: %v", err)
	}
}

// A rule set records the version it was taken under, so zero names nothing.
func TestRulesCarryAVersion(t *testing.T) {
	r := Rules{Jurisdiction: "de"}
	if err := r.Validate(); err == nil {
		t.Fatal("a rule set with no version was accepted")
	}
	r.Version = 1
	if err := r.Validate(); err != nil {
		t.Errorf("an ordinary rule set was refused: %v", err)
	}
}

// A negative window would reach forward, which is a window that has already
// expired for every record — the same failure class the retention period
// guards against.
func TestANegativeWindowIsRefused(t *testing.T) {
	r := Rules{Jurisdiction: "de", Version: 1, ReplyWindow: -time.Hour}
	if err := r.Validate(); err == nil {
		t.Error("a negative reply window was accepted")
	}
}

// A malformed jurisdiction code is refused: the registry keys on it, so a code
// nothing can resolve is a rule set that looks registered and never applies.
func TestAMalformedJurisdictionIsRefused(t *testing.T) {
	if err := (Rules{Jurisdiction: "DE", Version: 1}).Validate(); err == nil {
		t.Error("an upper-case jurisdiction code was accepted")
	}
}
