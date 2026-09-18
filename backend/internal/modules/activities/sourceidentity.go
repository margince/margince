// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One message is one activity, whichever provider hands it over.
//
// An activity's own (source_system, source_id) is where the FIRST arrival filed
// it, and the two doors file differently: captured mail keys on 'email' plus
// the RFC Message-ID, an import keys on its own namespace plus its own record
// id. Those keys never meet, so the same email arriving twice became two rows.
//
// activity_identity is the identity both doors agree on. Its primary key is the
// arbiter: one external identity resolves to exactly one activity, and a second
// arrival claiming it collides there rather than quietly creating a second row.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The identity kinds activity_identity carries, matching its CHECK.
const (
	IdentityKindMail    = "mail"
	IdentityKindMeeting = "meeting"
)

// MeetingIdentityKey is the identity of ONE occurrence of a calendar event.
//
// A recurring series shares a single iCal UID across every occurrence — a
// weekly call is one UID and fifty-two meetings — so the UID names the series
// and the occurrence's own start names the meeting within it. Keying on the UID
// alone would resolve every occurrence to the first one.
//
// BOTH ingestion doors compose their key here, which is what stops them
// disagreeing: an importer states `ical_instance` as text, a connector reads the
// occurrence from the provider as an instant, and normalizedInstant folds either
// into one spelling. A second composition would fail silently — the resolve
// would simply never match, and both doors would go on landing a row each.
func MeetingIdentityKey(icalUID, instance string) string {
	return strings.TrimSpace(icalUID) + "/" + normalizedInstant(strings.TrimSpace(instance))
}

// normalizedInstant folds an occurrence start to one spelling, so the same
// instant written by two callers compares equal.
//
// The contract asks for "the occurrence's own original start, as the calendar
// states it", which leaves the offset and the precision to whoever writes it:
// Google says `2026-09-23T10:00:00+02:00`, Graph says
// `2026-09-23T08:00:00.0000000Z`, and a client may say either. One instant, so
// one key.
//
// A value that will not parse is kept VERBATIM rather than refused. This key is
// an opaque identity, not a validated field, and failing here would refuse an
// import over a format question the identity does not care about — while two
// callers who spell an unparseable instance the same way still meet.
func normalizedInstant(instance string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, instance); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return instance
}

// ResolveIdentity answers which activity already holds an external identity.
//
// Reports false when the identity is free. A caller that gets an id has found
// the message it was about to store, under whatever name the other door filed
// it: that is the whole mechanism.
func ResolveIdentity(ctx context.Context, tx pgx.Tx, kind, key string) (ids.ActivityID, bool, error) {
	if key == "" {
		return ids.ActivityID{}, false, nil
	}
	var id ids.ActivityID
	err := tx.QueryRow(ctx,
		`SELECT activity_id FROM activity_identity WHERE identity_kind = $1 AND identity_key = $2`,
		kind, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("activities: resolving a message identity: %w", err)
	}
	return id, true, nil
}

// ClaimIdentity binds an external identity to an activity, and reports whether
// this activity ended up holding it.
//
// A claim that LOSES is not an error. Somebody else's row holds the identity —
// a racing arrival that got there first, or a message this caller may not see —
// and either way their own row still stands: it keeps its own
// (source_system, source_id), its links and its content, and only the shared
// identity belongs to somebody else.
//
// Failing the write instead would answer a question no caller may ask. A
// Message-ID is typed by whoever sent the message, so refusing a guessed one
// tells the guesser that somebody here already holds it — the existence of
// another colleague's mail, disclosed by a status code. Reporting false is the
// mitigation: a lost claim is indistinguishable from a message that had no
// identity to claim.
//
// Already claimed by THIS activity reports true, because a replay re-states
// what it stated before.
func ClaimIdentity(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind, key, attestedBy string) (bool, error) {
	if key == "" {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (identity_kind, identity_key) DO NOTHING`,
		kind, key, activityID, attestedBy)
	if err != nil {
		return false, fmt.Errorf("activities: claiming a message identity: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}
	holder, found, err := ResolveIdentity(ctx, tx, kind, key)
	if err != nil {
		return false, err
	}
	return found && holder == activityID, nil
}

// TransferIdentities moves every identity from one activity to another.
//
// Merging two rows leaves the loser archived with its own key released. Its
// identities have to travel, or they stay pointing at a row nobody can reach —
// a live claim on a dead record, which would send the next arrival of that
// message to the wrong place.
//
// A conflict cannot arise: the survivor and the loser cannot both hold one
// identity, because the primary key is what stopped that.
func TransferIdentities(ctx context.Context, tx pgx.Tx, from, to ids.ActivityID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE activity_identity SET activity_id = $2 WHERE activity_id = $1`,
		from, to); err != nil {
		return fmt.Errorf("activities: carrying a merged message's identities: %w", err)
	}
	return nil
}

