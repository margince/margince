// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who is waiting for a reply.
//
// The deal page answers this for ONE deal by walking its timeline newest-first.
// This asks the whole workspace at once and cannot be the same walk: a per-deal
// scan cannot find the contact with no deal, nor run once per record on a page
// that must render in one read. A test feeds both the same timeline and requires
// the same answer.
//
// ITS OWN READ rather than a filter over at-risk deals: a fresh inbound makes a
// deal LESS quiet, so it leaves the quiet-deal candidate set exactly when
// somebody starts waiting on it. Deriving "waiting" from "quiet" would lose the
// newest and most urgent cases.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WaitingReply is one inbound message nobody has answered.
type WaitingReply struct {
	// ActivityID is the message itself — what a draft would reply to.
	ActivityID ids.UUID
	// Kind tells an email from a channel message. The query spans both, and
	// only an email has an email's shape: a caller drawing the canonical email
	// row for a chat would put a mail icon on a message that never travelled
	// on one.
	DirectRecipients []ids.UUID
	Kind             string
	Subject          string
	// Sender is the address the message came from, so a caller can tell a
	// contact waiting from a machine sending. Empty when no sender was recorded.
	Sender string
	// OccurredAt is when they wrote, which is what the wait is measured from.
	OccurredAt time.Time
	// The record the thread is filed under, when it names one.
	ContactID ids.UUID
	CompanyID ids.UUID
	DealID    ids.UUID
	// HasOpenDeal reports whether an open deal is on this thread. Inside the
	// horizon it is what lets a caller keep a stale wait with money behind it
	// above routine work; past the horizon no wait survives on it.
	//
	// Read through the SAME visibility-gated links as the record ids above, so
	// it means "an open deal this reader can see" rather than "an open deal
	// exists". The looser reading would let somebody learn a deal is there by
	// watching a row they can see decline to go stale.
	HasOpenDeal bool
	// OwedVerdict is the classifier's judgement of whether this message asks
	// its recipient side for something: OwedVerdictAsksUs, OwedVerdictInformsUs
	// or empty for unjudged.
	//
	// Empty is not a third verdict. An unjudged message ranks exactly as it did
	// before the column existed, because a classifier that has not run, has run
	// out of budget or answered below its confidence floor must not change what
	// a rep sees.
	OwedVerdict  string
	CaptureLabel string
	// Threaded reports that this message belongs to a conversation.
	//
	// Not every captured message does: a first contact from an address nobody
	// has written to, or a provider that hands over no chain to root on,
	// arrives with no thread_key at all and still reaches this queue.
	//
	// It travels because two of the three judgements a rep may make about a
	// waiting row are keyed on the thread, and a row without one can perform
	// neither. Deciding that here, where the column is read, rather than
	// letting the caller infer it from a record id that is absent for other
	// reasons too.
	Threaded bool
	// ThreadKey is the conversation the message belongs to, or empty. With Kind
	// and ChannelProvider it names the conversation, as the thread walks do.
	ThreadKey       string
	ChannelProvider string
	// EarlierRequests counts the conversation's earlier unanswered requests a
	// caller folded into this row, and FirstAskedAt is when the first of them
	// arrived. Zero when nothing was folded.
	EarlierRequests int
	FirstAskedAt    time.Time
	// MeetingBookedAt is when the soonest booked meeting with this message's
	// sender starts; nil when none is booked OR when none is discoverable to
	// THIS reader — the column is fenced by the reader's own discover gate, so
	// it is what this reader may be shown, never a global booking fact. A
	// booked meeting does not settle the wait — only a held one does — but the
	// caller stops counting waiting days against work that is already
	// scheduled. bookedMeetingSQL holds the rule.
	MeetingBookedAt *time.Time
	// Engaged reports that this workspace wrote on this thread BEFORE the
	// message arrived — the evidence that a conversation is one we are already
	// in, rather than one that merely reached a mailbox.
	//
	// Reported, never used to hide. Thread identity comes from the reply
	// headers, and a client that strips them gives every message its own
	// thread: an exclusion keyed on this would drop a live customer silently,
	// which is the one failure this queue must not have. A caller demotes an
	// unengaged wait instead, so being wrong costs a scroll.
	Engaged bool
	// AddressedElsewhere reports that the message names header recipients and
	// none of them is this reader — mail written to a colleague that reached
	// this mailbox.
	//
	// Phrased as the exception so FALSE is the answer whenever there is no
	// evidence: no header recipients recorded, or the reader's own addresses
	// unresolved. REPORTED, never used to exclude, for Engaged's reason — the
	// caller demotes what it cannot prove.
	AddressedElsewhere bool
	// OwnerID is the sole verified direct recipient for email. A linked
	// record's owner does not establish responsibility for a reply. Zero means
	// non-email or no sole direct recipient.
	OwnerID ids.UUID
	// OwnerKnown is true after recipient resolution, including no duty.
	OwnerKnown bool
	// SenderWasStranger, BccOnly and ForeignLink are what capture judged about
	// the mail (strangerMailSQL); false where it judged nothing. BccOnly is
	// this reader's own import, so a system reader never sees it.
	SenderWasStranger bool
	BccOnly           bool
	ForeignLink       bool
	// SenderIsMachine is the message's sender_is_machine stamp: a sending
	// system wrote it, by mailidentity.InboundFromSendingSystem.
	SenderIsMachine bool
}

