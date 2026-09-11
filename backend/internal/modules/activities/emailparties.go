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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// emptyParties is an empty list rather than a nil one: a viewer that sees no
// `to` array cannot tell "nobody" from "the field was not sent", and the
// difference decides whether it renders a recipient row at all.
func emptyParties() []crmcontracts.EmailParty {
	return []crmcontracts.EmailParty{}
}

type emailParties struct {
	from, to, cc, bcc []crmcontracts.EmailParty
}

// readEmailParties reads the message's normalised recipients. They are stored
// as activity_participant rows with a role, which is why the viewer never has
// to parse a provider payload to learn who was on a message.
//
// A person's name is resolved only through the caller's own row scope: an
// address the caller may not see a contact for stays an address, which is the
// truth rather than a blank.
func readEmailParties(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (emailParties, error) {
	args := []any{id}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return emailParties{}, err
	}
	personJoin := `LEFT JOIN person p ON p.id = ap.person_id AND p.archived_at IS NULL`
	if scope != "" {
		personJoin += ` AND (` + scope + `)`
	}
	// Three sources for one name, in the order of how much this installation
	// knows the person. The contact record first — it is ours and it is
	// maintained. Then the SEAT, for a colleague. Then the name the sender typed
	// into the header, which capture keeps and nothing here read: it is the
	// weakest, because it is whatever the other side wrote, but it beats
	// printing a bare address at a reader.
	//
	// A colleague's SEAT is named from app_user, not from person. A message to
	// somebody in this workspace records their user_id and, for a seat capture
	// resolved rather than read off a header, no address at all — so a party
	// joined only against `person` came back with an empty address and no name,
	// and the reader saw a bare comma where a colleague should be. That reader
	// is usually the very person it stood for: this is how a rep could not tell
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
	// resolved by id to render a name does not ask whether the person still
	// works here.
	rows, err := tx.Query(ctx, `
		SELECT ap.role, coalesce(ap.address, ''), p.id,
		       coalesce(p.full_name, u.display_name, ap.display_name), ap.user_id,
		       coalesce(u.email, '')
		  FROM activity_participant ap
		  `+personJoin+`
		  LEFT JOIN app_user u ON u.id = ap.user_id
		 WHERE ap.activity_id = $1
		   AND ap.role IN ('from', 'to', 'cc', 'bcc')
		 ORDER BY CASE ap.role
		            WHEN 'from' THEN 1 WHEN 'to' THEN 2 WHEN 'cc' THEN 3 ELSE 4
		          END, ap.created_at, ap.id`, args...)
	if err != nil {
		return emailParties{}, err
	}
	defer rows.Close()

	// Empty, not nil. Every one of these four is `required` in the contract, and
	// a nil slice marshals to `null` rather than `[]` — so a message with
	// nobody in copy served a null the viewer is entitled to treat as a list.
	// It read `.length` off it and the drawer died where the message should be.
	out := emailParties{
		from: emptyParties(),
		to:   emptyParties(),
		cc:   emptyParties(),
		bcc:  emptyParties(),
	}
	for rows.Next() {
		var role, address, seatEmail string
		var personID, userID *ids.UUID
		var fullName *string
		if err := rows.Scan(&role, &address, &personID, &fullName, &userID, &seatEmail); err != nil {
			return emailParties{}, err
		}
		// The seat's own address, when the participant row carries none. Capture
		// writes an address for a party it read off a header and only a user_id
		// for one it resolved to a seat, so this is the difference between a
		// header line naming a colleague and one with a gap in it.
		if address == "" {
			address = seatEmail
		}
		party := crmcontracts.EmailParty{Address: address, DisplayName: fullName}
		if personID != nil {
			pid := openapi_types.UUID(*personID)
			party.PersonId = &pid
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
		}
	}
	return out, rows.Err()
}

// counterpartyOf names the other side for a row: the first party the caller
// can name, and how many more there were. A message whose participants all
// resolve to nothing gets no counterparty rather than an invented stranger.
func counterpartyOf(parties []crmcontracts.EmailParty) *string {
	if len(parties) == 0 {
		return nil
	}
	var named string
	for _, p := range parties {
		if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
			named = *p.DisplayName
			break
		}
		if named == "" && p.Address != "" {
			named = p.Address
		}
	}
	if named == "" {
		return nil
	}
	if extra := len(parties) - 1; extra > 0 {
		named += " +" + strconv.Itoa(extra)
	}
	return &named
}
