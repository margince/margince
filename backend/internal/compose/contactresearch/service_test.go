// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactresearch

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/contactdata"
)

// A provider's URL is untrusted input. A javascript: or data: source becomes a
// link the reader clicks, so it is refused HERE — at the boundary — rather than
// left for each consumer to remember.
func TestAClaimWhoseSourceIsNotAWebURLIsDropped(t *testing.T) {
	claims := wireClaims([]contactdata.Claim{
		{Body: "clickable payload", Sources: []contactdata.Source{{Label: "Bio", URL: "javascript:alert(1)"}}},
		{Body: "smuggled document", Sources: []contactdata.Source{{Label: "Doc", URL: "data:text/html,<script>"}}},
		{Body: "a real citation", Sources: []contactdata.Source{{Label: "Company site", URL: "https://example.com/team"}}},
	})
	if len(claims) != 1 {
		t.Fatalf("kept %d claims, want only the https one — a scheme that executes is not a citation", len(claims))
	}
	if claims[0].Body != "a real citation" {
		t.Errorf("kept %q, want the https-sourced claim", claims[0].Body)
	}
}

// A claim that loses every source loses its evidence, and a claim a reader
// cannot check is exactly what the citation rule exists to keep off the page.
func TestAClaimLeftWithNoOpenableSourceIsDropped(t *testing.T) {
	claims := wireClaims([]contactdata.Claim{
		{Body: "unsourced", Sources: []contactdata.Source{{Label: "nowhere", URL: "ftp://example.com/x"}}},
	})
	if len(claims) != 0 {
		t.Fatalf("kept %d claims, want none", len(claims))
	}
}

// Save is human-only whatever it would have written: an agent posting an
// EMPTY claims list must see the same refusal a non-empty one gets, not the
// zero-claims no-op a human gets. A zero-value Service is enough to prove
// it — the refusal has to land before s.contacts is ever touched, so a nil
// store that would panic on the write path is the check that the ordering
// really is what it claims.
func TestSaveRefusesAnAgentEvenWithNoClaims(t *testing.T) {
	agentCtx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
	})
	s := &Service{}
	saved, err := s.Save(agentCtx, ids.From[ids.ContactKind](ids.NewV7()), crmcontracts.SaveContactResearchRequest{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("Save(agent, no claims) err = %v, want ErrPermissionDenied", err)
	}
	if saved != 0 {
		t.Errorf("Save(agent, no claims) saved = %d, want 0", saved)
	}
}
