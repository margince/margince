// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

// AudienceKey binds a send to the roster the participant reviewed. Names are
// absent: renaming a person cannot change who receives the post.
func AudienceKey(conversation Conversation) (string, error) {
	if !conversation.AudienceKnown || conversation.ID == "" || len(conversation.Members) == 0 {
		return "", ErrUnavailable
	}
	members := slices.Clone(conversation.Members)
	slices.SortFunc(members, func(a, b Member) int { return cmp.Compare(a.ID, b.ID) })
	type recipient struct {
		Bot      bool
		ID       string
		Active   bool
		External bool
	}
	recipients := make([]recipient, 0, len(members))
	for i, m := range members {
		if m.ID == "" || i > 0 && members[i-1].ID == m.ID {
			return "", ErrUnavailable
		}
		recipients = append(recipients, recipient{Bot: m.Bot, ID: m.ID, Active: m.Active, External: m.External})
	}
	encoded, err := json.Marshal(struct {
		Conversation string
		Access       string
		Kind         string
		External     bool
		Recipients   []recipient
	}{Access: conversation.Access, Conversation: conversation.ID, Kind: conversation.Kind, External: conversation.External, Recipients: recipients})
	if err != nil {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// HasActiveMember checks whether the native roster includes a current participant.
func HasActiveMember(members []Member, id string) bool {
	return slices.ContainsFunc(members, func(member Member) bool { return member.ID == id && member.Active })
}

// ConversationLabel keeps a native title or names the other participants in roster order.
func ConversationLabel(conversation Conversation, self string) string {
	if conversation.Name != "" {
		return conversation.Name
	}
	names := []string{}
	for _, member := range conversation.Members {
		if member.ID != self {
			names = append(names, member.Name)
		}
	}
	return strings.Join(names, ", ")
}
