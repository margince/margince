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

// SendPurpose says why a message goes into a chat.
type SendPurpose int

const (
	// PurposeReply is a reply a colleague or an agent writes to a conversation.
	PurposeReply SendPurpose = iota
	// PurposeClarification is the question a company account asks a colleague
	// about messages the colleague forwarded to it.
	PurposeClarification
)

// AllowChannelSend is the one rule behind every refusal to send into a chat.
// A reply into a customer's business conversation is refused, because that
// conversation is only read here. A clarification is the question a company
// account asks a colleague about what they forwarded. It goes only to a
// verified member of the workspace, on any thread.
//
// The send path, the agent's staging check and the clarification job all call
// it, so a message cannot be staged that the send would refuse.
func AllowChannelSend(purpose SendPurpose, provider, threadKey string, recipientIsVerifiedMember bool) error {
	if purpose == PurposeClarification {
		if recipientIsVerifiedMember {
			return nil
		}
		return ErrClarificationRecipient
	}
	if IsCaptureOnlyThread(provider, threadKey) {
		return &CaptureOnlyConversationError{Provider: provider}
	}
	return nil
}

// RefuseCaptureOnlyThread refuses a reply into a capture-only conversation.
func RefuseCaptureOnlyThread(provider, threadKey string) error {
	return AllowChannelSend(PurposeReply, provider, threadKey, false)
}
