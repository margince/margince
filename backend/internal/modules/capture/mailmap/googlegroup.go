// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"net/url"
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
//
// The rewrite is honoured only where the message agrees with itself about the
// group: From is the address the List-ID names, which is how a group spells
// itself on every post.
func googleGroupSender(header mail.Header) ([]*mail.Address, bool) {
	if strings.TrimSpace(header.Get("X-Google-Group-Id")) == "" {
		return nil, false
	}
	from, err := header.AddressList("From")
	if err != nil || len(from) != 1 || !groupNamesItself(from[0].Address, header.Get("List-ID")) {
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
	for link := range strings.SplitSeq(value, ",") {
		link = strings.Trim(strings.TrimSpace(link), "<>")
		if link == "" {
			continue
		}
		if !isGoogleGroupsLink(link) {
			return true
		}
	}
	return false
}

// isGoogleGroupsLink reports a mailto to googlegroups.com or an https link to
// groups.google.com, judged by the address's or URL's own host — never by the
// text appearing somewhere in the link, which a sender could put in a query.
func isGoogleGroupsLink(link string) bool {
	if address, ok := strings.CutPrefix(strings.ToLower(link), "mailto:"); ok {
		address, _, _ = strings.Cut(address, "?")
		at := strings.LastIndex(address, "@")
		return at >= 0 && address[at+1:] == "googlegroups.com"
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "groups.google.com")
}

// groupNamesItself reports whether address is the group the List-ID names:
// Google writes the group's address with its @ turned into a dot, so
// `asia.sales@ourco.example` is `<asia.sales.ourco.example>`.
func groupNamesItself(address, listID string) bool {
	id := strings.ToLower(strings.Trim(strings.TrimSpace(listID), "<>"))
	address = strings.ToLower(strings.TrimSpace(address))
	return id != "" && strings.Contains(address, "@") && id == strings.Replace(address, "@", ".", 1)
}

// hasListUnsubscribe is the List-Unsubscribe reading for one message, over
// every occurrence of the header: present at all for ordinary mail, and one of
// the author's own for a group post.
func hasListUnsubscribe(values []string, viaGroup bool) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if !viaGroup || groupListUnsubscribe(value) {
			return true
		}
	}
	return false
}
