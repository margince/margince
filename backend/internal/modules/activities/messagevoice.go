// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who wrote an activity. Every surface that names a speaker reads this one
// statement, because direction is not authorship. A mail that came in was
// written by its From party, often not the contact whose page shows it.

import (
	"context"
	"fmt"
	"maps"
	"slices"

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
	invitedBy, err := invitationSendersOf(ctx, tx, readable)
	if err != nil {
		return err
	}
	seats, err := seatNamesFor(ctx, tx, append(loggingSeatsOf(page), slices.Collect(maps.Values(invitedBy))...))
	if err != nil {
		return err
	}
	reader := ids.Nil
	if actor, ok := principal.Actor(ctx); ok {
		reader = actor.UserID
	}
	var unresolved []unresolvedAuthor
	for i := range page {
		if !contentReadable(page[i]) {
			continue
		}
		author := authorOf(page[i], parties[ids.UUID(page[i].Id)], seats)
		if seat, ok := invitedBy[ids.UUID(page[i].Id)]; ok {
			author = seatAuthor(seat, seats)
		}
		if author != nil && author.UserId != nil && reader != ids.Nil && ids.UUID(*author.UserId) == reader {
			author.IsReader = true
		}
		if author != nil && author.Kind == crmcontracts.ActivityAuthorKindUnresolved && author.Address != nil {
			unresolved = append(unresolved, unresolvedAuthor{activity: ids.UUID(page[i].Id), author: author})
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

// identifiedFirst prefers a party a seat or contact identifies. Next comes one
// that states an address, and last a row the reader can name nothing from.
func identifiedFirst(parties []crmcontracts.EmailParty) crmcontracts.EmailParty {
	for _, p := range parties {
		if p.UserId != nil || p.ContactId != nil {
			return p
		}
	}
	for _, p := range parties {
		if p.Address != "" {
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

// invitationSendersOf is the seat capture recorded as the sender of each
// invitation on the page. It is a column no reading gate looks at, so naming
// the colleague never widens who may read the copy.
func invitationSendersOf(ctx context.Context, tx pgx.Tx, readable []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, invitation_sender FROM activity
		 WHERE id = ANY($1) AND invitation_sender IS NOT NULL`, readable)
	if err != nil {
		return nil, fmt.Errorf("activities: reading who sent the invitations of a page: %w", err)
	}
	senders, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Activity, Seat ids.UUID }])
	if err != nil {
		return nil, fmt.Errorf("activities: reading who sent the invitations of a page: %w", err)
	}
	out := make(map[ids.UUID]ids.UUID, len(senders))
	for _, sender := range senders {
		out[sender.Activity] = sender.Seat
	}
	return out, nil
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

// unresolvedAuthor is a sender stated only as an address, with its row.
type unresolvedAuthor struct {
	activity ids.UUID
	author   *crmcontracts.ActivityAuthor
}

// authorCandidate is one live contact that may be a row's sender, and whether
// the reader may see it.
type authorCandidate struct {
	Activity ids.UUID
	Contact  ids.UUID
	Name     *string
	Visible  bool
}

// resolveAuthorContacts names a sender stated as a bare address by its contact.
// The candidate is the From row's own contact when it names one, else every
// live contact holding the address. It is decided over all contacts, so a
// contact out of the reader's sight is never replaced by a visible namesake.
// The one candidate is named only when the reader may see it.
func resolveAuthorContacts(ctx context.Context, tx pgx.Tx, authors []unresolvedAuthor) error {
	if len(authors) == 0 || !auth.ReadGranted(ctx, "contact") {
		return nil
	}
	activityIDs := make([]ids.UUID, 0, len(authors))
	addresses := make([]string, 0, len(authors))
	for _, a := range authors {
		activityIDs = append(activityIDs, a.activity)
		addresses = append(addresses, correspondence.Fold(*a.author.Address))
	}
	args := []any{activityIDs, addresses}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return err
	}
	if scope == "" {
		scope = "true"
	}
	rows, err := tx.Query(ctx, `
		WITH wanted(activity_id, address) AS (SELECT * FROM unnest($1::uuid[], $2::text[])),
		stated AS (
		  SELECT DISTINCT ON (ap.activity_id) ap.activity_id, ap.contact_id
		    FROM activity_participant ap JOIN wanted w ON w.activity_id = ap.activity_id
		   WHERE ap.role = 'from' AND ap.contact_id IS NOT NULL
		   ORDER BY ap.activity_id, ap.created_at, ap.id)
		SELECT w.activity_id, cand.id, cand.full_name, cand.visible
		  FROM wanted w
		  LEFT JOIN stated s ON s.activity_id = w.activity_id
		 CROSS JOIN LATERAL (
		  SELECT p.id, p.full_name, coalesce(`+scope+`, false) AS visible
		    FROM contact p
		   WHERE p.archived_at IS NULL
		     AND CASE WHEN s.contact_id IS NOT NULL THEN p.id = s.contact_id
		              ELSE EXISTS (SELECT 1 FROM contact_email e
		                            WHERE e.contact_id = p.id AND e.archived_at IS NULL AND e.email = w.address) END
		   LIMIT 2) cand`, args...)
	if err != nil {
		return fmt.Errorf("activities: naming the senders of a page: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[authorCandidate])
	if err != nil {
		return fmt.Errorf("activities: naming the senders of a page: %w", err)
	}
	byActivity := make(map[ids.UUID][]authorCandidate, len(candidates))
	for _, c := range candidates {
		byActivity[c.Activity] = append(byActivity[c.Activity], c)
	}
	for _, a := range authors {
		nameAuthorContact(a.author, byActivity[a.activity])
	}
	return nil
}

// nameAuthorContact names the author by its one candidate contact, when the
// reader may see it. Two candidates, or one out of sight, leave the address.
func nameAuthorContact(author *crmcontracts.ActivityAuthor, candidates []authorCandidate) {
	if len(candidates) != 1 || !candidates[0].Visible {
		return
	}
	id := openapi_types.UUID(candidates[0].Contact)
	author.ContactId, author.Kind = &id, crmcontracts.ActivityAuthorKindContactRecord
	if name := candidates[0].Name; name != nil && *name != "" {
		author.DisplayName = name
	}
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