// BindableTo reports whether an arrival may join the activity that already
// holds its identity, and the two cases are not the same question.
//
// A Message-ID is typed by whoever sent the message. Binding two rows together
// means each one's content becomes reachable through the other, so a forged
// identity is a way to reach somebody else's mail.
//
//   - The SAME seat wrote both rows → bind. Nobody gains access they did not
//     already have: one colleague is joining their own record to their own record,
//     which is exactly the import-then-capture case this exists for.
//   - DIFFERENT principals → refuse. The arrival goes on to create its own row.
//     A duplicate is a visible, fixable annoyance; a cross-principal bind on a
//     forged header is not.
//
// Unparseable, absent or non-human on either side is NOT a match. That is the
// one direction this must not fail in: treating "I cannot tell" as "the same
// seat" would open the case the rule exists to close.
//
// Both sides reduce to a bare user id before they are compared, because one
// seat is stamped differently by the two doors: `human:<uuid>` when they log a
// message themselves, `connector:gmail:<uuid>` when their mailbox syncs it.
func BindableTo(ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID) (bool, error) {
	var capturedBy string
	err := tx.QueryRow(ctx,
		`SELECT captured_by FROM activity WHERE id = $1`, incumbent).Scan(&capturedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading who captured a message: %w", err)
	}
	return actingHumanOf(capturedBy) == arrivingHuman(ctx), nil
}

// AddressProver answers whether ONE seat has proven ONE address is theirs — the
// question capture's own tables can answer and this module's cannot.
//
// It is a seam because the answer lives in `capture` (the connection a provider
// attested at grant, and the aliases the receiving server's own Delivered-To
// established) while the binding decision lives here, and neither module may
// import the other. Compose injects it.
//
// PROVEN, not merely known. The implementation must refuse an address a seat
// simply declared about themselves and must refuse a domain claim, because both
// are things a seat can assert with no third party behind them — see
// capture.SeatProvedAddressTx, which is where that is argued and enforced.
type AddressProver func(ctx context.Context, tx pgx.Tx, seat ids.UUID, address string) (bool, error)

