// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The channel vocabulary: which kind names a messaging conversation. Which
// transports this installation can reply on is the composition's, decided at
// boot by what the binary composed. ADR-0107/A158 stopped one answer standing
// in for the other.

// KindMessage is the one interaction kind that names a channel conversation
// (ADR-0107/A158). Which transport carried it is activity.channel_provider, a
// separate axis — reading one off the other is exactly what that decision
// retired.
const KindMessage = "message"

// IsChannelKind reports whether an activity kind names a messaging-channel
// conversation.
//
// Since the narrowing this is a comparison, not a set membership test: every
// channel message is `message` regardless of transport. Kept as a function
// rather than inlined at its callers because those callers ask a domain
// question — "is this conversation repliable in principle" — and the answer
// changing shape once already is the argument for it having one name.
func IsChannelKind(kind string) bool {
	return kind == KindMessage
}
