// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The signing secret: the only thing that admits a request to the anonymous
// edge, and the one value this unit shows a member exactly once.
//
// SHOWN ONCE, AND NEVER RETURNED AGAIN. No operation reads it back, masked or
// otherwise, because a credential a governed surface can read back is one every
// holder of that surface's RBAC object holds — and holding this one is
// indistinguishable from being the sender. What a member gets back later is the
// endpoint row, which says an endpoint exists and who owns it.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// signingSecretBytes is the minted key's length. Thirty-two, because the MAC is
// HMAC-SHA256: a longer key buys nothing — HMAC folds anything over the hash's
// block size back through the hash first — and a shorter one is the only part
// of this scheme an attacker could search.
const signingSecretBytes = 32

// mintSecret seals a fresh signing secret for the caller's own endpoint and
// hands it back once.
//
// The same operation opens and rotates, because they are the same act: after
// either, every secret this endpoint previously issued stops verifying. There
// is no operation that adds a second valid secret, so a member rotating theirs
// knows exactly what breaks and when.
func mintSecret(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		EndpointID string `json:"endpoint_id"`
	}](in)
	if err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "minting a signing secret")
	if err != nil {
		return nil, err
	}
	var (
		secret string
		stored endpoint
	)
	// THE SEAL HAPPENS BETWEEN TWO TRANSACTIONS AND INSIDE NEITHER, and that
	// shape is forced rather than chosen. Secrets().PutUser opens a pool
	// transaction of its own, so calling it while one of this unit's is open
	// takes a second connection while holding the first — which on a small
	// pool does not fail, it hangs, and enough concurrent mints hang every
	// caller until cancellation. It is the same nesting the capture port
	// refuses outright.
	//
	// So: claim the right to mint under a row lock, close that transaction,
	// seal, then record under a version the claim pinned. The claim is what
	// serialises two overlapping mints — the second blocks on the first's
	// FOR UPDATE and comes away with a different version — and the recording
	// transaction refuses if the version moved under it, so a mint that lost
	// the race is told it lost instead of reporting success.
	var claimed int
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err := lockedEndpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if before == nil {
			// Refused BEFORE anything is sealed: material stored under a
			// member who owns no endpoint is a credential this surface has no
			// operation to revoke.
			return errNoEndpoint()
		}
		if before.ID != args.EndpointID {
			// A staged approval names its subject by id, which means the id is
			// a request argument — but this operation still acts on the
			// caller's own endpoint only. Naming another one answers exactly
			// as an endpoint that does not exist: existence stays hidden, and
			// there is no way to distinguish "wrong id" from "somebody else's
			// endpoint" from the outside.
			return errNoEndpoint()
		}
		secret, err = newSigningSecret()
		if err != nil {
			return err
		}
		// The seal, and then the record of it. In that order because the
		// sealed material is what actually changes who can reach this
		// installation: a ledger row written first would name a moment at
		// which nothing had yet changed, and "when did this endpoint's secret
		// last change" is exactly the question the row exists to answer. The
		// row lock above is what keeps this seal from racing another one.
		claimed = before.Version
		return nil
	}); err != nil {
		return nil, err
	}

	// Outside every transaction of this unit's, for the reason stated above.
	if err := rt.Secrets().PutUser(ctx, extension.UserID(member), inboundSecretKey, []byte(secret)); err != nil {
		return nil, err
	}

	var before *endpoint
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err = lockedEndpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if before == nil || before.Version != claimed {
			// Another mint sealed between this one's claim and this write.
			// Both secrets reached the store and the later one is what the
			// endpoint now verifies with — which may not be the one this call
			// is holding, so it must not be returned as though it were live.
			return fmt.Errorf("%w: another signing secret was minted for this endpoint while this one was being sealed, so the value this call would have returned may already have been replaced — mint again and use the secret that mint returns", extension.ErrConflict)
		}
		stored, err = scanEndpoint(tx.QueryRow(ctx,
			`UPDATE `+endpointTable+` SET version = version + 1, updated_at = now()
			 WHERE user_id = $1::uuid AND slug = $2
			 RETURNING `+endpointColumns, member, inboundSlug).Scan)
		if err != nil {
			return err
		}
		return recordEndpoint(ctx, tx, extension.AuditUpdate, eventSecretMinted, before, &stored)
	}); err != nil {
		// EVERY failure below the seal reaches here, and every one of them
		// happens after the material was stored: the endpoint verifies with
		// the new secret from that moment, so a bare error would read as
		// "nothing changed" — the one answer that is false. A caller told that
		// leaves every configured sender broken and does not know it.
		//
		// The conflict is passed through as itself so the sentinel survives:
		// it is the one failure here that means "somebody else's secret is the
		// live one", which is a different instruction from "yours is live and
		// unrecorded".
		if errors.Is(err, extension.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("openchannel: the signing secret has already rotated and every already-configured sender must be reconfigured with the new one, but recording that rotation failed, so the new value was not recorded: %s", err.Error())
	}
	return json.Marshal(struct {
		SigningSecret string   `json:"signing_secret"`
		Endpoint      endpoint `json:"endpoint"`
	}{SigningSecret: secret, Endpoint: stored})
}

// newSigningSecret mints the key a sender will sign with, hex-encoded so it
// survives being pasted into whatever configures that sender.
func newSigningSecret() (string, error) {
	key := make([]byte, signingSecretBytes)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("openchannel: the system random source is unavailable, so no signing secret can be minted: %w", err)
	}
	return hex.EncodeToString(key), nil
}
