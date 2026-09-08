// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package testmailbox is the QC-only, no-network connector that both
// captures and sends synthetic mail (issue #4974) — the outbound twin of
// offlinedemo, which deliberately CANNOT send (see its own package doc and
// TestTheConnectorCannotSend, both untouched by this package).
//
// NEVER REACHABLE ON A REAL DEPLOYMENT. Registration (compose's
// NewCaptureRegistry), send authority (compose's mailAppConfigured), and the
// connect endpoint (compose/connectors_testmailbox.go) are each conditioned
// on deployconfig.Operations.AllowTestMailbox, which defaults to false.
//
// Its addresses are quarantined to RFC 2606 reserved domains
// (example.com/.net/.org, .test/.example/.invalid/.localhost) — SendEmail
// refuses anything outside that set before minting a receipt. It never
// dials out: no import in this package reaches net or net/http, which
// TestPackageNeverImportsTheNetwork pins structurally.
package testmailbox

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Name is the provider id, matching the capture_connection CHECK.
const Name = "test_mailbox"

// SendScope is this connector's own synthetic authority: there is no real
// OAuth grant, so this is the one scope GrantedScopes always reports holding,
// and the one comms.mailSendScopes demands back (compose/sendscope_test.go
// holds the two against each other, the same way it does for gmail/graph).
const SendScope = "urn:margince:test_mailbox:send"

// Ledger is what this connector needs of its own sent-echo bookkeeping.
// Implemented by capture.TestMailboxLedger; injected here (rather than the
// connector holding a *database.DB itself) so the connector stays a pure
// consumer of one small port, exactly like offlinedemo's Directory.
//
// Unechoed returns capture.SentMessage directly rather than a package-local
// mirror of it: Go requires exact type identity to satisfy an interface
// method's signature, so a same-shaped-but-distinct testmailbox.SentMessage
// would NOT be satisfied by capture.TestMailboxLedger's real return type.
// Importing capture.SentMessage here is the cheap, correct fix — this
// package already imports capture for ActivityFields, exactly like
// offlinedemo does, so this adds no new dependency edge.
type Ledger interface {
	RecordSent(ctx context.Context, userID ids.UUID, messageID string, toAddresses []string, subject string) error
	Unechoed(ctx context.Context, userID ids.UUID) ([]capture.SentMessage, error)
	MarkEchoed(ctx context.Context, id ids.UUID) error
}

// Connector is the test_mailbox capture+send provider.
type Connector struct{ ledger Ledger }

// New builds the connector over its own sent-echo ledger.
func New(ledger Ledger) *Connector { return &Connector{ledger: ledger} }

// Descriptor declares read-only capture scopes and the auto-execute tier,
// mirroring gmail's own descriptor: send authority is a separate question,
// answered by GrantedScopes and comms.mailSendScopes, never by this.
func (c *Connector) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name:     Name,
		Version:  "1",
		Scopes:   []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute,
		Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

// Authenticate returns the payload verbatim — there is nothing to
// authenticate against, and the "credential" the connect handler mints
// (compose/connectors_testmailbox.go) is an opaque marker never checked
// against anything real.
func (c *Connector) Authenticate(_ context.Context, req connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth(req.Payload), nil
}

// HealthCheck always succeeds — there is no remote to be unhealthy.
func (c *Connector) HealthCheck(context.Context, connector.Auth) error { return nil }

// GrantedScopes always reports holding SendScope. There is no OAuth grant to
// read this back from — the connector's own presence IS the authority, so it
// answers the question comms.SendCapable asks (via
// capture.Registry.GrantedScopesFor, which stores whatever this returns at
// connect time) with the one scope it always has.
func (c *Connector) GrantedScopes(connector.Auth) ([]string, error) {
	return []string{SendScope}, nil
}

// assert the connector satisfies the ports it declares.
var _ connector.GrantedScoper = (*Connector)(nil)
