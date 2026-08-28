// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Reading a delivery-status notification (RFC 3464) for the one thing the
// product can act on: WHICH sent message bounced, and how finally.
//
// This is a second pass over the raw bytes rather than a branch inside
// Parse: a DSN never becomes a timeline row (SkipReason drops the
// delivery-system sender before capture), so the two readings share nothing
// but the wire format, and the main parse stays free of a shape it would
// carry for every ordinary message.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/textproto"
	"strings"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// dsnPartCap bounds how much of any single report part is read. A real
// delivery-status part is a few hundred bytes; the cap only exists so a
// hostile message cannot buy an unbounded read.
const dsnPartCap = 64 * 1024

// ParseBounce reads raw as a delivery-status notification and reports what
// bounced. ok is false for anything that is not a parseable DSN naming an
// original message — including the "delivery has been delayed, no action
// required" notices that are a courtesy, not an outcome. The caller decides
// what to do with a report; this only reads one.
func ParseBounce(raw []byte) (connector.BounceReport, bool) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return connector.BounceReport{}, false
	}
	contentType, params, err := reader.Header.ContentType()
	if err != nil || contentType != "multipart/report" || params["report-type"] != "delivery-status" {
		return connector.BounceReport{}, false
	}

	var status dsnStatus
	var originalID string
	for {
		part, err := reader.NextPart()
		if err != nil {
			// EOF or a structural fault: either way the walk is over and
			// whatever was read is what there is.
			break
		}
		partType := partContentType(part)
		switch partType {
		case "message/delivery-status":
			status = readDeliveryStatus(part.Body)
		case "message/rfc822", "text/rfc822-headers":
			originalID = originalMessageID(part.Body)
		}
	}
	if originalID == "" {
		// Some systems put the original id on the report envelope instead of
		// returning the message.
		originalID = strings.Trim(reader.Header.Get("In-Reply-To"), "<> \t")
	}
	if originalID == "" || !status.found {
		return connector.BounceReport{}, false
	}
	kind := connector.BounceSoft
	if status.hard {
		kind = connector.BounceHard
	}
	return connector.BounceReport{MessageID: originalID, Kind: kind, Reason: status.reason}, true
}

// partContentType answers a part's content type whichever header shape the
// reader gave it — the report's machine parts arrive as either, depending on
// how the sending system set Content-Disposition.
func partContentType(part *mail.Part) string {
	switch h := part.Header.(type) {
	case *mail.InlineHeader:
		t, _, err := h.ContentType()
		if err != nil {
			return ""
		}
		return t
	case *mail.AttachmentHeader:
		t, _, err := h.ContentType()
		if err != nil {
			return ""
		}
		return t
	}
	return ""
}

// dsnStatus is what the delivery-status part said about the FIRST recipient
// whose delivery actually failed.
type dsnStatus struct {
	found  bool
	hard   bool
	reason string
}

// readDeliveryStatus reads the RFC 3464 field groups: one per-message group,
// then one group per recipient. A recipient group with Action: failed is a
// bounce; its Status class answers how final (5.x.x permanent → hard,
// anything else → soft), and Diagnostic-Code is the receiving system's own
// sentence about why.
func readDeliveryStatus(body io.Reader) dsnStatus {
	fields := textproto.NewReader(bufio.NewReader(io.LimitReader(body, dsnPartCap)))
	for {
		group, err := fields.ReadMIMEHeader()
		if len(group) == 0 && err != nil {
			return dsnStatus{}
		}
		action := strings.ToLower(strings.TrimSpace(group.Get("Action")))
		if action != "failed" {
			if err != nil {
				return dsnStatus{}
			}
			continue
		}
		return dsnStatus{
			found:  true,
			hard:   strings.HasPrefix(strings.TrimSpace(group.Get("Status")), "5"),
			reason: diagnosticReason(group.Get("Diagnostic-Code")),
		}
	}
}

// diagnosticReason keeps the human half of a Diagnostic-Code, whose defined
// shape is "<type>; <text>" — the type (almost always "smtp") tells an
// operator nothing the text does not.
func diagnosticReason(code string) string {
	if _, text, found := strings.Cut(code, ";"); found {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(code)
}

// originalMessageID reads the returned message's own Message-ID from a
// message/rfc822 or text/rfc822-headers part — both open with header lines,
// which is all this needs.
func originalMessageID(body io.Reader) string {
	fields := textproto.NewReader(bufio.NewReader(io.LimitReader(body, dsnPartCap)))
	header, err := fields.ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) && len(header) == 0 {
		return ""
	}
	return strings.Trim(header.Get("Message-Id"), "<> \t")
}

// RecordIfBounce hands raw to the sink when it is a delivery report naming a
// message, and does nothing otherwise. It is the one spelling of the drop
// branch every mail connector shares: a nil sink is a connector wired without
// bounce recording, and keeps the old behaviour — the report is dropped.
// A sink error propagates, because losing a bounce silently is the exact
// invisibility this path exists to end; the pull retries the message later.
func RecordIfBounce(ctx context.Context, raw []byte, sink connector.BounceSink) error {
	if sink == nil {
		return nil
	}
	report, ok := ParseBounce(raw)
	if !ok {
		return nil
	}
	return sink.RecordBounce(ctx, report)
}
