// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Where a test reads the confirm-details link from.
//
// Issuance of a double-opt-in token was withdrawn: the endpoint refuses, nothing
// writes consent_doi_token, and the redemption arm is gone. A purpose requiring
// double opt-in now confirms through MailboxProof alone, earned by SPENDING a
// link that was mailed to the subject's own live primary address.
//
// So a test needing a granted marketing consent has to do what the subject does,
// and the plaintext token is deliberately absent from every operator-facing
// response — which means there is no shortcut for a test either. Reading the
// link is the same access the subject has and no more; a helper that reached
// past it would assert a confirmation nobody performed, which is the exact
// defect the endpoint was withdrawn for.
//
// It is read from the VAULT rather than from a relay double, because that is
// where the link now is at this point in its life. The installation's own mail
// rides the durable send lane: minting seals the link and stages a message
// carrying a placeholder, and the two meet in memory only when the dispatcher
// transmits. A harness that runs no worker never reaches that substitution, so
// the relay would see nothing — and asserting on an empty relay would be
// asserting that no mail exists, which is the opposite of what these suites are
// about.

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// confirmLinkToken returns the token from the most recently sealed confirm link.
//
// The delivery is found by its own template, not by guessing: a controller row
// staged from the record-confirmation template is the message that carries this
// link, and its payload_ref names the vault entry holding it.
func confirmLinkToken(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	ctx := context.Background()

	var ref string
	if err := e.Pool.QueryRow(ctx, `
		SELECT payload_ref FROM comms_outbound
		 WHERE sender_kind = 'controller' AND payload_ref IS NOT NULL
		 ORDER BY created_at DESC
		 LIMIT 1`).Scan(&ref); err != nil {
		t.Fatalf("no controller delivery holds a sealed confirm link, so there is nothing for "+
			"the subject to spend: %v", err)
	}

	// The installation's own workspace, which is the one a vault entry is
	// scoped to: the seal ran inside a request bound to it.
	var workspace ids.UUID
	if err := e.Pool.QueryRow(ctx, `SELECT id FROM workspace ORDER BY created_at LIMIT 1`).
		Scan(&workspace); err != nil {
		t.Fatalf("reading the installation workspace: %v", err)
	}

	sealed, err := e.Vault.Get(ctx, ids.From[ids.WorkspaceKind](workspace), keyvault.Ref(ref))
	if err != nil {
		t.Fatalf("reading the sealed confirm link: %v", err)
	}

	link := regexp.MustCompile(`https?://\S+`).FindString(string(sealed))
	if link == "" {
		t.Fatalf("the sealed value is not a confirm link: %q", string(sealed))
	}
	token := link[strings.LastIndex(link, "/")+1:]
	if token == "" {
		t.Fatalf("the link carries no token: %q", link)
	}
	return token
}

// discardingMailer is the relay boundary for a suite that reads the link from
// the vault rather than from a message.
//
// It is still WIRED, and that is not incidental: a controller delivery resolves
// its transport before any gate runs, so a harness with no relay would park the
// message with "no mail relay configured" and the suite would be exercising the
// unconfigured path instead of the one it is about.
type discardingMailer struct{}

func (discardingMailer) Send(context.Context, string, string, string) error { return nil }
