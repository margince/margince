// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// testAuth builds the same opaque credential connectTestMailbox mints, by
// calling Credential directly rather than hand-marshalling a second copy of
// its wire shape — a change to that shape is then tested against the real
// function every caller in this package uses.
func testAuth(t *testing.T, userID ids.UUID) connector.Auth {
	t.Helper()
	auth, err := Credential(userID)
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	return auth
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

// TestAuthenticateReturnsThePayloadVerbatim: the connect handler's own
// credential is the only "authentication" this connector ever does — there is
// no remote to authenticate against, so the request payload IS the answer.
func TestAuthenticateReturnsThePayloadVerbatim(t *testing.T) {
	userID := ids.NewV7()
	want, err := Credential(userID)
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	got, err := New(nil).Authenticate(context.Background(), connector.AuthRequest{Payload: want})
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Authenticate(%s) = %s, want the payload unchanged", want, got)
	}
}

// fakeLedger is a stand-in for capture.TestMailboxLedger, used by send_test.go
// and sync_test.go. unechoedErr and markEchoedErr let a test drive Sync's own
// failure-wrapping branches without a real ledger.
type fakeLedger struct {
	recorded []recordedSend
	unechoed []capture.SentMessage
	marked   []markedEcho

	recordSentErr error
	unechoedErr   error
	markEchoedErr error
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
	if f.recordSentErr != nil {
		return f.recordSentErr
	}
	f.recorded = append(f.recorded, recordedSend{userID: userID.String(), messageID: messageID, to: to, cc: cc, subject: subject})
	return nil
}

func (f *fakeLedger) Unechoed(context.Context, ids.UUID) ([]capture.SentMessage, error) {
	if f.unechoedErr != nil {
		return nil, f.unechoedErr
	}
	return f.unechoed, nil
}

func (f *fakeLedger) MarkEchoed(_ context.Context, userID ids.UUID, id ids.UUID) error {
	if f.markEchoedErr != nil {
		return f.markEchoedErr
	}
	f.marked = append(f.marked, markedEcho{userID: userID.String(), id: id})
	return nil
}
