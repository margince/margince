// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The anonymous edge: verify a signed request, record it, reveal nothing.
//
// WHAT THE CORE HAS ALREADY DONE by the time receive runs — the method guard,
// the undeclared slug, an ungrammatical or over-long ref, both rate buckets,
// the body cap, the three headers and the freshness window — is not redone
// here. What is left is the part the core cannot do, because it interprets
// nothing in the ref and cannot read a secret in this unit's namespace: resolve
// the ref to its owner, read that owner's secret, verify, and enqueue.
//
// EVERY REFUSAL IS THE SAME REFUSAL. An address nobody holds — including the
// bare mounted path, which carries no address at all — a paused endpoint, an
// endpoint whose secret was never minted, a replayed nonce and a wrong
// signature all answer InboundUnauthenticated, which the core turns into one
// opaque 401 with an empty body. Nothing here logs which of them it was, and
// nothing here logs the secret or the presented signature: a log line is where
// the distinction reappears for whoever can read one, and telling them apart
// enumerates the installation's endpoints.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/margince/margince/backend/pkg/extension"
)

// maxPendingInbound bounds what one endpoint may hold un-acted-on.
//
// It is ten times the batch the core's own recovery sweep takes in one pass, so
// an edge stays open across several missed drains rather than refusing the
// moment one is late; and against the 64 KiB body cap this endpoint declares it
// holds the UN-ACTED-ON queue under 64 MiB per endpoint. Past it a sender is
// told 429 — a refusal it can act on — rather than being allowed to fill the
// table, because the alternative to a bounded queue is an anonymous party
// deciding how much storage this installation spends.
//
// IT IS NOT A BOUND ON WHAT THE TABLE HOLDS, and the difference matters to
// whoever sizes a disk. Only waiting rows are counted: a row that has landed or
// stalled leaves the queue and is kept as evidence until the retention sweep
// takes it. What the table holds is therefore bounded by the RETENTION WINDOW
// against the rate a sender is metered at, not by this number — see retention.go.
const maxPendingInbound = 1000

// signaturePrefix is what the published header carries in front of the hex
// digest. The expected value is built WITH it, so the comparison is over two
// strings of the same shape and a sender that sent a bare digest is refused by
// the comparison rather than by a parse this code would otherwise have to do.
const signaturePrefix = "sha256="

// absentEndpointKey is what a refusal with no real secret behind it verifies
// against.
//
// ITS VALUE IS IRRELEVANT AND IT IS NOT A SECRET: the comparison it feeds is
// discarded and the request is refused either way. What matters is that the
// comparison HAPPENS. Without it, a probe for a ref nobody minted would return
// before any MAC was computed and come back sooner than a wrong signature does.
// Deleting this as pointless is the failure mode it exists to prevent.
//
// WHAT IT DOES NOT EQUALIZE, stated plainly rather than left for a reader to
// assume: an absent ref takes this stand-in without a secret read, while a live
// one costs a database round trip and a decrypt — far more than the SHA-256
// this equalizes. So "that ref resolves to a live endpoint" remains
// distinguishable by clock, and the mitigation here is partial.
//
// It is left partial deliberately. A ref carries 128 bits from the system
// random source, so an oracle answering "does this one exist" is worth nothing
// without a candidate to ask about, and there is no way to produce one. Closing
// the rest would mean issuing a decoy secret read against a stand-in identity
// on every miss — a database round trip per unauthenticated probe, which hands
// an anonymous party a cheaper way to make this installation work than the one
// it would be closing.
const absentEndpointKey = "openchannel: no secret, and the answer is the same either way"

// inboundTarget is the endpoint an arriving request resolved to: the row to
// queue against, and the member whose name goes on what is queued.
type inboundTarget struct {
	id    string
	owner string
}

// receive is the unit's whole anonymous surface.
func receive(ctx context.Context, rt extension.Runtime, req extension.InboundRequest) (extension.InboundOutcome, error) {
	// THE REF, not the slug. The slug is declared and therefore the same for
	// every caller; the ref is the handle this unit minted for one member, and
	// resolving it is what turns one mounted edge into one endpoint per member.
	target, err := acceptingEndpoint(ctx, rt, req.Ref)
	if err != nil {
		return extension.InboundTransient, err
	}
	secret, err := admittingSecret(ctx, rt, target)
	if err != nil {
		return extension.InboundTransient, err
	}
	// The comparison runs for every request, including the ones already decided
	// against — see absentEndpointKey. Its result is combined with the target
	// afterwards rather than short-circuiting on it.
	presented := verified(secret, req)
	if target == nil || !presented {
		return extension.InboundUnauthenticated, nil
	}
	return enqueue(ctx, rt, *target, req)
}

