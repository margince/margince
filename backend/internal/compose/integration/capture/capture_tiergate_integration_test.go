// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The tiered creation gate (ADR-0072 §1) — which senders become records. Split
// from the auto-create suite because the tiers are their own subject: one file
// proves what each tier decides in isolation, the other what a capture writes.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The suppressing tiers, each on its own: a free-mail domain is a person and
// never a company, mail infrastructure is neither while its activity stands,
// and a lookalike sender no rule corroborates is an ordinary counterparty.
func TestCaptureTierGateSuppressesWhatIsNotACounterparty(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	t.Run("free-mail defers the person and never names a company", func(t *testing.T) {
		sync(t, email("bob@gmail.com", "Bob Person", captureOwner, "b1@gmail.com", ""))
		// A consumer mailbox settles the ORGANIZATION question by itself and
		// settles nothing about the person. A customer writing from their
		// private address and a founder's sister arrive in exactly this shape,
		// so minting on sight put nineteen private correspondents of one
		// mailbox into a shared CRM.
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'bob@gmail.com'`); n != 0 {
			t.Fatalf("%d persons for a free-mail sender, want 0 — the verdict decides who they are", n)
		}
		if n := countRows(t, e, `SELECT count(*) FROM organization WHERE display_name = 'gmail.com'`); n != 0 {
			t.Fatal("gmail.com must never become an organization")
		}
		// The sender goes on the ledger for a verdict, which is safe for the
		// company question in a way tier order used to be trusted for: the
		// verdict's own create path reaches deferOrgToTriage, which refuses a
		// consumer domain there too. Both writers of that refusal are needed —
		// the ladder never sees a verdict-created record.
		if n := countRows(t, e, `
			SELECT count(*) FROM capture_pending_counterparty WHERE email = 'bob@gmail.com'`); n != 1 {
			t.Fatalf("%d ledger rows for a free-mail sender, want 1", n)
		}
	})
	t.Run("transactional infrastructure keeps the activity, derives no counterparty", func(t *testing.T) {
		// A DocuSign envelope (exact infra eSLD, no corroboration needed) and a
		// conference blast on a prefix subdomain WITH a List-Unsubscribe header
		// (corroborated) both suppress person+org while the timeline row stands
		// (ADR-0072/A118, CAP-PARAM-6).
		sync(
			t,
			email("dse@eu.docusign.net", "DocuSign EU", captureOwner, "ds1@docusign.net", ""),
			emailWithListUnsub("hello@event.gitex.com", "GITEX", "gx1@event.gitex.com"),
		)
		if n := countRows(t, e, `SELECT count(*) FROM activity WHERE source_id IN ('ds1@docusign.net', 'gx1@event.gitex.com')`); n != 2 {
			t.Fatalf("%d transactional activities captured, want 2 — the timeline row must stand", n)
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email IN ('dse@eu.docusign.net', 'hello@event.gitex.com')`); n != 0 {
			t.Fatal("transactional infrastructure must derive no person")
		}
		if n := countRows(t, e, `SELECT count(*) FROM organization WHERE display_name IN ('Docusign', 'Gitex')`); n != 0 {
			t.Fatal("transactional infrastructure must derive no organization")
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM system_log
			WHERE action = 'capture_transactional_suppressed' AND detail->>'source_id' IN ('ds1@docusign.net', 'gx1@event.gitex.com')`); n != 2 {
			t.Fatalf("%d transactional-suppression breadcrumbs, want 2", n)
		}
		// A message that will link to no record is not thereby the whole
		// workspace's: it is held inside its participants.
		if n := countRows(t, e, `SELECT count(*) FROM activity WHERE source_id IN ('ds1@docusign.net', 'gx1@event.gitex.com') AND audience = 'participants'`); n != 2 {
			t.Fatalf("%d suppressed link-less activities held to their participants, want 2", n)
		}
	})
	t.Run("a conference blast WITHOUT corroboration is deferred, not suppressed", func(t *testing.T) {
		// The same prefix subdomain, but no List-Unsubscribe and a human
		// localpart: T2 does not fire, because a real company can live at
		// event.*. What used to happen next was a record on sight; the sender
		// is now the ambiguous class and waits for a verdict instead.
		sync(t, email("ada@event.realco.example", "Ada Real", captureOwner, "rc1@event.realco.example", ""))

		if n := countRows(t, e, `
			SELECT count(*) FROM system_log
			WHERE action = 'capture_transactional_suppressed' AND detail->>'source_id' = 'rc1@event.realco.example'`); n != 0 {
			t.Fatal("an uncorroborated prefix sender was suppressed — a real company can live at event.*")
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM capture_pending_counterparty
			WHERE email = 'ada@event.realco.example' AND status = 'pending'`); n != 1 {
			t.Fatalf("%d pending ledger rows, want 1 — an unknown sender defers rather than creating", n)
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'ada@event.realco.example'`); n != 0 {
			t.Fatal("a first-time sender minted a person — ADR-0063's create-on-sight is what this amends")
		}
		if n := countRows(t, e, `SELECT count(*) FROM activity WHERE source_id = 'rc1@event.realco.example'`); n != 1 {
			t.Fatal("the activity must stand — deferring the record never drops the message")
		}
		// Deferred is not terminal: a later verdict may admit the sender and
		// link the message, so the LADDER adds no limit of its own — the row
		// carries whatever the mailbox's posture asked for and nothing stricter.
		// The mailbox here is classified, so that is `posture`; a suppressed
		// sender's message on the same mailbox reads `no_record` instead, which
		// is the difference this asserts.
		if n := countRows(t, e, `SELECT count(*) FROM activity
			 WHERE source_id = 'rc1@event.realco.example' AND audience_reason = 'posture'`); n != 1 {
			t.Fatal("a deferred sender's message was limited before its verdict")
		}
	})
}

// T1 outranks T2, and only on evidence the provider vouched for. Writing to
// someone spares their later bulk mail; a forged From:owner buys nothing; and
// outranking T2 never promotes a free-mail domain into a company.
func TestCaptureTierGateLetsCorrespondencePrecedeSuppression(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	t.Run("writing to an address spares its later bulk mail from suppression", func(t *testing.T) {
		// T1 runs BEFORE T2 and the order is load-bearing (ADR-0072 §1): once
		// the workspace has written to someone, their newsletter footer must not
		// turn them into infrastructure. The same address and the same
		// List-Unsubscribe corroboration that suppressed above now survive,
		// because the mailbox owner wrote to them first.
		syncSent(t, map[string]bool{"ev1@myco.example": true},
			email(captureOwner, "", "team@event.expo.example", "ev1@myco.example", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM activity
			WHERE counterparty_email = 'team@event.expo.example' AND counterparty_outbound_attested`); n != 1 {
			t.Fatalf("%d attested outbound activities, want 1 — the T1 evidence must be stamped", n)
		}

		// They answer on our thread, which is the exchange the create tier needs.
		// The newsletter that follows is not that answer: a List-Unsubscribe
		// says a list sent it, and a list writes back to nobody.
		sync(t, email("team@event.expo.example", "Expo", captureOwner,
			"ev1r@event.expo.example", "ev1@myco.example"))
		sync(t, emailWithListUnsub("team@event.expo.example", "Expo", "ev2@event.expo.example"))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'team@event.expo.example'`); n != 1 {
			t.Fatalf("%d persons for a corresponded-with sender, want 1 — T1 must spare it from T2", n)
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM system_log
			WHERE action = 'capture_correspondence_spared' AND detail->>'source_id' = 'ev2@event.expo.example'`); n != 1 {
			t.Fatalf("%d spare breadcrumbs, want 1 — an overridden suppression must be as visible as a suppression", n)
		}
	})
	t.Run("writing to an expense tool does not make its robot a contact", func(t *testing.T) {
		// The other half of the precedence, and the half correspondence must NOT
		// win. A prefix rule guesses that a subdomain is a sender lane, and a
		// contact the workspace writes to overrules the guess. An exact
		// infrastructure domain is not a guess: nobody is reachable behind
		// `receipts@` at an expense tool, so a founder replying to their own
		// receipts is a person answering a robot. Reading that as correspondence
		// is what put an expense tool's "Receipts" in a real CRM as a person.
		syncSent(t, map[string]bool{"exp1@myco.example": true},
			email(captureOwner, "", "receipts@expensify.com", "exp1@myco.example", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM activity
			WHERE counterparty_email = 'receipts@expensify.com' AND counterparty_outbound_attested`); n != 1 {
			t.Fatalf("%d attested outbound activities, want 1 — the T1 evidence is genuinely there", n)
		}

		sync(t, email("receipts@expensify.com", "Expensify", captureOwner, "exp2@expensify.com", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'receipts@expensify.com'`); n != 0 {
			t.Fatalf("%d persons for an expense tool's robot, want 0 — correspondence must not spare an infrastructure domain", n)
		}
		// And the message itself still lands: suppression withholds the contact,
		// never the timeline row.
		if n := countRows(t, e, `
			SELECT count(*) FROM activity WHERE source_id = 'exp2@expensify.com'`); n != 1 {
			t.Fatalf("%d activities, want 1 — a suppressed sender's mail is still a timeline item", n)
		}
	})
	t.Run("a machine local part on a mixed domain is not spared either", func(t *testing.T) {
		// The domain carries both a robot and the people who work there, so the
		// rule keys on the LOCAL PART. Writing to the robot must not admit it.
		syncSent(t, map[string]bool{"gh1@myco.example": true},
			email(captureOwner, "", "notifications@github.com", "gh1@myco.example", ""))
		sync(t, email("notifications@github.com", "GitHub", captureOwner, "gh2@github.com", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'notifications@github.com'`); n != 0 {
			t.Fatalf("%d persons for a notification robot, want 0", n)
		}
	})
	t.Run("a forged From:owner does not whitelist the address it names", func(t *testing.T) {
		// The attack the T1 evidence exists to refuse: inbound mail whose From
		// header claims the mailbox owner. It parses as outbound — that is all
		// direction can mean — but the provider filed it in the inbox and
		// attested nothing, so the address stays suppressed infrastructure.
		sync(t, email(captureOwner, "", "blast@sendgrid.net", "forge1@evil.example", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM activity
			WHERE counterparty_email = 'blast@sendgrid.net' AND counterparty_outbound_attested`); n != 0 {
			t.Fatal("a forged From:owner message attested correspondence — the gate reads the header, not the provider")
		}
		sync(t, emailWithListUnsub("blast@sendgrid.net", "Blast", "forge2@sendgrid.net"))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'blast@sendgrid.net'`); n != 0 {
			t.Fatal("a forged From:owner whitelisted an ESP address past T2 suppression")
		}
	})
	t.Run("a corresponded-with free-mail address is still never a company", func(t *testing.T) {
		// T1 overrides T2 suppression ONLY. Free-mail's org rule is about what a
		// domain can honestly name, not about whether its sender is trusted, so
		// writing to a gmail.com address buys its owner a person and never an
		// organization called "Gmail" — the junk this ADR exists to prevent.
		syncSent(t, map[string]bool{"fm1@myco.example": true},
			email(captureOwner, "", "carol@gmail.com", "fm1@myco.example", ""))
		// Carol answers, which is what makes her a contact without a verdict.
		// The subject here is what her DOMAIN can name, so the exchange is
		// fixture rather than finding.
		sync(t, email("carol@gmail.com", "Carol", captureOwner, "fm1r@gmail.com", "fm1@myco.example"))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'carol@gmail.com'`); n != 1 {
			t.Fatalf("%d persons for carol, want 1", n)
		}
		if n := countRows(t, e, `SELECT count(*) FROM organization WHERE display_name IN ('Gmail', 'gmail.com')`); n != 0 {
			t.Fatal("a corresponded-with free-mail address minted an organization")
		}
	})
}

