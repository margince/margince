// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Who was in the interaction (ACT-DDL-3 / ADR-0078). The activity row records
// that a message happened; activity_link records which RECORDS it concerns.
// Neither says which of OUR people was in it, and that is the whole reason
// "who on our team knows this contact" cannot be answered today.
//
// Capture is the one place that knows. The connector principal carries the
// granting human's id — the mailbox owner, per-user-per-provider from
// capture_connection — so the our-side participant is a fact at ingest, not an
// inference from a `captured_by` string that connector mail never sets to a
// human in the first place.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Participant roles. The set is closed at the database (the ACT-DDL-3 CHECK);
// these constants are the Go spelling of it, so a typo is a compile error
// rather than a constraint violation at 3am.
const (
	roleFrom = "from"
	roleTo   = "to"
)

// stampCaptureParticipants records the two ends of a captured message: the
// mailbox owner whose connection produced it, and the counterparty it was
// exchanged with.
//
// The counterparty lands as an ADDRESS, not a person: capture creates the
// person after this transaction commits (the tiered creation gate may also
// decide not to create one at all), so the address is the honest answer at
// this point. promoteParticipantToPerson upgrades the row later, when and if
// an identity resolves. Recording the address now rather than waiting is what
// keeps a suppressed or deferred counterparty from vanishing from the record
// of who was in the conversation.
//
// Direction decides the roles and nothing else: on an outbound message our
// user is the sender, on an inbound one they are the recipient. That
// distinction is what lets the edge derivation tell a real exchange from a
// hundred unanswered sends.
func stampCaptureParticipants(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	ownerUserID ids.UUID,
	kind string,
	direction string,
	counterpartyEmail string,
) error {
	// The same kinds the hand-logged path accepts. Without this a captured note
	// becomes a conversation while an identical hand-logged one does not, and
	// the backfill disagrees with both.
	if !relstrength.IsParticipantKind(kind) {
		return nil
	}
	ourRole, theirRole := roleFrom, roleTo
	if direction == connector.DirectionInbound {
		ourRole, theirRole = roleTo, roleFrom
	}

	if ownerUserID != ids.Nil {
		if err := insertParticipant(ctx, tx, activityID, ourRole, &ownerUserID, nil, ""); err != nil {
			return fmt.Errorf("capture: stamping the mailbox owner as a participant: %w", err)
		}
	}
	// Normalized the same way person_email is, so the promotion below and the
	// erasure lookup both match without a runtime case fold.
	address := strings.ToLower(strings.TrimSpace(counterpartyEmail))
	// Never the owner's OWN address as the other end. The connector derives the
	// counterparty by comparing the From header against the one address the
	// grant names, so a message the owner sent from an alias arrives with that
	// alias as its counterparty — and stamping it here would record the owner
	// at both ends of their own message, in the rows the interaction graph
	// reads as "who talked to whom".
	//
	// Nothing promotes such a row to a person (the promotion needs a
	// person_email, and the capture gates keep an alias from having one), so
	// what this prevents is a durable falsehood about the exchange rather than
	// a contact record.
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return err
	}
	if self.Covers(address) {
		address = ""
	}
	if address != "" {
		if err := insertParticipant(ctx, tx, activityID, theirRole, nil, nil, address); err != nil {
			return fmt.Errorf("capture: stamping the counterparty as a participant: %w", err)
		}
	}
	return nil
}

// RecordAttendeeNames settles the display_name of every participant row on one
// activity from what the invitation said, and marks the rest as unnamed.
//
// It lives here because capture owns no table either — activities does — but
// this is the module that already writes activity_participant, and a second
// spelling of that write in compose would be a second place for the column set
// to drift. The recovery pass in compose reads stored originals and calls this,
// exactly as it calls StampFurtherParticipants for the rows themselves.
//
// EVERY name-less row on the activity is settled, named or not. A row left NULL
// because the invitation did not name its attendee would be offered to the
// recovery pass again on its next tick, and the pass would never drain.
func RecordAttendeeNames(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	participants []connector.MessageParticipant,
) error {
	addresses := make([]string, 0, len(participants))
	names := make([]string, 0, len(participants))
	for _, p := range participants {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		name := strings.TrimSpace(p.DisplayName)
		if address == "" || name == "" {
			continue
		}
		addresses = append(addresses, address)
		names = append(names, name)
	}
	if len(addresses) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE activity_participant ap
			   SET display_name = inp.display_name
			  FROM unnest($2::text[], $3::text[]) AS inp(address, display_name)
			 WHERE ap.activity_id = $1
			   AND ap.address = inp.address
			   AND ap.display_name IS NULL`,
			activityID, addresses, names); err != nil {
			return fmt.Errorf("capture: recording the name an invitation gave: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity_participant SET display_name = ''
		 WHERE activity_id = $1 AND display_name IS NULL`, activityID); err != nil {
		return fmt.Errorf("capture: settling the attendees an invitation did not name: %w", err)
	}
	return nil
}

