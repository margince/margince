// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// WHICH captured mail a noise disposition may act on, and how far past its own
// verdict it may reach.
//
// Its own file because it is its own concept and the whole of it is one
// judgement about an UNAUTHENTICATED header: every clause below exists to keep a
// forged From from turning a verdict about a stranger's mail into authority over
// somebody else's. The sweeps that compose it are pendingsweeps.go.

import "time"

// noiseMailScope decides WHICH captured mail a noise disposition is allowed to
// act on, and it is deliberately much narrower than "every message bearing this
// address".
//
// counterparty_email comes from the message's own From header, which is
// unauthenticated: an outsider can forge any address they like. Acting on the
// address alone would hand them a weapon — mail one message as
// bigcustomer@corp.com, write it to read as bulk marketing, and a `noise`
// verdict would hide and then redact the workspace's real correspondence with
// that company, in both directions. The verdict is evidence about the mail the
// stranger actually sent, so it may only reach mail of that same kind:
//
//   - INBOUND only. The workspace's own sent mail is its own record, and a
//     stranger's forged header must never reach it.
//
//   - Never attested outbound (the T1 evidence), for the same reason — whether
//     the attestation came from a connector reading the owner's sent copy or
//     from the governed send path stamping its own outbound row.
//
//   - Never linked to a person the message is WITH, and never for an address a
//     person EXISTS for. A linked message belongs to somebody's record; and once
//     the workspace has a contact at that address — by any route, including a
//     human typing it in to correct a wrong verdict — the sender is a
//     counterparty and a stale disposition has no authority over their mail.
//     Linkage alone is not enough: a manually created contact backfills no
//     activity_link. A person filed under only because they were COPIED does not
//     count either: capture files a message under every participant it resolves
//     (sinkmaillinks.go), so a newsletter naming one contact in Cc would carry a
//     person link forever, and a sender judged noise afterwards could never be
//     hidden or redacted.
//
// And the disposition stops applying entirely once the workspace CORRESPONDS
// with the address: writing to someone is the T1 signal that they are a
// counterparty, and it is the recovery path that makes an automatic hide safe to
// live with — reply to a wrongly-hidden sender and the sweep lets go.
const noiseMailScope = `
	  a.kind = 'email' AND a.captured_by LIKE 'connector:%'
	  AND a.direction = 'inbound'
	  AND NOT a.counterparty_outbound_attested
	  AND NOT EXISTS (
	    SELECT 1 FROM activity_link l
	     WHERE l.activity_id = a.id AND l.person_id IS NOT NULL
	       AND NOT EXISTS (
	         SELECT 1 FROM activity_participant cc
	          WHERE cc.activity_id = a.id AND cc.person_id = l.person_id
	            AND cc.role <> 'from'
	            AND NOT EXISTS (
	              SELECT 1 FROM activity_participant au
	               WHERE au.activity_id = a.id AND au.person_id = l.person_id
	                 AND au.role = 'from')))
	  AND NOT EXISTS (
	    SELECT 1 FROM activity c
	     WHERE c.counterparty_email = p.email
	       AND c.direction = 'outbound' AND c.counterparty_outbound_attested)
	  AND NOT EXISTS (
	    SELECT 1 FROM person_email pe JOIN person pr ON pr.id = pe.person_id
	     WHERE pe.email = p.email AND pr.archived_at IS NULL
	       AND pe.from_correspondence)`

// noiseVerdictReach bounds how far past its own verdict a `noise` disposition
// may reach forward in time.
//
// Without a bound the disposition is permanent and unbounded, and that is an
// outsider's opening: forge one message as an address the workspace has never
// written to, shape it to read as bulk marketing, and every mail the REAL owner
// of that address sends afterwards is hidden within the hour and destroyed a
// week later — never seen by a human, so the documented "reply to recover"
// escape is unreachable in practice.
//
// A verdict is evidence about the mail that was in front of it. Mail arriving
// materially later is NEW evidence: it falls outside the disposition's reach, so
// it is not hidden, and it raises its own question to be judged on its own
// merits. The grace period keeps the common case whole — a newsletter that sends
// again the next morning is the same evidence, not new evidence.
//
// Keyed on created_at, the capture clock, NOT on occurred_at: the latter is the
// message's own Date header, as forgeable as the From this whole scope rule
// exists to distrust. A sender who stamped a date a fortnight in the future
// would otherwise fall outside every reach predicate at once and opt their bulk
// mail out of the noise effect entirely.
const noiseVerdictReach = 14 * 24 * time.Hour

// withinVerdictReach is the scope clause that bounds a disposition to the mail
// it is actually evidence about. Composed per query rather than folded into
// noiseMailScope because it carries a duration the const cannot interpolate.
func withinVerdictReach() string {
	return `
	  AND a.created_at <= p.resolved_at + ` + quoteInterval(noiseVerdictReach)
}
