// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A provider with no folders is a fact about the provider rather than a fault,
// and it has to be distinguishable: the card renders its other two kinds on
// this answer, where an error would leave the reader with a broken panel.
func TestListContainersReportsAProviderThatCannotList(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil, nil, nil, nil)
	r.Register(&listlessConnector{})

	_, err := r.ListContainers(context.Background(), listlessName, ids.UserID{})
	if !errors.Is(err, ErrContainersUnsupported) {
		t.Fatalf("err = %v, want ErrContainersUnsupported", err)
	}
}

// A provider this build did not compile in is absent, not unsupported: the two
// are different answers and a caller acts on them differently.
func TestListContainersReportsAnUnknownProviderAsAbsent(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil, nil, nil, nil)

	_, err := r.ListContainers(context.Background(), "nosuchprovider", ids.UserID{})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not-found for a provider that is not compiled in", err)
	}
	if errors.Is(err, ErrContainersUnsupported) {
		t.Error("an absent provider was reported as one that cannot list")
	}
}

// listlessName is registered by the stub below. A real provider name, because
// Register holds the contract's ProviderRef pattern and a made-up one would
// fail registration rather than the case under test.
const listlessName = ProviderTelegram

// listlessConnector implements the Connector contract and NOT ContainerLister
// — a channel transport, which has no folders to offer. Spelled out rather
// than embedding a mail fixture, because embedding would inherit the verb and
// the case would stop being the one this asserts.
type listlessConnector struct{}

func (listlessConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: listlessName, Version: "fixture"}
}

func (listlessConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return nil, errors.New("listlessConnector: not authenticated in this test")
}

func (listlessConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (listlessConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (listlessConnector) HealthCheck(context.Context, connector.Auth) error { return nil }
