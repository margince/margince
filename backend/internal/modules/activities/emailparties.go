// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who was on a message, and which one of them the row names.
//
// Recipients are stored as activity_participant rows with a role, so nothing
// here parses a provider payload: the question "who was this with" is answered
// from the product's own records, under the caller's own row scope.

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// emptyParties is an empty list rather than a nil one: a viewer that sees no
// `to` array cannot tell "nobody" from "the field was not sent", and the
// difference decides whether it renders a recipient row at all.
func emptyParties() []crmcontracts.EmailParty {
	return []crmcontracts.EmailParty{}
}

type emailParties struct {
	from, to, cc, bcc     []crmcontracts.EmailParty
	organizers, attendees []crmcontracts.EmailParty
}

// readEmailParties reads the message's normalised recipients. They are stored
// as activity_participant rows with a role, which is why the viewer never has
// to parse a provider payload to learn who was on a message.
func readEmailParties(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (emailParties, error) {
	byMessage, err := readEmailPartiesOf(ctx, tx, []ids.UUID{id.UUID})
	if err != nil {
		return emailParties{}, err
	}
	return byMessage[id.UUID], nil
}

// readEmailPartiesOf is readEmailParties for a page of messages in one read.
// Every message asked about has an entry, empty when nobody is on it.
//
// A contact's name is resolved only through the caller's own row scope: an
// address the caller may not see a contact for stays an address, which is the
// truth rather than a blank.
func readEmailPartiesOf(ctx context.Context, tx pgx.Tx, messages []ids.UUID) (map[ids.UUID]emailParties, error) {
	args := []any{messages}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// Row scope assumes the object grant. A reader without contact read keeps
	// the header's own address and name, and learns no contact behind them.
	contactJoin := `LEFT JOIN contact p ON false`
	if auth.ReadGranted(ctx, "contact") {
		scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
		if err != nil {
			return nil, err
		}
		contactJoin = `LEFT JOIN contact p ON p.id = ap.contact_id AND p.archived_at IS NULL`
		if scope != "" {
			contactJoin += ` AND (` + scope + `)`
		}
	}
	// Three sources for one name, in the order of how much this installation
	// knows the contact. The contact record first — it is ours and it is
	// maintained. Then the SEAT, for a colleague. Then the name the sender typed
	// into the header, which capture keeps and nothing here read: it is the
	// weakest, because it is whatever the other side wrote, but it beats
	// printing a bare address at a reader.
	//
	// A colleague's SEAT is named from app_user, not from contact. A message to
	// somebody in this workspace records their user_id and, for a seat capture
	// resolved rather than read off a header, no address at all — so a party
	// joined only against `contact` came back with an empty address and no name,
	// and the reader saw a bare comma where a colleague should be. That reader
	// is usually the very contact it stood for: this is how a rep could not tell
	// why a message had reached them.
	//
	// A seat is not row-scoped the way a contact is. The workspace roster is
	// readable to every member — it is who a reader shares an installation with
	// — so naming one discloses nothing the roster does not already.
	//
	// And NO liveness filter on that join. Who a message went to in August is a
	// fact about August: a colleague who has since left was still on it, and
	// dropping their name would leave a gap in a header that is otherwise
	// complete — the very defect this join fixes, reappearing for anybody who
	// resigns. This is the case livemember_test names as outside its rule: a row
	// resolved by id to render a name does not ask whether the contact still
	// works here.
	// The contact's own address, for a row capture resolved to a contact
	// without keeping an address — the logged path writes those. The address
	// the MESSAGE stated wins over the contact's primary one, so the fold
	// below recognises the bare header row it duplicates even when the sender
	// wrote from a secondary address. It rides the SCOPED contact join, so an
	// address surfaces only where the contact itself may be named.
	rows, err := tx.Query(ctx, `
		SELECT activity_id, role, address, contact_id, name, user_id, seat_email, contact_email FROM (
		SELECT ap.activity_id, ap.role, coalesce(ap.address, '') AS address, p.id AS contact_id,
		       coalesce(p.full_name, u.display_name, ap.display_name) AS name, ap.user_id,
		       coalesce(u.email, '') AS seat_email, coalesce(stated_pe.email, own_pe.email, '') AS contact_email,
		       ap.created_at, ap.id
		  FROM activity_participant ap
		  `+contactJoin+`
		  LEFT JOIN app_user u ON u.id = ap.user_id
		  LEFT JOIN LATERAL (
		       SELECT e.email FROM contact_email e
		        WHERE e.contact_id = p.id AND e.archived_at IS NULL
		          AND EXISTS (
		           SELECT 1 FROM activity_participant stated
		            WHERE stated.activity_id = ap.activity_id AND stated.role = ap.role
		              AND lower(stated.address) = e.email)`+
		contactaddress.ReachableOrder+` LIMIT 1) stated_pe ON p.id IS NOT NULL
		  LEFT JOIN LATERAL (
		       SELECT e.email FROM contact_email e
		        WHERE e.contact_id = p.id AND e.archived_at IS NULL`+
		contactaddress.ReachableOrder+` LIMIT 1) own_pe ON p.id IS NOT NULL
		 WHERE ap.activity_id = ANY($1)
		   AND ap.role IN ('from', 'to', 'cc', 'bcc', 'organizer', 'attendee')
		   AND NOT `+unstatedContactSQL("ap")+`
		UNION ALL
		`+receivedFromSQL()+`) parties
		 ORDER BY CASE role
		            WHEN 'from' THEN 1 WHEN 'to' THEN 2 WHEN 'cc' THEN 3 ELSE 4
		          END, created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	out, err := collectEmailParties(rows, messages)
	if err != nil {
		return nil, err
	}
	for id, parties := range out {
		// The logged path keeps a stated header address beside the contact or
		// seat it resolved to. Unfolded, one sender reads as two parties.
		parties.from = foldStatedDuplicates(parties.from)
		parties.to = foldStatedDuplicates(parties.to)
		parties.cc = foldStatedDuplicates(parties.cc)
		parties.bcc = foldStatedDuplicates(parties.bcc)
		out[id] = parties
	}
	return out, nil
}

// receivedFromSQL is the sender of each received message whose capture
// recorded no sender participant, as older captures did. On a received
// message the counterparty is the sender. It rides the party read as one more
// arm, so a page pays no statement of its own for it.
func receivedFromSQL() string {
	return `
		SELECT a.id, 'from', a.counterparty_email, NULL::uuid, NULL::text, NULL::uuid, '', '',
		       a.created_at, a.id
		  FROM activity a
		 WHERE a.id = ANY($1) AND a.archived_at IS NULL AND a.direction = 'inbound'
		   AND coalesce(a.counterparty_email, '') <> ''
		   AND NOT EXISTS (SELECT 1 FROM activity_participant ap
		         WHERE ap.activity_id = a.id AND ap.role = 'from' AND NOT ` + unstatedContactSQL("ap") + `)`
}

// counterpartyOf names the other side for a row: the first party the caller
// can name, how many more there were, and WHICH CONTACT supplied the name.
//
// The id travels beside the phrase because the phrase cannot be turned back
// into a record. A client keying a face on the words alone has to guess which
// contact they mean, and guesses wrong in both directions: a contact renamed
// since capture stops matching and draws a second colour, and two contacts
// sharing a name cannot be told apart at all. It is the id of the party the
// name came FROM, not of the row — a message names one far side and the ` +N`
// counts the rest.
//
// A message whose participants all resolve to nothing gets no counterparty
// rather than an invented stranger, and an unresolved address names no contact.
func counterpartyOf(parties []crmcontracts.EmailParty) (*string, *openapi_types.UUID) {
	if len(parties) == 0 {
		return nil, nil
	}
	var named string
	var namedBy *openapi_types.UUID
	for _, p := range parties {
		if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
			named, namedBy = *p.DisplayName, p.ContactId
			break
		}
		if named == "" && p.Address != "" {
			named, namedBy = p.Address, p.ContactId
		}
	}
	if named == "" {
		return nil, nil
	}
	if extra := len(parties) - 1; extra > 0 {
		named += " +" + strconv.Itoa(extra)
	}
	return &named, namedBy
}

// foldStatedDuplicates drops an address-only party whose address an
// identified party on the same header already carries: the header's
// statement and capture's resolution both describe that human, and the
// envelope draws them as one line. A bare address nobody resolved stays —
// it is the only record of that party.
func foldStatedDuplicates(parties []crmcontracts.EmailParty) []crmcontracts.EmailParty {
	identified := make(map[string]bool, len(parties))
	for _, p := range parties {
		if (p.ContactId != nil || p.UserId != nil) && p.Address != "" {
			identified[strings.ToLower(p.Address)] = true
		}
	}
	if len(identified) == 0 {
		return parties
	}
	kept := parties[:0]
	for _, p := range parties {
		if p.ContactId == nil && p.UserId == nil && identified[strings.ToLower(p.Address)] {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// collectEmailParties reads the participant rows into the four header lists
// of each message.
func collectEmailParties(rows pgx.Rows, messages []ids.UUID) (map[ids.UUID]emailParties, error) {
	defer rows.Close()
	// Empty, not nil. Every one of these four is `required` in the contract, and
	// a nil slice marshals to `null` rather than `[]` — so a message with
	// nobody in copy served a null the viewer is entitled to treat as a list.
	// It read `.length` off it and the drawer died where the message should be.
	byMessage := make(map[ids.UUID]emailParties, len(messages))
	for _, id := range messages {
		byMessage[id] = emailParties{from: emptyParties(), to: emptyParties(), cc: emptyParties(), bcc: emptyParties()}
	}
	for rows.Next() {
		var message ids.UUID
		var role, address, seatEmail, contactEmail string
		var contactID, userID *ids.UUID
		var fullName *string
		if err := rows.Scan(&message, &role, &address, &contactID, &fullName, &userID, &seatEmail, &contactEmail); err != nil {
			return nil, err
		}
		out := byMessage[message]
		// The seat's own address, when the participant row carries none. Capture
		// writes an address for a party it read off a header and only a user_id
		// for one it resolved to a seat, so this is the difference between a
		// header line naming a colleague and one with a gap in it. A contact
		// resolved without an address gets their own the same way.
		if address == "" {
			address = seatEmail
		}
		if address == "" {
			address = contactEmail
		}
		party := crmcontracts.EmailParty{Address: address, DisplayName: fullName}
		if contactID != nil {
			pid := openapi_types.UUID(*contactID)
			party.ContactId = &pid
		}
		if userID != nil {
			uid := openapi_types.UUID(*userID)
			party.UserId = &uid
		}
		switch role {
		case roleFrom:
			out.from = append(out.from, party)
		case roleTo:
			out.to = append(out.to, party)
		case roleCc:
			out.cc = append(out.cc, party)
		case roleBcc:
			out.bcc = append(out.bcc, party)
		case "organizer":
			out.organizers = append(out.organizers, party)
		case "attendee":
			out.attendees = append(out.attendees, party)
		}
		byMessage[message] = out
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return byMessage, nil
}