// namesSomebody reports whether any of these parties arrived carrying a name.
//
// It is what keeps the naming pass off every ordinary mail capture: mail brings
// no display name through this path, so without the guard every message would
// buy a query that can only answer "nobody".
func namesSomebody(participants []connector.MessageParticipant) bool {
	for _, p := range participants {
		if strings.TrimSpace(p.DisplayName) != "" {
			return true
		}
	}
	return false
}

// StampFurtherParticipants records everyone in the interaction who is neither
// the mailbox owner nor the counterparty: the CCs on a thread, the organizer
// and attendees of a meeting.
//
// It is exported for the replay pass, which re-reads stored originals for
// activities captured before this existed. That pass runs in compose because
// it spans the mail and calendar parsers, and it must write these rows the
// same way live capture does — one spelling of the resolution, so a recovered
// row and a captured one are indistinguishable to the graph that reads them.
//
// Unlike the counterparty, these addresses are RESOLVED here rather than left
// for a later promotion. The reason is the interaction graph: its recompute
// joins a participant's user_id to a person_id (search.RecomputeEdgesForActivities),
// so an address-only row is invisible to it — and answering "who on our team
// knows this contact" from CC lines was the entire point of recording them.
// The counterparty can wait because the ensure path promotes its row moments
// later; nothing promotes a CC.
//
// A party who resolves to neither a colleague nor a known contact is still
// recorded by address. An attendee nobody has a record for is a fact about the
// meeting, and dropping them is what the body-text fold already does badly.
//
// A CHAT NAMES THE SAME FACT DIFFERENTLY. The third human in a group has an
// account at the provider and no address anywhere, so channelProvider is taken
// alongside the parties: an account id means nothing without the transport that
// issued it, and the record already names that one. The party is then recorded
// by account exactly as a mail party is recorded by address — the difference is
// only which lookup can resolve them to a person.
//
// partyListIsAttested is what decides whether a colleague may be bound by
// user_id, and it has two sources: our own provider attesting that this seat
// SENT the mail (so the Cc line is what our user typed), or the provider
// enumerating a calendar's attendees over an authenticated connection. Both are
// the provider's word; neither is a sender's text. ParticipantListAttested is
// the one place that decision is taken.
func StampFurtherParticipants(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	kind string,
	channelProvider string,
	partyListIsAttested bool,
	participants []connector.MessageParticipant,
) error {
	if !relstrength.IsParticipantKind(kind) || len(participants) == 0 {
		return nil
	}
	addresses := make([]string, 0, len(participants))
	roles := make([]string, 0, len(participants))
	// Names are coalesced to '' rather than left null: '' says the stamp ran
	// and this transport named nobody, while NULL is reserved for rows written
	// before the column existed, which is what the recovery pass selects on.
	names := make([]string, 0, len(participants))
	// A chat names the third human in a group by the provider's account id and
	// by nothing else, so an address is not what makes a party recordable — an
	// identity is, and there are now two shapes of one. Both columns are kept
	// per party: a roster may carry an address as well, and the row records
	// which of each was actually seen rather than picking a winner.
	accounts := make([]string, 0, len(participants))
	// One account is one human, so a roster naming the same one twice
	// contributes one row. The uniqueness index cannot say that — it separates
	// two entries that differ by address, which is right for mail and wrong
	// here, since a party described once with an address and once without is
	// the same party described twice. Left to the index, the graph would count
	// them as two people in the room.
	// Where a party is named twice, the second description FILLS IN the first
	// rather than being dropped. Skipping it would make the row depend on
	// roster order — a party listed by account and then by account with an
	// address would keep the address, and the same two entries the other way
	// round would lose it, along with every reader that resolves through it.
	rowOf := make(map[string]int, len(participants))
	// The same collapse by ADDRESS, for the party a roster describes once with
	// an account and once without. One address is one human just as surely, and
	// the uniqueness index cannot say so here either — the two rows differ on
	// the account column, so it keeps both and the graph counts one person
	// twice. Only an entry carrying NO account of its own folds this way: an
	// account is the stronger identity, and two accounts sharing an address are
	// two rows on purpose.
	rowOfAddress := make(map[string]int, len(participants))
	for _, p := range participants {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		account := strings.TrimSpace(p.ChannelUserID)
		name := strings.TrimSpace(p.DisplayName)
		// AN ACCOUNT WITHOUT A TRANSPORT IS NOT AN IDENTITY, and storing one
		// would be worse than dropping it. Every reader of this column pairs it
		// with activity.channel_provider, and the database forces that column
		// NULL for every kind but a message — so an account written beside a
		// call or a meeting matches nothing, including the Art. 17 scrub and
		// the retention sweep, and the id would stand past both, attributable
		// to nobody. A unit that names one is told so at the ingress door.
		if strings.TrimSpace(channelProvider) == "" {
			account = ""
		}
		if address == "" && account == "" {
			continue
		}
		// Which row already speaks for this party, if any. An account names one
		// first; an entry carrying none falls back to its address, because a
		// party described once with an account and once without is still one
		// human.
		at, already := -1, false
		if account != "" {
			at, already = rowOf[account]
		} else if address != "" {
			at, already = rowOfAddress[address]
		}
		if already {
			// Only an EMPTY field is filled: the first description of a party
			// stands, and a second spelling of something they already gave is
			// not a correction.
			if addresses[at] == "" {
				addresses[at] = address
				// The row becomes reachable BY THAT ADDRESS the moment it
				// gains one. A party named first by account alone and later
				// by the bare address would otherwise find no row on the
				// second look and open a second one for the same human —
				// exactly the split this index exists to prevent, reached
				// through the fill rather than through a fresh row.
				if _, named := rowOfAddress[address]; address != "" && !named {
					rowOfAddress[address] = at
				}
			}
			if names[at] == "" {
				names[at] = name
			}
			continue
		}
		if account != "" {
			rowOf[account] = len(accounts)
		}
		if _, named := rowOfAddress[address]; address != "" && !named {
			rowOfAddress[address] = len(accounts)
		}
		addresses = append(addresses, address)
		accounts = append(accounts, account)
		roles = append(roles, p.Role)
		names = append(names, name)
	}
	// Nobody recordable, which is not the same as no parties offered: a roster
	// of accounts on a record naming no transport arrives here and leaves
	// nothing behind.
	if len(roles) == 0 {
		return nil
	}

	// Both lookups run under the workspace GUC, so neither can resolve an
	// address to somebody in another tenant.
	//
	// The COLLEAGUE arm is gated on partyListIsAttested, and that gate is the
	// load-bearing part. A recipient list on an INBOUND message is written by
	// whoever sent it: nothing authenticates it, and DKIM does not cover a Cc
	// line the sender chose. Binding a user_id from one would let an outsider
	// mail a synced mailbox with `Cc: ceo@ourcompany.com` and manufacture an
	// interaction edge — the graph would then name that colleague as the
	// warmest route to the sender's own contact, on evidence the sender wrote.
	//
	// A calendar attendee list passes the gate for the opposite reason: it is
	// not text on a message at all. It is the provider's own record of who was
	// invited, read back over the authenticated connection of a seat that is on
	// the event, and the connector core stamps that attestation from the
	// registry rather than from anything the record said about itself.
	//
	// Nothing is lost by refusing it. A colleague genuinely copied on inbound
	// mail receives that message in their OWN mailbox, where their own
	// connection stamps them as its owner — attested rather than asserted. The
	// edge arrives either way; only the forgery does not.
	//
	// The address is kept alongside whichever id resolved, matching what the
	// counterparty promotion does — the row records which address was actually
	// written to, and a person may hold several.
	//
	// THE ACCOUNT ARM IS NOT UNDER THAT GATE, because it never reaches a seat.
	// A chat roster resolves a party to a PERSON record through the binding
	// person_channel_identity already holds, and to nothing else — no fact in
	// this system attests that a channel account belongs to a member, so there
	// is no colleague arm for the attestation to guard. That is what keeps a
	// roster a statement about who was in the room rather than a grant: the
	// discovery gate reads `address IS NOT NULL` as its evidence, and a party
	// known only by account carries neither that nor a user_id.
	//
	// The person arm is unguarded for both shapes for the same reason it always
	// was: naming an existing contact on an activity discloses nothing to them
	// and creates no reader.
	//
	// THE ACCOUNT IS TRIED FIRST, which is the core's own precedence rather
	// than this query's preference: a channel identity NAMES a human and an
	// address beside it CORROBORATES them (connector.Counterparty says so, and
	// the counterparty ladder resolves in that order). Reversed, a party
	// carrying both would file under whoever holds the ADDRESS while the row
	// kept somebody else's account — one row naming two people, and an erasure
	// keyed on either of them reaching a record about the other.
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, person_id, address, channel_user_id, role, display_name)
		SELECT $1, u.id, pe.person_id, NULLIF(inp.address, ''), NULLIF(inp.account, ''), inp.role, inp.display_name
		  FROM unnest($2::text[], $3::text[], $5::text[], $6::text[]) AS inp(address, role, display_name, account)
		  LEFT JOIN app_user u
		         ON $4 AND inp.address <> '' AND lower(u.email) = inp.address
		  LEFT JOIN LATERAL (
		       SELECT coalesce(
		           (SELECT c.person_id
		              FROM person_channel_identity c
		             WHERE inp.account <> '' AND c.provider = $7 AND c.channel_user_id = inp.account
		               AND c.archived_at IS NULL
		             ORDER BY c.person_id
		             LIMIT 1),
		           (SELECT p.person_id
		              FROM person_email p
		             WHERE inp.address <> '' AND p.email = inp.address AND p.archived_at IS NULL
		             ORDER BY p.person_id
		             LIMIT 1)) AS person_id) pe ON u.id IS NULL
		ON CONFLICT DO NOTHING`,
		activityID, addresses, roles, partyListIsAttested, names, accounts, channelProvider); err != nil {
		return fmt.Errorf("capture: stamping the further participants of an interaction: %w", err)
	}
	return nil
}

// insertParticipant writes one participant row, idempotently. Capture's sync
// loop is at-least-once and its whole write path is keyed on the source
// natural key, so a replay must add nothing — hence ON CONFLICT DO NOTHING
// against the ACT-DDL-3 uniqueness index rather than a prior SELECT, which
// would race with a concurrent replay of the same message.
func insertParticipant(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	role string,
	userID *ids.UUID,
	personID *ids.PersonID,
	address string,
) error {
	// The user arm rides a SELECT over app_user for the same reason the logged
	// path does: a principal's UserID need not name a workspace member, and
	// the composite FK would reject it — failing an ingest we have already
	// read off the wire, over a participant row that is a nicety rather than
	// the point of the write.
	_, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, person_id, address, role)
		SELECT $1, $2, $3, NULLIF($4, ''), $5
		 WHERE $2::uuid IS NULL
		    OR EXISTS (SELECT 1 FROM app_user u WHERE u.id = $2)
		ON CONFLICT DO NOTHING`,
		activityID, userID, personID, address, role)
	return err
}

