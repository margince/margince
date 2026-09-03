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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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
	rules, applicable, err := g.applicableRules(ctx, tx)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !applicable || rules.FrequencyCap == nil {
		return d, nil
	}
	if err := lockAddressForCap(ctx, tx, address); err != nil {
		return commsauthz.Decision{}, err
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

// lockAddressForCap serializes the count-and-decide for one address.
//
// The address is hashed rather than used directly because the lock key is an
// integer. A hash collision costs two unrelated addresses a moment of
// serialization and never costs correctness — the opposite trade from a lock
// that could be MISSED, which would let the cap be exceeded.
//
// Transaction-scoped (pg_advisory_xact_lock, not the session form): the lock
// ends with the transaction that records the decision, so nothing has to
// remember to release it and a failed transaction cannot hold an address shut.
func lockAddressForCap(ctx context.Context, tx pgx.Tx, address string) error {
	h := fnv.New32a()
	// Hash writes never fail; the interface returns an error for io.Writer
	// compatibility. Lower-cased so two spellings of one mailbox take the same
	// lock, matching how the count below compares addresses.
	if _, err := h.Write([]byte(strings.ToLower(address))); err != nil {
		return fmt.Errorf("consent: key the frequency-cap lock: %w", err)
	}
	// The single-argument bigint form, so the namespace and the hash occupy
	// separate halves of one key with no conversion that could narrow either.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`,
		capLockNamespace|int64(h.Sum32())); err != nil {
		return fmt.Errorf("consent: hold the frequency-cap lock for this address: %w", err)
	}
	return nil
}

// advertisingMessagesReceived counts what the recipient ACTUALLY GOT.
//
// The join is the invariant. A count over communication_decision alone would
// include a decision taken in observe mode (which records what the engine would
// have said while the old gate ruled) and a delivery that was staged and then
// parked — both of which describe a message nobody received. Counting either
// would consume somebody's statutory allowance for mail that never arrived, and
// the person would be silenced for a day by an accounting error.
//
// So the count starts from comms_outbound rows that reached 'sent' — the status
// every reader in this tree takes to mean the provider accepted the message —
// and joins each to its own transmit decision to establish that it was
// ADVERTISING. Neither half is sufficient alone: the delivery says it went, the
// decision says what it was.
func advertisingMessagesReceived(ctx context.Context, tx pgx.Tx, address string, since time.Time) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT o.id)
		  FROM comms_outbound o
		  JOIN communication_decision d ON d.delivery_id = o.id
		 WHERE o.status = 'sent'
		   AND o.sent_at IS NOT NULL
		   AND o.sent_at >= $2
		   AND d.phase = 'transmit'
		   AND d.verdict = 'allow'
		   AND d.resolved_category = $3
		   AND lower(d.recipient_address) = lower($1)`,
		address, since, string(commsauthz.CategoryMarketing)).Scan(&count)
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
func (g *Gate) applicableRules(ctx context.Context, tx pgx.Tx) (messagingrules.Rules, bool, error) {
	code, err := g.installationCountry(ctx, tx)
	if err != nil {
		return messagingrules.Rules{}, false, err
	}
	if code == "" {
		return messagingrules.Rules{}, false, nil
	}
	rules, _, found := messagingrules.Strictest(code)
	return rules, found, nil
}
