// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// This unit's failure vocabulary: one entry for every way its drain can fail
// that an operator would act on DIFFERENTLY.
//
// WHY THE SENTENCES LIVE HERE rather than being composed from the cause: a job
// failure reaches an admin through river_job.errors, a fleet-wide column every
// workspace's admin reads. The job layer therefore persists nothing but a
// sentence from a closed vocabulary — and until a unit declares one, a tick that
// had already worked out what was broken could report only that its failure was
// unclassifiable.
//
// EVERY STRING BELOW IS ONE THIS UNIT WROTE. Nothing an anonymous sender put in
// a request body can reach that column through this file, because nothing a
// sender said is in it — which matters more here than in a connector that talks
// to a provider somebody chose, since the bodies this unit drains arrive from
// parties with no session at all.
//
// THE SAME TOKENS ARE last_error_class ON THE QUEUE ROW, deliberately: the
// connector screen and the Maintenance screen describe the same stall, and an
// operator comparing them must read one fact rather than two vocabularies.
//
// TERMINAL VERSUS RETRYABLE is the other axis, and it is not carried in the
// class — see drainfailure.go. A class names WHAT went wrong; whether a row can
// ever land is a separate question, and folding the two together would leave no
// way to say that a payload nothing will ever accept and a member whose role was
// restored this morning both failed "invalid".

import "github.com/margince/margince/backend/pkg/extension"