// attributableTo reports whether an IMPORTED row belongs to the arriving seat,
// on the strength of an address the import states AND the seat has proven.
//
// This is the cross-seat case: an admin imports the company's history and the
// rep whose mailbox actually held those messages syncs them afterwards. Under
// the same-seat rule alone the two never bind and every message in the overlap
// is filed twice.
//
// THE DIRECTION IS THE WHOLE THING, and it looks exactly like a design that was
// rejected twice, so it is spelled out here rather than left to be re-derived.
// The addresses this reads are the IMPORTER'S OWN TEXT — participantlog.go
// writes activity_participant.address from the request body — and are therefore
// forgeable. They are used only as a LOOKUP KEY. The ANSWER comes from the
// arriving seat's proven set, which the importer cannot write. So:
//
//   - A forged address that names nobody resolves to nothing, and nothing binds.
//   - A forged address that names a COLLEAGUE does not help the forger either:
//     it can only ever attribute the row to that colleague, never to the forger,
//     because the proof is asked of the ARRIVING seat and about THEIR addresses.
//
// What an importer can still do is cause a row they filed to be joined by the
// colleague whose address they named. That colleague sees their own mail either
// way; what changes is whether they see it once or twice. It buys the importer
// no reach into anything.
//
// Only an ASSERTED incumbent is attributable. A row a connector captured is
// already covered by the same-seat rule, and widening this to observed rows
// would let one mailbox join another's captured copy on a stated address.
func attributableTo(
	ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID, proved AddressProver,
) (bool, error) {
	if proved == nil {
		return false, nil
	}
	seat := arrivingHuman(ctx)
	if seat == "" {
		return false, nil
	}
	seatID, err := ids.Parse(seat)
	if err != nil {
		return false, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.address
		  FROM activity_participant p
		  JOIN activity a ON a.id = p.activity_id
		 WHERE p.activity_id = $1
		   AND a.captured_by LIKE 'human:%'
		   AND coalesce(p.address, '') <> ''`, incumbent)
	if err != nil {
		return false, fmt.Errorf("activities: reading the addresses an import states: %w", err)
	}
	defer rows.Close()
	var stated []string
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return false, fmt.Errorf("activities: reading the addresses an import states: %w", err)
		}
		stated = append(stated, address)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("activities: reading the addresses an import states: %w", err)
	}
	for _, address := range stated {
		proven, err := proved(ctx, tx, seatID, address)
		if err != nil {
			return false, err
		}
		if proven {
			return true, nil
		}
	}
	return false, nil
}

// arrivingHuman is the seat behind the call trying to bind, or empty when there
// is no human behind it.
//
// Read off the principal rather than off storekit.CapturedBy, and the
// difference is what makes the rule work at all. CapturedBy answers the
// principal's ID, which for a connector is the bare `connector:gmail` with no
// seat in it. The stamp that same connector WRITES onto the row is
// `connector:gmail:<uuid>` — capture's connectorProvenance appends the granting
// user. Comparing those two shapes never matches, so every seat would look like
// a different seat and no import-then-mailbox pair would ever bind.
//
// A principal with no user behind it — system, a bare connection — answers
// empty and therefore matches nobody, including another copy of itself. "I
// cannot tell who this is" must never read as "the same seat".
func arrivingHuman(ctx context.Context) string {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return ""
	}
	return actor.UserID.String()
}

// actingHumanOf pulls the user id out of a captured_by stamp, or answers empty
// when there is no human in it.
//
// The stamps are structured: 'human:<uuid>' for somebody writing directly,
// 'connector:<provider>:<uuid>' for a mailbox they connected. One colleague
// importing their own mail and syncing their own mailbox therefore appears as
// two different stamps carrying one uuid, which is exactly the pair that may
// bind.
//
// Anything with no uuid — 'system', a malformed stamp, an empty one — names
// nobody, and nobody must never compare equal to nobody. Two system writes are
// not "the same seat"; they are two writes with no colleague behind them.
func actingHumanOf(capturedBy string) string {
	candidate, ok := trailingIDOf(capturedBy)
	if !ok {
		return ""
	}
	if _, err := ids.Parse(candidate); err != nil {
		// Not a uuid: a stamp shape this does not understand names nobody, and
		// naming nobody must never read as naming the same seat.
		return ""
	}
	return candidate
}

// trailingIDOf pulls the id off the end of a captured_by stamp, for the two
// shapes that carry one.
//
// 'human:<uuid>' is somebody writing directly. 'connector:<provider>:<uuid>' is
// a mailbox they connected, where the provider is not part of who they are —
// one colleague's Gmail and IMAP stamps must name one seat.
func trailingIDOf(capturedBy string) (string, bool) {
	if rest, ok := strings.CutPrefix(capturedBy, "human:"); ok {
		return strings.TrimSpace(rest), true
	}
	rest, ok := strings.CutPrefix(capturedBy, "connector:")
	if !ok {
		return "", false
	}
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		// 'connector:gmail' with no seat behind it names a provider, not a
		// colleague.
		return "", false
	}
	return strings.TrimSpace(rest[idx+1:]), true
}

// ResolveBindableIdentity answers which activity holds an external identity AND
// may be joined by the caller resolving it.
//
// Both ingestion doors ask through here, so the rule about who may bind is
// stated once: the import door through boundToKnownMessage, the capture door
// through the IdentityResolver seam compose injects. A second copy of the rule
// is how the two doors would drift, and the drift would be silent — each door
// would go on answering, just differently.
//
// Held by: TestOneFunctionDecidesWhoMayBindToAMessage (backend/gates/bindableidentityonce_test.go)
//
// Three refusals, all reported as not-found rather than as errors, because the
// answer each one leads to is the same: file your own row.
//
//   - The identity is free. Nothing holds it.
//   - A DIFFERENT seat holds it. A Message-ID is typed by whoever sent the
//     message, so binding across seats would hand one colleague's mail to
//     another on the strength of a value either could guess (BindableTo).
//   - The holder is not live. An archived row is a message that was erased,
//     redacted as noise, or merged away. Binding to it would write content back
//     over what retention destroyed, and would disclose that the message was
//     erased.
//
// Not-found for all three is deliberate beyond convenience: a caller that could
// tell "somebody else holds this" from "nothing holds this" would have an
// existence oracle over colleagues' mail, keyed on a header they can type.
func ResolveBindableIdentity(ctx context.Context, tx pgx.Tx, kind, key string) (ids.ActivityID, bool, error) {
	return bindableIdentityUnder(ctx, tx, kind, key, nil)
}

// ResolveBindableIdentityProving is ResolveBindableIdentity with the cross-seat
// arm armed: an imported row also binds when it states an address the arriving
// seat has PROVEN is theirs (attributableTo).
//
// Two entry points rather than a changed signature, because the import door and
// the capture door want different answers to the same question. An import
// resolving its own write has no mailbox behind it and nothing to prove, so it
// keeps the same-seat rule alone; a mailbox sync is exactly the case the proof
// is for. Compose injects the prover into the capture seam only.
func ResolveBindableIdentityProving(proved AddressProver) func(context.Context, pgx.Tx, string, string) (ids.ActivityID, bool, error) {
	return func(ctx context.Context, tx pgx.Tx, kind, key string) (ids.ActivityID, bool, error) {
		return bindableIdentityUnder(ctx, tx, kind, key, proved)
	}
}

func bindableIdentityUnder(
	ctx context.Context, tx pgx.Tx, kind, key string, proved AddressProver,
) (ids.ActivityID, bool, error) {
	incumbent, found, err := ResolveIdentity(ctx, tx, kind, key)
	if err != nil || !found {
		return ids.ActivityID{}, false, err
	}
	bindable, err := BindableTo(ctx, tx, incumbent)
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	if !bindable {
		// The same seat did not write both rows. One more way to say yes, and
		// only one: the incumbent is an IMPORT stating an address this seat has
		// proven is theirs. Everything else still refuses.
		bindable, err = attributableTo(ctx, tx, incumbent, proved)
		if err != nil || !bindable {
			return ids.ActivityID{}, false, err
		}
	}
	live, err := identityHolderIsLive(ctx, tx, incumbent)
	if err != nil || !live {
		return ids.ActivityID{}, false, err
	}
	return incumbent, true, nil
}

// identityHolderIsLive reports whether the row holding an identity is still a
// live message rather than an archived one.
//
// A predicate rather than a lock: this only decides whether to bind, and the
// write that follows takes its own LockRow(LiveOnly). Locking here would hold a
// row for the rest of a capture that may never write to it.
func identityHolderIsLive(ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx,
		`SELECT archived_at IS NULL FROM activity WHERE id = $1`, incumbent).Scan(&live)
	// A holder that is gone entirely is nothing to bind to, not a failure: the
	// identity row outlived the activity it named, and the arrival files its
	// own copy exactly as it would for a free identity.
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading whether %s is still live: %w", incumbent, err)
	}
	return live, nil
}
