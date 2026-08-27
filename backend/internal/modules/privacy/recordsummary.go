// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// One audit row as a plain-language sentence: who, and what they did.
//
// Split from the paging read because the SUBJECT half now has two predicates in
// front of it — "… updated the record" and edgesummary.go's "… linked Acme as
// cto" — and attribution that read one way on a record line and another on an
// edge line, on the same page, is the one thing this phrasing exists to prevent.

import "fmt"

// machineQualifier names the tool a delegated change was typed through, as
// the phrase that qualifies the PERSON who authorized it — "via an agent",
// not "an agent".
//
// It is the FALLBACK now. When the passport came from an OAuth grant the line
// names the client instead — "Demo Admin, via Claude, created the record" —
// because "via an agent" on every row of a company's history tells a rep
// nothing they did not already know, and the question they actually have is
// which tool did it.
//
// The name comes from oauth_client.client_name, not from passport.label. The
// concern the generic word was protecting against — a revoked passport's label
// outliving the grant on every row it ever wrote — does not apply: client_name
// is the registered identity of the tool, it does not change when a grant is
// revoked, and a client row that is gone falls back to this map.
var machineQualifier = map[string]string{
	actorTypeAgent:     "via an agent",
	actorTypeConnector: "via a connector",
}

// composeRecordSummary renders one audit row as a plain-language sentence,
// the record-history read's `summary` field. It is pure: callers resolve
// actorDisplayName/onBehalfOfName (app_user lookups) before calling in, so
// this stays testable without a database. onBehalfOfName is set only for a
// machine acting under a human's delegated authority (D2's authority
// weaving); an empty string is treated the same as nil — a resolved-but-
// blank name is not authority to report.
//
// The sentence NAMES THE PERSON FIRST and says a machine did the typing
// second (PD-002). A rep working through a passport is the rep: the line
// reads "Devin, via an agent, archived the record", never "an agent archived
// the record" with the person demoted to a trailing phrase. Attribution
// exists so somebody can be asked about a change, and a machine is not a
// party to anything — a line whose subject is the tool lets every human in
// the chain disclaim it.
func composeRecordSummary(actorType, actorDisplayName string, onBehalfOfName *string,
	action string, passportBacked bool, agentClientName *string,
) string {
	verb := recordHistoryVerbs[action]
	if verb == "" {
		verb = action
	}
	subject := recordSummarySubject(actorType, actorDisplayName, onBehalfOfName, passportBacked, agentClientName)
	return fmt.Sprintf("%s %s the record", subject, verb)
}

// recordSummarySubject is the sentence's SUBJECT — everything before the verb.
// Separated because an edge line needs the same subject in front of a different
// predicate ("Uma, via Claude, linked Acme as cto"), and attribution that read
// one way on a record line and another on an edge line, on the same page, would
// be the defect: the whole point of the phrasing is that a person can be asked
// about a change.
func recordSummarySubject(actorType, actorDisplayName string, onBehalfOfName *string,
	passportBacked bool, agentClientName *string,
) string {
	if qualifier, delegated := machineQualifier[actorType]; delegated && onBehalfOfName != nil && *onBehalfOfName != "" {
		if agentClientName != nil && *agentClientName != "" {
			qualifier = "via " + *agentClientName
		}
		// The trailing comma belongs to the SUBJECT: it closes the appositive that
		// names the tool, and every predicate that follows one needs it —
		// "Devin, via an agent, archived the record", "…, linked Acme as cto".
		return fmt.Sprintf("%s, %s,", *onBehalfOfName, qualifier)
	}
	switch {
	case actorType == actorTypeHuman:
		return actorDisplayName
	case passportBacked:
		// A PASSPORT was presented and yet no human resolved behind it. That
		// is a gap: passport.on_behalf_of is NOT NULL, so the authority
		// existed when the grant was made and the row failed to carry it (the
		// pre-0260 scheduled sends in compose/commsscheduled.go are the live
		// example). The line says so rather than falling back to "System",
		// which is reserved for a change that genuinely has nobody behind it —
		// letting system absorb a failed attribution would hide the gap on the
		// one surface that exists to expose it.
		return "A machine with no recorded human authority"
	case actorType == actorTypeSystem:
		return "System"
	case actorType == actorTypeAgent:
		// A background writer with no passport: an installation-wide pass that
		// nobody's context ran, so there is no human to name and no gap to
		// report. compose/extjobsrun.go writes one per extension job tick.
		// Calling this a missing authority would report a defect where there
		// is none — the distinction that matters is whether a grant was
		// presented, not which machine word the actor_type happens to carry.
		return "Agent"
	case actorType == actorTypeConnector:
		// Same reasoning as the bare agent above: some connectors have no
		// connect flow and therefore no granting human by design
		// (compose/jobs_finance.go writes one).
		return "Connector"
	default:
		return actorType
	}
}
