// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who wrote an activity. Every surface that names a speaker reads this one
// statement, because direction is not authorship. A mail that came in was
// written by its From party, often not the contact whose page shows it.

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

// writerSeatExpr is the seat a row's own columns name as its writer. That is
// the colleague capture recorded as an invitation's sender, a meeting's host,
// or the seat that logged a note, call or task. It holds no ", ", because the
// projection's column list is checked by splitting on it.
const writerSeatExpr = `CASE
	WHEN a.invitation_sender IS NOT NULL THEN a.invitation_sender
	WHEN a.kind = 'meeting' THEN a.host_user_id
	WHEN a.kind IN ('note','call','task') AND a.captured_by ~ '^human:[0-9a-f-]{36}$'
	  THEN substr(a.captured_by,7)::uuid END`

// The writer seat and its current name and login, aliased so a select list
// that draws a column of the same name stays unambiguous. No liveness filter:
// who wrote a row in August is a fact about August.
const (
	writerSeatSQL      = `(` + writerSeatExpr + `) AS writer_seat`
	writerSeatNameSQL  = `(SELECT u.display_name FROM app_user u WHERE u.id = (` + writerSeatExpr + `)) AS writer_seat_name`
	writerSeatEmailSQL = `(SELECT u.email FROM app_user u WHERE u.id = (` + writerSeatExpr + `)) AS writer_seat_email`
)

// writerSeatVoice is the author the row's own columns name, before
// WithMessageVoices weighs it against the message's parties.
func writerSeatVoice(seat *ids.UUID, name, email *string) *crmcontracts.ActivityVoice {
	if seat == nil {
		return nil
	}
	id := openapi_types.UUID(*seat)
	author := &crmcontracts.ActivityAuthor{Kind: crmcontracts.ActivityAuthorKindColleague, UserId: &id}
	if name != nil && *name != "" {
		author.DisplayName = name
	}
	if email != nil && *email != "" {
		author.Address = email
	}
	return &crmcontracts.ActivityVoice{Author: author}
}

// WithMessageVoices fills `voice.author` on every readable row of a page.
// A withheld row gets no voice: who wrote it is content.
func WithMessageVoices(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	parties, err := partiesForVoices(ctx, tx, page)
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
			page[i].Voice = nil
			continue
		}
		author := authorOf(page[i], parties[ids.UUID(page[i].Id)])
		markReader(author, reader)
		if author != nil && author.Kind == crmcontracts.ActivityAuthorKindUnresolved && author.Address != nil {
			unresolved = append(unresolved, unresolvedAuthor{activity: ids.UUID(page[i].Id), author: author})
		}
		page[i].Voice = &crmcontracts.ActivityVoice{Author: author}
	}
	return resolveAuthorContacts(ctx, tx, unresolved)
}

// partiesForVoices reads the parties of the readable rows whose author is a
// party: messages and meetings. A page of notes and tasks reads nothing.
func partiesForVoices(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) (map[ids.UUID]emailParties, error) {
	var wanted []ids.UUID
	for i := range page {
		switch page[i].Kind {
		case crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage, crmcontracts.ActivityKindMeeting:
			if contentReadable(page[i]) {
				wanted = append(wanted, ids.UUID(page[i].Id))
			}
		}
	}
	if len(wanted) == 0 {
		return map[ids.UUID]emailParties{}, nil
	}
	return readEmailPartiesOf(ctx, tx, wanted)
}

// markReader flags an author who is the signed-in caller.
func markReader(author *crmcontracts.ActivityAuthor, reader ids.UUID) {
	if author != nil && author.UserId != nil && reader != ids.Nil && ids.UUID(*author.UserId) == reader {
		author.IsReader = true
	}
}

// authorOf is the author the recorded facts name, or nil when none does. It
// never infers a colleague from an address: only capture, which holds the
// own-domain and DMARC checks, may record a seat as a mail's sender.
func authorOf(a crmcontracts.Activity, parties emailParties) *crmcontracts.ActivityAuthor {
	recorded := recordedWriter(a)
	switch a.Kind {
	case crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage:
		if recorded != nil {
			return recorded
		}
		if len(parties.from) > 0 {
			return partyAuthor(identifiedFirst(parties.from), machineSent(a))
		}
		return importedAuthor(a.Author)
	case crmcontracts.ActivityKindMeeting:
		if len(parties.organizers) > 0 {
			return partyAuthor(identifiedFirst(parties.organizers), false)
		}
		if recorded != nil {
			return recorded
		}
		return importedAuthor(a.Author)
	case crmcontracts.ActivityKindNote, crmcontracts.ActivityKindCall, crmcontracts.ActivityKindTask:
		if imported := importedAuthor(a.Author); imported != nil {
			return imported
		}
		return recorded
	}
	return nil
}

// recordedWriter is the writer seat the projection read off the row's own
// columns (writerSeatVoice), copied so the page's voice owns its author.
func recordedWriter(a crmcontracts.Activity) *crmcontracts.ActivityAuthor {
	if a.Voice == nil || a.Voice.Author == nil || a.Voice.Author.Kind != crmcontracts.ActivityAuthorKindColleague {
		return nil
	}
	author := *a.Voice.Author
	return &author
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
// The one candidate is named only when the reader may see it. Candidate ids
// come from the From row or the address index, so no contact is scanned.
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
		scope = sqlTrue
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
		     AND p.id IN (SELECT s.contact_id WHERE s.contact_id IS NOT NULL
		                  UNION ALL
		                  SELECT e.contact_id FROM contact_email e
		                   WHERE s.contact_id IS NULL AND e.archived_at IS NULL AND e.email = w.address)
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