// WaitingScanCap bounds the work one read does. Beyond this the answer is
// "there are more", never a silent truncation reported as a total.
//
// Exported because the caller has to compare against it. The seam filters what
// this read returns — machine senders out, duplicate threads folded — so only a
// comparison against the ROWS THIS RETURNED can tell a truncated scan from a
// complete one, and the number doing the comparing has to be this one rather
// than a copy that can drift from it.
const WaitingScanCap = 200

// waitingHorizonDays is how far back a wait can reach and still be work.
//
// Past this, an unanswered message is history rather than an obligation. Coarse
// on purpose: the bands separating an urgent wait from a stale one are the
// caller's.
//
// No thread is exempt, an open deal included: only a request a human holds
// outlives the horizon (heldRequestSQL). A deal gone quiet is the risk lane's
// to raise, not the reply queue's.
//
// Applied BEFORE the cap, which is the whole shape of this query: a filter after
// LIMIT lets two hundred rows nobody wants fill the scan and push a real
// customer past it, and the page then says nobody is waiting.
const waitingHorizonDays = storekit.DefaultWaitingHorizonDays

// What "still live" means, per record type, as one spelling each.
//
// Both predicates take a table alias, because every reader needs them under a
// different one. They exist as constants because this file needed each rule
// twice and lasttouch.go states the same two in its own dispatch: four copies
// of "a deal is open" in one package is four places to edit when archiving
// changes, and nothing fails when the fourth is missed.
//
// The lead list is the WORKING part of the lifecycle. A promoted or
// disqualified lead is finished business, and a wait on one is history rather
// than work.
const (
	openDealPredicate    = `%[1]s.status = 'open' AND %[1]s.archived_at IS NULL`
	workingLeadPredicate = `%[1]s.status IN ('new', 'contacted', 'engaged') AND %[1]s.archived_at IS NULL`
)

// neverRelaxed is the relaxation predicate an ordinary caller passes: a literal
// FALSE, so the clause it fronts decides the row exactly as it did before the
// hole existed.
//
// A named constant rather than `"FALSE"` typed at each call site, because a
// caller that reached for `"false"` or `"0"` would compile and would still be
// wrong — and what a reader of that call site needs is the WORD for what it
// means, not the value.
const neverRelaxed = "FALSE"

// liveRecord renders one of the predicates above under a caller's alias.
func liveRecord(predicate, alias string) string {
	return fmt.Sprintf(predicate, alias)
}

// Two of the holes are RELAXATIONS, both defaulting to off. %[12]s and %[13]s
// widen the not_sales judgement and the sales-link requirement by OR-ing a
// caller-supplied predicate in front of each clause rather than removing it.
// Every ordinary caller passes `neverRelaxed`, so the clause is unreachable and
// Postgres plans it away.
//
// They exist for the hidden-backlog guardrail, which asks what each hiding rule
// keeps off the queue — a question only the query owning the OTHER rules can
// answer. A second statement restating the anti-joins and the live-record
// predicates would be a second answer to "is this contact waiting".
//
// Sprintf'd directly at ALL call sites rather than through a wrapper, which over
// this many positional arguments would be the same call once removed with a
// second parameter order to keep in sync. What must not fork is the SQL TEXT,
// and sharing one constant holds that.

