// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// CaptureOnlyConversationError refuses a reply into a conversation that is only
// read here, such as a Telegram Business chat. Sending needs the business
// account's own connection, and staff answer in the provider's app. It maps to
// 422 and stages nothing.
type CaptureOnlyConversationError struct{ Provider string }

func (e *CaptureOnlyConversationError) Error() string {
	return "this " + e.Provider + " conversation is read-only here: it belongs to a linked business account, so reply in the " +
		e.Provider + " app"
}

// FieldFault refuses a send into a capture-only conversation.
func (e *CaptureOnlyConversationError) FieldFault() (field, code, message string) {
	return "id", "conversation_capture_only", e.Error()
}

// RefuseCaptureOnlyThread is the one rule behind every refusal to reply into a
// capture-only conversation. The send path and the agent's staging check both
// call it, so a reply cannot be staged that the send would refuse.
func RefuseCaptureOnlyThread(provider, threadKey string) error {
	if IsCaptureOnlyThread(provider, threadKey) {
		return &CaptureOnlyConversationError{Provider: provider}
	}
	return nil
}
