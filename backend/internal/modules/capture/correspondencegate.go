// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Does this workspace CORRESPOND with an address.
//
// Its own file because it is its own question and it has one answer for several
// callers: the sink's admission gates next door ask it, and so does the verdict
// engine, where the answer calls off an effect that hides and destroys mail. A
// second spelling of "corresponded" would be two answers to the question that
// decides whether a counterparty's mail survives.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// correspondencePositiveTx reports whether the workspace has ever sent mail to
// email — the T1 evidence (ADR-0072 §1). It reads only
// `counterparty_outbound_attested` and never `direction`: direction is derived
// by comparing the forgeable From header against the owner, so honoring it here
// would let a spoofed From:owner message delivered to the inbox whitelist any
// address it names past the T2 suppression gate.
//
// Two writers set that column, and both are unforgeable statements that THIS
// installation sent the message: a connector attesting the mailbox owner's own
// sent copy, and the governed send path itself (activities.SendEmail), whose
// outbound row IS the sent copy — the provider's echo of it upserts onto the
// same natural key and writes nothing, so the evidence has to be stamped at
// send or it is never stamped at all.
//
// A single cold inbound is NOT correspondence — receiving mail is not intent.
// The first outbound message to an address counts immediately: writing to
// someone is affirmative intent toward them, and it is the message being
// captured right now that supplies it (the activity commits before this runs).
//
// One shape is excluded, and it is narrow on purpose: a SINGLE outbound whose
// own text declines. A founder answering unsolicited mail with "not interested"
// produced exactly one attested outbound, and that was enough to admit the
// spammer ahead of every suppression rule — the record for "PE Insights" in a
// real import came from precisely that reply. Declining is the one thing a
// contact writes that means the opposite of intent.
//
// Everything else still counts on sight. A reply that engages — a question, a
// price, a meeting — is intent no matter that it answered rather than opened,
// and so is any second outbound. The test is the WORDS, not the direction or
// the order: a rule that demoted every reply would have refused a prospect who
// wrote first and got answered, which is the most ordinary shape there is.
func correspondencePositiveTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	// Body and subject travel SEPARATELY because only the body carries a quoted
	// thread. Concatenating them first would leave no way to strip the sender's
	// own words out of our reply.
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(subject, ''), COALESCE(body, '')
		  FROM activity
		 WHERE counterparty_email = $1 AND counterparty_outbound_attested
		   AND `+auth.ActivityAvailableClause("activity")+`
		 LIMIT 2`, normalized)
	if err != nil {
		return false, fmt.Errorf("capture: correspondence-positive gate: %w", err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var subject, body string
		if err := rows.Scan(&subject, &body); err != nil {
			return false, fmt.Errorf("capture: correspondence-positive gate: %w", err)
		}
		// Only what the SENDER of this outbound message wrote. A stored body
		// keeps the quoted thread beneath the reply (mailmap caps it at 8000
		// runes, it does not strip it), so a spammer who writes "not interested"
		// in their own mail would otherwise put those words into our reply the
		// moment somebody hits Reply — and talk themselves out of the CRM by
		// feeding this gate a decline the mailbox owner never wrote.
		texts = append(texts, subject+" "+textlang.NewTextOnly(body))
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("capture: correspondence-positive gate: %w", err)
	}
	switch len(texts) {
	case 0:
		return false, nil
	case 1:
		return !isDecliningReply(texts[0]), nil
	default:
		// The LIMIT 2 above is why this is safe on a high-volume address: the
		// query stops at the only distinction that matters, one outbound versus
		// more than one, and never loads a whole correspondence to count it.
		// Two or more outbound messages are a correspondence whatever any one of
		// them says; nobody declines twice and keeps writing.
		return true, nil
	}
}

// isDecliningReply reports whether an outbound message is a refusal rather than
// engagement. It reads the message's own words; the LLM verdict that follows
// reads the whole thread and has the final say.
func isDecliningReply(text string) bool {
	// Whitespace is collapsed before matching: a mail client wraps lines where
	// it likes, so "not\ninterested" is the same sentence as "not interested"
	// and a matcher that missed it would be defeated by the window width.
	lowered := strings.Join(strings.Fields(strings.ToLower(text)), " ")
	for _, phrase := range declinePhrases {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

// CorrespondsWith reports whether this workspace has provably written to an
// address — the T1 signal, read back on the verdict side.
//
// The verdict engine needs it because a `newsletter` or `spam` answer suppresses
// the sender's whole DOMAIN, and that effect is workspace-wide and standing.
// While only unjudged strangers reached the ledger this could not misfire; once
// a sender the workspace corresponds with can be asked about, an answer of
// `newsletter` about one marketing blast would refuse a company the business
// actively works with.
//
// It delegates to the ladder's own T1 gate rather than asking the simpler
// question this effect first asked. The two must agree, and they differ in a
// case that matters here: a SINGLE outbound whose text declines ("not
// interested") is not correspondence. A spammer who drew that one reply would
// otherwise be spared the domain suppression by the very message telling them
// to stop.
func (s *PendingStore) CorrespondsWith(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	// Taken BEFORE the read, because the caller acts on the answer inside this
	// same transaction: the verdict reads `no` and then archives the sender's
	// mail. An attested outbound insert committing in between is a workspace
	// that has just written to them, and under READ COMMITTED the archive would
	// never see it — so the two serialize on one key rather than on luck. The
	// writers take it in capture's sink and in the activities store.
	if err := storekit.LockWriteIdentity(ctx, tx,
		correspondence.LockEntity, correspondence.LockIdentity(email)); err != nil {
		return false, err
	}
	return correspondencePositiveTx(ctx, tx, email)
}

// wroteBackTx reports whether this address has written to the workspace on a
// thread the workspace itself started.
//
// It is the other half of "a correspondence", and the stronger half. One
// outbound message is intent, and intent is often unreturned: a founder mails
// forty contacts about a conference and hears from six. Recording all forty as
// contacts on the strength of the send alone is what filled a CRM with contacts
// who never answered — and among them the test addresses, the one-off errands
// and the introductions that went nowhere.
//
// A REPLY is different in kind. Somebody read the message and chose to answer,
// which is the thing a contact record is actually about.
//
// Bulk mail does not count. An address the workspace wrote to once and which
// then sent a newsletter has not written back; it has added the workspace to a
// list, and reading that as a reply would admit exactly the senders the
// transactional gates exist to refuse.
//
// The outbound leg has to be mail THIS WORKSPACE SENT TO THIS ADDRESS, before
// the reply, on the same medium. Every one of those four is load-bearing, and
// thread_key is why: it is the message's own References root, so a sender
// chooses it verbatim (sinkreply.go says the same thing about the same column).
//
//   - the same address, or a stranger forging the root of a thread the owner
//     wrote on to somebody else manufactures an exchange out of a colleague's
//     correspondence;
//   - the same medium, because thread_key is one flat namespace holding both a
//     mail root and a channel's `<provider>:<bot>:<chat>` key, and a forged
//     References naming a discoverable chat id would cross between them;
//   - attested, which is the provider's own filing and not a header.
//
// It deliberately does NOT order the two in time. The intuition is that an
// outbound sent AFTER a cold email should not turn that email into an answer —
// but occurred_at is the sender's own Date header on the inbound side, so
// ordering on it would let a sender post-date their mail past our reply and
// defeat the very rule it was meant to add. What is actually being asked is
// whether these two parties have a conversation on one thread, and a thread the
// workspace wrote on to this address is that whichever leg landed first.
//
// Two neighbouring guards refuse the forged case as well today — the
// correspondence rung has no attested send to that address, and the ledger has
// already opened a question about them — so no test through the ladder can
// isolate this clause. It stays because it is the only one of the three that is
// ABOUT this question, and the other two are holding their own invariants.
//
// An inbound message on a thread nobody here wrote on is a stranger's first
// approach, which is the ambiguous class the verdict engine owns — counting it
// would restore create-on-sight for every cold email.
func wroteBackTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	var replied bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1
		    FROM activity inbound
		   WHERE inbound.counterparty_email = $1
		     AND inbound.direction = 'inbound'
		     AND NOT inbound.bulk_mail_attested
		     AND inbound.thread_key <> ''
		     AND `+auth.ActivityAvailableClause("inbound")+`
		     AND EXISTS (
		           SELECT 1
		             FROM activity ours
		            WHERE ours.thread_key = inbound.thread_key
		              AND ours.counterparty_email = $1
		              AND ours.kind = inbound.kind
		              AND ours.counterparty_outbound_attested
		              AND `+auth.ActivityAvailableClause("ours")+`))`,
		normalized).Scan(&replied); err != nil {
		return false, fmt.Errorf("capture: reading whether the address wrote back: %w", err)
	}
	return replied, nil
}