// WaitingReplies answers who is waiting on this reader for a reply.
//
// One row per thread — the newest inbound in it — because a customer who wrote
// three times is waiting once, and three rows would read as three obligations.
// Oldest first: the longest wait is the one most likely to have been forgotten.
//
// One PAGE of them, bounded by the scan cap. Read the whole backlog with
// WaitingRepliesBefore, which continues where a page left off; this spelling
// asks for the newest page and stops.
//
// Paged because the caller folds conversations and drops sending systems
// after this cap. The next page is how a customer comes back.
//
// The machine-sender rule stays one Go capability. Capture and the activity
// writer stamp its verdict on sender_is_machine, and a data task stamps older
// mail. The card tail (waitingcards.go) drops stamped mail before its fold;
// the caller asks the rule itself of every row, so unstamped mail is judged too.
func (s *Store) WaitingReplies(ctx context.Context, asOf time.Time) ([]WaitingReply, error) {
	return s.WaitingRepliesBefore(ctx, asOf, WaitingPosition{})
}

// WaitingRepliesBefore is WaitingReplies continued: the next page of the same
// scan, after the timestamp and identity given.
//
// One implementation, two names. The unpaged spelling is what almost every
// caller wants — one page is two hundred rows and more than a queue shows —
// and it says so by taking no cursor at all, rather than every call site
// carrying a zero somebody has to recognise as "the beginning".
func (s *Store) WaitingRepliesBefore(ctx context.Context, asOf time.Time, before WaitingPosition) ([]WaitingReply, error) {
	return s.scanWaitingReplies(ctx, asOf, before, mailSeats{})
}

// WaitingRepliesForOwnerBefore narrows mail responsibility before the scan bound.
func (s *Store) WaitingRepliesForOwnerBefore(ctx context.Context, asOf time.Time, before WaitingPosition, owner ids.UUID) ([]WaitingReply, error) {
	return s.scanWaitingReplies(ctx, asOf, before, mailSeats{owner: &owner})
}

// WaitingRepliesAnsweredByBefore is WaitingRepliesBefore kept, before the scan
// bound, to the mail one of these seats answers for. A message that is not
// mail answers to nobody and stays, as it does in the unnarrowed read.
func (s *Store) WaitingRepliesAnsweredByBefore(ctx context.Context, asOf time.Time, before WaitingPosition, seats []ids.UUID) ([]WaitingReply, error) {
	return s.scanWaitingReplies(ctx, asOf, before, mailSeats{answeredBy: seats, bySeats: true})
}

func (s *Store) scanWaitingReplies(ctx context.Context, asOf time.Time, before WaitingPosition, seats mailSeats) ([]WaitingReply, error) {
	var waiting []WaitingReply
	err := s.readWaitingQueue(ctx, asOf, seats, waitingShape{drawsRows: true},
		func(slots waitingSlots, arg func(any) int) string {
			slots.values[waitingKeysetSlot] = olderThan(before, arg)
			return "SELECT waiting.*, " + directMailSeatsSQL(slots.identities) + " FROM (" + fmt.Sprintf(waitingRepliesSQL, slots.values...) +
				") waiting JOIN activity a ON a.id = waiting.id ORDER BY waiting.occurred_at DESC, waiting.id DESC"
		},
		func(rows pgx.Rows) (err error) {
			waiting, err = collectWaiting(rows, seats.owner)
			return err
		})
	if err != nil {
		return nil, fmt.Errorf("activities: reading who is waiting for a reply: %w", err)
	}
	return waiting, nil
}

// readWaitingQueue sends one statement over the waiting queue in its own
// transaction.
func (s *Store) readWaitingQueue(
	ctx context.Context, asOf time.Time, seats mailSeats, shape waitingShape,
	statement func(waitingSlots, func(any) int) string, read func(pgx.Rows) error,
) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.queryWaitingQueue(ctx, tx, asOf, seats, shape, statement, read)
	})
}

// queryWaitingQueue sends one statement over the waiting queue: the page or a
// card tail read. Each gates every row it judges, so the statement stops at
// its own LIMIT instead of judging the whole horizon first.
func (s *Store) queryWaitingQueue(
	ctx context.Context, tx pgx.Tx, asOf time.Time, seats mailSeats, shape waitingShape,
	statement func(waitingSlots, func(any) int) string, read func(pgx.Rows) error,
) error {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	slots, err := s.waitingQueueSlots(ctx, tx, asOf, seats, shape, arg)
	if err != nil {
		return err
	}
	return perBind(ctx, true, func(ctx context.Context) error {
		rows, err := tx.Query(ctx, statement(slots, arg), planEachBind(args)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return read(rows)
	})
}

// OwnDomains reports the email domains this installation's own contacts write
// from — the set a message's sender is tested against to tell a colleague from
// a customer.
//
// A seam rather than a query because the domains are capture's to define, and a
// module may not read a sibling's tables. It returns DATA the queue tests in
// SQL, not a Go predicate: the test has to run before the scan cap, or two
// hundred colleague threads fill the scan and push a real customer past it.
//
// Read inside the CALLER's transaction, so the strict read and every relaxed
// read beside it see one snapshot — otherwise the set could change between two
// counts meant to differ by exactly one rule.
type OwnDomains interface {
	Domains(ctx context.Context, tx pgx.Tx) ([]string, error)
	MailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error)
	// ReaderAddresses supplies private self identity for the addressing annotation.
	ReaderAddresses(ctx context.Context, tx pgx.Tx, reader ids.UUID) ([]string, error)
}