var (
	// classPayloadUnusable is a stored body this connector cannot build a record
	// from: no message id, or no party it could name. It will never land, so the
	// row parks on the first attempt rather than on the third.
	classPayloadUnusable = extension.FailureClass{
		Class:    "payload_unusable",
		Sentence: "a received request carried nothing this connector could turn into a timeline entry",
		Remedy:   "The sending system is posting a shape this connector does not accept. The parked request stays on the member's own openchannel screen; whoever runs that sender has to correct what it posts.",
	}
	// classRefusedByTheCore is the core refusing the built record as invalid —
	// over-long text, a field it will not take. Also terminal: the same bytes
	// build the same record on every attempt.
	classRefusedByTheCore = extension.FailureClass{
		Class:    "refused_by_the_core",
		Sentence: "the CRM refused a captured message as invalid, so no timeline entry was created for it",
		Remedy:   "The refusal is in this job's process log. A request that lands here is one the sending system must post differently; retrying it unchanged cannot help, so it is parked rather than reattempted.",
	}
	// classMemberNotPermitted is the OWNER's own authority having gone. Every
	// record lands on what that member may do right now, so demoting or
	// archiving them stops their queue draining without anything about this
	// connector changing. Retryable: restoring the role is what fixes it.
	classMemberNotPermitted = extension.FailureClass{
		Class:    "member_not_permitted",
		Sentence: "the member an arrived request was captured for may no longer capture what it lands",
		Remedy:   "Restore that member's role, or pause their openchannel endpoint if they are not meant to capture into the CRM any more — a paused endpoint stops new requests arriving behind the stalled ones.",
	}
	// classCaptureNotDeclared is this unit reaching the capture port and being
	// told it may not: the ingress declaration is missing, or the tick reached it
	// on the wrong kind of invocation. Nothing about a request causes it and no
	// number of attempts changes it, so it FAILS rather than postponing — a
	// postponement would hide a mis-composed installation behind a row that looks
	// like it is waiting patiently.
	classCaptureNotDeclared = extension.FailureClass{
		Class:    "capture_not_declared",
		Sentence: "this connector was refused at the capture port, so nothing it received can reach the timeline",
		Remedy:   "This installation's composition does not admit openchannel's ingress. Nothing on any screen will fix it; report it with this job's process log.",
	}
	// classCaptureUnavailable is the fleet-wide outage that needs NO
	// intervention, and saying so is the point: no row was advanced, so the next
	// tick drains the same requests and nothing is lost.
	classCaptureUnavailable = extension.FailureClass{
		Class:    "capture_unavailable",
		Sentence: "the CRM's capture pipeline could not be reached, so nothing that arrived was landed this pass",
		Remedy:   "Nothing to do: the drain catches up by itself and no received request is lost. If every tick reports it, this installation's database is what to look at.",
	}
	// classEveryRequestFailed is the mixed case, and it exists because one tick
	// drains many members' requests rather than exactly one.
	//
	// It is reported ONLY when nothing landed AND the stalled requests did not
	// stall the same way. Everything stalling identically is not a fleet
	// condition needing its own name — it is that one condition happening
	// everywhere, and reporting the shared class is what turns a screenful of
	// dead jobs into a sentence naming the thing to fix. Requests stalling for
	// DIFFERENT reasons is the genuinely different situation: nothing is common
	// to them, so there is no single outage to chase.
	classEveryRequestFailed = extension.FailureClass{
		Class:    "every_request_failed",
		Sentence: "nothing in the queue could be landed this pass, and not every request stalled for the same reason",
		Remedy:   "Read each request's own class on the members' openchannel screens: the stalls have nothing in common, so there is no single outage behind them.",
	}
	// classDrainFailed is the catch-all, and it is honest about being one. Its
	// remedy has to say where to look next, because a class that names nothing
	// and points nowhere is the sentence this whole file exists to stop printing.
	classDrainFailed = extension.FailureClass{
		Class:    "drain_failed",
		Sentence: "the openchannel drain failed for a cause this connector does not yet classify",
		Remedy:   "Read the cause in this job's process log. A failure that keeps landing here is one this connector owes a name, so report it with that line.",
	}
	// classDeliveryRefused is one of the OUTBOUND direction's classes. It is
	// declared beside the drain's because a unit declares ONE vocabulary — but no
	// job returns it: a send runs on the product's own delivery ladder, and what
	// reaches an operator there is the delivery, not a River row. It is here
	// because it is what the outbound attempt ledger records, and one spelling
	// for one failure is the rule this file exists for.
	classDeliveryRefused = extension.FailureClass{
		Class:    "delivery_refused",
		Sentence: "a member's registered address refused a message this connector posted to it",
		Remedy:   "The member's own system answered and declined. Their openchannel screen lists the attempt; whoever runs the receiving system has to say why it refused.",
	}
	// classDeliveryUnanswered is the POST that went out with no usable answer
	// coming back. It is a class rather than a degree of the one above because
	// the message may be at the recipient and may not, and no later attempt can
	// find out.
	classDeliveryUnanswered = extension.FailureClass{
		Class:    "delivery_unanswered",
		Sentence: "a message left for a member's registered address and no answer to it ever came back",
		Remedy:   "It is stopped rather than retried, because a repeat would deliver twice with nothing able to detect it. Whoever runs the receiving system can confirm from their side whether it arrived.",
	}
	// classDeliveryBlocked is THIS INSTALLATION declining to dial the address a
	// member registered, because it resolved to something not publicly routable.
	// Separate from the two above because the remedy is ours and not the
	// receiver's, and because nothing was transmitted: recording it as
	// unanswered would park a delivery that certainly never left, and point the
	// operator at a system that was never called.
	classDeliveryBlocked = extension.FailureClass{
		Class:    "delivery_blocked",
		Sentence: "a member's registered address resolved to somewhere this installation will not post to",
		Remedy:   "Nothing was sent. The address resolves to a private, reserved or loopback address; the member has to register one that resolves publicly.",
	}
)

// failureClasses is the set this unit declares, in the order an operator meets
// them: what a human must fix first, then what fixes itself, then the two
// catch-alls, then the outbound direction's own classes.
//
// It is ONE list and every other reference is to it, so a class that exists in
// the code and not in the declaration cannot happen — an undeclared class reaches
// the wire as the unvetted substitute, which is exactly the vague sentence this
// unit is getting rid of.
var failureClasses = []extension.FailureClass{
	classPayloadUnusable,
	classRefusedByTheCore,
	classMemberNotPermitted,
	classCaptureNotDeclared,
	classCaptureUnavailable,
	classEveryRequestFailed,
	classDrainFailed,
	classDeliveryRefused,
	classDeliveryUnanswered,
	classDeliveryBlocked,
}
