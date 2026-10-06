// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"strings"
	"testing"
)

// An outsider's mail to a shared address, as the seat's mailbox receives it
// from a Google Group: From rewritten to the group, the author kept in
// X-Original-From, the group's own list headers added.
func groupDeliveredMail(msgID string) []byte {
	return []byte(strings.Join([]string{
		"Delivered-To: " + captureOwner,
		"Received: by 2002:a05:7412:8082 with SMTP id f2csp2016359rda",
		`From: "'Henry Example' via Myco Info" <info@myco.example>`,
		"To: info@myco.example",
		`X-Original-From: "Henry Example" <henry@shop.example>`,
		`Reply-To: "Henry Example" <henry@shop.example>`,
		"X-Google-Group-Id: 217077506295",
		"Precedence: list",
		"List-ID: <info.myco.example>",
		"List-Unsubscribe: <mailto:googlegroups-manage+1+unsubscribe@googlegroups.com>",
		"Subject: Kurzer Austausch morgen?",
		"Date: Thu, 01 Oct 2026 17:27:00 +0200",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "Hallo zusammen", "",
	}, "\r\n"))
}

// Captured as the author's correspondence: not dropped as the group writing to
// itself, not filed under the group, and not a newsletter.
func TestMailAnOutsiderSentThroughAGroupIsCapturedAsTheirs(t *testing.T) {
	env := newCaptureEnv(t)

	env.sync(t, groupDeliveredMail("group-1@shop.example"))

	if n := countRows(t, env.e, `
		SELECT count(*) FROM activity
		 WHERE source_id = 'group-1@shop.example' AND direction = 'inbound'
		   AND counterparty_email = 'henry@shop.example' AND NOT bulk_mail_attested`); n != 1 {
		t.Errorf("found %d activities from henry@shop.example as personal inbound mail, want 1", n)
	}
}
