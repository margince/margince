// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// An import run keeps the workspace facts it judged its first message by, and
// a fact that moves mid-run reaches the next message.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// skippingPage walks its page the way the mail connectors do: a message the
// sink skips is counted, and the page goes on.
type skippingPage struct {
	*mailPageConnector
}

func (p *skippingPage) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, sink connector.Sink) (connector.BackfillPageResult, error) {
	res := connector.BackfillPageResult{NextToken: p.nextToken}
	for _, raw := range p.raws {
		msg, err := mailmap.Parse(raw, captureOwner)
		if err != nil {
			return connector.BackfillPageResult{}, err
		}
		res.Scanned++
		msg = msg.AttestSentByOwner(p.sent[msg.ID()])
		_, err = sink.Upsert(ctx, msg.ToRecord("gmail", raw))
		switch {
		case errors.Is(err, connector.ErrSkip):
			res.Skipped++
		case err != nil:
			return connector.BackfillPageResult{}, err
		default:
			res.Captured++
		}
		if p.afterMessage != nil {
			p.afterMessage()
		}
	}
	return res, nil
}

func TestAnOwnDomainRemovedMidRunNoLongerDropsTheNextMessage(t *testing.T) {
	t.Parallel()
	const ours = "partner-fact.example"
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	domains := capturemod.NewOwnDomainStore(e.DB())
	if _, err := domains.Add(domainAdmin(e), ours); err != nil {
		t.Fatal(err)
	}
	first, second := "fact-run-1@myco.example", "fact-run-2@myco.example"
	page := &skippingPage{&mailPageConnector{
		nextToken: "next",
		raws:      [][]byte{email(captureOwner, "", "ana@"+ours, first, ""), email(captureOwner, "", "ana@"+ours, second, "")},
		sent:      map[string]bool{first: true, second: true},
	}}
	removed := false
	page.afterMessage = func() {
		if removed {
			return
		}
		removed = true
		if err := domains.Remove(domainAdmin(e), ours); err != nil {
			t.Errorf("removing %s: %v", ours, err)
		}
	}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(page)
	actor := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(actor, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatal(err)
	}
	run, err := registry.StartBackfill(actor, capturemod.ProviderMailbox("gmail"), ids.From[ids.UserKind](e.Rep1), 6, connector.BackfillEstimate{Messages: 2}, enqueueNothing)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := registry.RunBackfillStep(principal.WithWorkspaceID(context.Background(), e.WS), run.ID); err != nil {
		t.Fatal(err)
	}
	stored := func(messageID string) bool {
		return countRows(t, e, "SELECT count(*) FROM activity WHERE source_id = '"+messageID+"'") > 0
	}
	if stored(first) {
		t.Fatal("the first message was stored although its only party was an own domain; the fixture proves nothing")
	}
	if !stored(second) {
		t.Fatal("the message after the domain was removed was dropped as internal: the run used its old own domains")
	}
}