// actorUserID is the mailbox owner behind the acting connector principal —
// the granting human the registry stamped onto it (capture_connection is
// per-user-per-provider). It answers ids.Nil when no actor is bound, which the
// caller treats as "no our-side participant" rather than an error: the sink
// has already refused a non-connector principal by the time this runs, so a
// zero here means a code path that built a principal without a grantor, and
// losing one participant row is a better outcome than failing a message we
// have already read off the wire.
func actorUserID(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil
	}
	return actor.UserID
}

// ParticipantListAttested answers whether this record's party list came from the
// PROVIDER rather than from a sender, which is what StampFurtherParticipants
// needs before it binds a colleague's user_id.
//
// One function because two writers ask it — live capture and the replay pass in
// compose — and a second spelling is how one of them ends up trusting a list the
// other refuses. Either half is sufficient and both are the provider's word:
//
//   - the provider attested that our own seat SENT this mail, so its recipient
//     list is what our user typed;
//   - the provider enumerated a calendar's attendees, which is its record of who
//     was invited rather than anybody's prose.
//
// A record that carries neither leaves the answer false and keeps the strict
// mail rule, which is what the extension ingress gets: it copies a third-party
// unit's kind through with no vocabulary check, so a unit calling its record a
// meeting attests nothing by saying so.
func ParticipantListAttested(rec connector.NormalizedRecord) bool {
	return rec.Counterparty.SentByOwner() || rec.ParticipantsAreProviderAttested()
}

// meetingHostUserID is the seat whose calendar this meeting was captured from,
// for the activity.host_user_id column — nil for every other kind, and for a
// principal carrying no user.
//
// It records WHOSE CALENDAR the row came from, not a claim that they organized
// the invitation: an attendee's own connector captures an event somebody else
// called, and this names that attendee. The brief lanes read it to say whose
// meeting a row is. It is not an authorization input — no read is granted from
// it alone, because a label saying "this came off your calendar" is ownership,
// not membership.
func meetingHostUserID(ctx context.Context, kind string) *ids.UUID {
	if kind != meetingKind {
		return nil
	}
	user := actorUserID(ctx)
	if user.IsZero() {
		return nil
	}
	return &user
}
