// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The chassis's admission and response discipline, provider-agnostic: a
// fixed test secret stands in for either provider's real one, and each test
// asserts one rule from design §6.5 — the method guard, the no-detail
// secret rejection, and the two Disposition outcomes that give a poison
// payload and a transient fault opposite HTTP treatment on purpose.

package compose

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const webhookTestSecret = "chassis-secret" // test fixture, not a credential

// webhookTestSpec wires a fixed secret and MaxBody so each test only needs
// to supply the Handle behaviour it is asserting.
func webhookTestSpec(t *testing.T, handle func(ctx context.Context, r *http.Request, body []byte) (Disposition, error)) WebhookSpec {
	t.Helper()
	return WebhookSpec{
		Provider: "test",
		MaxBody:  1 << 10,
		Secret: func(r *http.Request) (string, string) {
			return webhookTestSecret, r.URL.Query().Get("secret")
		},
		Handle:   handle,
		OnAccept: http.StatusOK,
	}
}

func webhookTestHandler(t *testing.T, handle func(ctx context.Context, r *http.Request, body []byte) (Disposition, error)) http.Handler {
	t.Helper()
	return Webhook(webhookTestSpec(t, handle), slog.New(slog.DiscardHandler))
}

func TestWebhookRejectsNonPostWith405(t *testing.T) {
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		t.Fatal("Handle must not run for a non-POST request")
		return Accepted, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/webhooks/test?secret="+webhookTestSecret, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestWebhookRejectsAWrongSecretWithoutABody(t *testing.T) {
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		t.Fatal("Handle must not run once the secret comparison fails")
		return Accepted, nil
	})

	// A wrong secret must answer identically regardless of the request
	// body's length — nothing here narrows it down to a body-inspection
	// bug rather than the secret check itself.
	for name, requestBody := range map[string]string{
		"empty body": "",
		"short body": "x",
		"large body": strings.Repeat("a", 900),
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/test?secret=wrong", strings.NewReader(requestBody))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("body = %q, want empty — a wrong secret must name no connection ids", rec.Body.String())
			}
		})
	}
}

func TestWebhookReturnsSuccessForAPoisonPayload(t *testing.T) {
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		return Poison, errors.New("malformed payload")
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/test?secret="+webhookTestSecret, strings.NewReader("garbage"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// A poison payload must NOT invite redelivery: the provider gets the
	// same 2xx it would for success, because the same bytes would fail
	// identically every time.
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status = %d, want 2xx", rec.Code)
	}
}

func TestWebhookReturns500ForATransientFault(t *testing.T) {
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		return Transient, errors.New("database unreachable")
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/test?secret="+webhookTestSecret, strings.NewReader("payload"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// A transient fault MUST invite redelivery: only 500 tells the
	// provider to retry the same delivery later.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// webhookRequest is one POST at addr, carrying secret.
func webhookRequest(secret, addr string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/test?secret="+secret, strings.NewReader("{}"))
	req.RemoteAddr = addr
	return req
}

// status answers one request through the chassis and reports the status code.
//
// Not extinbound_test.go's `serve`: that one takes a *http.ServeMux, and these
// tests hold the chassis handler itself so a mount is not standing between the
// assertion and the rule it is about.
func status(h http.Handler, req *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestWebhookStopsServingAnAddressThatKeepsGuessingTheSecret(t *testing.T) {
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		t.Fatal("Handle must not run for a request that never cleared admission")
		return Accepted, nil
	})

	// Every guess inside the budget is refused the same way, telling an
	// attacker nothing about how close to the ceiling they are.
	for guess := 1; guess <= webhookRefusalLimit; guess++ {
		if got := status(h, webhookRequest("wrong", "198.51.100.7:4001")); got != http.StatusUnauthorized {
			t.Fatalf("guess %d: status = %d, want 401", guess, got)
		}
	}

	if got := status(h, webhookRequest("wrong", "198.51.100.7:4002")); got != http.StatusTooManyRequests {
		t.Fatalf("the guess past the budget: status = %d, want 429 — an unmetered edge lets a "+
			"deployment-wide secret be guessed at line rate, and Graph signs nothing that would "+
			"stop it", got)
	}
}

func TestWebhookGoesOnServingTheProviderWhileAnotherAddressIsBlocked(t *testing.T) {
	delivered := 0
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		delivered++
		return Accepted, nil
	})

	for guess := 0; guess <= webhookRefusalLimit; guess++ {
		status(h, webhookRequest("wrong", "198.51.100.7:4001"))
	}

	// The budget is per address, so the flood cannot cost the provider its
	// notifications — which is the whole reason a meter is safe to put here.
	if got := status(h, webhookRequest(webhookTestSecret, "203.0.113.9:5001")); got != http.StatusOK {
		t.Fatalf("the provider's delivery: status = %d, want 200", got)
	}
	if delivered != 1 {
		t.Fatalf("Handle ran %d time(s), want 1", delivered)
	}
}

func TestWebhookNeverMetersADeliveryThatCarriesTheRightSecret(t *testing.T) {
	delivered := 0
	h := webhookTestHandler(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
		delivered++
		return Accepted, nil
	})

	// Well past the budget, from ONE address: a Pub/Sub burst for a large
	// fleet arrives this way, and nobody has measured what its peak is. The
	// meter counts refusals, so the answer does not depend on that number.
	const burst = webhookRefusalLimit * 5
	for delivery := 1; delivery <= burst; delivery++ {
		if got := status(h, webhookRequest(webhookTestSecret, "203.0.113.9:5001")); got != http.StatusOK {
			t.Fatalf("delivery %d: status = %d, want 200 — a meter on deliveries would need a "+
				"measurement nobody has, and a ceiling set too low drops real mail", delivery, got)
		}
	}
	if delivered != burst {
		t.Fatalf("Handle ran %d time(s), want %d", delivered, burst)
	}
}

func TestWebhookRefusalSaysWhichAddressItCameFrom(t *testing.T) {
	var written strings.Builder
	h := Webhook(
		webhookTestSpec(t, func(context.Context, *http.Request, []byte) (Disposition, error) {
			return Accepted, nil
		}),
		slog.New(slog.NewTextHandler(&written, nil)),
	)

	status(h, webhookRequest("wrong", "198.51.100.7:4001"))

	// Until this line existed the only record of a refusal was the generic
	// access log, which carries no remote address: an operator could see that
	// a brute force was happening and not where from.
	if !strings.Contains(written.String(), "198.51.100.7") {
		t.Fatalf("the refusal log names no client address:\n%s", written.String())
	}
}
