// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The installation's AI models, lent to a unit that declares CapabilityModels.
// The core owns the choice of model, the key, the budget and the call records.
// A unit sends messages and gets text; it never sees a provider or a key.

import "context"

// Message is one turn of a conversation. Role is "user" or "assistant".
type Message struct {
	Role string
	Text string
}

// CompleteRequest is what a unit asks a model.
//
// A request carrying a contact's data names the records it was built from in
// About, so a contact under a restriction of processing is refused. A request
// carrying none says so in NoContactData, with the reason. A request with
// neither is refused, because the core cannot tell which of the two it is.
type CompleteRequest struct {
	System   string
	Messages []Message
	// MaxTokens caps the answer; zero leaves the installation's default.
	MaxTokens int
	// About names the core records the request's text was built from. Types
	// are contact, company, deal, activity, project and lead.
	About         []EntityRef
	NoContactData string
}

// Models is the unit's door to the installation's AI models.
type Models interface {
	// Complete answers one request. It returns ErrNotDeclared unless the unit
	// declares CapabilityModels, and ErrInvalid for a request with no messages,
	// an unknown role or an unknown record type.
	Complete(ctx context.Context, req CompleteRequest) (string, error)
}
