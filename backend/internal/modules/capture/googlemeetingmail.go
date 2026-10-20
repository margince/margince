// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Google's meeting mail: calendar notifications, Meet recordings and
// transcripts, and the notes Gemini takes. It is the evidence that a meeting
// took place, so no model verdict may judge it noise. One noise answer hides
// every later message from that sender, the notes included.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// googleMeetingSenders are the exact From addresses Google sends meeting mail
// from. Exact addresses, not google.com: the rest of that domain is ordinary
// mail a verdict may judge like any other.
var googleMeetingSenders = []string{
	"calendar-notification@google.com",
	"gemini-notes@google.com",
	"meetings-noreply@google.com",
}

// IsGoogleMeetingMail reports whether a message from email is Google's meeting
// mail. authenticated is the message's inbound_sender_authenticated stamp:
// Gmail's receiving server passed DMARC for google.com. A sender types its own
// From, so without the stamp the address proves nothing and the message is
// judged like any other.
func IsGoogleMeetingMail(email string, authenticated bool) bool {
	return authenticated && slices.Contains(googleMeetingSenders, correspondence.Fold(email))
}

// googleMeetingMailTx is IsGoogleMeetingMail for a captured activity, read off
// the stamp the activity row was written with. The stamp speaks for the From
// address only, so email must be that address.
func googleMeetingMailTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID, email string) (bool, error) {
	if !IsGoogleMeetingMail(email, true) {
		return false, nil
	}
	var authenticated bool
	if err := tx.QueryRow(ctx, `
		SELECT inbound_sender_authenticated AND counterparty_email IS NOT DISTINCT FROM $2
		  FROM activity WHERE id = $1`,
		activityID, correspondence.Fold(email)).Scan(&authenticated); err != nil {
		return false, fmt.Errorf("capture: reading whether the message's sender was authenticated: %w", err)
	}
	return authenticated, nil
}

// googleMeetingMailSpared keeps a model's noise verdict, stored ones included,
// off authenticated Google meeting mail; a seat's keep_out (p.machine false)
// still reaches it. The addresses are literals because the scope is shared by
// queries with different placeholder numbering. TestGoogleMeetingSendersAreSQLSafe
// holds them to characters a literal can carry.
func googleMeetingMailSpared() string {
	quoted := make([]string, len(googleMeetingSenders))
	for i, address := range googleMeetingSenders {
		quoted[i] = "'" + address + "'"
	}
	return `
	  AND NOT (p.machine AND a.inbound_sender_authenticated
	           AND a.counterparty_email IN (` + strings.Join(quoted, ", ") + `))`
}
