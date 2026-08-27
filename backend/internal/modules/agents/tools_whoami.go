// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// whoami (🟢 read): the human this passport acts for.
//
// The surface knew and did not say. Every write carries on_behalf_of, and the
// audit log records it, but nothing published it — so an assistant could not
// say "I'll assign this to you", could not filter "my deals" without asking,
// and could not set owner_id on a record it was creating even though the field
// is accepted. It also could not know which language to write stored prose in,
// which is how a German sentence ended up in an English workspace's company
// description.
//
// Locale alone did not close that, because it is empty until somebody picks a
// language and most people never do: an English workspace whose members had
// chosen nothing still got German records off an English conversation. So the
// answer carries ProseLanguage as well — the member's choice where they made
// one, the installation's base language otherwise — and it is never empty,
// because a field that is usually absent teaches the reader to ignore it.
//
// A tool rather than a field on the capabilities resource: that document is
// shape-versioned and cached, while identity is per-call and must never be
// served from a cache belonging to a different passport.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ActingIdentity is the human a call is made on behalf of.
type ActingIdentity struct {
	UserID      ids.UUID
	DisplayName string
	Email       string
	// Locale is the language this person chose, empty when they never did.
	// The caller decides what to do with empty rather than being handed a
	// default that cannot be told apart from a choice.
	Locale string
	// ProseLanguage is that decision already made: the language stored prose
	// must be written in, resolved from the member's choice or the
	// installation's base language. Unlike Locale it is never empty, so the
	// agent is never left to infer a language from the workspace's timezone
	// and currency.
	ProseLanguage string
	Timezone      string
}

// IdentityReader answers who the call acts for. Declared here and implemented
// in compose, so this module never imports identity.
type IdentityReader func(ctx context.Context) (ActingIdentity, error)

// RegisterWhoamiTool joins whoami to the surface. A nil reader registers
// nothing — the same conditional registration the other injected-seam tools
// take.
func RegisterWhoamiTool(r *Registry, read IdentityReader) {
	if read == nil {
		return
	}
	r.Register(whoami{read: read})
}

type whoami struct{ read IdentityReader }

func (t whoami) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "whoami", Title: "Who this passport acts for", Version: toolVersionV1,
		Description:   whoamiCopy.render(),
		RequiredScope: principal.ScopeRead, SelfDescribing: true, Tier: mcp.TierAutoExecute,
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[WhoamiResult](),
	}
}

func (t whoami) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	who, err := t.read(ctx)
	if err != nil {
		return nil, err
	}
	// No noteEvidence: the acting user is not a record this answer rests on,
	// it is who is asking. Stamping it would put the caller's own seat in the
	// evidence list of every call that begins by asking who they are.
	return json.Marshal(WhoamiResult{
		ActingUserID:  who.UserID,
		DisplayName:   who.DisplayName,
		Email:         who.Email,
		Locale:        who.Locale,
		ProseLanguage: who.ProseLanguage,
		Timezone:      who.Timezone,
	})
}