// The corroboration rule (CAP-PARAM-6) suppresses a prefix-subdomain sender
// only when something confirms it is bulk infrastructure — a List-Unsubscribe
// header, or a machine localpart. Both halves of ADR-0072 §1's promise hold at
// once: the derivation is suppressed AND the message still reaches the
// timeline, which is the whole reason a DocuSign envelope is worth capturing.
// A role mailbox is correspondence-positive exactly like a customer — a mailbox
// owner writes to `billing@` and `support@` all the time — so T1's evidence is
// true and its conclusion was still wrong: real correspondence, and no person to
// name. This is the tier that put contacts called "Billing" and "support" in a
// founder's CRM, each one a department with a human's shape.
//
// The message is kept and stays visible. What is refused is the record.
func TestCaptureTierGateMintsNoPersonForARoleMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	for _, tc := range []struct {
		name, address, display, sentID, replyID string
	}{
		{"a billing department", "billing_apac@habyt.com", "APAC Billing", "rm1@myco.example", "rm2@habyt.com"},
		{"a role behind a ticket tag", "support+idy4dl62@getmyinvoices.zendesk.com", "support", "rm3@myco.example", "rm4@zendesk.com"},
		{"a compound role local part", "hello.events@thesentry.com.vn", "Events The Sentry", "rm5@myco.example", "rm6@thesentry.com.vn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The T1 evidence is genuinely there: the owner wrote to the queue.
			syncSent(t, map[string]bool{tc.sentID: true},
				email(captureOwner, "", tc.address, tc.sentID, ""))
			if n := countRows(t, e, `
				SELECT count(*) FROM activity
				WHERE counterparty_email = '`+tc.address+`' AND counterparty_outbound_attested`); n != 1 {
				t.Fatalf("%d attested outbound activities, want 1 — the T1 evidence must be stamped", n)
			}

			sync(t, email(tc.address, tc.display, captureOwner, tc.replyID, ""))

			if n := countRows(t, e, `
				SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
				WHERE pe.email = '`+tc.address+`'`); n != 0 {
				t.Fatalf("%d persons for a role mailbox, want 0 — a department is not a contact", n)
			}
			// The correspondence itself is not the thing being refused: somebody
			// answers that queue, and losing their mail would cost the owner a
			// real conversation to spare them a wrong contact.
			if n := countRows(t, e, `
				SELECT count(*) FROM activity WHERE source_id = '`+tc.replyID+`'`); n != 1 {
				t.Fatalf("%d activities for the role mailbox's message, want 1 — the mail must be kept", n)
			}
		})
	}
}

