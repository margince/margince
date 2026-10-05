// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Mail that reached the seat's mailbox through a list, a group address or a
// Bcc names none of the seat's addresses on its recipient lines. The receiving
// server's Delivered-To is what says it arrived, and the seat's import of it
// must be recorded all the same.

import (
	"strings"
	"testing"
)

// listMail is a message addressed to a list, with Delivered-To written above or
// below the receiving hop's Received line.
func listMail(deliveredTo string, aboveReceived bool, msgID string) []byte {
	delivered := "Delivered-To: " + deliveredTo
	received := "Received: from mx.example by mail.google.com"
	lines := []string{received, delivered}
	if aboveReceived {
		lines = []string{delivered, received}
	}
	lines = append(lines,
		"From: organiser@customer.example",
		"To: all-partners@customer.example",
		"Subject: partner update",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <"+msgID+">",
		"Content-Type: text/plain", "", "hello", "")
	return []byte(strings.Join(lines, "\r\n"))
}

func TestListMailDeliveredToTheSeatIsRecordedAsTheirImport(t *testing.T) {
	env := newCaptureEnv(t)

	env.sync(t, listMail(captureOwner, true, "list-1@customer.example"))

	if n := importRowsFor(t, env, activityBySource(t, env, "list-1@customer.example"), env.e.Rep1); n != 1 {
		t.Errorf("list mail delivered to %s has %d import rows for the seat, want 1", captureOwner, n)
	}
}

// A Delivered-To below the receiving hop is the sender's own text, and proves
// nothing about whose mailbox the message reached.
func TestAForgedDeliveredToRecordsNoImport(t *testing.T) {
	env := newCaptureEnv(t)

	env.sync(t, listMail(captureOwner, false, "list-2@customer.example"))

	if n := importRowsFor(t, env, activityBySource(t, env, "list-2@customer.example"), env.e.Rep1); n != 0 {
		t.Errorf("a Delivered-To the sender wrote earned %d import rows, want 0", n)
	}
}
