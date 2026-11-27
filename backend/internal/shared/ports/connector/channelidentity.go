// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// ChannelIdentity is one messaging-channel identity as the provider reports
// it: the resolution key a channel connector hands the CRM so an inbound
// message finds the human behind it.
//
// Provider and ChannelUserID together ARE the key. The bot (channel) id is
// deliberately absent because Telegram user ids are global rather than
// bot-scoped, so a workspace that rotates its bot keeps every binding it had.
//
// Username is display only. A Telegram handle can be released and re-claimed
// by a different human, so nothing may route, authorize or deduplicate on it.
type ChannelIdentity struct {
	Provider string
	// AccountScope names the connected account the identity was seen through.
	// It is empty for everything the workspace's own bot receives.
	AccountScope  string
	ChannelUserID string
	Username      string
	// Alias is a second identity the same record names for the same human, such
	// as a WhatsApp customer's business-scoped user id beside their phone. It is
	// bound to the contact the main identity resolves to, so an erasure of the
	// contact suppresses both. It is a pointer so the identity stays comparable.
	Alias *ChannelIdentity
}

// All lists the identity and its alias, the identity first.
func (ci ChannelIdentity) All() []ChannelIdentity {
	own := ci
	own.Alias = nil
	if ci.Alias == nil {
		return []ChannelIdentity{own}
	}
	return []ChannelIdentity{own, *ci.Alias}
}