// A judged queue is not re-judged. The refusal above leaves the address on the
// ordinary path so a FIRST sighting opens the ledger question — and once that
// question is answered, later mail must stop asking it.
//
// The ladder's settled early return cannot cover this case: reaching it needs
// !corresponded, and a role mailbox the owner writes to is corresponded by
// definition. Without a second arm the queue re-defers on every message, the
// live-row index quietly absorbs each write, and the verdict pass re-answers a
// settled question for as long as the mailbox keeps writing.
func TestCaptureTierGateAsksAboutARoleMailboxOnlyOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	const addr = "billing@recur.example"

	syncSent(t, map[string]bool{"rr1@myco.example": true},
		email(captureOwner, "", addr, "rr1@myco.example", ""))
	sync(t, email(addr, "Billing", captureOwner, "rr2@recur.example", ""))
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'billing@recur.example'`); n != 1 {
		t.Fatalf("%d ledger rows after first contact, want exactly 1 — the queue must be asked about once", n)
	}

	// The ledger as the verdict engine leaves it.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = 'real', kind = 'role_mailbox',
			       disposition_reason = 'capture_counterparty_verdict', resolved_at = now()
			 WHERE email = $1`, addr)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	sync(t, email(addr, "Billing", captureOwner, "rr3@recur.example", ""))

	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'billing@recur.example' AND resolved_at IS NULL`); n != 0 {
		t.Fatalf("%d re-opened questions for a settled role mailbox, want 0 — decided means decided", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'billing@recur.example'`); n != 0 {
		t.Fatalf("%d persons for a settled role mailbox, want 0", n)
	}
}