// WithOwnDomains wires the colleague-domain reader the waiting queue needs.
//
// Unbound, the queue admits every sender: a deployment that cannot say which
// domains are its own must not guess, and the honest failure is a queue with
// colleagues in it rather than one that silently drops a customer whose domain
// resembles ours.
func (s *Store) WithOwnDomains(own OwnDomains) *Store {
	s.ownDomains = own
	return s
}

// WithOwnDomains on Handlers wires the same reader the store needs, so the
// timeline's `waiting_reply=true` filter answers with the queue's rule rather
// than a looser one.
func (h Handlers) WithOwnDomains(own OwnDomains) Handlers {
	h.store = h.store.WithOwnDomains(own)
	return h
}

// ownDomainList reads the colleague domains for one query, or none when no
// reader is wired.
// readerAddressList is the reader's own addresses, or none when no seam is
// wired — which admits every message, the same failure direction ownDomainList
// takes and for the same reason: a reader whose addresses cannot be resolved
// must not have their real waiting mail silently demoted.
func (s *Store) readerAddressList(ctx context.Context, tx pgx.Tx, reader ids.UUID) ([]string, error) {
	if s.ownDomains == nil {
		return nil, nil
	}
	addresses, err := s.ownDomains.ReaderAddresses(ctx, tx, reader)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the reader's own addresses: %w", err)
	}
	return addresses, nil
}

func (s *Store) ownDomainList(ctx context.Context, tx pgx.Tx) ([]string, error) {
	if s.ownDomains == nil {
		return nil, nil
	}
	domains, err := s.ownDomains.Domains(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which domains are our own: %w", err)
	}
	return domains, nil
}

// ownDomainSenderSQL is "this message came from one of our own domains" under
// a caller's activity alias, with the placeholder holding Set's folded domains.
// The waiting queue and the response reading share it so they agree on who is
// waiting. It compares the sender's folded domain, never the address, so it
// agrees with the reply path's owndomain.Set.Covers. An unfolded row stays in
// the queue as an outside sender.
//
// Held by: TestEveryOwnDomainComparisonInSQLReadsFoldedDomains
// (backend/gates/owndomainpredicate_test.go)
func ownDomainSenderSQL(alias string, domainArg int) string {
	return fmt.Sprintf(`EXISTS (
	         SELECT 1 FROM activity_participant ours
	          WHERE ours.activity_id = %[1]s.id
	            AND ours.role = 'from'
	            AND EXISTS (
	                  SELECT 1 FROM unnest($%[2]d::text[]) AS own(domain)
	                   WHERE %[3]s))`, alias, domainArg, owndomain.CoversSQL("ours.address_domain_folded", "own.domain"))
}

func scanWaitingReply(record pgx.Row) (WaitingReply, error) {
	var row WaitingReply
	err := record.Scan(&row.ActivityID, &row.Kind, &row.Subject, &row.Sender, &row.OccurredAt,
		&row.ContactID, &row.CompanyID, &row.DealID,
		&row.HasOpenDeal, &row.OwedVerdict, &row.CaptureLabel, &row.AddressedElsewhere,
		&row.Engaged, &row.OwnerID, &row.OwnerKnown, &row.Threaded, &row.ThreadKey, &row.ChannelProvider,
		&row.MeetingBookedAt, &row.SenderWasStranger, &row.BccOnly, &row.ForeignLink, &row.SenderIsMachine)
	if !row.OwnerKnown {
		row.OwnerID = ids.Nil
	}
	return row, err
}

// collectWaiting reads the scanned waits for this owner, leaving out a mail
// addressed to more than one seat, which is nobody's automatic duty.
func collectWaiting(rows pgx.Rows, owner *ids.UUID) ([]WaitingReply, error) {
	waiting := []WaitingReply{}
	for rows.Next() {
		row, err := scanWaitingRecipients(rows)
		if err != nil {
			return nil, err
		}
		if ambiguouslyAddressed(row) {
			continue
		}
		waiting = append(waiting, waitingReplyForOwner(row, owner))
	}
	return waiting, rows.Err()
}