// acceptingEndpoint resolves a ref to the endpoint that is currently taking
// requests on it, or to nothing.
//
// A paused endpoint answers nothing, exactly as an unminted ref does, and so
// does the EMPTY one a caller sends by addressing the bare mounted path: from
// outside, "this installation is not accepting here" is the whole of what a
// sender is entitled to learn, and a second answer would say that the endpoint
// exists. The empty ref takes no branch of its own for that reason — it matches
// no row, which is the same thing a wrong ref does.
//
// The ref is read out of the path, and it is used HERE and nowhere else: it is
// resolved against this unit's own table and never keyed on, so nothing
// downstream is addressed by a value a caller chose.
func acceptingEndpoint(ctx context.Context, rt extension.Runtime, ref string) (*inboundTarget, error) {
	var found *inboundTarget
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var (
			target  inboundTarget
			enabled bool
		)
		err := tx.QueryRow(ctx,
			`SELECT id::text, user_id::text, enabled FROM `+endpointTable+` WHERE ref = $1`, ref).
			Scan(&target.id, &target.owner, &enabled)
		if err != nil {
			if errors.Is(err, extension.ErrNoRows) {
				return nil
			}
			return err
		}
		if enabled {
			found = &target
		}
		return nil
	})
	return found, err
}

// admittingSecret reads the owner's sealed signing secret, or the stand-in that
// makes a refusal cost what an admission costs.
//
// GetUser takes an EXPLICIT user id rather than reading the caller, which is
// what lets this edge work at all: the invocation has nobody behind it, and the
// member whose secret admits the request is the one the endpoint row names.
func admittingSecret(ctx context.Context, rt extension.Runtime, target *inboundTarget) ([]byte, error) {
	if target == nil {
		return []byte(absentEndpointKey), nil
	}
	secret, err := rt.Secrets().GetUser(ctx, extension.UserID(target.owner), inboundSecretKey)
	if err != nil {
		if errors.Is(err, extension.ErrSecretNotFound) {
			// An endpoint that was opened and never minted admits nothing. It
			// takes the stand-in rather than returning here, so that it is
			// refused by the same comparison as a wrong signature.
			return []byte(absentEndpointKey), nil
		}
		return nil, err
	}
	return secret, nil
}

// verified reports whether the presented signature is the one this secret
// produces over the material the sender signed.
func verified(secret []byte, req extension.InboundRequest) bool {
	mac := hmac.New(sha256.New, secret)
	// SignedPayload, and nothing else. A verifier that re-spelled the
	// concatenation would one day spell it differently from the sender, and the
	// failure would look like a wrong secret rather than a rule that moved.
	mac.Write(req.SignedPayload())
	expected := signaturePrefix + hex.EncodeToString(mac.Sum(nil))
	// hmac.Equal, never ==: it does not return on the first differing byte.
	// Both operands are the fixed-length hex of a 32-byte digest under a fixed
	// prefix, so the comparison leaks neither content nor length.
	return hmac.Equal([]byte(expected), []byte(req.Signature))
}

// enqueue records one verified request, or says why it will not.
//
// The queue check and the insert are separate statements in one transaction, so
// a queue that fills between them admits one extra row. That is the honest
// shape of a soft cap: making it exact would mean locking the endpoint's whole
// queue on every arrival, which is a far larger cost than one row.
func enqueue(ctx context.Context, rt extension.Runtime, target inboundTarget, req extension.InboundRequest) (extension.InboundOutcome, error) {
	outcome := extension.InboundAccepted
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var pending int64
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM `+inboundTable+` WHERE endpoint_id = $1::uuid AND state = $2`,
			target.id, stateWaiting).Scan(&pending); err != nil {
			return err
		}
		if pending >= maxPendingInbound {
			// The core meters and logs this one, because a full queue is a fact
			// about the installation rather than about the sender's credentials
			// — which is exactly why it is not the opaque refusal.
			outcome = extension.InboundOverCapacity
			return nil
		}
		var landed string
		err := tx.QueryRow(ctx,
			`INSERT INTO `+inboundTable+` (endpoint_id, user_id, nonce, body, sent_at)
			 VALUES ($1::uuid, $2::uuid, $3, $4, $5)
			 ON CONFLICT (endpoint_id, nonce) DO NOTHING
			 RETURNING id::text`,
			// The owner comes from the ENDPOINT ROW and never from the payload.
			// A user id an anonymous sender supplies is a user id an anonymous
			// sender chooses, and it is the name everything downstream acts
			// under.
			target.id, target.owner, req.Nonce, req.Body, req.Timestamp).Scan(&landed)
		if err != nil {
			if errors.Is(err, extension.ErrNoRows) {
				// A nonce this endpoint has already taken. Nothing lands, and
				// the answer is the same refusal as a wrong signature: a sender
				// told "already seen" has been told the previous one was
				// accepted, which is a captured request's whole value to
				// whoever captured it.
				outcome = extension.InboundUnauthenticated
				return nil
			}
			return err
		}
		// The traffic counters, in the same transaction as the row they count,
		// so a screen never shows an arrival the queue does not hold. It is
		// deliberately not a ledger row and not a version bump: an arrival is
		// not a decision anybody made, and recording one per anonymous request
		// would bury the decisions in the trail that exists to hold them.
		_, err = tx.Exec(ctx,
			`UPDATE `+endpointTable+` SET inbound_received = inbound_received + 1,
			        last_inbound_at = now(), updated_at = now()
			 WHERE id = $1::uuid`, target.id)
		return err
	})
	if err != nil {
		return extension.InboundTransient, err
	}
	return outcome, nil
}