// Inviting somebody to a meeting is not writing to them.
//
// Google Calendar sends an invitation FROM the organizer with the attendee in
// To, and Gmail files a copy in Sent — so the shape is indistinguishable from
// ordinary outbound mail, and T1 read every attendee as an address the
// workspace writes to. A founder's spouse, his language teacher and his own
// second address all became contacts this way.
//
// The message is kept: the meeting is a real fact about the owner's week, and
// the attendees stay recorded as participants, which is what they are. What is
// withheld is the contact.
func TestCaptureTierGateMintsNoContactForACalendarInvite(t *testing.T) {
	env := newCaptureEnv(t)
	e, syncSent := env.e, env.syncSent

	// The invite goes out from Sent, exactly as the provider files it — the same
	// attestation that makes an ordinary outbound message T1 evidence.
	syncSent(t, map[string]bool{"cal1@myco.example": true},
		calendarInvite("attendee@partner.example", "cal1@myco.example"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'attendee@partner.example'`); n != 0 {
		t.Fatalf("%d persons for a meeting attendee, want 0 — an invitation is not correspondence", n)
	}
	// The evidence itself must not be stamped: an invitation vouches for
	// nobody, so a LATER message from that address cannot inherit T1's spare.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity
		WHERE counterparty_email = 'attendee@partner.example'
		  AND counterparty_outbound_attested`); n != 0 {
		t.Fatalf("%d attested outbound activities for an invitation, want 0 — groupware composed it", n)
	}
	// No ledger question either: a question about an attendee is one the verdict
	// engine would have to spend a model call answering, about somebody who was
	// never a counterparty.
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'attendee@partner.example'`); n != 0 {
		t.Fatalf("%d ledger questions about an attendee, want 0 — nobody asked to be a contact", n)
	}
	// And the meeting itself is kept.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity WHERE source_id = 'cal1@myco.example'`); n != 1 {
		t.Fatalf("%d activities for the invitation, want 1 — the meeting is not lost", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM activity_participant ap
		 JOIN activity a ON a.id = ap.activity_id
		WHERE a.source_id = 'cal1@myco.example' AND lower(ap.address) = 'attendee@partner.example'`); n != 1 {
		t.Fatalf("%d participant rows for the attendee, want 1 — they were on the meeting", n)
	}
}

// One send is intent. An exchange is a correspondence.
//
// The tier that creates on the strength of an outbound message read "we wrote
// here" as "this is a contact", and intent is often unreturned: a founder mails
// forty people about a conference and hears from six. The other thirty-four
// became contacts anyway, along with the test addresses and the one-off
// errands.
//
// Nothing is refused, only deferred. A single send sends the address to the
// verdict, which reads the message and answers on its merits — so a real
// prospect written to once still becomes a contact, a moment later and for a
// reason.
func TestCaptureTierGateNeedsAnExchangeRatherThanASend(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent

	t.Run("one send defers instead of creating", func(t *testing.T) {
		syncSent(t, map[string]bool{"ex1@myco.example": true},
			email(captureOwner, "", "quiet@prospect.example", "ex1@myco.example", ""))

		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'quiet@prospect.example'`); n != 0 {
			t.Fatalf("%d persons after one send, want 0 — writing once is intent, not a correspondence", n)
		}
		// Deferred, not dismissed: the verdict still gets to answer, and a real
		// prospect becomes a contact that way.
		if n := countRows(t, e, `
			SELECT count(*) FROM capture_pending_counterparty
			WHERE email = 'quiet@prospect.example' AND status = 'pending'`); n != 1 {
			t.Fatalf("%d open questions after one send, want 1 — the sender is deferred, not refused", n)
		}
	})

	t.Run("their reply on our thread is the exchange", func(t *testing.T) {
		syncSent(t, map[string]bool{"ex2@myco.example": true},
			email(captureOwner, "", "answers@prospect.example", "ex2@myco.example", ""))
		sync(t, email("answers@prospect.example", "Answers", captureOwner,
			"ex2r@prospect.example", "ex2@myco.example"))

		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'answers@prospect.example'`); n != 1 {
			t.Fatalf("%d persons after they wrote back, want 1 — somebody read our mail and answered", n)
		}
	})

	t.Run("two separate threads are an exchange too", func(t *testing.T) {
		// Nobody writes to the same address on two different threads by
		// accident, so the second send is its own evidence — no reply needed.
		syncSent(t, map[string]bool{"ex3@myco.example": true},
			email(captureOwner, "", "twice@prospect.example", "ex3@myco.example", ""))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'twice@prospect.example'`); n != 0 {
			t.Fatalf("%d persons after the first send, want 0 — the fixture never reaches the case under test", n)
		}
		syncSent(t, map[string]bool{"ex4@myco.example": true},
			email(captureOwner, "", "twice@prospect.example", "ex4@myco.example", ""))

		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'twice@prospect.example'`); n != 1 {
			t.Fatalf("%d persons after two threads, want 1 — writing twice is not an accident", n)
		}
	})

	t.Run("a bulk reply is not writing back", func(t *testing.T) {
		// An address the workspace wrote to once, which then sends a newsletter,
		// has not answered anybody: it added the workspace to a list. Reading
		// that as a reply would admit exactly the senders the transactional
		// gates exist to refuse.
		syncSent(t, map[string]bool{"ex5@myco.example": true},
			email(captureOwner, "", "list@prospect.example", "ex5@myco.example", ""))
		sync(t, emailWithListUnsub("list@prospect.example", "List", "ex5r@prospect.example"))

		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'list@prospect.example'`); n != 0 {
			t.Fatalf("%d persons for a list that mailed back, want 0 — a list answers nobody", n)
		}
	})
}

