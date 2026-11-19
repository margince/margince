// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The deliverable ADDRESS a reply goes to, as distinct from the name it greets.
//
// ReplyRecipientFor beside this answers a NAME, and a name is all a greeting
// needs — an activity with no resolvable contact yields an unnamed greeting and
// that is a correct draft. An address cannot degrade the same way: a send with
// no addressee is not a quieter send, it is a message with nowhere to go, so
// this refuses where the greeting shrugs.
//
// It exists because something has to compose a send with no human at the
// keyboard. Every other caller supplies `to` explicitly — the SPA prefills it,
// an agent reads a draft and passes the address back on the send call — but an
// automation drafts and stages in one pass with nobody in the middle to name
// the recipient, and a staged draft that hides who it will go to is not
// something a human can meaningfully approve.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NoReplyAddressError refuses to compose a reply that has nowhere to go.
//
// It is a 422 with a field fault rather than a 500: the activity is real and
// the caller is entitled to it, there is simply no counterparty address on it —
// a manually logged call, or a thread whose participants were never captured.
// An automation instance pointed at such a record is misconfigured, and the
// operator can act on that.
//
// Colleague marks the other way a thread can have nobody to answer: the only
// addressee left is on the workspace's own domain. The store cannot tell that
// (the own-domain set is capture's), so the caller that can sets it, and the
// refusal reads the same to everyone that handles this error.
type NoReplyAddressError struct {
	Colleague bool
}

func (e *NoReplyAddressError) Error() string {
	if e.Colleague {
		return "the message being replied to is with a colleague on the workspace's own domain; " +
			"a reply is composed only to a counterparty outside it"
	}
	return "the message being replied to records no counterparty address to answer; " +
		"a reply can only be composed for a thread that carries one"
}

// FieldFault names the field the caller has to correct.
func (e *NoReplyAddressError) FieldFault() (field, code, message string) {
	return "to", "no_reply_address", e.Error()
}

// ReplyAddressFor is the address ReplyAddresseeFor answers.
func (s *Store) ReplyAddressFor(ctx context.Context, id ids.ActivityID, colleague func(address string) bool) (string, error) {
	to, err := s.ReplyAddresseeFor(ctx, id, colleague)
	return to.Address, err
}

// ReplyAddressee is who a reply is delivered to. ContactID is empty unless this
// caller may read the contact behind the chosen address. A readable CC never
// stands in for an unreadable sender. A colleague skipped by domain lets the
// next counterparty be the addressee, with their own contact.
type ReplyAddressee struct {
	Address   string
	ContactID string
	// GreetingRegister is the register a rep stored on that contact, empty when
	// there is no readable contact or none was set. Read here so a draft's
	// register comes from the contact the mail is delivered to.
	GreetingRegister greetingregister.Register
}

// ReplyAddresseeFor answers the single address a reply to this activity is
// sent to, refusing when the thread carries none. It names the delivered-to
// contact beside it, and the reply's language is read from that contact's mail.
//
// WHOSE address, precisely. The counterparty's — never one of our own. That
// distinction is why this cannot simply reuse ReplyRecipientFor's ranking: on an
// OUTBOUND message the `from` participant is us, so a rank that puts `from`
// first would answer with our own address and cheerfully mail ourselves. A
// participant carrying user_id is one of this installation's own contacts, and is
// excluded here for exactly that reason.
//
// Among what remains the rank is the same shape as the greeting's, and for the
// same reason: on an inbound message the sender is who we are answering, and on
// our own outbound the addressee is. Then anyone else who was on it.
//
// One address, not a list. A group thread is the same shape ReplyRecipientFor
// declines to model, and answering all of them is a different product decision
// from answering the counterparty — this addresses the most likely one and a
// human sees it before anything sends.
//
// colleague names the addresses that are ours by domain rather than by seat —
// a co-worker without a login. The candidates are walked in rank order and
// the first one that is not a colleague is the answer, so a stand-up with a
// colleague AND a guest is answered to the guest whatever the header order.
// A nil predicate treats nobody as a colleague. When every candidate is one,
// the refusal says so.
func (s *Store) ReplyAddresseeFor(ctx context.Context, id ids.ActivityID, colleague func(address string) bool) (ReplyAddressee, error) {
	delivery, err := s.deliveryOf(ctx, id, colleague)
	return delivery.ReplyAddressee, err
}

