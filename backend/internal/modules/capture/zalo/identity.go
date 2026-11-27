// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import "github.com/margince/margince/backend/internal/shared/ports/connector"

// Provider is the channel_connection provider, the raw_capture source system and
// the contact channel identity provider of a Zalo Official Account.
const Provider = "zalo_oa"

// CustomerIdentity is the one place a customer's key is spelled, for the erasure
// check on arrival and for the contact binding that files the message. Zalo user
// ids are treated as scoped to one OA until Zalo documents otherwise, so the same
// id under two OAs is two customers.
func CustomerIdentity(ev Event) connector.ChannelIdentity {
	return connector.ChannelIdentity{Provider: Provider, AccountScope: ev.OAID, ChannelUserID: ev.UserID}
}

// RawSourceID is the redelivery key of a stored message. Zalo's msg_id repeats
// on every retry of the same delivery, and the OA id keeps two OAs apart.
func RawSourceID(oaID, msgID string) string {
	return oaID + "|" + msgID
}