func TestCaptureTierGateSuppressesAMachineLocalpartWithoutLosingTheMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email("no-reply@em.vendor.example", "Vendor", captureOwner, "v1@em.vendor.example", ""))

	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE source_id = 'v1@em.vendor.example'`); n != 1 {
		t.Fatalf("%d activities for the vendor envelope, want 1 — the timeline row must stand", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'no-reply@em.vendor.example'`); n != 0 {
		t.Fatal("a machine localpart on a prefix subdomain must derive no person")
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM system_log
		WHERE action = 'capture_transactional_suppressed' AND detail->>'source_id' = 'v1@em.vendor.example'`); n != 1 {
		t.Fatalf("%d suppression breadcrumbs, want 1 — the corroboration rule must be the one that fired", n)
	}
}

// A noise verdict settles only the mail it can still reach (ADR-0072 §4). Past
// that window the sender's next message is new evidence and raises its own
// question — without which one forged message would bar an address from ever
// becoming a record, while its later mail went neither judged nor hidden.
func TestCaptureTierGateReopensASenderWhoseNoiseVerdictHasAged(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email("stale@judged.example", "Judged", captureOwner, "aged1@judged.example", ""))
	resolveDispositionAged(t, e, "stale@judged.example", "noise", 30*24*time.Hour)

	sync(t, email("stale@judged.example", "Judged", captureOwner, "aged2@judged.example", ""))

	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'stale@judged.example' AND status = 'pending'`); n != 1 {
		t.Fatalf("%d fresh questions for a sender whose verdict aged out, want 1", n)
	}
	// A RECENT verdict still settles the matter — the rule is about age, not
	// about ignoring the ledger.
	if n := countRows(t, e, `
		SELECT count(*) FROM system_log WHERE action = 'capture_noise_sender'`); n != 0 {
		t.Fatal("an aged-out verdict was still treated as settling the sender")
	}
}

