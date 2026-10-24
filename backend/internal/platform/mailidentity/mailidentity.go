// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailidentity normalizes mailbox identities and excludes machine-only discoveries.
package mailidentity

import (
	"net/mail"
	"strings"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

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

// foldLocalpart lowercases a local part and drops the separators that spell
// the same mailbox several ways (mailer-daemon, mailer_daemon, mailer.daemon).
func foldLocalpart(localpart string) string {
	local := strings.ToLower(strings.TrimSpace(localpart))
	return strings.NewReplacer(".", "", "-", "", "_", "").Replace(local)
}

// deliverySystemNames are the folded local parts the message-transport system
// itself writes from.
var deliverySystemNames = map[string]bool{
	"mailerdaemon": true, "postmaster": true, "bounce": true, "bounces": true,
}

// IsMachineLocalpart recognizes address local parts that cannot hold a human reply.
func IsMachineLocalpart(localpart string) bool {
	local := foldLocalpart(localpart)
	if deliverySystemNames[local] {
		return true
	}
	switch local {
	case "noreply", "donotreply", "nreply", "notifications", "notification", "mailer", "newsletter":
		return true
	}
	return false
}

// IsDeliverySystemSender flags mail from the message-transport system itself:
// a bounce, a DSN, a postmaster notice. There is no correspondent behind it, so
// capture drops it and the import write door refuses it as received mail.
//
// Narrower than "looks automated". A no-reply or notifications address is a
// real company writing to the workspace (a signed envelope, an invoice). Such a
// message keeps its place on the timeline. The tier gate suppresses the contact
// and company derivation.
func IsDeliverySystemSender(address string) bool {
	local, _, found := strings.Cut(strings.ToLower(address), "@")
	if !found {
		return false
	}
	// BATV/prvs tags sit before any separator a fold would strip. The scheme
	// signs the return path, which this never sees. A relay that rewrites From
	// with a tag has its mail dropped: a timeline row lost, nothing granted.
	if strings.HasPrefix(local, "prvs=") || strings.HasPrefix(local, "msprvs1=") {
		return true
	}
	// VERP encodes the original recipient into the bounce address
	// (bounces-12345@, bounce+tag@), so the plus tag and trailing digits go first.
	local = foldLocalpart(correspondence.LocalPartBase(local))
	return deliverySystemNames[strings.TrimRight(local, "0123456789")]
}

var machineMarkers = map[string]bool{
	"noreply": true, "noreplies": true, "donotreply": true, "notreply": true,
	"nreply": true, "notification": true, "notifications": true, "notify": true,
	"mailerdaemon": true, "autoreply": true, "automailer": true, "automated": true,
	"receipt": true, "receipts": true,
}

// HasMachineMarker keeps separators and marker positions so personal names such as
// anna.notify.weber are not mistaken for machine senders such as calendar-notification.
func HasMachineMarker(localpart string) bool {
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
	return machineMarkers[strings.Join(parts, "")] ||
		machineMarkers[strings.Join(parts[1:], "")]
}

// notificationHosts are full mail hosts that only a groupware service's
// document, drive and calendar notices come from. They are matched whole, never
// by registrable domain, because a human sender can hold a mailbox at the base domain.
var notificationHosts = map[string]bool{
	"docs.google.com":     true,
	"drive.google.com":    true,
	"calendar.google.com": true,
}

// tokenReplyMinLength is the shortest opaque token after "<letter>+" that
// counts as a reply-routing token. A human sender's own name+tag address is far
// shorter.
const tokenReplyMinLength = 32

// isTokenReplyLocalpart matches the reply address a notification mailer mints:
// one letter, a plus, and a long opaque token (d+<token>).
func isTokenReplyLocalpart(local string) bool {
	return len(local) >= 2+tokenReplyMinLength && local[1] == '+' &&
		(local[0] >= 'a' && local[0] <= 'z' || local[0] >= 'A' && local[0] <= 'Z')
}

// machineShape is the one rule for "this address belongs to a sending system":
// a marker local part, a token reply address, or a notification host.
func machineShape(address string) bool {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return false
	}
	local, host := address[:at], strings.ToLower(address[at+1:])
	return IsMachineLocalpart(local) || HasMachineMarker(local) ||
		isTokenReplyLocalpart(local) || notificationHosts[host]
}

// HasMachineLocalpart reports whether an address names a sending system rather
// than a human, by its local part or its notification host. No domain
// allowlist overrules it: nobody answers at noreply@ on any domain.
func HasMachineLocalpart(address string) bool { return machineShape(address) }

// IsMachineAddress reports mail infrastructure and machine-only sender addresses.
func IsMachineAddress(address string) bool {
	if machineShape(address) {
		return true
	}
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return false
	}
	base := freemail.Registrable(address[at+1:])
	if base == "" {
		return false
	}
	_, transactional := transactionalBaseline[base]
	return transactional
}

// Infrastructure reports a registrable domain reserved for transactional mail.
func Infrastructure(domain string) bool { _, ok := transactionalBaseline[domain]; return ok }

// BareAddress extracts the address from a provider display label.
func BareAddress(label string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(label))
	if err != nil {
		return label
	}
	return parsed.Address
}

// ReaderAddress folds a declared identity without reviving a machine-only discovery.
func ReaderAddress(label, source string) string {
	if DiscoveredMachineAddress(source, label) {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(BareAddress(label)))
}

// DiscoveredMachineAddress rejects only machine identities inferred from delivery.
func DiscoveredMachineAddress(source, label string) bool {
	return source == "delivered_to" && IsMachineAddress(BareAddress(label))
}

// InboundFromSendingSystem is an activity's sender_is_machine stamp: a
// received email whose From address names a sending system. Capture, the
// create door and the mailbox take-over all ask it.
func InboundFromSendingSystem(kind, direction, from string) bool {
	return kind == "email" && direction == "inbound" && IsMachineAddress(from)
}
