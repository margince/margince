// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

var slackReference = regexp.MustCompile(`<([^<>]+)>`)

func readableText(text string, members []messaging.Member) string {
	names := map[string]string{}
	for _, member := range members {
		names[member.ID] = member.Name
	}
	return html.UnescapeString(slackReference.ReplaceAllStringFunc(text, func(reference string) string {
		value := strings.TrimSuffix(strings.TrimPrefix(reference, "<"), ">")
		if id, ok := strings.CutPrefix(value, "@"); ok {
			if name := names[id]; name != "" {
				return "@" + name
			}
			return "@" + id
		}
		address, label, named := strings.Cut(value, "|")
		if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "http://") {
			if named {
				return label + " (" + address + ")"
			}
			return address
		}
		if strings.HasPrefix(address, "#") && named {
			return "#" + label
		}
		return reference
	}))
}

func slackMessageText(fallback string, blocks, attachments []json.RawMessage, members []messaging.Member) (string, bool, error) {
	parts := []string{}
	seen := map[string]bool{}
	add := func(text string) {
		text = strings.TrimSpace(readableText(text, members))
		if text != "" && !seen[text] {
			parts = append(parts, text)
			seen[text] = true
		}
	}
	add(fallback)
	incomplete := false
	for _, content := range [][]json.RawMessage{blocks, attachments} {
		if len(content) == 0 {
			continue
		}
		raw, err := json.Marshal(content)
		if err != nil {
			return "", false, messaging.ErrUnavailable
		}
		text, complete := messaging.RichContent(raw)
		add(text)
		incomplete = incomplete || !complete
	}
	if incomplete {
		add("[Some structured content was not read]")
	}
	return strings.Join(parts, "\n"), incomplete, nil
}
