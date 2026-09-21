// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The §27 lock keys on the account, because a lock keyed on the caller is
// defeated by a caller who changes address. Keyed that way it also answers
// whoever trips it: five wrong passwords sent for somebody else's address lock
// the account's owner out, from anywhere, as often as the sender repeats them.
//
// A device proof separates the owner from the sender without weakening the
// lock against guessing. A browser that has signed in to the account holds one;
// while the account is locked, a login presenting a proof that vouches for THAT
// account is judged on its password as though no lock were set. Everyone else
// is refused exactly as before. Holding a proof means the password was already
// known on that browser, so exempting it buys a guesser nothing.
//
// The proof is an HMAC over the account id and its issue time, keyed with the
// account's stored password hash. That key is a secret the server already
// holds per account and never discloses, so nothing new is stored and there is
// no installation key to provision; and because a reset or a change replaces
// the hash, it retires every proof the old password vouched for without a
// revocation list.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DeviceCookieName carries a device proof between successful logins.
const DeviceCookieName = "crm_device"

// deviceProofTTL is how long a browser stays recognised after the login that
// issued its proof. Every successful login reissues it, so an account in use
// keeps its browsers; one idle past this is judged like a stranger's.
const deviceProofTTL = 90 * 24 * time.Hour

// deviceProofVersion prefixes the proof, so a future change of shape is refused
// by the old reader rather than misread by it.
const deviceProofVersion = "v1"

// deviceProofContext separates this MAC from any other use of the same key.
const deviceProofContext = "margince login device\x00"

// deviceProofSkew tolerates an issue time slightly ahead of this process's
// clock, which a proof minted by a sibling replica can carry.
const deviceProofSkew = 5 * time.Minute

// mintDeviceProof issues the proof a successful login hands the browser.
func mintDeviceProof(user ids.UserID, passwordHash string, now time.Time) string {
	issued := strconv.FormatInt(now.Unix(), 10)
	return strings.Join([]string{deviceProofVersion, user.String(), issued, deviceProofMAC(user, issued, passwordHash)}, ".")
}

// deviceProofVouches reports whether proof was issued to user under its current
// password hash and is still inside its lifetime. Any malformed, foreign,
// expired or future-dated proof answers false — the caller then treats the
// login as having presented none.
func deviceProofVouches(proof string, user ids.UserID, passwordHash string, now time.Time) bool {
	parts := strings.Split(proof, ".")
	if len(parts) != 4 || parts[0] != deviceProofVersion || parts[1] != user.String() {
		return false
	}
	issuedUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return false
	}
	issued := time.Unix(issuedUnix, 0)
	if issued.After(now.Add(deviceProofSkew)) || now.Sub(issued) > deviceProofTTL {
		return false
	}
	want := deviceProofMAC(user, parts[2], passwordHash)
	return hmac.Equal([]byte(parts[3]), []byte(want))
}

func deviceProofMAC(user ids.UserID, issued, passwordHash string) string {
	mac := hmac.New(sha256.New, []byte(passwordHash))
	mac.Write([]byte(deviceProofContext + user.String() + "\x00" + issued))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
