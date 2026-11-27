// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "encoding/json"

// ChatSendAuth is the credential a send through one company chat account needs.
// Token is the account's access token. Account names what the token alone does
// not: the WhatsApp phone number id, or the Telegram Business connection id.
// Zalo needs none, because its token belongs to one Official Account.
type ChatSendAuth struct {
	Token   string `json:"token"`
	Account string `json:"account,omitempty"`
}

// Auth is the opaque bundle a MessageSender is handed.
func (a ChatSendAuth) Auth() Auth {
	// Two strings always marshal.
	raw, _ := json.Marshal(a) //craft:ignore swallowed-errors a struct of two strings cannot fail to marshal
	return Auth(raw)
}

// ParseChatSendAuth reads a bundle made by ChatSendAuth.Auth. It reports false
// for any other bundle, such as a bot's bare token.
func ParseChatSendAuth(auth Auth) (ChatSendAuth, bool) {
	var out ChatSendAuth
	if err := json.Unmarshal(auth, &out); err != nil || out.Token == "" {
		return ChatSendAuth{}, false
	}
	return out, true
}
