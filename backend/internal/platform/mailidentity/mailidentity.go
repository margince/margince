// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailidentity normalizes mailbox identities and excludes machine-only discoveries.
package mailidentity

import (
	"net/mail"
	"strings"

	"github.com/margince/margince/backend/internal/platform/freemail"
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

// IsMachineLocalpart recognizes address local parts that cannot hold a human reply.
func IsMachineLocalpart(localpart string) bool {
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

// HasMachineLocalpart reports whether an address's local part names a sending
// system rather than a human. No domain allowlist overrules it: nobody answers
// at noreply@ on any domain.
func HasMachineLocalpart(address string) bool {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return false
	}
	local := address[:at]
	return IsMachineLocalpart(local) || HasMachineMarker(local)
}

// IsMachineAddress reports mail infrastructure and machine-only sender addresses.
func IsMachineAddress(address string) bool {
	if HasMachineLocalpart(address) {
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
