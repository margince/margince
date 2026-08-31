// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The transactional / ESP suppression registry (CAP-PARAM-6, ADR-0072/A118).
// Some mail arrives over infrastructure that is not the counterparty's company:
// a DocuSign envelope from dse@eu.docusign.net, a conference blast from
// no-reply@event.gitex.com, a SendGrid relay. Naming an organization after that
// domain manufactures junk ("eu.docusign.net" as a company). This gate
// suppresses BOTH person and org derivation for such senders while KEEPING the
// activity — a DocuSign envelope is a real timeline item, it just has no CRM
// counterparty. That is the difference from the free-mail gate (CAP-PARAM-5),
// which suppresses only the org and keeps the person.
//
// Precedence is deliberate and conservative, because a false suppression hides a
// real contact:
//
//   allowlist wins → exact eSLD infra suppresses standalone →
//   subdomain-prefix rule suppresses ONLY with corroboration → otherwise keep.
//
// This is the pure decision only. The Sink runs it after the internal-domain
// gate and records every suppression durably for observability. The T1
// correspondence-positive spare that lets a known contact through lands in
// phase 2b (ADR-0072), on an authenticated provider outbound signal — the
// forgeable From header cannot be trusted to grant a suppression bypass.

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/freemail"
)

// transactionalBaseline is the pinned set of registrable domains (eTLD+1) that
// are mail infrastructure, never a counterparty's own company. Matched exactly
// (against the PSL-normalized eTLD+1), so every subdomain of a listed eSLD is
// covered. Additions land through the spec (CAP-PARAM-6), not ad-hoc edits; a
// deployment appends via margince.yaml capture.transactional_extra.
var transactionalBaseline = map[string]struct{}{
	"sendgrid.net":      {}, // Twilio SendGrid relay
	"sendgrid.com":      {},
	"mailgun.org":       {}, // Mailgun
	"mailgun.info":      {},
	"amazonses.com":     {}, // Amazon SES
	"mandrillapp.com":   {}, // Mailchimp Transactional
	"mcsv.net":          {}, // Mailchimp
	"mcdlv.net":         {},
	"mailchimpapp.net":  {},
	"rsgsv.net":         {}, // Mailchimp campaign
	"postmarkapp.com":   {}, // Postmark
	"sparkpostmail.com": {}, // SparkPost
	"createsend.com":    {}, // Campaign Monitor
	"cmail19.com":       {},
	"docusign.net":      {}, // DocuSign envelope infrastructure
}

// personalServiceDomains are product companies whose mail to a mailbox owner is
// the product talking to its user: expense reports, itineraries, invoices.
//
// They are kept OUT of transactionalBaseline deliberately. That set is read by
// IsMachineAddress, which the attention queue uses to drop rows — and these are
// companies with named salespeople, not relay infrastructure like sendgrid.net.
// Listing them there would hide a real human waiting on a reply, which the
// queue's reader cannot recover from because nothing on the page says somebody
// was hidden.
//
// What they DO justify is refusing to mint a contact from the owner's own
// traffic with the service. Each one put a person in a real CRM — an expense
// tool's "Receipts", an itinerary service's "Plans" — because a founder
// replying to their own receipts reads as correspondence.
var personalServiceDomains = map[string]struct{}{
	"expensify.com":       {},
	"tripit.com":          {},
	"xero.com":            {},
	"concur.com":          {},
	"concursolutions.com": {},
	"docusign.com":        {}, // the product's own mail; docusign.net is its relay
}

// transactionalPrefixes are subdomain labels that MARK a sender subdomain as an
// email-blast lane. A prefix hit alone is NOT enough to suppress — a real
// company can live at news.acme.com — so a prefix suppresses only with
// corroboration (a List-Unsubscribe header or a machine localpart).
var transactionalPrefixes = map[string]struct{}{
	"em":      {},
	"news":    {},
	"event":   {},
	"events":  {},
	"bounce":  {},
	"bounces": {},
	"notify":  {},
	"mailer":  {},
	"mailing": {},
	"e":       {},
	"t":       {},
}