// delivery is the one resolution of who a reply goes to. Counterparty is the
// readable contact of the first participant who is an outsider. It is set even
// when they have no address, so a greeting can name a contact whose address is
// retired.
type delivery struct {
	ReplyAddressee
	Counterparty string
}

// deliveryOf resolves the delivered-to participant once. Colleagues, by seat or
// by the own-domain predicate, are skipped the same way for the address, the
// name and the register.
func (s *Store) deliveryOf(ctx context.Context, id ids.ActivityID, colleague func(address string) bool) (delivery, error) {
	// Reaching a contact's address is a contact read, exactly as reaching their
	// name is: a caller who may read activities but not contacts must not be told
	// through this door what the contacts surface withholds.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return delivery{}, err
	}

	var to delivery
	var noAddress error
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The link-walk scope decides whether this caller may see the anchor at
		// all. Reading a counterparty off an activity they cannot reach would
		// answer an address their own scope withholds.
		if _, err := readActivityContent(ctx, tx, id, storekit.LiveOnly); err != nil {
			return err
		}

		// Two sources, ranked together rather than tried in sequence. The
		// participant's own address is preferred over the contact's primary
		// email because it is the address that actually corresponded. The
		// contact's primary email is the fallback for a participant recorded by
		// identity alone.
		top, err := firstCounterparty(ctx, tx, id, colleague)
		if err != nil {
			return err
		}
		q, args, err := counterpartyCandidates(ctx, id)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("resolve the address a reply answers: %w", err)
		}
		defer rows.Close()
		to.ReplyAddressee, err = firstOutside(rows, colleague, top.participant)
		rows.Close()
		to.Counterparty, to.GreetingRegister = top.contact, top.register
		// No address to answer is an outcome to report beside the counterparty,
		// not a failure that discards who they are.
		if _, none := errors.AsType[*NoReplyAddressError](err); none {
			noAddress, err = err, nil
		}
		return err
	})
	if err != nil {
		return delivery{}, err
	}
	return to, noAddress
}

// counterpartyCTE lists the participants a reply could go to: not our own seats,
// and only rows that stand as a presence on the message.
func counterpartyCTE() string {
	return `
		WITH counterparty AS (
		     SELECT ap.*, CASE ap.role WHEN 'from' THEN 1 WHEN 'to' THEN 2 ELSE 3 END AS rank
		       FROM activity_participant ap
		       JOIN activity a ON a.id = ap.activity_id
		      WHERE ap.activity_id = $1
		        AND ap.user_id IS NULL
		        AND ` + auth.ParticipantStandsSQL("ap", "a") + `
		)`
}

// counterpartyCandidates ranks the address candidates of an activity's
// counterparties. The contact row scope is applied here to every arm that reads
// a contact. The tiebreak columns carry both levels: the participant arm ranks
// by which participant, and the contact-email arm separates one contact's
// addresses by is_primary and position.
func counterpartyCandidates(ctx context.Context, id ids.ActivityID) (string, []any, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return "", nil, err
	}
	args := []any{id}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", func(v any) int {
		args = append(args, v)
		return len(args)
	})
	if err != nil {
		return "", nil, err
	}
	readable := scope
	if readable == "" {
		readable = sqlTrue
	}
	q := counterpartyCTE() + `
		SELECT addr, addressee, participant FROM (
		     SELECT c.address AS addr, CASE WHEN EXISTS (
		                SELECT 1 FROM contact p
		                 WHERE p.id = c.contact_id AND p.archived_at IS NULL AND (` + readable + `))
		            THEN c.contact_id END AS addressee, c.id AS participant, c.rank, 1 AS source,
		            true AS primary_first, 0 AS position, c.created_at, c.id
		       FROM counterparty c
		      WHERE c.address IS NOT NULL AND c.address <> ''
		     UNION ALL
		     SELECT e.email AS addr, c.contact_id, c.id AS participant, c.rank, 2 AS source,
		            e.is_primary AS primary_first, e.position, c.created_at, c.id
		       FROM counterparty c
		       JOIN contact p ON p.id = c.contact_id
		       JOIN contact_email e ON e.contact_id = p.id AND e.archived_at IS NULL
		      WHERE c.contact_id IS NOT NULL
		        AND p.archived_at IS NULL`
	if scope != "" {
		q += ` AND (` + scope + `)`
	}
	q += `
		) ranked
		 ORDER BY rank, source, primary_first DESC, position, created_at, id, addr`
	return q, args, nil
}

