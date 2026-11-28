// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// LocateMessage resolves a message reference within the selected native channel.
func (b *Bot) LocateMessage(_ context.Context, channel, reference string) (chatwire.MessageLocation, error) {
	selected := strings.TrimSpace(reference)
	team, room, ok := strings.Cut(channel, "/")
	if !ok || team != b.team {
		return chatwire.MessageLocation{}, messaging.ErrInvalid
	}
	selected, root, err := teamsMessageReference(selected, room, team, b.graph.tenant)
	if err != nil {
		return chatwire.MessageLocation{}, err
	}
	if root == "" {
		root = selected
	}
	for _, id := range []string{selected, root} {
		if id == "" || len(id) > 1024 || strings.ContainsAny(id, "/\\?#'\" ") {
			return chatwire.MessageLocation{}, messaging.ErrInvalid
		}
	}
	return chatwire.MessageLocation{Message: selected, Root: root}, nil
}

func teamsMessageReference(selected, room, team, expectedTenant string) (string, string, error) {
	root := ""
	if strings.Contains(selected, "://") {
		link, err := url.Parse(selected)
		if err != nil || link.Scheme != httpsScheme || link.Host != "teams.microsoft.com" || link.User != nil {
			return "", "", messaging.ErrInvalid
		}
		parts := strings.Split(strings.Trim(link.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "l" || parts[1] != messageKind || parts[2] != room {
			return "", "", messaging.ErrInvalid
		}
		query := link.Query()
		if group := query.Get("groupId"); group != "" && group != team {
			return "", "", messaging.ErrInvalid
		}
		if tenant := query.Get("tenantId"); tenant != "" && tenant != expectedTenant {
			return "", "", messaging.ErrInvalid
		}
		selected, root = parts[3], query.Get("parentMessageId")
	}
	return selected, root, nil
}
