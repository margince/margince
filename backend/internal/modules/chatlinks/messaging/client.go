// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package messaging defines attended conversation operations. Credentials belong
// to the participant whose provider permissions admit each read and send.
package messaging

import (
	"context"
	"errors"
	"time"
)

// Provider failures preserve consent, membership, source and delivery distinctions for repair.
var (
	ErrUnauthorized    = errors.New("messaging: reconnect the sending account")
	ErrConsentRequired = errors.New("messaging: app permissions need renewed consent")
	ErrForbidden       = errors.New("messaging: check access to this conversation and the provider policy")
	ErrUnavailable     = errors.New("messaging: the conversation is unavailable")
	ErrInvalid         = errors.New("messaging: review the destination and message")
	ErrUnsupported     = errors.New("messaging: this operation is unavailable")
	ErrDeliveryUnknown = errors.New("messaging: check the conversation before repeating this send")
)

// Member keeps native identity available for CRM recipient matching.
type Member struct {
	Bot      bool
	ID       string
	Name     string
	Email    string
	External bool
	Active   bool
}

// Identity binds personal consent to one provider workspace and sender.
type Identity struct {
	Member    Member
	Workspace string
}

// Conversation carries native room identity and audience-verification state.
type Conversation struct {
	SupportsOutcomeDraft *bool
	SourceURL            string
	Access               string
	SupportsThreads      bool
	ID                   string
	Name                 string
	Kind                 string
	Members              []Member
	AudienceKnown        bool
	External             bool
}

// Message retains native identifiers used to detect source changes.
type Message struct {
	ReplyCount        int
	ContentIncomplete bool
	SourceURL         string
	ID                string
	Root              string
	Author            string
	Text              string
	At                time.Time
	Edited            bool
	Deleted           bool
}

// Page keeps message pagination explicit for personal conversation reads.
type Page struct {
	NextDirection string
	Messages      []Message
	Next          string
}

// Send uses the participant's identity for a reviewed personal message.
type Send struct {
	Conversation     string
	Root             string
	Text             string
	RequestID        string
	ExpectedAudience string
}

// Receipt distinguishes provider acknowledgment from an unresolved delivery.
type Receipt struct {
	Conversation string
	ID           string
}

// Client operates under a single verified participant grant. The caller admits
// the CRM context independently before exposing a provider conversation.
type Client interface {
	Identity(context.Context) (Identity, error)
	Channels(context.Context, string) ([]Conversation, string, error)
	ResolvePerson(context.Context, string) (Member, error)
	People(context.Context, string, string) ([]Member, string, error)
	Conversation(context.Context, string) (Conversation, error)
	OpenPeople(context.Context, []string) (Conversation, error)
	Messages(context.Context, string, string, string) (Page, error)
	Send(context.Context, Send) (Receipt, error)
}
