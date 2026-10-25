// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who wrote an activity. Every surface that names a speaker reads this one
// statement: direction is not authorship, and a mail that came in was written
// by its From party, who is often not the contact whose page shows it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatName is a seat's display name and login address.
type seatName struct{ name, email string }

// WithMessageVoices fills `voice.author` on every readable row of a page.
// A withheld row gets no voice: who wrote it is content.
func WithMessageVoices(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	readable := readableIDsOf(page)
	if len(readable) == 0 {
		return nil
	}
	parties, err := readEmailPartiesOf(ctx, tx, readable)
	if err != nil {
		return err
	}
	seats, err := seatNamesFor(ctx, tx, loggingSeatsOf(page))
	if err != nil {
		return err
	}
	reader := ids.Nil
	if actor, ok := principal.Actor(ctx); ok {
		reader = actor.UserID
	}
	var unresolved []*crmcontracts.ActivityAuthor
	for i := range page {
		if !contentReadable(page[i]) {
			continue
		}
		author := authorOf(page[i], parties[ids.UUID(page[i].Id)], seats)
		if author != nil && author.UserId != nil && reader != ids.Nil && ids.UUID(*author.UserId) == reader {
			author.IsReader = true
		}
		if author != nil && author.Kind == crmcontracts.ActivityAuthorKindUnresolved && author.Address != nil {
			unresolved = append(unresolved, author)
		}
		page[i].Voice = &crmcontracts.ActivityVoice{Author: author}
	}
	return resolveAuthorContacts(ctx, tx, unresolved)
}

// authorOf is the author the recorded facts name, or nil when none does. It
// never infers a colleague from an address: only capture, which holds the
// own-domain and DMARC checks, may stamp a seat on a From party.
func authorOf(a crmcontracts.Activity, parties emailParties, seats map[ids.UUID]seatName) *crmcontracts.ActivityAuthor {
	switch a.Kind {
	case crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage:
		if len(parties.from) > 0 {
			return partyAuthor(identifiedFirst(parties.from), machineSent(a))
		}
		return importedAuthor(a.Author)
	case crmcontracts.ActivityKindMeeting:
		if len(parties.organizers) > 0 {
			return partyAuthor(identifiedFirst(parties.organizers), false)
		}
		if a.HostUserId != nil {
			return seatAuthor(ids.UUID(*a.HostUserId), seats)
		}
		return importedAuthor(a.Author)
	case crmcontracts.ActivityKindNote, crmcontracts.ActivityKindCall, crmcontracts.ActivityKindTask:
		if imported := importedAuthor(a.Author); imported != nil {
			return imported
		}
		if seat, ok := loggingSeat(a.CapturedBy); ok {
			return seatAuthor(seat, seats)
		}
	}
	return nil
}

// machineSent is capture's sending-system stamp, as automatic_kind carries it.
func machineSent(a crmcontracts.Activity) bool {
	return a.AutomaticKind != nil && *a.AutomaticKind == crmcontracts.ActivityAutomaticKindSendingSystem
}

// identifiedFirst prefers a party a seat or contact identifies over a bare
// header address for the same role.
func identifiedFirst(parties []crmcontracts.EmailParty) crmcontracts.EmailParty {
	for _, p := range parties {
		if p.UserId != nil || p.ContactId != nil {
			return p
		}
	}
	return parties[0]
}

// partyAuthor names one participant. A seat is a colleague; a sending system
// stays a machine even when a contact was minted for its address.
func partyAuthor(p crmcontracts.EmailParty, machine bool) *crmcontracts.ActivityAuthor {
	author := &crmcontracts.ActivityAuthor{DisplayName: p.DisplayName, ContactId: p.ContactId, UserId: p.UserId}
	if p.Address != "" {
		address := p.Address
		author.Address = &address
	}
	switch {
	case p.UserId != nil:
		author.Kind = crmcontracts.ActivityAuthorKindColleague
	case machine:
		author.Kind = crmcontracts.ActivityAuthorKindMachine
	case p.ContactId != nil:
		author.Kind = crmcontracts.ActivityAuthorKindContactRecord
	default:
		author.Kind = crmcontracts.ActivityAuthorKindUnresolved
	}
	return author
}

// importedAuthor is the author an importer named on the source record.
func importedAuthor(source *crmcontracts.SourceAuthor) *crmcontracts.ActivityAuthor {
	if source == nil {
		return nil
	}
	name := source.DisplayName
	author := &crmcontracts.ActivityAuthor{DisplayName: &name, Kind: crmcontracts.ActivityAuthorKindUnresolved}
	if source.UserId != nil {
		author.UserId = source.UserId
		author.Kind = crmcontracts.ActivityAuthorKindColleague
	}
	return author
}

