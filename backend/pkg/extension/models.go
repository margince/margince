// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The installation's AI models, lent to a unit that declares CapabilityModels.
// The core owns the choice of model, the key, the budget and the call records.
// A unit sends messages and gets text; it never sees a provider or a key.

import (
	"context"
	"encoding/json"
)

// Message is one turn of a conversation. Role is "user" or "assistant".
type Message struct {
	Role string
	Text string
}

// ModelTier is the kind of model a unit asks for. The installation decides
// which model sits behind each tier, and its budget and keys still apply.
type ModelTier string

const (
	// ModelTierDefault leaves the choice to the core.
	ModelTierDefault ModelTier = ""
	// ModelTierCheap asks for the installation's low-cost model.
	ModelTierCheap ModelTier = "cheap"
	// ModelTierPremium asks for the installation's higher-quality model.
	ModelTierPremium ModelTier = "premium"
)

// Attachment is one file sent with a request: an image (png, jpeg, webp or
// gif) or a PDF, such as a scanned form.
type Attachment struct {
	ContentType string
	Data        []byte
	// Name is optional and only labels the file in the call record.
	Name string
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
	// Tier pins the request to one tier. A tier the installation has not bound
	// a model to fails the call and does not fall back to the other.
	Tier ModelTier
	// Attachments are sent with the last user message, at most MaxAttachments
	// and MaxAttachmentBytes each, so a multi-page scan fits. A file in an
	// attachment is contact data when it is a filled-in form: name it through
	// About like any text. A model that cannot read a file's type fails the
	// call with ErrInvalid and never drops the file.
	Attachments []Attachment
	// JSONSchema, when set, makes the model answer one JSON value of that
	// schema. The answer is the bare JSON, with no code fence.
	JSONSchema json.RawMessage
	// About names the core records the request's text was built from. Types
	// are contact, company, deal, activity, project and lead.
	About         []EntityRef
	NoContactData string
}

// MaxAttachments and MaxAttachmentBytes bound the files of one request.
const (
	MaxAttachments     = 8
	MaxAttachmentBytes = 20 << 20
)

// Models is the unit's door to the installation's AI models.
type Models interface {
	// Complete answers one request. It returns ErrNotDeclared unless the unit
	// declares CapabilityModels, and ErrInvalid for a request with no messages,
	// an unknown role or an unknown record type.
	Complete(ctx context.Context, req CompleteRequest) (string, error)
}
