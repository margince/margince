// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// testAuth builds the same opaque credential connectTestMailbox mints.
func testAuth(t *testing.T, userID string) connector.Auth {
	t.Helper()
	b, err := json.Marshal(authPayload{UserID: userID})
	if err != nil {
		t.Fatalf("marshal auth: %v", err)
	}
	return connector.Auth(b)
}

func TestImplementsEmailSenderAndGrantedScoper(t *testing.T) {
	var c any = New(nil)
	if _, ok := c.(connector.EmailSender); !ok {
		t.Error("test_mailbox must implement connector.EmailSender — that is the whole reason it exists")
	}
	if _, ok := c.(connector.GrantedScoper); !ok {
		t.Error("test_mailbox must implement connector.GrantedScoper — it has no real OAuth grant to read a scope from, so it declares one itself")
	}
	if _, ok := c.(connector.Connector); !ok {
		t.Error("test_mailbox must implement connector.Connector")
	}
}

func TestDescriptorIsReadOnlyCapture(t *testing.T) {
	// Mirrors gmail's own descriptor: send authority is granted separately
	// (comms.mailSendScopes + GrantedScopes), not through the capture
	// descriptor's scopes or risk tier.
	d := New(nil).Descriptor()
	if d.Name != Name {
		t.Errorf("Descriptor().Name = %q, want %q", d.Name, Name)
	}
	if d.RiskTier != mcp.TierAutoExecute {
		t.Errorf("Descriptor().RiskTier = %v, want TierAutoExecute", d.RiskTier)
	}
}

func TestGrantedScopesAlwaysReportsTheSyntheticScope(t *testing.T) {
	scopes, err := New(nil).GrantedScopes(nil)
	if err != nil {
		t.Fatalf("GrantedScopes: %v", err)
	}
	if len(scopes) != 1 || scopes[0] != SendScope {
		t.Errorf("GrantedScopes() = %v, want [%q] — there is no real grant to introspect, so this connector always reports holding its own scope", scopes, SendScope)
	}
}

func TestHealthCheckAlwaysSucceeds(t *testing.T) {
	if err := New(nil).HealthCheck(context.Background(), nil); err != nil {
		t.Errorf("HealthCheck() = %v, want nil — there is no remote to be unhealthy", err)
	}
}

// fakeLedger is a stand-in for capture.TestMailboxLedger, used by send_test.go
// and sync_test.go.
type fakeLedger struct {
	recorded []recordedSend
	unechoed []capture.SentMessage
	marked   []ids.UUID
}

type recordedSend struct {
	userID    string
	messageID string
	to        []string
	subject   string
}

func (f *fakeLedger) RecordSent(_ context.Context, userID ids.UUID, messageID string, to []string, subject string) error {
	f.recorded = append(f.recorded, recordedSend{userID: userID.String(), messageID: messageID, to: to, subject: subject})
	return nil
}
func (f *fakeLedger) Unechoed(context.Context, ids.UUID) ([]capture.SentMessage, error) {
	return f.unechoed, nil
}
func (f *fakeLedger) MarkEchoed(_ context.Context, id ids.UUID) error {
	f.marked = append(f.marked, id)
	return nil
}