// wroteOnTwoThreadsTx reports whether this workspace has written to the address
// on two SEPARATE threads.
//
// Nobody starts a second conversation with somebody by accident, which is what
// makes this evidence of a relationship without needing an answer. It counts
// distinct threads rather than messages: two mails on one thread are one
// conversation — a send and its own follow-up — and reading them as two would
// admit exactly the unreturned intent this rule exists to refuse.
//
// A message with no thread key is not counted at all rather than counted as its
// own thread. Mail always has one (its own Message-ID roots it when nothing
// else does), so a blank key is a record from somewhere else, and guessing it
// apart from its neighbours would invent a second conversation.
func wroteOnTwoThreadsTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	var threads int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM (
		  SELECT DISTINCT thread_key
		    FROM activity
		   WHERE counterparty_email = $1
		     AND counterparty_outbound_attested
		     AND thread_key <> ''
		     AND `+auth.ActivityAvailableClause("activity")+`
		   LIMIT 2) AS distinct_threads`, normalized).Scan(&threads); err != nil {
		return false, fmt.Errorf("capture: counting the threads this workspace wrote on: %w", err)
	}
	return threads >= 2, nil
}

// metInPersonTx reports whether this workspace and that address share a meeting
// a connector captured.
//
// It is the third shape of "we have actually dealt with this contact", beside a
// reply and two outbound threads, and it is the strongest of them. Mail is
// evidence about intent — a founder mails forty contacts and hears from six — and
// a meeting is evidence about time: both sides put an hour in a calendar, which
// nobody does by accident.
//
// The ladder could not see one, and the shape of the miss was worse than a gap.
// A calendar invitation reaches the mailbox as machine-generated mail, so the
// classifier read it as `transactional` and judged the INVITED PARTNER noise on
// it — the record was created owner-scoped and invisible, on the strength of the
// message that proves the relationship.
//
// The address is matched against the participant rows rather than
// counterparty_email, because a meeting carries no counterparty: attendance is a
// LIST and the calendar mapper leaves the field unset. Either the participant
// row resolved to a contact holding this address, or it is the bare address the
// invitation named — a contact the workspace does not have yet is exactly the
// case this decides.
//
// THREE BOUNDS, three of the four the consent arm applies (consent's
// meetingQualifyingEvent, which says why it needs a fourth), because they answer
// the same question about the same row and two readings of one fact drift:
//
//   - a CONNECTOR captured it, so a hand-logged "meeting" cannot mint a contact;
//   - it was not declined or abandoned (no_show, canceled);
//   - it is not dated past the horizon, so a far-future date cannot stand in for
//     a relationship.
//
// An ARCHIVED meeting is excluded too, which is not one of the three: a meeting
// somebody removed is not evidence of anything, and the activity kind index is
// partial on archived_at IS NULL, so naming it here is also what lets this
// query use one — it runs per captured address.
// TWO ANSWERS, because the meeting has to carry different weight depending on
// what is being asked, and one bool cannot say both.
//
// `met` counts every qualifying meeting, the row being captured included. That
// is what creates the contact: a calendar record names a guest, and the guest
// is the point of it.
//
// `elsewhere` counts only meetings OTHER than the row being decided about, and
// it is what may outrank a settled judgement about the sender. `kind` is a word
// the CALLER supplies — a connector reports it beside the raw bytes, the
// extension ingress copies Activity.Kind off a third-party record with no
// vocabulary check, and the column carries no CHECK constraint. A message that
// calls itself a meeting must not therefore be the evidence that lifts the
// suppression standing against its own sender: that is a caller writing one
// word to publish judged mail to the whole workspace.
func metInPersonTx(
	ctx context.Context, tx pgx.Tx, email string, capturing ids.UUID,
) (met, elsewhere bool, err error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, false, nil
	}
	if err := tx.QueryRow(ctx, `
		SELECT count(*) > 0, count(*) FILTER (WHERE a.id <> $3) > 0
		    FROM activity a
		    JOIN activity_participant p ON p.activity_id = a.id
		   WHERE a.kind = 'meeting'
		     AND a.archived_at IS NULL
		     AND a.captured_by LIKE 'connector:%'
		     AND (a.meeting_status IS NULL OR a.meeting_status NOT IN ('no_show', 'canceled'))
		     AND a.occurred_at <= now() + $2::interval
		     AND (
		          -- The BARE invitation address, bounded by the address's own
		          -- tenure. An activity_participant row records what the
		          -- invitation said, and archiving a contact_email never
		          -- rewrites it — so once an address changes hands, a meeting
		          -- its FORMER holder attended would go on standing as evidence
		          -- about the new one, and mint a contact for a stranger.
		          -- Archiving the old holder's address is the only mark the
		          -- hand-over leaves, so it is the boundary: meetings after it
		          -- are the new holder's, meetings before it are not anybody's
		          -- to inherit. An address that never changed hands has no such
		          -- row and is unaffected.
		          (lower(p.address) = $1 AND a.occurred_at > COALESCE(
		             (SELECT max(pe.archived_at) FROM contact_email pe
		               WHERE lower(pe.email) = $1 AND pe.archived_at IS NOT NULL),
		             '-infinity'::timestamptz))
		          OR EXISTS (
		            SELECT 1 FROM contact_email pe
		             WHERE pe.contact_id = p.contact_id
		               AND lower(pe.email) = $1
		               AND pe.archived_at IS NULL))
		     AND `+auth.ActivityAvailableClause("a"),
		normalized, meetingHorizonInterval, capturing).Scan(&met, &elsewhere); err != nil {
		return false, false, fmt.Errorf("capture: reading whether this workspace is meeting the sender: %w", err)
	}
	return met, elsewhere, nil
}

// meetingHorizonInterval bounds how far into the diary a meeting still says a
// relationship is live. Spelled again here rather than imported because a module
// never imports a sibling; what must not drift is the WINDOW, and the consent
// arm holds the same one against the same rows.
const meetingHorizonInterval = "90 days"
