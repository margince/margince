// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"

	"github.com/emersion/go-message/mail"
)

// googleGroupSender answers who wrote a message a Google Group delivered, and
// whether it was one.
//
// A group rewrites From to itself — `"Henry via Gradion Info" <info@…>` — so a
// mail an outsider sent to a shared address reads as the group writing to
// itself: internal, and dropped, or captured with the group as the
// counterparty. The group keeps the author in X-Original-From. Both headers are
// the sender's text when the mail did not pass through a group, exactly as From
// is, so trusting them claims no more than reading From already does.
func googleGroupSender(header mail.Header) ([]*mail.Address, bool) {
	if strings.TrimSpace(header.Get("X-Google-Group-Id")) == "" {
		return nil, false
	}
	original, err := header.AddressList("X-Original-From")
	if err != nil || len(original) == 0 || strings.TrimSpace(original[0].Address) == "" {
		return nil, false
	}
	return original, true
}

// groupListUnsubscribe reports whether a group-delivered message carries a
// List-Unsubscribe of its own. A group stamps its own unsubscribe links on
// every post, which says the GROUP is a list and nothing about the author; only
// a link to somewhere else marks the post itself as bulk mail.
func groupListUnsubscribe(value string) bool {
	for _, link := range strings.Split(value, ",") {
		link = strings.ToLower(strings.TrimSpace(link))
		if link == "" {
			continue
		}
		if !strings.Contains(link, "googlegroups.com") && !strings.Contains(link, "groups.google.com") {
			return true
		}
	}
	return false
}

// hasListUnsubscribe is the List-Unsubscribe reading for one message: present
// at all for ordinary mail, and one of the author's own for a group post.
func hasListUnsubscribe(value string, viaGroup bool) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	return !viaGroup || groupListUnsubscribe(value)
}
