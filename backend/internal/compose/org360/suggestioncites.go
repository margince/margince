// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What a suggestion cites, and how a citation identifies the situation it
// fired on. The rules in suggestions.go say WHEN to advise; this file is the
// vocabulary of the receipt under the advice — the record's own words, the
// date they are dated, where they came from — and the fingerprint, which is
// deliberately blind to all of that.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A title and a date are OPTIONAL on the wire, so both are pointers. Taken
// through a helper rather than inline, because a suggestion that carried a
// zero time would render as "due 1 January year one" — a date is either the
// evidence's or it is absent.
func ptrString(v string) *string     { return &v }
func ptrTime(v time.Time) *time.Time { return &v }

// nonEmpty is a citation field that is either the record's own words or
// absent. An empty string on the wire would draw an empty quote block, which
// reads as a record that said nothing rather than as words this reader may
// not see.
func nonEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// Where a citation's words came from, in the reader's terms. Server-authored
// like the reason and the title above them, so the three voices on a row
// agree; the client labels the chip with the record's kind and prints the
// date in the reader's calendar, and this says only what neither of those can.
const (
	originStalledDeal   = "Open deal, last worked"
	originOpenDeal      = "Open deal"
	originContractEnded = "Read from their correspondence"
)

// sentOrigin names the channel we spoke on. The rule fires on an OUTBOUND
// exchange only, so "you" is always right; the channel is what makes
// "Email you sent" a claim a reader can check against their own sent folder.
func sentOrigin(kind string) string {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindEmail:
		return "Email you sent"
	case crmcontracts.ActivityKindMessage:
		return "Message you sent"
	case crmcontracts.ActivityKindCall:
		return "Call you made"
	case crmcontracts.ActivityKindMeeting:
		return "Meeting you held"
	default:
		return "Sent by you"
	}
}

// fingerprint identifies a suggestion by what it fired ON, not by what kind
// it is.
//
// That is what lets a dismissal be both durable and self-expiring: the same
// situation stays dismissed, and a changed one raises again on its own. A
// kind-keyed dismissal would bury every future stall on the account, and the
// surface would get quieter the longer it ran regardless of what happened.
func fingerprint(kind, subject string, evidence []crmcontracts.OrganizationBriefEvidence) string {
	parts := []string{kind, subject}
	for _, cited := range evidence {
		parts = append(parts, string(cited.EntityType)+":"+cited.EntityId.String())
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
