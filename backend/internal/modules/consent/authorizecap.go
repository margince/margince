// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The jurisdiction frequency ceiling: how many advertising messages one address
// may receive in a rolling window, and why the count is of messages that were
// actually delivered.
//
// A cap is a fact about VOLUME rather than about a person. Nothing the
// recipient did refuses the message, and the same message becomes lawful again
// once the window rolls — which is why a cap refusal is not one of the absolute
// denials and why its reason code says so.

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// capLockNamespace keys this module's advisory locks apart from every other
// user of pg_advisory_xact_lock in the installation. It occupies the high half
// of the single-argument bigint key, so a collision with another subsystem's
// lock on the same number is impossible rather than unlikely.
const capLockNamespace = int64(0x636d7361) << 32 // "cmsa"

// applyFrequencyCap refuses an advertising message that would exceed the
// applicable jurisdiction's ceiling for this address.
//
// It runs only for a decision the engine has ALREADY decided to allow, and only
// for advertising. A cap cannot rescue a refused message and does not bind an
// operational one, so asking about either would be counting for nothing.
//
// The lock comes before the count, and that ordering is the whole guarantee.
// Two dispatches for the same address that both read "two below the cap" would
// both send a third message, and the ceiling would be exceeded by exactly the
// number of concurrent workers. Holding a transaction-scoped advisory lock on
// the address makes the read-decide-send sequence serial per address, and the
// lock is released by the commit that records the decision — so a crash cannot
// strand it.
// lockCapAddresses takes every recipient's cap lock before any of them is
// counted, when a ceiling is in force. It is separate from applyFrequencyCap
// because the ORDER matters across the whole message: the locks must all be
// held, sorted, before the first count, and a per-recipient lock taken inside
// the decide loop would order them by the caller's To list.
func (g *Gate) lockCapAddresses(ctx context.Context, tx pgx.Tx, recipients []connector.Recipient) error {
	rules, applicable, err := g.store.applicableRules(ctx, tx)
	if err != nil {
		return err
	}
	if !applicable || rules.FrequencyCap == nil {
		return nil
	}
	addresses := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if a := strings.TrimSpace(r.Email); a != "" {
			addresses = append(addresses, a)
		}
	}
	if len(addresses) == 0 {
		return nil
	}
	return lockAddressesForCap(ctx, tx, addresses)
}

func (g *Gate) applyFrequencyCap(ctx context.Context, tx pgx.Tx, d commsauthz.Decision, now time.Time) (commsauthz.Decision, error) {
	if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryMarketing {
		return d, nil
	}
	address := strings.TrimSpace(d.Recipient.Email)
	if address == "" {
		// A channel recipient has no address for a per-address ceiling to bind
		// to. The cap the packs declare is on advertising EMAIL, so a channel
		// message is out of its scope rather than exempt from an applicable
		// rule — stated here because silence would read as an oversight.
		return d, nil
	}
	rules, applicable, err := g.store.applicableRules(ctx, tx)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !applicable || rules.FrequencyCap == nil {
		return d, nil
	}
	received, err := advertisingMessagesReceived(ctx, tx, address, now.Add(-rules.FrequencyCap.Window))
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if received < rules.FrequencyCap.Messages {
		return d, nil
	}
	d.Verdict = commsauthz.VerdictDeny
	d.ReasonCode = commsauthz.ReasonFrequencyCapReached
	return d, nil
}

// lockAddressesForCap serializes the count-and-decide for every address this
// message will be judged against, ALL OF THEM UP FRONT and in sorted order.
//
// One lock per recipient taken as the decide loop reaches it would take them in
// the order the caller supplied the recipients. Two messages naming the same
// two addresses in opposite orders would then take the same pair of locks in
// opposite orders, Postgres would resolve it by killing one transaction, and a
// legitimate send would burn its retry ladder — a deadlock a caller can provoke
// by choosing a To order. Sorting and deduplicating first makes that
// impossible to express, which is the discipline storekit.LockSubjectKeys
// already applies to subject locks for the same reason.
//
// The address is hashed because the lock key is an integer. A hash collision
// costs two unrelated addresses a moment of serialization and never costs
// correctness — the opposite trade from a lock that could be MISSED, which
// would let the ceiling be exceeded.
//
// Transaction-scoped (pg_advisory_xact_lock, not the session form): the locks
// end with the transaction that records the decisions, so nothing has to
// remember to release them and a failed transaction cannot hold an address
// shut.
func lockAddressesForCap(ctx context.Context, tx pgx.Tx, addresses []string) error {
	for _, key := range capLockKeys(addresses) {
		// The single-argument bigint form, so the namespace and the hash
		// occupy separate halves of one key with no conversion that could
		// narrow either.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
			return fmt.Errorf("consent: hold the frequency-cap lock for this address: %w", err)
		}
	}
	return nil
}

