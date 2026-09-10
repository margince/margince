// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// TOTP (RFC 6238 over RFC 4226): the second factor an authenticator app
// computes and this code checks. Implemented here rather than pulled in, because
// the algorithm is a dozen lines of standard-library HMAC and a dependency for
// it would be more surface than the thing it replaces.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined over HMAC-SHA1; every authenticator app computes it this way. Not a message-integrity or collision use.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	totpDigits    = 6
	totpStep      = 30 * time.Second
	totpSecretLen = 20 // 160 bits, the key length RFC 4226 §4 recommends for HMAC-SHA1.
)

// totpEncoding is RFC 4648 base32 without padding — the form an authenticator
// app scans from an otpauth:// URI and the seal stores.
var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret returns a fresh base32 shared secret.
func newTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("identity: mint totp secret: %w", err)
	}
	return totpEncoding.EncodeToString(buf), nil
}

// totpCodeAt is the HMAC-SHA1 of the 30-second counter under the secret,
// dynamically truncated (RFC 4226 §5.3) to `digits` decimal digits.
func totpCodeAt(secret string, t time.Time, digits int) (string, error) {
	key, err := totpEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("identity: totp secret is not base32: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(t.Unix())/uint64(totpStep.Seconds()))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, truncated%mod), nil
}

// verifyTOTPCode accepts a code for the current 30-second step or either
// adjacent one — one step of clock skew, which a phone and a server drift into
// routinely — comparing in constant time so a near miss leaks nothing by timing.
func verifyTOTPCode(secret, code string, now time.Time) (bool, error) {
	for _, skew := range []time.Duration{0, -totpStep, totpStep} {
		want, err := totpCodeAt(secret, now.Add(skew), totpDigits)
		if err != nil {
			return false, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true, nil
		}
	}
	return false, nil
}
