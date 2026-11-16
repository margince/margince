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

// Party is one end of a chat as the client names it: an account and a name, and
// never an address, because an address is a merge key a caller could aim at a
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
	// Account is the counterparty's identity on this provider: network/account.
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
	n := NormalizedMessage{PushMessage: m}
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
	case strings.Contains(other.Account, ":") || strings.Contains(m.GroupID, ":"):
		return n, invalid("an account or group id may not contain ':'; send the bare account")
	}
	n.Account = n.Network + "/" + other.Account
	n.Name = other.Name
	n.ThreadKey = Provider + ":" + connection.String() + ":" + n.Account
	if m.GroupID != "" {
		n.ThreadKey += ":" + m.GroupID
	}
	n.SourceID = connection.String() + ":" + n.Network + ":" + m.MessageID
	return n, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, apperrors.ErrInvalidArgument)...)
}