// capLockKeys is the order the locks are taken in: sorted and deduplicated, so
// two messages naming the same addresses take the same locks in the same order
// whatever order their recipients arrived in. This is the ordering itself
// rather than a step inside the locker, so a test can assert the property
// without running two transactions and hoping they cross.
func capLockKeys(addresses []string) []int64 {
	keys := make([]int64, 0, len(addresses))
	for _, address := range addresses {
		keys = append(keys, capLockKey(address))
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// capLockKey is one address's lock key: the namespace in the high half, the
// address hash in the low half. Lower-cased and trimmed so two spellings of one
// mailbox take the same lock, matching how the count compares addresses.
func capLockKey(address string) int64 {
	h := fnv.New32a()
	// hash.Hash documents Write as never returning an error; it carries one
	// only to satisfy io.Writer. Discarded explicitly rather than silently, so
	// a reader can see the claim and check it.
	//craft:ignore T2 hash.Hash.Write is documented never to fail
	_, _ = h.Write([]byte(normalizeCapAddress(address)))
	return capLockNamespace | int64(h.Sum32())
}

// normalizeCapAddress answers "which mailbox is this" for both the lock key and
// the count's comparison. Two answers would mean an address could be locked
// under one form and counted under another, and the ceiling would bind to
// neither.
//
// Held by: TestTheLockAndTheCountNormalizeAlike (authorizecaplock_test.go)
func normalizeCapAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// advertisingMessagesReceived counts what the recipient has GOT or is ABOUT TO
// GET, and both halves are load-bearing.
//
// The delivered half is the obvious one, and the join under it is the rule. A
// count over communication_decision alone would include a decision taken in
// observe mode (which records what the engine would have said while the old
// gate ruled) and a delivery that was staged and then parked — both of which
// describe a message nobody received. Counting either would consume somebody's
// statutory allowance for mail that never arrived, and the person would be
// silenced for a day by an accounting error. So a delivered message is a
// comms_outbound row that reached 'sent' joined to its own transmit decision:
// the delivery says it went, the decision says what it was.
//
// The IN-FLIGHT half is what makes the ceiling actually bind. An authorization
// commits before the provider is called — it must, because provider I/O cannot
// sit inside a transaction — and 'sent' is written afterwards by a different
// transaction. So between deciding and sending there is a window in which a
// message is going out and no 'sent' row says so. Counting only delivered mail
// would let every worker in that window read the same number and each send,
// and the ceiling would be exceeded by however many workers happened to be
// running. An allowing transmit decision against a delivery that is still
// pending IS that message: it was written under the address lock, it exists
// exactly once per attempt, and it resolves within the ladder to either a
// 'sent' row (which the first half then counts, and DISTINCT keeps from
// counting twice) or a park (which stops matching, returning the allowance).
//
// Art. 17 erasure rewrites recipient_address to a placeholder, so a subject's
// advertising history stops matching this count and a re-captured address
// starts from zero. That is the right answer — the count is evidence about a
// person, and erasure is meant to destroy it — but it is worth saying, because
// nothing else in this file would tell a reader that another engine can empty
// the record the ceiling rests on.
func advertisingMessagesReceived(ctx context.Context, tx pgx.Tx, address string, since time.Time) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT o.id)
		  FROM comms_outbound o
		  JOIN communication_decision d ON d.delivery_id = o.id
		 WHERE d.phase = 'transmit'
		   AND d.verdict = 'allow'
		   AND d.resolved_category = $3
		   -- Trimmed and lowered on BOTH sides, matching normalizeCapAddress.
		   -- The stored key is the address as the caller supplied it (the
		   -- dispatcher already normalizes, other callers of this public gate
		   -- need not), so comparing raw would let a leading space or a
		   -- capital reach a capped mailbox again.
		   AND lower(btrim(d.recipient_address)) = $1
		   AND (
		         (o.status = 'sent' AND o.sent_at IS NOT NULL AND o.sent_at >= $2)
		      OR (o.status = 'pending' AND d.decided_at >= $2)
		       )`,
		normalizeCapAddress(address), since, string(commsauthz.CategoryMarketing)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("consent: count the advertising this address received: %w", err)
	}
	return count, nil
}

// applicableRules resolves which jurisdiction's messaging rules bind this
// installation's outbound mail.
//
// Today that is the country the installation declares. The recipient's own
// country is the other half and is not read yet: the product records no
// reliable recipient country, and guessing one from an email domain would apply
// a foreign ceiling on a guess. When that evidence exists the resolution folds
// both codes through messagingrules.Strictest, which is why this returns the
// FOLD of a code list rather than a single lookup.
//
// An unstated or unknown country resolves to no rules, and no rules means no
// ceiling — not because an unknown country is permissive, but because a cap is
// a jurisdiction's own number and there is no universal one to fall back to.
// The consent requirement that DOES bind everywhere is enforced by the marketing
// verdict itself, which runs before this and does not depend on a pack.
func (s *Store) applicableRules(ctx context.Context, tx pgx.Tx) (messagingrules.Rules, bool, error) {
	code, err := s.installationCountry(ctx, tx)
	if err != nil {
		return messagingrules.Rules{}, false, err
	}
	if code == "" {
		return messagingrules.Rules{}, false, nil
	}
	rules, _, found := messagingrules.Strictest(code)
	return rules, found, nil
}