// resolveDispositionAged puts an address's disposition into a terminal state
// resolved the given duration ago.
func resolveDispositionAged(t *testing.T, e *integration.SearchEnv, email, status string, ago time.Duration) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = $2, resolved_at = now() - $3::interval,
			       next_attempt_at = NULL, claimed_until = NULL, claimed_by = NULL
			 WHERE email = $1`, email, status, ago.String())
		return err
	})
	if err != nil {
		t.Fatalf("aging the disposition: %v", err)
	}
}

// grantedSendGate lets the send through the consent seam: this suite is about
// the correspondence evidence a send records, not about suppression, which the
// consent suites prove on the same store method.
type grantedSendGate struct{}

func (grantedSendGate) RequireGrantedForEmails(context.Context, []string, string) error { return nil }

// The channel spelling of the same verdict: one gate answers for both
// transports, so a stub that granted one and not the other would let a suite
// pass a send the real gate refuses.
func (grantedSendGate) RequireGrantedForRecipients(context.Context, []connector.Recipient, string) error {
	return nil
}

// discardSendStager accepts the delivery so the send commits. Transmission is
// not what this suite proves; what the send WROTE is.
type discardSendStager struct{}

func (discardSendStager) StageTx(context.Context, pgx.Tx, activities.DeliveryRequest) error {
	return nil
}

// A send composed in the CRM is correspondence, and after ADR-0072's natural
// key collapse it is the ONLY record of it: the activity this send writes
// carries the key the provider's echo of the same message carries, so that
// echo's ON CONFLICT DO NOTHING upsert finds the row and writes nothing. The
// evidence the echo used to bring — who the message was with, and that the
// workspace sent it — therefore has to be written by the send itself, or a
// prospect the CRM emailed first is a stranger when they reply.
func TestCaptureTierGateHonorsCorrespondenceFromACRMOriginatedSend(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	ctx := e.AsFullUser()

	anchorID := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1,
			        'email', 'Intro', now(), 'manual', 'human:x')`, anchorID)
		return err
	}); err != nil {
		t.Fatalf("seed anchor: %v", err)
	}

	if _, err := activities.NewStore(e.DB()).SendEmail(ctx, activities.FromActivity(ids.From[ids.ActivityKind](anchorID)),
		activities.SendEmailInput{
			Recipients:     []string{"Team@News.Prospect.Example"},
			Subject:        "Following up",
			Body:           "Good to meet you.",
			ConsentPurpose: "transactional",
		}, grantedSendGate{}, discardSendStager{}); err != nil {
		t.Fatalf("CRM-originated send: %v", err)
	}

	// The evidence both readers of this column consult: the T1
	// correspondence-positive gate, and the noise sweep's escape hatch that
	// stops a wrongly-suppressed sender's reply from being hidden. It is
	// stored normalized, so the recipient's header casing cannot hide it.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity
		WHERE counterparty_email = 'team@news.prospect.example'
		  AND direction = 'outbound' AND counterparty_outbound_attested`); n != 1 {
		t.Fatalf("%d attested outbound activities after a CRM send, want 1 — the echo will not record it later", n)
	}

	// They answer on the thread the CRM send started, and their mail carries a
	// List-Unsubscribe header — the exact shape T2 suppresses for an unknown
	// sender. Because the workspace wrote to them first and they wrote back, T1
	// spares it.
	//
	// The reply is threaded, which is what makes it a reply: an unthreaded
	// message from the same address is a first approach that happens to share a
	// sender, and the exchange rule reads the thread rather than the address.
	//
	// The thread key is read back rather than guessed: the CRM send generated
	// it, so a literal here would silently stop threading the day that changes.
	var crmThread string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT thread_key FROM activity
			 WHERE counterparty_email = 'team@news.prospect.example'
			   AND direction = 'outbound' AND counterparty_outbound_attested
			 LIMIT 1`).Scan(&crmThread)
	}); err != nil {
		t.Fatalf("reading the CRM send's thread: %v", err)
	}
	if crmThread == "" {
		t.Fatal("the CRM send joined no thread — the reply below could not answer it")
	}
	// A person at that address answers the CRM's mail. This is the exchange the
	// create tier now needs, and it is deliberately NOT the bulk-headered
	// message below: a List-Unsubscribe says a list sent this, and a list has
	// not written back to anybody.
	sync(t, email("team@news.prospect.example", "Prospect", captureOwner,
		"pr0@news.prospect.example", crmThread))
	// Their newsletter then arrives on a prefix subdomain WITH a
	// List-Unsubscribe header — the exact shape T2 suppresses for an unknown
	// sender. Because the workspace wrote to them and they answered, T1 spares
	// it.
	sync(t, emailWithListUnsub("team@news.prospect.example", "Prospect", "pr1@news.prospect.example"))
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'team@news.prospect.example'`); n != 1 {
		t.Fatalf("%d persons for a sender the CRM had written to, want 1 — a CRM send must count as correspondence", n)
	}
	// A ledger row here is expected, and it is not a deferral. #3719 made both
	// create tiers open the same question the deferred tier opens, because a
	// sender created on sight and never judged stayed the mailbox owner's
	// permanently. T1 still decides STORAGE — the person above exists, which is
	// what "nothing defers" was ever about. What the row asks is whose record it
	// is, and `advisor` is a legitimate answer that keeps it private, so the
	// question cannot be skipped for a corresponded-with sender either.
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'team@news.prospect.example'
		  AND status = 'pending' AND resolved_at IS NULL`); n != 1 {
		t.Fatalf("%d open visibility questions for a corresponded-with sender, want 1 — "+
			"T1 creates the record on sight and still asks whose it is", n)
	}
}

