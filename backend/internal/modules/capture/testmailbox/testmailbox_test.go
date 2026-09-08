// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// The connector.EmailSender/GrantedScoper/Connector assertions are compile-time
// (var _ lines in send.go, testmailbox.go, sync.go) — a build that does not
// satisfy them never reaches a test binary at all, so a runtime re-check here
// would prove strictly less than the code already guarantees.

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
	if !slices.Equal(d.Scopes, []principal.Scope{principal.ScopeRead}) {
		t.Errorf("Descriptor().Scopes = %v, want exactly [ScopeRead]", d.Scopes)
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
	marked   []markedEcho
}

type recordedSend struct {
	userID    string
	messageID string
	to, cc    []string
	subject   string
}

type markedEcho struct {
	userID string
	id     ids.UUID
}

func (f *fakeLedger) RecordSent(_ context.Context, userID ids.UUID, messageID string, to, cc []string, subject string) error {
	f.recorded = append(f.recorded, recordedSend{userID: userID.String(), messageID: messageID, to: to, cc: cc, subject: subject})
	return nil
}

func (f *fakeLedger) Unechoed(context.Context, ids.UUID) ([]capture.SentMessage, error) {
	return f.unechoed, nil
}

func (f *fakeLedger) MarkEchoed(_ context.Context, userID ids.UUID, id ids.UUID) error {
	f.marked = append(f.marked, markedEcho{userID: userID.String(), id: id})
	return nil
}