// TransactionalInput names one captured message's sender for the gate.
type TransactionalInput struct {
	// Domain is the counterparty's mail domain (any subdomain depth; lowercased
	// here). Normalized to its eTLD+1 for the exact-infra check.
	Domain string
	// Localpart is the address's local part — the machine-sender corroboration
	// for a prefix rule ("no-reply@event.acme.com").
	Localpart string
	// ListUnsubscribe reports whether the message carried a List-Unsubscribe
	// header — the primary corroboration a prefix rule needs (RFC 2369 bulk mail).
	ListUnsubscribe bool
}

// TransactionalList answers "is this sender mail infrastructure, not a
// counterparty?" against the pinned baseline plus deployment config.
type TransactionalList struct {
	extra map[string]struct{} // capture.transactional_extra additions
	never map[string]struct{} // capture.transactional_never allowlist (wins over every suppression)
}

// NewTransactionalList builds the matcher. extra appends infra eSLDs; never is
// the allowlist of registrable domains an operator declares always-legitimate
// (it wins over every suppression). Both may be nil.
func NewTransactionalList(extra, never []string) *TransactionalList {
	return &TransactionalList{
		extra: normalizedSet(extra),
		never: normalizedSet(never),
	}
}

// Suppress reports whether record creation must be suppressed for this sender,
// and a stable reason breadcrumb (recorded for observability). The activity is
// unaffected — only person/org derivation is gated.
func (l *TransactionalList) Suppress(in TransactionalInput) (bool, string) {
	base := freemail.Registrable(in.Domain)
	if base == "" {
		return false, ""
	}
	if _, allow := l.never[base]; allow {
		// The operator vouched for this domain — never suppress it.
		return false, ""
	}
	if _, hit := transactionalBaseline[base]; hit {
		return true, "transactional_infra:" + base
	}
	if _, hit := l.extra[base]; hit {
		return true, "transactional_infra:" + base
	}
	if prefix, ok := senderPrefix(in.Domain, base); ok {
		if _, listed := transactionalPrefixes[prefix]; listed && corroborated(in) {
			return true, "transactional_prefix:" + prefix
		}
	}
	return false, ""
}

// Allowlisted reports whether an operator declared this domain always-legitimate
// through capture.transactional_never. Every refusal in this package consults it
// first, so one declaration covers the registry and the record-worthiness gate
// rather than only whichever one an author remembered.
func (l *TransactionalList) Allowlisted(domain string) bool {
	base := freemail.Registrable(domain)
	if base == "" {
		return false
	}
	_, allow := l.never[base]
	return allow
}

// corroborated reports whether a prefix-rule sender carries the extra evidence a
// bare prefix cannot supply: a bulk-mail List-Unsubscribe header, or a machine
// localpart. Without it a prefix hit is not enough to suppress.
func corroborated(in TransactionalInput) bool {
	return in.ListUnsubscribe || isMachineLocalpart(in.Localpart)
}

// isMachineLocalpart flags the common no-reply / notification local parts. The
// mail mapper deliberately lets these through — dropping them there would leave
// this rule nothing to judge (ADR-0072 §1: the activity stands, only the
// derivation is suppressed) — so this is where they are recognized.
func isMachineLocalpart(localpart string) bool {
	local := strings.ToLower(strings.TrimSpace(localpart))
	local = strings.ReplaceAll(local, ".", "")
	local = strings.ReplaceAll(local, "-", "")
	local = strings.ReplaceAll(local, "_", "")
	switch local {
	case "noreply", "donotreply", "nreply", "mailerdaemon", "postmaster",
		"bounce", "bounces", "notifications", "notification", "mailer", "newsletter":
		return true
	}
	return false
}

