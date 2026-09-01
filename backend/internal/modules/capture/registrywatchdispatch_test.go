// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Which call a renewal makes, and on what evidence.
//
// A provider that issues a handle for its subscription can be asked to extend
// THAT one; a provider that does not has to be asked whether a subscription
// exists at all, which for Graph costs a paged listing per mailbox per cycle.
// The dispatch is what turns a stored handle into the cheaper question, and
// getting it wrong is invisible: both paths end with a stored deadline and a
// working watch, and only the round trips differ.
func TestARenewalUsesTheStoredHandleWhenTheConnectorRenewsByOne(t *testing.T) {
	t.Parallel()
	stored, empty := "sub-stored", ""
	for name, tc := range map[string]struct {
		connector  connector.Connector
		ref        *string
		wantRenews bool
	}{
		"a handle and a connector that renews by one": {&renewingConnector{}, &stored, true},
		// The first registration: there is no handle yet, so the only question
		// available is "does one exist".
		"no handle yet":   {&renewingConnector{}, nil, false},
		"an empty handle": {&renewingConnector{}, &empty, false},
		// Gmail's shape: the watch is addressed by the mailbox and the topic,
		// and there is nothing to renew by.
		"a connector with no handle to renew by": {&watchOnlyConnector{}, &stored, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			watcher, isWatcher := tc.connector.(connector.Watcher)
			if !isWatcher {
				t.Fatalf("the fixture %T is not a Watcher, so this case drives nothing", tc.connector)
			}
			if _, err := renewOrRegisterWatch(
				context.Background(), tc.connector, watcher, nil, "https://api.test/hook", tc.ref,
			); err != nil {
				t.Fatalf("renewOrRegisterWatch: %v", err)
			}
			renewed, registered := calls(tc.connector)
			if tc.wantRenews && (renewed != 1 || registered != 0) {
				t.Errorf("renewed %d, registered %d — a renewal that knows the handle asked whether a "+
					"subscription exists instead of extending the one it has", renewed, registered)
			}
			if !tc.wantRenews && (renewed != 0 || registered != 1) {
				t.Errorf("renewed %d, registered %d — a renewal with no usable handle addressed "+
					"something it cannot name", renewed, registered)
			}
		})
	}
}

func calls(c connector.Connector) (renewed, registered int) {
	switch typed := c.(type) {
	case *renewingConnector:
		return typed.renewed, typed.registered
	case *watchOnlyConnector:
		return 0, typed.registered
	}
	return 0, 0
}

// watchOnlyConnector is a provider whose watch has no handle — Gmail's shape.
type watchOnlyConnector struct {
	registered int
}

func (c *watchOnlyConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "watchonly"}
}

func (c *watchOnlyConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return nil, errors.New("not used")
}

func (c *watchOnlyConnector) Sync(
	context.Context, connector.Auth, connector.Cursor, connector.Sink,
) (connector.Cursor, error) {
	return nil, errors.New("not used")
}

func (c *watchOnlyConnector) HealthCheck(context.Context, connector.Auth) error {
	return errors.New("not used")
}

func (c *watchOnlyConnector) Normalize(
	context.Context, connector.RawRecord,
) ([]connector.NormalizedRecord, error) {
	return nil, errors.New("not used")
}

func (c *watchOnlyConnector) Watch(context.Context, connector.Auth, string) (connector.WatchResult, error) {
	c.registered++
	return connector.WatchResult{ExpiresAt: time.Now().Add(time.Hour)}, nil
}

// renewingConnector is a provider that issues one — Graph's shape.
type renewingConnector struct {
	watchOnlyConnector
	renewed int
}

func (c *renewingConnector) RenewWatch(
	_ context.Context, _ connector.Auth, _, ref string,
) (connector.WatchResult, error) {
	c.renewed++
	return connector.WatchResult{ExpiresAt: time.Now().Add(time.Hour), Ref: ref}, nil
}
