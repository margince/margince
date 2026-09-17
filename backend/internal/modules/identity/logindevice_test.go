// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A device proof exempts one browser from a §27 lock on one account, so every
// way a proof could vouch for something it was not issued for has to answer
// false: another account, another password, another shape, too old, too new.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noDevice is the login a caller makes without presenting a device proof.
const noDevice = ""

const (
	proofHash      = "stands-in-for-a-password-hash"
	proofOtherHash = "stands-in-for-the-hash-after-a-reset"
)

func TestADeviceProofVouchesOnlyForTheAccountAndPasswordItWasIssuedUnder(t *testing.T) {
	owner := ids.UserID{UUID: ids.NewV7()}
	stranger := ids.UserID{UUID: ids.NewV7()}
	issued := lockoutEpoch
	proof := mintDeviceProof(owner, proofHash, issued)

	if !deviceProofVouches(proof, owner, proofHash, issued.Add(time.Hour)) {
		t.Fatal("a fresh proof does not vouch for the account it was issued to")
	}

	withMAC := func(mac string) string {
		parts := strings.Split(proof, ".")
		parts[3] = mac
		return strings.Join(parts, ".")
	}
	forged := mintDeviceProof(stranger, proofHash, issued)
	for name, tc := range map[string]struct {
		proof string
		user  ids.UserID
		hash  string
		now   time.Time
	}{
		"presented for another account": {proof, stranger, proofHash, issued},
		"another account's proof":       {forged, owner, proofHash, issued},
		"the password has changed":      {proof, owner, proofOtherHash, issued},
		"past its lifetime":             {proof, owner, proofHash, issued.Add(deviceProofTTL + time.Second)},
		"issued in the future":          {proof, owner, proofHash, issued.Add(-deviceProofSkew - time.Minute)},
		"a tampered MAC":                {withMAC(strings.Repeat("A", 43)), owner, proofHash, issued},
		"an empty MAC":                  {withMAC(""), owner, proofHash, issued},
		"none presented":                {noDevice, owner, proofHash, issued},
		"another version":               {"v0" + strings.TrimPrefix(proof, deviceProofVersion), owner, proofHash, issued},
		"a non-numeric issue time":      {strings.Join([]string{deviceProofVersion, owner.String(), "soon", "x"}, "."), owner, proofHash, issued},
		"too few parts":                 {deviceProofVersion + "." + owner.String(), owner, proofHash, issued},
	} {
		t.Run(name, func(t *testing.T) {
			if deviceProofVouches(tc.proof, tc.user, tc.hash, tc.now) {
				t.Fatalf("proof %q vouched", tc.proof)
			}
		})
	}
}

// A replica whose clock runs slightly ahead issues proofs this process must
// still accept; the tolerance is what keeps a multi-replica install from
// refusing its own browsers.
func TestADeviceProofFromASlightlyFastClockStillVouches(t *testing.T) {
	owner := ids.UserID{UUID: ids.NewV7()}
	proof := mintDeviceProof(owner, proofHash, lockoutEpoch.Add(deviceProofSkew-time.Second))
	if !deviceProofVouches(proof, owner, proofHash, lockoutEpoch) {
		t.Fatal("a proof issued inside the clock-skew tolerance was refused")
	}
}

// The proof outlives the session on purpose, and carries the session cookie's
// protections: no script reads it and no cross-site request sends it. What a
// browser sends back is what the lock is judged on.
func TestTheDeviceCookieIsHttpOnlyStrictAndReadBack(t *testing.T) {
	rec := httptest.NewRecorder()
	setDeviceCookie(rec, "stands-in-for-a-proof")
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != DeviceCookieName || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Errorf("cookie = %+v, want %s HttpOnly, Secure, SameSite=Strict on /", c, DeviceCookieName)
	}
	if c.MaxAge != int(deviceProofTTL/time.Second) {
		t.Errorf("MaxAge = %d, want the proof lifetime %d", c.MaxAge, int(deviceProofTTL/time.Second))
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	if got := presentedDeviceProof(req); got != noDevice {
		t.Errorf("a request with no cookie presented %q, want none", got)
	}
	req.AddCookie(c)
	if got := presentedDeviceProof(req); got != "stands-in-for-a-proof" {
		t.Errorf("presented %q, want the proof the cookie carries", got)
	}
}
