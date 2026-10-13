// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package diffhash_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
)

// Two different proposed changes must not take the same diff_hash.
//
// A diff_hash is the idempotency key: staging reads "same kind, same target, same
// hash, still pending" as "this exact call is already staged" and drops the second
// call as a duplicate. So a collision is not a cosmetic rounding — it is one
// caller's change silently replaced by another's, with the caller told their
// request was already handled.
//
// 2^53+1 is the first integer a float64 cannot hold, so it canonicalized to 2^53
// and hashed as it.
func TestTwoChangesThatDifferAboveTwoToTheFiftyThreeHashDifferently(t *testing.T) {
	t.Parallel()

	const justOverTheLine = `{"amount_minor":9007199254740993}`
	const onTheLine = `{"amount_minor":9007199254740992}`

	overBytes, overHash, err := diffhash.Canonical(json.RawMessage(justOverTheLine))
	if err != nil {
		t.Fatalf("canonicalizing %s: %v", justOverTheLine, err)
	}
	_, onHash, err := diffhash.Canonical(json.RawMessage(onTheLine))
	if err != nil {
		t.Fatalf("canonicalizing %s: %v", onTheLine, err)
	}

	if overHash == onHash {
		t.Errorf("%s and %s take the same diff_hash %s — staging reads the second as a duplicate "+
			"of the first and drops it", justOverTheLine, onTheLine, overHash)
	}
	// And the value that TRAVELS is the one the caller sent: these bytes are what
	// modify-then-approve re-decodes and a resumed agent run acts on.
	if !strings.Contains(string(overBytes), "9007199254740993") {
		t.Errorf("the canonical bytes read %s, want the amount the caller proposed — the rounded "+
			"value is what a resumed run would act on", overBytes)
	}
}

// Every integer a bigint column can hold survives, at every depth. int64's maximum
// is the one that rounded to a value int64 cannot even store.
func TestCanonicalKeepsEveryBigintItIsGiven(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"int64 max at the top level":  `{"n":9223372036854775807}`,
		"int64 min at the top level":  `{"n":-9223372036854775808}`,
		"nested in an object":         `{"outer":{"n":9223372036854775807}}`,
		"inside an array":             `{"list":[9223372036854775807]}`,
		"beside a float that is real": `{"n":9223372036854775807,"rate":1.5}`,
	} {
		canonical, _, err := diffhash.Canonical(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(string(canonical), "9223372036854775807") &&
			!strings.Contains(string(canonical), "-9223372036854775808") {
			t.Errorf("%s: canonicalized to %s, losing the integer", name, canonical)
		}
	}
}

// A real fraction is still a fraction: keeping integers exact must not turn 1.5
// into something else, or a rate or a multiplier would canonicalize wrong.
func TestCanonicalKeepsAFractionAFraction(t *testing.T) {
	t.Parallel()

	canonical, _, err := diffhash.Canonical(json.RawMessage(`{"rate":1.5,"zero":0,"neg":-2.25}`))
	if err != nil {
		t.Fatalf("canonicalizing: %v", err)
	}
	for _, want := range []string{`"rate":1.5`, `"zero":0`, `"neg":-2.25`} {
		if !strings.Contains(string(canonical), want) {
			t.Errorf("canonicalized to %s, want it to carry %s", canonical, want)
		}
	}
}

// The canonicalization still sorts keys at every depth, which is the property the
// whole package exists for — "identical call" has to be about content, not a
// client's key order.
func TestCanonicalStillSortsKeysAtEveryDepth(t *testing.T) {
	t.Parallel()

	_, oneWay, err := diffhash.Canonical(json.RawMessage(`{"b":1,"a":{"d":2,"c":9007199254740993}}`))
	if err != nil {
		t.Fatalf("canonicalizing: %v", err)
	}
	_, otherWay, err := diffhash.Canonical(json.RawMessage(`{"a":{"c":9007199254740993,"d":2},"b":1}`))
	if err != nil {
		t.Fatalf("canonicalizing: %v", err)
	}
	if oneWay != otherWay {
		t.Errorf("the same object in two key orders hashed %s and %s", oneWay, otherWay)
	}
}

// One object, not a stream: a payload carrying a second value would otherwise hash
// only the first, against input the rest contradicts.
func TestCanonicalRefusesWhatIsNotOneObject(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"an array":         `[1,2]`,
		"a bare scalar":    `7`,
		"null":             `null`,
		"two objects":      `{"a":1} {"b":2}`,
		"trailing garbage": `{"a":1} x`,
		// A stray CLOSING delimiter, which is the case More() cannot see: it
		// answers false on `}` and `]`, so these read as one clean object and the
		// trailing byte vanished. The guarantee is enforced by a second Decode
		// requiring EOF, and these are what hold it.
		"a trailing brace":   `{"a":1}}`,
		"a trailing bracket": `{"a":1}]`,
	} {
		if _, _, err := diffhash.Canonical(json.RawMessage(raw)); err == nil {
			t.Errorf("%s (%s) canonicalized without complaint", name, raw)
		}
	}
}

// A comparison of two changes is lossless on both sides. sameJSONValue decodes each
// and re-marshals to compare, so through float64 two amounts differing only above
// 2^53 compare EQUAL — and that comparison is what decides whether an edit changed
// anything at all.
func TestTwoValuesThatDifferAboveTwoToTheFiftyThreeDoNotDecodeAlike(t *testing.T) {
	t.Parallel()

	over, err := diffhash.DecodeValue(json.RawMessage(`{"amount_minor":9007199254740993}`))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	on, err := diffhash.DecodeValue(json.RawMessage(`{"amount_minor":9007199254740992}`))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	overBytes, err := json.Marshal(over)
	if err != nil {
		t.Fatalf("re-marshalling: %v", err)
	}
	onBytes, err := json.Marshal(on)
	if err != nil {
		t.Fatalf("re-marshalling: %v", err)
	}
	if string(overBytes) == string(onBytes) {
		t.Errorf("two changes differing by one minor unit re-marshal alike as %s — an edit that "+
			"raised the amount reads as having changed nothing", overBytes)
	}
}

// And DecodeValue takes what is not an object, because a member of a change may be
// any JSON value — that is the whole reason it exists beside DecodeObject.
func TestDecodeValueTakesAnyJSONValue(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"an array":  `[9007199254740993]`,
		"a number":  `9007199254740993`,
		"a string":  `"x"`,
		"null":      `null`,
		"an object": `{"a":9007199254740993}`,
	} {
		if _, err := diffhash.DecodeValue(json.RawMessage(raw)); err != nil {
			t.Errorf("%s (%s): %v", name, raw, err)
		}
	}
	if _, err := diffhash.DecodeValue(json.RawMessage(`7]`)); err == nil {
		t.Error("a value with a trailing delimiter was accepted")
	}
}