// hasMachineMarker catches the compound no-reply names an exact match misses:
// `esignature-noreply@`, `calendar-notification@`, `jira-no-reply@`. The real
// world writes the marker into a longer local part far more often than it sends
// from a bare `noreply@`, and the exact list alone let a page of e-signature
// and calendar notifications open a rep's day.
//
// Kept to markers that are unambiguous ON THEIR OWN. "notification" and
// "no-reply" mean the same thing wherever they appear in a name; "mail" or
// "info" do not, and a rule that swept those would hide real people.
// machineMarkers are the words a sending SYSTEM names itself with. Listed once:
// the three places below ask about the same vocabulary, and three copies of it
// drift into meaning three different things.
var machineMarkers = map[string]bool{
	"noreply": true, "noreplies": true, "donotreply": true, "notreply": true,
	"nreply": true, "notification": true, "notifications": true, "notify": true,
	"mailerdaemon": true, "autoreply": true, "automailer": true, "automated": true,
}

func hasMachineMarker(localpart string) bool {
	// A marker counts where a sending system puts it: as the whole local part,
	// or at either END of it. Two rules learned from real addresses.
	//
	// Separators are boundaries, not noise: stripping them turned
	// `connor.eply@` into `connoreply` and matched "noreply" inside a person's
	// name.
	//
	// And position matters. `esignature-noreply@` is a machine while
	// `anna.notify.weber@` is a person, so a marker buried in the middle of a
	// name proves nothing — only the ends, where a service names itself.
	local := strings.ToLower(strings.TrimSpace(localpart))
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+'
	})
	if len(parts) == 0 {
		return false
	}
	if machineMarkers[parts[0]] || machineMarkers[parts[len(parts)-1]] {
		return true
	}
	// `no-reply` and `do-not-reply` split into parts that mean nothing alone,
	// so the whole and its tail are re-joined and asked as one word.
	return machineMarkers[strings.Join(parts, "")] ||
		machineMarkers[strings.Join(parts[1:], "")]
}

// senderPrefix returns the leftmost subdomain label below the registrable
// domain — the "sender lane" a prefix rule keys on. "event.gitex.com" over base
// "gitex.com" → "event"; a bare registrable domain has no prefix.
func senderPrefix(domain, base string) (string, bool) {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
	sub := strings.TrimSuffix(domain, "."+base)
	if sub == "" || sub == domain {
		return "", false
	}
	first, _, _ := strings.Cut(sub, ".")
	return first, first != ""
}

// normalizedSet folds a config list into a set keyed the way Suppress keys its
// lookups: the registrable domain, punycoded.
//
// Both sides have to fold the same way. Suppress derives its key through
// freemail.Registrable, which IDNA-folds Unicode labels, so a list entry kept
// in its Unicode spelling would never match again — and the entry that matters
// most is `transactional_never`, the operator's escape hatch for a domain that
// must NOT be suppressed. An entry that silently stops matching there turns a
// deliberate allowlist into a suppression.
// It also refuses anything that is not a hostname. `com` as an infra entry
// would suppress every sender under that suffix, and a malformed one would sit
// in the config doing nothing while looking configured — freemail.Hostname is
// the same floor the consumer-mail lists use.
func normalizedSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if base, ok := freemail.Hostname(v); ok {
			set[base] = struct{}{}
		}
	}
	return set
}

// IsMachineAddress reports whether an address belongs to a machine rather than
// a person: a no-reply style local part, or a domain that exists to send
// transactional mail.
//
// It is the ADDRESS-ONLY half of Suppress, exported for readers that have an
// address and no headers — a queue asking "is a customer waiting on me?"
// cannot ask about `Auto-Submitted` on a row it is ranking, and a notification
// from `noreply@` is not a customer waiting whatever its headers said.
//
// Deliberately NARROWER than Suppress: without the header corroboration a
// prefix rule needs, the subdomain arm is left out. Under-recognising costs a
// row in a queue; over-recognising hides a real customer, and only one of those
// is recoverable by the reader.
func IsMachineAddress(address string) bool {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return false
	}
	if isMachineLocalpart(address[:at]) || hasMachineMarker(address[:at]) {
		return true
	}
	base := freemail.Registrable(address[at+1:])
	if base == "" {
		return false
	}
	_, transactional := transactionalBaseline[base]
	return transactional
}
