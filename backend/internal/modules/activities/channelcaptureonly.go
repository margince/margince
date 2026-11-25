// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

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
