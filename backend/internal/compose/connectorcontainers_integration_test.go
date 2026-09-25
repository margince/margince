// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The folder listing's path through the database: which connection it reads,
// and whose.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// listingConnector answers a fixed set of folders and records the auth it was
// handed, which is what proves the registry resolved a credential rather than
// calling with nothing.
type listingConnector struct{ sawAuth connector.Auth }

func (c *listingConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "imap", Version: "fixture"}
}

func (c *listingConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return nil, errors.New("listingConnector: not authenticated in this test")
}

func (c *listingConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (c *listingConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (c *listingConnector) HealthCheck(context.Context, connector.Auth) error { return nil }

func (c *listingConnector) ListContainers(_ context.Context, auth connector.Auth) ([]connector.NamedContainer, error) {
	c.sawAuth = auth
	return []connector.NamedContainer{{ID: "INBOX/Privat", Name: "INBOX/Privat"}}, nil
}

// The happy path, and the thing it really asserts: the registry read THIS
// seat's connection and handed its credential to the connector.
func TestListContainersResolvesTheSeatsOwnCredential(t *testing.T) {
	e := integration.Setup(t)
	seedListableConnection(t, e, e.Rep1, []byte(`{"mailbox":"INBOX"}`))

	conn := &listingConnector{}
	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, nil)
	r.Register(conn)

	got, err := r.ListContainers(e.As(e.Rep1, nil, integration.AccountRepPerms), "imap",
		ids.From[ids.UserKind](e.Rep1))
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(got) != 1 || got[0].Name != "INBOX/Privat" {
		t.Fatalf("got %+v, want the mailbox's folder", got)
	}
	if string(conn.sawAuth) != `{"mailbox":"INBOX"}` {
		t.Errorf("connector saw auth %q, want the stored credential", conn.sawAuth)
	}
}

// A seat with no connection has no folders to enumerate, and is told so as
// ABSENT: whether somebody else's mailbox exists is not a thing to confirm,
// and an empty list would read as "your mailbox has no folders".
func TestListContainersAnswersAbsentForASeatWithNoConnection(t *testing.T) {
	e := integration.Setup(t)

	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, nil)
	r.Register(&listingConnector{})

	_, err := r.ListContainers(e.As(e.Rep2, nil, integration.AccountRepPerms), "imap",
		ids.From[ids.UserKind](e.Rep2))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not-found for a seat with no connection", err)
	}
}

// A colleague's connection is not this seat's to enumerate: the read matches
// on the caller, so the answer is absence rather than somebody else's folders.
func TestListContainersDoesNotReachAColleaguesMailbox(t *testing.T) {
	e := integration.Setup(t)
	seedListableConnection(t, e, e.Rep1, []byte(`{"mailbox":"INBOX"}`))

	conn := &listingConnector{}
	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, nil)
	r.Register(conn)

	_, err := r.ListContainers(e.As(e.Rep2, nil, integration.AccountRepPerms), "imap",
		ids.From[ids.UserKind](e.Rep2))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not-found rather than the colleague's folders", err)
	}
	if conn.sawAuth != nil {
		t.Error("the connector was called with a colleague's credential")
	}
}

// seedListableConnection puts one seat's connection in place with its
// credential in the legacy column — the row shape resolveCredential falls back
// to, and the one a test can make without a vault.
func seedListableConnection(t *testing.T, e *integration.Env, owner ids.UUID, auth []byte) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, auth)
			VALUES ('imap', $1, 'connected', $2)`, owner, auth)
		return err
	}); err != nil {
		t.Fatalf("seeding the connection: %v", err)
	}
}