func seatAuthor(seat ids.UUID, seats map[ids.UUID]seatName) *crmcontracts.ActivityAuthor {
	id := openapi_types.UUID(seat)
	author := &crmcontracts.ActivityAuthor{Kind: crmcontracts.ActivityAuthorKindColleague, UserId: &id}
	if named, ok := seats[seat]; ok {
		if named.name != "" {
			author.DisplayName = &named.name
		}
		if named.email != "" {
			author.Address = &named.email
		}
	}
	return author
}

// loggingSeat is the seat a hand-written row's captured_by names. A connector
// or agent stamp names no human author.
func loggingSeat(capturedBy *string) (ids.UUID, bool) {
	if capturedBy == nil {
		return ids.Nil, false
	}
	return principal.HumanUserID(*capturedBy)
}

// loggingSeatsOf lists the seats a page's rows may need a name for.
func loggingSeatsOf(page []crmcontracts.Activity) []ids.UUID {
	var out []ids.UUID
	for i := range page {
		if page[i].HostUserId != nil {
			out = append(out, ids.UUID(*page[i].HostUserId))
		}
		if seat, ok := loggingSeat(page[i].CapturedBy); ok {
			out = append(out, seat)
		}
	}
	return out
}

// seatNamesFor reads each seat's name. No liveness filter: who wrote a row in
// August is a fact about August, the rule readEmailPartiesOf follows too.
func seatNamesFor(ctx context.Context, tx pgx.Tx, seats []ids.UUID) (map[ids.UUID]seatName, error) {
	out := map[ids.UUID]seatName{}
	if len(seats) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id, coalesce(display_name, ''), coalesce(email, '')
		  FROM app_user WHERE id = ANY($1)`, seats)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the seats that wrote a page: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var named seatName
		if err := rows.Scan(&id, &named.name, &named.email); err != nil {
			return nil, fmt.Errorf("activities: reading the seats that wrote a page: %w", err)
		}
		out[id] = named
	}
	return out, rows.Err()
}

// resolveAuthorContacts names a sender stored as a bare address by the one
// contact holding that address, within the reader's contact scope. An address
// two contacts share names neither.
func resolveAuthorContacts(ctx context.Context, tx pgx.Tx, authors []*crmcontracts.ActivityAuthor) error {
	if len(authors) == 0 || !auth.ReadGranted(ctx, "contact") {
		return nil
	}
	addresses := make([]string, 0, len(authors))
	for _, a := range authors {
		addresses = append(addresses, correspondence.Fold(*a.Address))
	}
	args := []any{addresses}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return err
	}
	visible := ""
	if scope != "" {
		visible = " AND (" + scope + ")"
	}
	rows, err := tx.Query(ctx, `
		SELECT e.email, (array_agg(p.id))[1], (array_agg(p.full_name))[1]
		  FROM contact_email e
		  JOIN contact p ON p.id = e.contact_id AND p.archived_at IS NULL`+visible+`
		 WHERE e.email = ANY($1) AND e.archived_at IS NULL
		 GROUP BY e.email
		HAVING count(DISTINCT p.id) = 1`, args...)
	if err != nil {
		return fmt.Errorf("activities: naming the senders of a page: %w", err)
	}
	type named struct {
		id   ids.UUID
		name *string
	}
	byAddress := map[string]named{}
	for rows.Next() {
		var address string
		var n named
		if err := rows.Scan(&address, &n.id, &n.name); err != nil {
			rows.Close()
			return fmt.Errorf("activities: naming the senders of a page: %w", err)
		}
		byAddress[address] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("activities: naming the senders of a page: %w", err)
	}
	for _, a := range authors {
		n, ok := byAddress[correspondence.Fold(*a.Address)]
		if !ok {
			continue
		}
		id := openapi_types.UUID(n.id)
		a.ContactId, a.Kind = &id, crmcontracts.ActivityAuthorKindContactRecord
		if n.name != nil && *n.name != "" {
			a.DisplayName = n.name
		}
	}
	return nil
}

func contentReadable(a crmcontracts.Activity) bool {
	return a.ContentState == nil || *a.ContentState != crmcontracts.ActivityContentStateWithheld
}

func readableIDsOf(page []crmcontracts.Activity) []ids.UUID {
	var out []ids.UUID
	for i := range page {
		if contentReadable(page[i]) {
			out = append(out, ids.UUID(page[i].Id))
		}
	}
	return out
}
