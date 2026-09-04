// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The mail-shaped gates of the tiered creation ladder (ADR-0072 §1), gathered
// in one place: each one reads a mail domain or a mail address, and each is
// therefore about mail alone. A channel record carries neither, which is why it
// never enters the ladder these serve (sinkchannel.go).

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The breadcrumb action and reason for a message dropped as internal. The
// reason is the `internal_only` value the capture.skipped event carries
// (event-bus EVT-SEM-10); the row names the natural key and nothing else — an
// address, subject or body in the operational ledger would re-create in
// `system_log` exactly the disclosure the drop exists to prevent.
const (
	actionCaptureInternalDropped = "capture_internal_dropped"
	reasonInternalOnly           = "internal_only"
)

// internalOnlyTx reports whether every party to this record is on one of the
// workspace's own mail domains — the zero-rows condition (ADR-0082/A127,
// formulas §20).
//
// A lead is not correspondence and is not judged. A channel record (Telegram,
// WhatsApp) does reach the ActivityFields arm, and is excluded by the address
// guard below rather than by the type: it names provider identities, never mail
// addresses, so there is nothing here to measure it against.
//
// A record that enumerates no addresses is NOT internal. A connector reporting
// an empty set is saying it could not read the parties, which is a different
// statement from "there were none", and the direction to fail in is toward
// keeping the message.
func (s *Sink) internalOnlyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	if _, ok := rec.Fields.(ActivityFields); !ok {
		return false, nil
	}
	if len(rec.Addresses) == 0 {
		return false, nil
	}
	own, err := trustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return false, err
	}
	// The acting seat's own other addresses count as internal too. A founder
	// writing between their work address and their private domain is one person
	// talking to themselves, which is less of a customer relationship than two
	// colleagues talking — and the whole point of a declared alias is that it
	// is not somebody else.
	//
	// Removed before the test rather than folded into the own-domain set: these
	// are ONE seat's claim, and AllInternal answers a question about the
	// workspace. A message whose only remaining parties are the seat's own
	// addresses has nothing external left, which is the same zero-rows rule
	// AllInternal already applies.
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	// A capture whose mailbox owner cannot be named is not judged against
	// anybody's claims — the set is empty, and this gate answers exactly as it
	// did before the feature existed. The message is kept and the ladder
	// refuses to create a record for it (derivationStart's no-granting-human
	// arm), which is the same direction every other gate here fails in: toward
	// keeping a message rather than acting on an owner nobody can name.
	external := self.WithoutSelf(rec.Addresses)
	if len(external) == 0 {
		// Every party was the seat themselves. Internal by the same reasoning
		// AllInternal uses, without asking the workspace's domains about
		// addresses that are not on them.
		return !self.Empty(), nil
	}
	return own.AllInternal(external), nil
}

// internalDomainTx reports whether domain is one of the workspace's own mail
// domains (the colleagues gate). Runs on the capture transaction: the tier
// ladder decides and records atomically with the activity it is about.
func (s *Sink) internalDomainTx(ctx context.Context, tx pgx.Tx, domain string) (bool, error) {
	if domain == "" {
		return false, nil
	}
	own, err := ownDomainsTx(ctx, tx)
	if err != nil {
		return false, err
	}
	// Through the same matcher every other caller uses, so a colleague at
	// mail.acme.com is a colleague here too. Exact equality read the same table
	// and answered differently, which minted contacts for the workspace's own
	// employees whenever they wrote from a subdomain.
	return own.CoversDomain(domain), nil
}

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
// person writes that means the opposite of intent.
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

