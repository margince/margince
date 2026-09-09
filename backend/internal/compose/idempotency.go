// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Transport-level idempotency (crm.yaml `IdempotencyKey`): a mutating
// request carrying an Idempotency-Key is safe to retry — the first
// attempt claims the key inside the caller's workspace scope, a replay
// within 24h returns the recorded response verbatim, and the same key
// with a DIFFERENT body is refused (409 idempotency_key_conflict, never
// a silent replay of mismatched intent). The claim row is written
// insert-first, so two concurrent attempts under one key can never both
// execute: the loser sees the claim and answers 409 while the first is
// in flight.
//
// A 2xx outcome is recorded for replay. A refusal is judged by whether it
// CHANGED anything, which is not one answer:
//
//   - A refusal that wrote nothing releases the claim, so the client may retry
//     the same key. Replaying stored failures would pin transient faults for
//     24h and would break the stage-then-redeem approval flow, whose retry is
//     the same request.
//   - A refusal that had already written is recorded instead. Releasing it
//     hands back a key whose retry re-runs the committed half — the one thing
//     the key exists to prevent — so the next attempt meets the claimFailed
//     answer: this request failed after it had already started, check whether
//     it took effect before retrying under a new key. Handlers report a write
//     through markWriteCommitted (partialwrite.go); the per-field split's
//     staging half is the only door that does today.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const idempotencyKeyHeader = "Idempotency-Key"

// replayWindow is how long a settled claim stays replayable — the contract's
// 24h. Spelled once: claimKey re-claims a row past it, and the retention sweep
// (idempotencyretention.go) deletes one past it, and those two must be talking
// about the same moment or the sweep would delete rows a replay could still
// legitimately serve.
const replayWindow = 24 * time.Hour

// claimOutcome is what the claim transaction decided.
type claimOutcome int

const (
	claimFresh      claimOutcome = iota // this request executes
	claimReplay                         // recorded response is returned
	claimInProgress                     // first attempt has not finished
	claimMismatch                       // same key, different request digest
	claimFailed                         // an earlier attempt RAN and recorded no result
)

// The claim itself — claimKey, settleClaim, releaseClaim — takes a CONTEXT and
// the four values that identify a claim, never an *http.Request: the governed
// tool surface claims keys through the same three functions for a tools/call
// (agentidempotency.go), and two transports each holding their own claim
// transaction would be two answers to the question "is this the same call".
//
// idempotency is a contract-router middleware; it rides inside the
// session middleware, so workspace and principal are bound (the claim
// queries below key on principal_id themselves — the table carries no
// workspace column and needs none).
func idempotency(pool *pgxpool.Pool, probes map[string]replayProbe) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(idempotencyKeyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			route := r.Method + " " + chi.RouteContext(r.Context()).RoutePattern()
			if _, replayable := replayableOperations[route]; !replayable {
				next.ServeHTTP(w, r)
				return
			}
			if len(key) > 255 {
				httperr.Write(w, r, httperr.Validation(idempotencyKeyHeader, "too_long", "Idempotency-Key exceeds 255 characters"))
				return
			}
			actor, ok := principal.Actor(r.Context())
			if !ok {
				next.ServeHTTP(w, r) // unauthenticated requests fail auth downstream
				return
			}

			// Bound the buffer at the site (the chassis LimitBodies cap also
			// applies, but the invariant should be visible here, as it is in
			// the agent gate's maxGatedBody read).
			body, err := io.ReadAll(io.LimitReader(r.Body, maxGatedBody+1))
			if err != nil || len(body) > maxGatedBody {
				httperr.Write(w, r, httperr.Validation("body", "unreadable", "request body unreadable or too large"))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			sum := sha256.Sum256(body)
			digest := hex.EncodeToString(sum[:])
			// The concrete path, not the pattern: the contract scopes the
			// key per request-path, so /deals/A and /deals/B never collide.
			endpoint := r.Method + " " + r.URL.Path

			outcome, hold, stored, err := claimKey(r.Context(), pool, actor.ID, key, endpoint, digest)
			if err != nil {
				// Degraded, not refused: idempotency is a retry-safety layer,
				// and refusing the request because the layer itself hiccupped
				// would make retries LESS safe than not sending the header at
				// all. The tool surface REFUSES instead, and the two are
				// reasoned about together in claimKey's own comment.
				slog.ErrorContext(r.Context(), "idempotency claim failed; executing without replay protection", "err", err)
				// And no hold: executing without replay protection means there
				// is nothing to settle, so the settle below writes nothing
				// rather than writing under three columns it does not own.
				outcome, hold, stored = claimFresh, claimHold{}, storedResponse{}
			}
			if outcome != claimFresh {
				writeClaimOutcome(w, r, pool, probes, route, outcome, stored)
				return
			}

			rec := &replayRecorder{ResponseWriter: w, status: http.StatusOK}
			// The handler is given somewhere to say that it wrote before it
			// refused. Read after it returns, because a refusal that changed
			// something must not give the key back — see settleClaim.
			ctx, effect := withWriteEffect(r.Context())
			next.ServeHTTP(rec, r.WithContext(ctx))
			// Zero records: this door charges no read bound, so its claims carry
			// no cost to record (0198).
			if err := settleClaim(r.Context(), pool, hold,
				rec.status, rec.buf.String(), rec.Header().Get("Content-Type"), 0, effect.committed); err != nil {
				slog.ErrorContext(r.Context(), "idempotency claim settlement failed", "err", err)
			}
		})
	}
}

