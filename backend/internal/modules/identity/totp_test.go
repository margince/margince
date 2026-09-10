// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"testing"
	"time"
)

// The RFC 6238 Appendix B reference vectors (SHA-1, 8 digits), seed
// "12345678901234567890" — base32 GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ. Proving the
// algorithm against the published vectors is what lets every authenticator app
// agree with this code without a shared test between them.
func TestTOTPMatchesRFC6238Vectors(t *testing.T) {
	const seed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, c := range cases {
		got, err := totpCodeAt(seed, time.Unix(c.unix, 0), 8)
		if err != nil {
			t.Fatalf("totpCodeAt(%d): %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("totp at %d = %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestVerifyTOTPAcceptsCurrentAndAdjacentStepsOnly(t *testing.T) {
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatalf("newTOTPSecret: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)

	current, err := totpCodeAt(secret, now, totpDigits)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := verifyTOTPCode(secret, current, now); !ok {
		t.Error("the current code was rejected")
	}

	// A code from the previous 30s step is accepted — one step of clock skew is
	// the allowance a phone and a server drift into routinely.
	prev, _ := totpCodeAt(secret, now.Add(-30*time.Second), totpDigits)
	if ok, _ := verifyTOTPCode(secret, prev, now); !ok {
		t.Error("a code one step old was rejected; the skew window is too tight")
	}

	// Two steps away is outside the window and must be refused — otherwise the
	// window is wide enough to double a code's useful life.
	stale, _ := totpCodeAt(secret, now.Add(-60*time.Second), totpDigits)
	if ok, _ := verifyTOTPCode(secret, stale, now); ok {
		t.Error("a code two steps old was accepted; the window is too wide")
	}
}
