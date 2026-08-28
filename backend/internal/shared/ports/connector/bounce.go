// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "context"

// BounceKind separates the two facts a bounce consumer can act on: a hard
// bounce means the address does not accept mail and retrying is sending to
// nobody; a soft bounce is a temporary refusal (full mailbox, greylisting)
// that says nothing durable about the address.
type BounceKind string

// BounceHard and BounceSoft are the only two kinds a report can carry.
const (
	BounceHard BounceKind = "hard"
	BounceSoft BounceKind = "soft"
)

// BounceReport is what a delivery-status notification says about a message
// this installation previously sent: which message, how final the refusal
// is, and the receiving system's stated reason.
type BounceReport struct {
	// MessageID is the RFC 5322 Message-ID of the ORIGINAL message the
	// report is about — never the report's own id.
	MessageID string
	// Recipient is the address the report says delivery failed FOR
	// (Final-Recipient). The recorder verifies it against the sent row's own
	// recipients: a report that names an address the message never went to
	// is a report about some other message — or a forgery — and records
	// nothing.
	Recipient string
	Kind      BounceKind
	// Reason is the report's Diagnostic-Code line, or empty when the report
	// carried none. External text: bounded and treated as data by every
	// consumer.
	Reason string
}

// BounceSink records a bounce against the outbound message it names. A mail
// connector that captures a delivery report hands it here instead of
// dropping it; the binding decides what "recording" means. Reports naming
// mail the installation never sent are a normal input — a shared mailbox
// sees reports for the owner's own mail client too — and recording one is
// a no-op, not an error.
type BounceSink interface {
	RecordBounce(ctx context.Context, report BounceReport) error
}
