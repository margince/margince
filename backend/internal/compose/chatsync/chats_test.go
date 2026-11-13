// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type refusingDoer struct{ called bool }

func (d *refusingDoer) Do(*http.Request) (*http.Response, error) {
	d.called = true
	return nil, errors.New("no network in this test")
}

func TestProductionRefusesAPlainHTTPAddressBeforeTheTokenIsSent(t *testing.T) {
	doer := &refusingDoer{}
	_, err := BotCheck{HTTP: doer}.Confirm(context.Background(), "http://chat.example.com", "gwapp_secret")
	if !errors.Is(err, apperrors.ErrInvalidArgument) || doer.called {
		t.Fatalf("http in production = %v, sent=%v; want a refusal before any request", err, doer.called)
	}
	_, err = BotCheck{HTTP: doer, AllowHTTP: true}.Confirm(context.Background(), "http://localhost:9000", "gwapp_secret")
	if !doer.called || errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("http in dev = %v, sent=%v; want the call made", err, doer.called)
	}
}

func TestTheDispactCheckTakesAnEventsSecret(t *testing.T) {
	var check capture.ChannelBotCheck = BotCheck{}
	events, ok := check.(capture.ChannelEventsCheck)
	if !ok {
		t.Fatal("the Dispact bot check takes no events secret, so push can never be turned on")
	}
	if err := events.ValidateEventsSecret("whsec_c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0"); err != nil {
		t.Fatalf("a whsec_ secret was refused: %v", err)
	}
	if err := events.ValidateEventsSecret("not-a-secret"); err == nil {
		t.Fatal("a secret without whsec_ was accepted")
	}
}