// The enrich-on-capture condition (ADR-0072/A118 §9): mail from a company the
// workspace ALREADY has must not re-trigger enrichment. That is the half worth
// holding, because a domain the workspace has already decided about must not
// re-ask on every message — that would spend the day's reads re-answering a
// settled question.
//
// The other half — that a NEW domain DOES queue a read — is not observable here
// and this test does not claim it. Starting a read needs an ambient River
// client; no test process binds one, so the attempt fails and its budget slot is
// refunded, leaving the same counter a domain that never triggered would. Every
// production capture runs inside a River job, so the condition is exercised
// there and nowhere a test can watch it.
func TestCaptureDoesNotReEnrichACompanyItAlreadyHas(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent

	// T1 correspondence-positive is what lets a first inbound message create
	// records at all: the owner wrote to them first, attested by the provider.
	syncSent(t, map[string]bool{"out1@myco.example": true},
		email(captureOwner, "", "cto@newco.example", "out1@myco.example", ""))
	sync(t, email("cto@newco.example", "CTO", captureOwner, "in1@newco.example", "out1@myco.example"))

	// The corresponded-with sender becomes a PERSON, and their domain becomes
	// one open company question — not a company invented from the domain label.
	// NOT is_anchor: the installation's own company is created by cold start,
	// not derived from a captured domain.
	if n := countRows(t, e, `SELECT count(*) FROM organization WHERE NOT is_anchor`); n != 0 {
		t.Fatalf("%d organizations from an unjudged domain, want 0", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM organization_domain_disposition
		WHERE domain = 'newco.example' AND status = 'pending'`); n != 1 {
		t.Fatalf("%d open company questions for newco.example, want exactly 1", n)
	}

	// A second message from the same company lands on the question that is
	// already open — one row, one crawl, however many colleagues write in.
	sync(t, email("sales@newco.example", "Sales", captureOwner, "in2@newco.example", ""))
	if n := countRows(t, e, `
		SELECT count(*) FROM organization_domain_disposition WHERE domain = 'newco.example'`); n != 1 {
		t.Fatalf("%d questions after a second message, want the one that was already open", n)
	}
	if n := countRows(t, e, `
		SELECT coalesce(sum(enqueued), 0) FROM capture_auto_enrich_budget`); n != 0 {
		t.Fatalf("budget spent = %d, want 0 — nothing here started a read, so nothing may stay reserved", n)
	}
}

// Declining is not a relationship. The T1 gate reads what the one outbound
// message SAYS, because "not interested" is the reply a person writes to end a
// conversation they never wanted — and admitting the sender on the strength of
// it is how a real import ended up with a customer-grade record for a firm that
// had cold-mailed the founder.
func TestCaptureTierGateRefusesToAdmitASenderTheOwnerDeclined(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent

	t.Run("a single declining reply does not admit the sender", func(t *testing.T) {
		sync(t, email("deals@peinsights.example", "PE Insights", captureOwner, "pe1@peinsights.example", ""))
		syncSent(t, map[string]bool{"pe2@myco.example": true},
			emailSaying("deals@peinsights.example", "pe2@myco.example", "pe1@peinsights.example",
				"Thanks, but we are not interested. Please remove me from your list."))

		// The reply IS attested outbound — the evidence exists, and the gate
		// still refuses it, which is the whole point.
		if n := countRows(t, e, `
			SELECT count(*) FROM activity
			WHERE counterparty_email = 'deals@peinsights.example' AND counterparty_outbound_attested`); n != 1 {
			t.Fatalf("%d attested outbound activities, want 1 — the scenario needs the evidence present", n)
		}
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'deals@peinsights.example'`); n != 0 {
			t.Fatalf("%d persons for a sender the owner declined, want 0", n)
		}
		// Not dropped — deferred. The verdict engine reads the thread and has
		// the final say; refusing T1 only means the question stays open.
		if n := countRows(t, e, `
			SELECT count(*) FROM capture_pending_counterparty
			WHERE email = 'deals@peinsights.example' AND status = 'pending'`); n != 1 {
			t.Fatalf("%d open questions for the declined sender, want exactly 1", n)
		}
	})

	t.Run("a reply that engages admits the sender immediately", func(t *testing.T) {
		// The other half, and the reason the rule reads words rather than
		// direction: answering a prospect is the most ordinary correspondence
		// there is, and a gate that demoted every reply would refuse it.
		sync(t, email("buyer@northwind.example", "Buyer", captureOwner, "nw1@northwind.example", ""))
		syncSent(t, map[string]bool{"nw2@myco.example": true},
			emailSaying("buyer@northwind.example", "nw2@myco.example", "nw1@northwind.example",
				"Happy to help — can you do a call on Thursday to talk through pricing?"))

		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'buyer@northwind.example'`); n != 1 {
			t.Fatalf("%d persons for a prospect the owner engaged, want exactly 1", n)
		}
	})

	t.Run("a second outbound admits the sender whatever the first said", func(t *testing.T) {
		// Nobody declines twice and keeps writing. Two attested outbounds are a
		// correspondence regardless of the words in either.
		sync(t, email("sales@laterdeal.example", "Later Deal", captureOwner, "ld1@laterdeal.example", ""))
		syncSent(t, map[string]bool{"ld2@myco.example": true},
			emailSaying("sales@laterdeal.example", "ld2@myco.example", "ld1@laterdeal.example",
				"Not interested right now."))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'sales@laterdeal.example'`); n != 0 {
			t.Fatalf("%d persons after the decline alone, want 0", n)
		}

		syncSent(t, map[string]bool{"ld3@myco.example": true},
			emailSaying("sales@laterdeal.example", "ld3@myco.example", "ld1@laterdeal.example",
				"Actually, let us revisit this in Q3."))
		if n := countRows(t, e, `
			SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
			WHERE pe.email = 'sales@laterdeal.example'`); n != 1 {
			t.Fatalf("%d persons after a second outbound, want exactly 1", n)
		}
	})
}

