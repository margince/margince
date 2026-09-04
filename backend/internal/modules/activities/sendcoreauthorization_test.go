// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a caller may do with the authorization request it is handed.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The request a caller reads cannot be used to rewrite the one that gets staged.
//
// PreparedSend keeps its fields unexported so a caller outside this package may
// carry a prepared send and may not forge one. A getter handing back the
// backing arrays would defeat that without touching a field: staging decides on
// the carried request, so a caller that rewrote a recipient through the value it
// previewed would send to somebody the engine was never asked about — the
// preview and the send disagreeing, which is the one thing this seam exists to
// prevent.
func TestTheAuthorizationHandedOutCannotRewriteTheOneThatStages(t *testing.T) {
	link := ids.NewV7()
	p := PreparedSend{
		authorization: commsauthz.Request{
			Recipients: connector.EmailRecipients([]string{"buyer@acme.test"}),
			Links:      []ids.UUID{link},
		},
	}

	got := p.Authorization()
	got.Recipients[0] = connector.EmailRecipients([]string{"stranger@elsewhere.test"})[0]
	got.Links[0] = ids.NewV7()

	if addr := p.authorization.Recipients[0].Email; addr != "buyer@acme.test" {
		t.Errorf("the staged recipient is now %q — a caller rewrote the send through the request it previewed", addr)
	}
	if p.authorization.Links[0] != link {
		t.Error("the staged links were rewritten through the previewed request")
	}
}