// writeClaimOutcome answers a claim that did not win the race: a replay
// (gated), a first attempt still in flight, or the same key reused for a
// different body.
func writeClaimOutcome(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, probes map[string]replayProbe, route string, outcome claimOutcome, stored storedResponse) {
	switch outcome {
	case claimReplay:
		// A replay is a read (API-CC-8): the recorded body only goes back if
		// the caller can still see the record it carries.
		if err := ensureReplayVisible(r.Context(), pool, probes, route, stored.body); err != nil {
			httperr.Write(w, r, err)
			return
		}
		// The replay repeats the ORIGINAL response verbatim — status, body,
		// and the media type recorded with it (0069), never a restamped
		// Content-Type.
		if stored.contentType != "" {
			w.Header().Set("Content-Type", stored.contentType)
		}
		w.WriteHeader(stored.status)
		if stored.body != "" {
			_, _ = io.WriteString(w, stored.body)
		}
	case claimInProgress:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   "idempotency_key_conflict",
			Detail: "a request with this idempotency key is still in progress",
		})
	case claimMismatch:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   "idempotency_key_conflict",
			Detail: "this idempotency key was already used with a different request body",
		})
	case claimFresh:
		// Unreachable: the caller returns early only for a non-fresh claim.
	case claimFailed:
		// Reached by two writers now: the tool surface records a run that
		// produced no result (agentidempotency.go), and this middleware records
		// a refusal whose handler had already committed (settleClaim). It
		// ANSWERS rather than falling through: an empty case here returns
		// without writing, and net/http then sends a bare 200 with no body — a
		// silent success for a call that failed.
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   "idempotency_key_conflict",
			Detail: "an earlier request with this idempotency key failed after it had already started; " +
				"check whether it took effect before retrying under a new key",
		})
	}
}

type storedResponse struct {
	status      int
	body        string
	contentType string
	// records is what the recorded answer cost the caller's read bound, for the
	// surface that charges one. Zero for a REST claim, which charges none.
	records int
}

// claimKey runs the insert-first claim and REPORTS a claim-infrastructure
// failure to its caller rather than deciding what it means.
//
// The two doors answer that question differently on purpose, so neither answer
// belongs here. This middleware degrades to executing — a client may not even
// have meant the header it sent, and refusing would leave its retries less safe
// than sending no header at all. The tool surface refuses, because there the
// argument IS the caller asking for at-most-once about an irreversible act
// (agents.Registry.claimFor). A caller reading claimFresh from this function
// must therefore check err first: fresh with an error means no claim was
// acquired.
func claimKey(ctx context.Context, pool *pgxpool.Pool, principalID, key, endpoint, digest string) (claimOutcome, claimHold, storedResponse, error) {
	outcome := claimFresh
	var hold claimHold
	var stored storedResponse
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		attempt, claimed, err := insertClaim(ctx, tx, principalID, key, endpoint, digest)
		if err != nil {
			return err
		}
		if claimed {
			hold = claimHold{principalID: principalID, key: key, endpoint: endpoint, attempt: attempt}
			return nil // fresh claim
		}
		var reclaimed ids.UUID
		outcome, reclaimed, stored, err = resolveExistingClaim(ctx, tx, principalID, key, endpoint, digest)
		hold = claimHold{principalID: principalID, key: key, endpoint: endpoint, attempt: reclaimed}
		return err
	})
	if err != nil {
		return claimFresh, claimHold{}, storedResponse{}, err
	}
	return outcome, hold, stored, nil
}

// claimHold names the ATTEMPT that took a claim, and is what every settlement
// is predicated on.
//
// The row is identified by (principal, key, endpoint), and that was the whole
// identity a settle or a release matched. But the row is RE-CLAIMED in place
// once past the replay window — new digest, cleared response, same three
// columns — so an attempt still in flight when its claim expired could settle
// the replacement's row: overwriting a result a later replay serves for the
// wrong call, or deleting a live claim and letting a third attempt run beside
// the second.
//
// A zero attempt means this caller holds nothing to settle: it read somebody
// else's claim rather than taking one. Every settle and release refuses such a
// hold outright rather than falling back to the three columns, because falling
// back is the behaviour being removed.
type claimHold struct {
	principalID string
	key         string
	endpoint    string
	attempt     ids.UUID
}