// declinePhrases are what a person writes to end a conversation they never
// wanted. Deliberately short and unambiguous: every entry here has to be a
// phrase whose presence means "stop", because a false positive sends a genuine
// prospect to the verdict engine (a delay, and recoverable) while a false
// negative admits a spammer permanently.
var declinePhrases = []string{
	"not interested", "no interest", "kein interesse", "nicht interessiert",
	"please remove me", "remove me from", "unsubscribe", "opt out", "opt-out",
	"austragen", "abmelden", "bitte löschen sie", "keine weiteren e-mails",
	"do not contact", "don't contact", "stop emailing", "stop contacting",
	"no thanks", "no thank you",
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

// registrySuppresses runs T2 against the transactional/ESP registry
// (CAP-PARAM-6): a DocuSign envelope or a SendGrid relay is not a
// counterparty's company, so person AND org derivation are suppressed while the
// activity stands — a signed envelope is a real timeline item — and the reason
// lands on the ledger so a wrong registry entry is queryable, not only logged.
//
// T1 OUTRANKS it, and the precedence is load-bearing: a known contact whose
// newsletter footer carries a List-Unsubscribe header is not infrastructure. A
// spare is recorded on its own, because it is the one path that lets an address
// the registry calls infrastructure become a record.
// subject is the party the ladder is about, which is not always the message's
// counterparty: on an introduction it is the outsider a colleague copied, and
// asking the registry about the colleague's own domain would test a domain that
// is never infrastructure instead of the one that might be.
func (s *Sink) registrySuppresses(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, subject connector.Counterparty, row dispositionRow, corresponded bool) (bool, string, error) {
	if s.transactional == nil {
		return false, "", nil
	}
	suppress, reason := s.transactional.Suppress(transactionalInput(subject))
	if !suppress {
		return false, "", nil
	}
	if corresponded {
		return false, "", s.logBreadcrumbTx(ctx, tx, "capture_correspondence_spared", rec, reason)
	}
	row.Status, row.Reason = PendingStatusSuppressed, reason
	// A suppression asks nothing and so is never capped; the flag is only
	// meaningful for the deferring tier.
	if _, err := recordDisposition(ctx, tx, row); err != nil {
		return true, "", err
	}
	// The TRACE gets the class, not this reason. The registry's answer embeds
	// the matched domain or prefix ("transactional_infra:sendgrid.net"), which
	// is a sender-derived string — and the trace's reason column promises a
	// class this installation chose, never content, whatever the payload
	// posture says. The breadcrumb beside it keeps the full detail, where an
	// operator debugging a wrong registry entry needs it.
	return true, traceSuppressionClass(reason), s.logBreadcrumbTx(ctx, tx, "capture_transactional_suppressed", rec, reason)
}

// traceSuppressionClass reduces the registry's reason to its stable class.
//
// It is also what keeps the rendered vocabulary closed: the screen resolves a
// reason through an i18n catalog, and a value carrying a domain matches no key
// and would render as the key itself.
func traceSuppressionClass(reason string) string {
	class, _, found := strings.Cut(reason, ":")
	if !found || class == "" {
		return reason
	}
	return class
}

// transactionalInput builds the transactional-gate input from a captured
// counterparty: the domain, the address local part (machine-sender
// corroboration), and the List-Unsubscribe signal the connector parsed.
func transactionalInput(cp connector.Counterparty) TransactionalInput {
	local, _, _ := strings.Cut(cp.Email, "@")
	return TransactionalInput{
		Domain:          cp.Domain,
		Localpart:       local,
		ListUnsubscribe: cp.ListUnsubscribe,
	}
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
	return correspondencePositiveTx(ctx, tx, email)
}

// wroteBackTx reports whether this address has written to the workspace on a
// thread the workspace itself started.
//
// It is the other half of "a correspondence", and the stronger half. One
// outbound message is intent, and intent is often unreturned: a founder mails
// forty people about a conference and hears from six. Recording all forty as
// contacts on the strength of the send alone is what filled a CRM with people
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
// It is the third shape of "we have actually dealt with this person", beside a
// reply and two outbound threads, and it is the strongest of them. Mail is
// evidence about intent — a founder mails forty people and hears from six — and
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
// row resolved to a person holding this address, or it is the bare address the
// invitation named — a person the workspace does not have yet is exactly the
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
		     AND (lower(p.address) = $1
		          OR EXISTS (
		            SELECT 1 FROM person_email pe
		             WHERE pe.person_id = p.person_id
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