// The attack the quote-strip exists to refuse: a sender plants the decline
// phrase in THEIR OWN mail, so that the moment the mailbox owner hits Reply the
// words appear inside our outbound body. Read naively that reads as the owner
// declining, and the sender talks themselves out of the CRM — a suppression
// anybody can trigger on anybody by writing one sentence.
func TestCaptureTierGateReadsOnlyWhatTheOwnerActuallyWrote(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent

	sync(t, email("rep@planted.example", "Rep", captureOwner, "pl1@planted.example", ""))
	// The reply the owner sent: their own line on top, the sender's planted
	// text quoted beneath it exactly as every mail client leaves it.
	syncSent(t, map[string]bool{"pl2@myco.example": true},
		emailSaying("rep@planted.example", "pl2@myco.example", "pl1@planted.example",
			"Yes please, send the contract over.\r\n\r\n"+
				"On Wed, Jun 4, 2026 at 08:00, Rep <rep@planted.example> wrote:\r\n"+
				"> Hello — if you are not interested, please remove me from your list\r\n"+
				"> and unsubscribe here.\r\n"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'rep@planted.example'`); n != 1 {
		t.Fatalf("%d persons, want 1 — the decline was quoted from the SENDER, not written by the owner", n)
	}
}

// A decided role mailbox stays nobody on its NEXT message.
//
// The verdict resolves such an address to the `real` LIFECYCLE — the mail is
// genuine correspondence and hiding it would be wrong — while creating no
// contact, because a shared mailbox has no human to name. The tier ladder then
// asks what the workspace already decided, and reading the status alone says
// "known counterparty, create the person": the contact the verdict declined
// would appear the moment support@ wrote again. The kind is what stops that.
func TestCaptureTierGateNeverMintsAPersonForADecidedRoleMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	const addr = "support@respacio.example"

	// The first message opens the question the way capture really does: a
	// first-time stranger defers, and the row it writes is the one the verdict
	// engine would answer.
	sync(t, email(addr, "Respacio Support", captureOwner, "rs1@respacio.example", ""))
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'support@respacio.example' AND status = 'pending'`); n != 1 {
		t.Fatalf("%d deferred questions after first contact, want exactly 1", n)
	}

	// The ledger as the verdict engine leaves it: real lifecycle, kind
	// role_mailbox, no person created.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = 'real', kind = 'role_mailbox',
			       disposition_reason = 'capture_counterparty_verdict', resolved_at = now()
			 WHERE email = $1`, addr)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// The SECOND message is the one that used to mint the contact.
	sync(t, email(addr, "Respacio Support", captureOwner, "rs2@respacio.example", ""))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'support@respacio.example'`); n != 0 {
		t.Fatalf("%d persons for a decided role mailbox, want 0 — the kind must outlive the verdict pass", n)
	}
	// Decided means decided: the question is not re-opened and re-billed.
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'support@respacio.example' AND status = 'pending'`); n != 0 {
		t.Fatalf("%d re-opened questions for a decided mailbox, want 0", n)
	}
	// The mail itself stays visible. This kind is real correspondence.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity
		WHERE counterparty_email = 'support@respacio.example' AND archived_at IS NULL`); n != 2 {
		t.Fatalf("%d visible messages, want 2 — a role mailbox is not noise", n)
	}
}

// The hazard that made free-mail decide at the ladder rather than defer: a
// deferred sender is judged later from a ledger row carrying only the domain,
// and a `real` verdict creating records from `gmail.com` would mint the company
// the ladder refused.
//
// Deferring is safe because the refusal is not the ladder's alone. The verdict's
// create path reaches deferOrgToTriage, which asks the same consumer-mail
// question at the chokepoint every writer passes. This is the test that says so
// — without it the two writers are one comment apart from disagreeing.
func TestAVerdictOnAFreeMailSenderMintsThePersonAndNoCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email("carla@gmail.com", "Carla Person", captureOwner, "c1@gmail.com", ""))
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'carla@gmail.com'`); n != 0 {
		t.Fatalf("%d persons before the verdict, want 0", n)
	}

	promoteByVerdict(t, e, "carla@gmail.com", activityIDOf(t, e, "c1@gmail.com"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'carla@gmail.com'`); n != 1 {
		t.Fatalf("%d persons after the verdict, want 1", n)
	}
	// No company, in either spelling: the name the ladder would have invented,
	// and the domain row an attach would have written.
	if n := countRows(t, e, `
		SELECT count(*) FROM organization WHERE display_name = 'gmail.com'`); n != 0 {
		t.Fatal("a verdict on a free-mail sender named gmail.com as a company")
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM organization_domain WHERE domain = 'gmail.com'`); n != 0 {
		t.Fatal("a verdict on a free-mail sender put gmail.com on an organization")
	}
	// And no QUESTION about the domain either, which is the assertion with
	// teeth. deferOrgToTriage creates nothing by itself — it opens a triage
	// question, and the crawl behind that question is what would eventually
	// mint the company. A test that only counted organizations would pass with
	// the consumer refusal deleted, because the company arrives a crawl later.
	if n := countRows(t, e, `
		SELECT count(*) FROM organization_domain_disposition WHERE domain = 'gmail.com'`); n != 0 {
		t.Fatal("a verdict on a free-mail sender opened a company question about gmail.com")
	}
}