// holdsClaim reports whether this hold names an attempt of its own.
func (h claimHold) holdsClaim() bool { return h.attempt != ids.Nil }

// insertClaim writes the claim row insert-first and reports whether THIS
// attempt is the one that claimed the key: false means a row for the same
// (workspace, principal, key, endpoint) already exists.
func insertClaim(ctx context.Context, tx pgx.Tx, principalID, key, endpoint, digest string) (ids.UUID, bool, error) {
	// RETURNING, so the attempt id comes from the row rather than from a value
	// this function chose and hoped matched. A settlement is predicated on it,
	// and a predicate compared against something the writer only believes was
	// stored is not a predicate.
	var attempt ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO idempotency_key (principal_id, key, endpoint, request_digest)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (principal_id, key, endpoint) DO NOTHING
		RETURNING attempt_id`,
		principalID, key, endpoint, digest).Scan(&attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		// DO NOTHING fired: a row for this key already exists and this attempt
		// did not claim it.
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, err
	}
	return attempt, true, nil
}

// resolveExistingClaim decides what an already-claimed key means for this
// attempt: a replay of the recorded response, a first attempt still in flight,
// the same key reused for a different body, or — past the replay window — a
// re-claim in place. The row is read FOR UPDATE inside the caller's
// transaction, so two concurrent attempts under one key cannot both execute.
func resolveExistingClaim(ctx context.Context, tx pgx.Tx, principalID, key, endpoint, digest string) (claimOutcome, ids.UUID, storedResponse, error) {
	return resolveClaimRow(ctx, tx, principalID, key, endpoint, digest, true)
}

// resolveClaimRow is resolveExistingClaim's body. mayReclaim is false on the
// second pass — the one that reads back what a rival left after this attempt
// lost the re-claim — so a row that vanishes twice inside one transaction
// cannot loop.
func resolveClaimRow(ctx context.Context, tx pgx.Tx, principalID, key, endpoint, digest string, mayReclaim bool) (claimOutcome, ids.UUID, storedResponse, error) {
	var storedDigest, contentType string
	var status *int
	var respBody *string
	var records int
	var expired bool
	err := tx.QueryRow(ctx, `
		SELECT request_digest, response_status, response_body, response_content_type, response_records,
		       created_at < now() - make_interval(secs => $4)
		FROM idempotency_key
		WHERE principal_id = $1 AND key = $2 AND endpoint = $3
		FOR UPDATE`,
		principalID, key, endpoint, replayWindow.Seconds()).Scan(&storedDigest, &status, &respBody, &contentType, &records, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		// The retention sweep removed the row between the INSERT above and
		// this read. It was past the replay window, so it was protecting
		// nothing — but simply erroring here would degrade to executing
		// with NO claim recorded, and two concurrent retries of one key
		// would then both execute. Claim it again instead.
		if !mayReclaim {
			// The row went twice inside one transaction. Nothing is left to
			// read, and in-flight is the answer that costs least if wrong:
			// it tells the caller to retry rather than inventing a result.
			return claimInProgress, ids.Nil, storedResponse{}, nil
		}
		attempt, claimed, err := insertClaim(ctx, tx, principalID, key, endpoint, digest)
		if err != nil {
			return claimFresh, ids.Nil, storedResponse{}, err
		}
		if claimed {
			return claimFresh, attempt, storedResponse{}, nil
		}
		// A rival re-created it in the same instant. What they left is not
		// necessarily in flight — it may already carry a stored response, or a
		// different request digest — and the four verdicts below are what
		// decide which. Reading it is the difference between answering
		// "already used with a different request body" and telling that caller
		// their own retry is still running.
		//
		// The read blocks rather than racing: the losing INSERT waited on the
		// rival's uncommitted row, so by the time it answered the winner had
		// committed and FOR UPDATE sees it.
		return resolveClaimRow(ctx, tx, principalID, key, endpoint, digest, false)
	}
	if err != nil {
		return claimFresh, ids.Nil, storedResponse{}, err
	}
	if expired {
		// Past the retention window the key means nothing anymore: re-claim it
		// in place for this attempt, under a NEW attempt id. That id is what
		// stops the previous holder — still in flight, or it would not be
		// returning — from settling the row it no longer owns.
		var reclaimed ids.UUID
		err := tx.QueryRow(ctx, `
			UPDATE idempotency_key
			SET request_digest = $4, response_status = NULL, response_body = NULL,
			    response_content_type = DEFAULT, response_records = DEFAULT,
			    created_at = now(), attempt_id = uuidv7()
			WHERE principal_id = $1 AND key = $2 AND endpoint = $3
			RETURNING attempt_id`,
			principalID, key, endpoint, digest).Scan(&reclaimed)
		return claimFresh, reclaimed, storedResponse{}, err
	}
	stored := storedResponse{contentType: contentType, records: records}
	if respBody != nil {
		stored.body = *respBody
	}
	switch {
	case storedDigest != digest:
		return claimMismatch, ids.Nil, storedResponse{}, nil
	case status == nil:
		return claimInProgress, ids.Nil, storedResponse{}, nil
	case *status < 200 || *status >= 300:
		// A RECORDED failure — the attempt reached its handler and answered a
		// refusal it could not take back. The tool surface writes one for a run
		// that produced no result; REST writes one when a handler reported that
		// it had already committed before refusing (settleClaim). It is not a
		// replay and not a free key: whether the effect landed is exactly what
		// the recording could not determine, so the body carries the reason and
		// the caller decides.
		stored.status = *status
		return claimFailed, ids.Nil, stored, nil
	default:
		stored.status = *status
		return claimReplay, ids.Nil, stored, nil
	}
}

// settleClaim records a 2xx outcome for replay, and decides what a refusal
// means from whether the request WROTE before it answered one.
//
// A refusal that changed nothing gives the key back: the caller fixes the call
// and retries, which is the whole point of releasing it. A refusal that changed
// something must not — the retry would run the applied half a second time under
// the same key, which is precisely what the key exists to prevent. Such a claim
// is RECORDED instead, and the next attempt under it meets the claimFailed
// answer: this request failed after it had already started, check whether it
// took effect before retrying under a new key. That is the honest sentence,
// because whether the effect landed is exactly what nobody here can determine.
//
// Only one door reports a write today (the per-field split's staging half), so
// the released path is still what every other refusal takes.
func settleClaim(ctx context.Context, pool *pgxpool.Pool, hold claimHold,
	status int, body, contentType string, records int, wrote bool,
) error {
	// A caller that took no claim settles nothing. It reached here after
	// reading somebody else's row — a replay, a mismatch, or an infrastructure
	// failure the door degraded past — and writing under those three columns
	// would be settling an attempt that is not this one.
	if !hold.holdsClaim() {
		return nil
	}
	if claimIsReleased(status, wrote) {
		return releaseClaim(ctx, pool, hold)
	}
	return recordClaimOutcome(ctx, pool, hold, status, body, contentType, records)
}

// claimIsReleased is the rule a refusal is judged by: a key goes back only for
// one that changed nothing.
//
// A function rather than an inline condition because it is the whole of what
// settleClaim decides, and a case can then state it without a database — the
// two arms it chooses between are each already held.
func claimIsReleased(status int, wrote bool) bool {
	return (status < 200 || status >= 300) && !wrote
}

// recordClaimOutcome writes a settled response onto a held claim, whatever that
// response says. settleClaim reaches it for a success; failClaim reaches it for
// a run that produced none.
func recordClaimOutcome(ctx context.Context, pool *pgxpool.Pool, hold claimHold,
	status int, body, contentType string, records int,
) error {
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		// attempt_id, so a settlement lands only on the attempt that made it.
		// Without it a holder whose claim expired mid-flight would overwrite
		// the REPLACEMENT's recorded result, and a later replay would serve the
		// wrong call's document under a key that looks settled.
		_, err := tx.Exec(ctx, `
			UPDATE idempotency_key
			SET response_status = $5, response_body = $6, response_content_type = $7, response_records = $8
			WHERE principal_id = $1 AND key = $2 AND endpoint = $3 AND attempt_id = $4`,
			hold.principalID, hold.key, hold.endpoint, hold.attempt, status, body, contentType, records)
		return err
	})
}

// releaseClaim gives an unsettled key back, so the caller may retry it. Only a
// claim with no recorded response is released: a settled one is a result
// somebody may still replay, and deleting it here would turn a completed call
// into one that executes a second time.
func releaseClaim(ctx context.Context, pool *pgxpool.Pool, hold claimHold) error {
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		// attempt_id here for the harder half of the same reason: a stale
		// release DELETES a live claim, and the key is then free while the
		// attempt holding it is still running — so a third attempt executes
		// beside the second, which is the one thing the key exists to prevent.
		_, err := tx.Exec(ctx, `
			DELETE FROM idempotency_key
			WHERE principal_id = $1 AND key = $2 AND endpoint = $3 AND attempt_id = $4
			  AND response_status IS NULL`,
			hold.principalID, hold.key, hold.endpoint, hold.attempt)
		return err
	})
}

// replayRecorder tees the response so a later replay can repeat it.
type replayRecorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *replayRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *replayRecorder) Write(p []byte) (int, error) {
	r.buf.Write(p)
	return r.ResponseWriter.Write(p)
}
