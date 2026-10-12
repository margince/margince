// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const groupOwner = "rep@ourco.example"

// groupPost is a message a Google Group delivered: From rewritten to the group,
// the author kept in X-Original-From, and the group's own list headers added.
func groupPost(extra ...string) []byte {
	lines := append([]string{
		`From: "'Henry Example' via Ourco Info" <info@ourco.example>`,
		"To: info@ourco.example",
		`X-Original-From: "Henry Example" <henry@shop.example>`,
		"X-Original-Sender: henry@shop.example",
		`Reply-To: "Henry Example" <henry@shop.example>`,
		"X-Google-Group-Id: 217077506295",
		"Precedence: list",
		"Mailing-list: list info@ourco.example; contact info+owners@ourco.example",
		"List-ID: <info.ourco.example>",
		"Subject: Kurzer Austausch morgen?",
		"Date: Thu, 01 Oct 2026 17:27:00 +0200",
		"Message-ID: <group-1@shop.example>",
	}, extra...)
	lines = append(lines, "Content-Type: text/plain", "", "Hallo zusammen", "")
	return []byte(strings.Join(lines, "\r\n"))
}

const googleUnsubscribe = "List-Unsubscribe: <mailto:googlegroups-manage+1+unsubscribe@googlegroups.com>," +
	" <https://groups.google.com/a/ourco.example/group/info/subscribe>"

func TestAGroupPostIsTheAuthorsMailNotTheGroups(t *testing.T) {
	msg, err := Parse(groupPost(googleUnsubscribe), groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	rec := msg.ToRecord("gmail", nil)
	cp := rec.Counterparty
	if cp.Email != "henry@shop.example" || cp.Direction != connector.DirectionInbound {
		t.Errorf("a group post reads as %s mail with %q, want inbound from henry@shop.example", cp.Direction, cp.Email)
	}
	// The group's own unsubscribe links make the GROUP a list, not the author's
	// mail a newsletter.
	if cp.ListUnsubscribe {
		t.Error("an author's post through a group was attested as bulk mail by the group's own unsubscribe links")
	}
	if !slices.Contains(rec.Addresses, "henry@shop.example") {
		t.Errorf("the author is missing from the message's parties %v, so it would read as internal", rec.Addresses)
	}
}

// A newsletter sent to a group keeps the unsubscribe link of whoever sent it.
func TestANewsletterThroughAGroupIsStillBulk(t *testing.T) {
	msg, err := Parse(groupPost("List-Unsubscribe: <https://esp.example/u/123>, "+
		"<mailto:googlegroups-manage+1+unsubscribe@googlegroups.com>"), groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.ToRecord("gmail", nil).Counterparty.ListUnsubscribe {
		t.Error("a newsletter's own unsubscribe link was dropped because a group delivered it")
	}
}

// Without the group's id the message did not come through a group, and an
// X-Original-From line is ignored: From stands, as it does for any mail.
func TestAnOriginalFromWithoutAGroupChangesNothing(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: sender@elsewhere.example",
		"To: " + groupOwner,
		"X-Original-From: someone-else@shop.example",
		"Subject: hi",
		"Date: Thu, 01 Oct 2026 17:27:00 +0200",
		"Message-ID: <nogroup-1@elsewhere.example>",
		"Content-Type: text/plain", "", "hi", "",
	}, "\r\n"))
	msg, err := Parse(raw, groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "sender@elsewhere.example" {
		t.Errorf("an X-Original-From outside a group moved the counterparty to %q", got)
	}
}

// A link that only mentions Google Groups somewhere in its text is the
// sender's own, and keeps the message bulk.
func TestALinkThatOnlyMentionsGoogleGroupsIsStillTheSenders(t *testing.T) {
	msg, err := Parse(groupPost("List-Unsubscribe: <https://esp.example/u?r=groups.google.com>"), groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.ToRecord("gmail", nil).Counterparty.ListUnsubscribe {
		t.Error("an unsubscribe link naming groups.google.com only in its query was taken for the group's own")
	}
}

// A group id on a message whose From is not the group the List-ID names is not
// a group rewrite, and its X-Original-From is ignored.
func TestAGroupIdOnMailFromSomewhereElseChangesNothing(t *testing.T) {
	raw := strings.Replace(string(groupPost()),
		`From: "'Henry Example' via Ourco Info" <info@ourco.example>`, "From: sender@elsewhere.example", 1)
	msg, err := Parse([]byte(raw), groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "sender@elsewhere.example" {
		t.Errorf("a group id on mail from elsewhere moved the counterparty to %q", got)
	}
}

// A second List-Unsubscribe the sender's own mail carried counts, whichever
// occurrence a reader happens to see first.
func TestEveryUnsubscribeHeaderOnAGroupPostIsRead(t *testing.T) {
	msg, err := Parse(groupPost(googleUnsubscribe, "List-Unsubscribe: <https://esp.example/u/123>"), groupOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.ToRecord("gmail", nil).Counterparty.ListUnsubscribe {
		t.Error("the sender's own unsubscribe header was missed behind the group's")
	}
}