// counterpartyHead is the first participant that is not a colleague.
type counterpartyHead struct {
	participant ids.UUID
	contact     string
	register    greetingregister.Register
}

// firstCounterparty finds the first participant, in rank order, that is not a
// colleague. A participant with no address cannot be judged a colleague, so a
// copied contact never stands in for them. contact and register are that
// participant's own when this caller may read the contact.
func firstCounterparty(ctx context.Context, tx pgx.Tx, id ids.ActivityID, colleague func(address string) bool) (counterpartyHead, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return counterpartyHead{}, err
	}
	args := []any{id}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "r", func(v any) int {
		args = append(args, v)
		return len(args)
	})
	if err != nil {
		return counterpartyHead{}, err
	}
	readable := scope
	if readable == "" {
		readable = sqlTrue
	}
	rows, err := tx.Query(ctx, counterpartyCTE()+`
		SELECT c.id, coalesce(c.address, ''), r.id, coalesce(r.greeting_register, '')
		  FROM counterparty c
		  LEFT JOIN contact r ON r.id = c.contact_id AND r.archived_at IS NULL AND (`+readable+`)
		 ORDER BY c.rank, c.created_at, c.id`, args...)
	if err != nil {
		return counterpartyHead{}, fmt.Errorf("resolve who a reply is delivered to: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var head counterpartyHead
		var contact *ids.UUID
		var address string
		if err := rows.Scan(&head.participant, &address, &contact, &head.register); err != nil {
			return counterpartyHead{}, fmt.Errorf("resolve who a reply is delivered to: %w", err)
		}
		if strings.TrimSpace(address) != "" && colleague != nil && colleague(address) {
			continue
		}
		if contact != nil {
			head.contact = contact.String()
		} else {
			head.register = ""
		}
		return head, rows.Err()
	}
	return counterpartyHead{}, rows.Err()
}

// firstOutside walks the ranked candidates and answers the first that is not
// a colleague. No candidate at all is the plain refusal; candidates that are
// all colleagues is the refusal that names why.
func firstOutside(rows pgx.Rows, colleague func(address string) bool, head ids.UUID) (ReplyAddressee, error) {
	sawColleague := false
	for rows.Next() {
		var address string
		var contact *ids.UUID
		var participant ids.UUID
		if err := rows.Scan(&address, &contact, &participant); err != nil {
			return ReplyAddressee{}, fmt.Errorf("resolve the address a reply answers: %w", err)
		}
		// A row that satisfies the predicate with whitespace is no addressee:
		// it would reach the send as "no recipients" rather than as the honest
		// answer that this thread has no counterparty to answer.
		if strings.TrimSpace(address) == "" {
			continue
		}
		if colleague != nil && colleague(address) {
			sawColleague = true
			continue
		}
		to := ReplyAddressee{Address: address}
		if contact != nil && participant == head {
			to.ContactID = contact.String()
		}
		return to, nil
	}
	if err := rows.Err(); err != nil {
		return ReplyAddressee{}, fmt.Errorf("resolve the address a reply answers: %w", err)
	}
	return ReplyAddressee{}, &NoReplyAddressError{Colleague: sawColleague}
}
