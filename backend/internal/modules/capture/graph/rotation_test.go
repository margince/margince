// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// recordingRotations captures what the connector reports, so a test can assert
// on the BUNDLE rather than on the call having happened.
type recordingRotations struct {
	got  []connector.Auth
	err  error
	seen int
}

func (r *recordingRotations) Rotated(_ context.Context, auth connector.Auth) error {
	r.seen++
	if r.err != nil {
		return r.err
	}
	r.got = append(r.got, auth)
	return nil
}

func rotatingConnector(t *testing.T, oauth *fakeOAuth, api API, sink connector.CredentialSink) connector.Connector {
	t.Helper()
	return New(oauth, api).WithCredentialSink(sink)
}

func TestASyncReportsTheReplacementCredentialWithTheRestOfTheBundleIntact(t *testing.T) {
	sink := &recordingRotations{}
	api := &fakeAPI{email: owner, initDelta: "https://graph/delta?$1"}
	c := rotatingConnector(t, &fakeOAuth{access: "a", rotated: "r-2"}, api, sink)

	if _, err := c.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("the sink saw %d rotation(s), want one", len(sink.got))
	}
	var st graphconn.AuthState
	if err := json.Unmarshal(sink.got[0], &st); err != nil {
		t.Fatalf("the reported bundle does not decode: %v", err)
	}
	if st.RefreshToken != "r-2" {
		t.Errorf("RefreshToken = %q, want the replacement", st.RefreshToken)
	}
	// The bundle is the WHOLE credential, not the secret alone: a re-seal that
	// dropped the owner or the granted scopes would leave the connection unable
	// to say whose mailbox it is or whether it may send.
	if st.Owner != owner {
		t.Errorf("Owner = %q, want it carried through the rotation", st.Owner)
	}
	if len(st.Granted) == 0 {
		t.Error("the rotated bundle dropped the granted scopes")
	}
}

// The common case: Google-style providers, and Microsoft rounds where nothing
// was replaced. A sink called on every sync would re-seal the vault and retire
// a blob for no change.
func TestASyncThatRotatesNothingReportsNothing(t *testing.T) {
	sink := &recordingRotations{}
	api := &fakeAPI{email: owner, initDelta: "https://graph/delta?$1"}
	c := rotatingConnector(t, &fakeOAuth{access: "a"}, api, sink)

	if _, err := c.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if sink.seen != 0 {
		t.Fatalf("the sink was called %d time(s) with no rotation to report", sink.seen)
	}
}

// A re-seal that cannot be written costs one cycle's freshness; failing the
// pull over it would cost the mail. The old credential is still valid, which is
// what makes that trade the right way round.
func TestARotationThatCannotBeRecordedDoesNotFailTheSync(t *testing.T) {
	sink := &recordingRotations{err: errors.New("the vault is unreachable")}
	api := &fakeAPI{email: owner, initDelta: "https://graph/delta?$1"}
	c := rotatingConnector(t, &fakeOAuth{access: "a", rotated: "r-2"}, api, sink)

	cur, err := c.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{})
	if err != nil {
		t.Fatalf("Sync = %v, want the pull to stand despite the failed re-seal", err)
	}
	if len(cur) == 0 {
		t.Error("the watermark was not advanced — the sync was penalised for a bookkeeping fault")
	}
}

// The rotation is reported BEFORE the pull because it has already happened by
// then: the replacement is issued and the old token spent the moment the token
// endpoint answers. Reporting it only on success would drop it on exactly the
// mailbox that most needs its credential kept fresh — one whose syncs keep
// failing.
func TestARotationIsReportedEvenWhenThePullThenFails(t *testing.T) {
	sink := &recordingRotations{}
	api := &fakeAPI{email: owner, deltaErr: ErrUnreachable}
	c := rotatingConnector(t, &fakeOAuth{access: "a", rotated: "r-2"}, api, sink)

	prior := marshalCursor("https://graph/delta?stale", "https://graph/sent?stale", owner)
	if _, err := c.Sync(context.Background(), sealedAuth(t), prior, &recordingSink{}); err == nil {
		t.Fatal("expected the pull to fail in this fixture")
	}
	if len(sink.got) != 1 {
		t.Fatalf("the sink saw %d rotation(s), want the one that had already happened", len(sink.got))
	}
}

// The shared instance must never carry a sink: one connector serves every
// connection the fleet pulls at once, and a field set in place would send one
// mailbox's replacement credential to another mailbox's re-seal.
func TestBindingASinkLeavesTheRegisteredConnectorAlone(t *testing.T) {
	registered := New(&fakeOAuth{access: "a", rotated: "r-2"}, &fakeAPI{email: owner, initDelta: "d"})
	mine := &recordingRotations{}
	bound := registered.WithCredentialSink(mine)

	if bound == connector.Connector(registered) {
		t.Fatal("WithCredentialSink returned the receiver — the registered instance now holds one connection's sink")
	}
	if _, err := registered.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
		t.Fatalf("Sync on the registered instance: %v", err)
	}
	if mine.seen != 0 {
		t.Fatal("a sync through the REGISTERED connector reported to a sink bound to a copy")
	}
}

// sealedAuth is the bundle a connected mailbox holds.
func sealedAuth(t *testing.T) connector.Auth {
	t.Helper()
	b, err := json.Marshal(graphconn.AuthState{
		RefreshToken: "r-1", Owner: owner,
		Granted: []string{"offline_access", "User.Read", "Mail.Read"},
	})
	if err != nil {
		t.Fatalf("marshal auth: %v", err)
	}
	return b
}
