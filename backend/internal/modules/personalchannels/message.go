// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The two directions a member's chat message can take.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// genericNetwork names a message whose client did not say which chat it came from.
const genericNetwork = "generic"

// networkGrammar keeps a network token short and plain, since it is part of an
// identity and of a thread key.
var networkGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Party is one end of a chat as the client names it: an account and a name. It
// carries no address. An address is a merge key, and a caller could aim it at a
// contact who is somebody else.
type Party struct {
	Account string
	Name    string
}

// PushMessage is one chat message a member's client sends in.
type PushMessage struct {
	MessageID, ThreadID, GroupID string
	Network, Direction           string
	Subject, Body                string
	OccurredAt                   time.Time
	From, To                     Party
	Participants                 []Party
	UploadIDs                    []ids.UUID
}

// NormalizedMessage is a PushMessage keyed for capture.
type NormalizedMessage struct {
	PushMessage
	Connection ids.UUID
	// Account is the counterparty's identity on this provider:
	// network/account/connection.
	Account   string
	Name      string
	ThreadKey string
	SourceID  string
}

// Normalize keys m for capture under connection, or says why it cannot be.
//
// The thread key keeps the counterparty's identity as its third segment, because
// erasure finds a subject's unlinked messages by that segment.
func Normalize(connection ids.UUID, m PushMessage) (NormalizedMessage, error) {
	n := NormalizedMessage{PushMessage: m, Connection: connection}
	if n.Network == "" {
		n.Network = genericNetwork
	}
	if n.Direction == "" {
		n.Direction = DirectionInbound
	}
	other := m.From
	switch n.Direction {
	case DirectionInbound:
	case DirectionOutbound:
		other = m.To
	default:
		return n, invalid("direction %q is neither inbound nor outbound", n.Direction)
	}
	switch {
	case !networkGrammar.MatchString(n.Network):
		return n, invalid("network %q must be lower-case letters, digits and _ (at most 32)", n.Network)
	case strings.TrimSpace(m.MessageID) == "":
		return n, invalid("a message needs its message_id, which makes a resend land nothing new")
	case strings.TrimSpace(other.Account) == "":
		return n, invalid("a message needs the other party's account")
	case strings.ContainsAny(other.Account, ":/") || strings.Contains(m.GroupID, ":"):
		return n, invalid("an account may not contain ':' or '/', nor a group id ':'; send the bare account")
	}
	n.Account = identity(connection, n.Network, other.Account)
	n.Name = other.Name
	n.ThreadKey = Provider + ":" + connection.String() + ":" + n.Account
	if m.GroupID != "" {
		n.ThreadKey += ":" + m.GroupID
	}
	n.SourceID = connection.String() + ":" + n.Network + ":" + m.MessageID
	return n, nil
}

// identity is a party's channel identity on this provider. It names the member's
// connection too, because nobody attests an account a client typed. The same
// account from another member's client is another identity. It never reaches a
// contact that member's channel brought in.
func identity(connection ids.UUID, network, account string) string {
	return network + "/" + account + "/" + connection.String()
}

// ParticipantIdentity is the identity of another party in m's conversation.
func ParticipantIdentity(m NormalizedMessage, account string) string {
	return identity(m.Connection, m.Network, account)
}

// SplitIdentity reads the network and account back out of an identity.
func SplitIdentity(id string) (network, account string, ok bool) {
	parts := strings.Split(id, "/")
	if len(parts) < 3 {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:len(parts)-1], "/"), true
}

func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, apperrors.ErrInvalidArgument)...)
}
