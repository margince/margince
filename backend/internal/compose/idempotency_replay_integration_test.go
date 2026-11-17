// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The replay half of the Idempotency-Key promise, against the real claim
// table: a replay repeats the ORIGINAL status, body and media type. A refusal
// that saved nothing releases the claim; one after a committed write does not.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// keyedTemplateRouter wires a stub per route, as the generated router does.
func keyedTemplateRouter(e *integration.Env, handler http.HandlerFunc) chi.Router {
	r := chi.NewRouter()
	r.With(idempotency(e.Pool, nil, nil)).Post("/v1/offer-templates", handler)
	return r
}

func keyedTemplateCall(ctx context.Context, r chi.Router, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/offer-templates", strings.NewReader(`{"name":"t"}`)).WithContext(ctx)
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// The first request's context ends as it answers; its answer is still recorded.
func TestIdempotencyReplayRepeatsTheRecordedContentType(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := e.Admin() // ONE principal: the claim is scoped per (workspace, principal, key, path)
	ending, end := context.WithCancel(ctx)

	calls := 0
	r := keyedTemplateRouter(e, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusCreated)
		if _, err := io.WriteString(w, `{"recorded":true}`); err != nil {
			t.Errorf("writing the stub response: %v", err)
		}
		end()
	})

	first := keyedTemplateCall(ending, r, "content-type-replay")
	if first.Code != http.StatusCreated {
		t.Fatalf("first keyed call = %d, want 201", first.Code)
	}

	replay := keyedTemplateCall(ctx, r, "content-type-replay")
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1 — the second call must be a replay", calls)
	}
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay = %d %q, want the recorded 201 %q", replay.Code, replay.Body.String(), first.Body.String())
	}
	if ct := replay.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("replayed Content-Type = %q, want the recorded application/problem+json — a replay repeats the original response verbatim, media type included", ct)
	}
}

// The first attempt's context ends before it answers; its key is still released.
func TestIdempotencyFailedAttemptRetryIsAFreshExecution(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ending, end := context.WithCancel(e.Admin())

	calls := 0
	r := keyedTemplateRouter(e, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		end()
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		if _, err := io.WriteString(w, `{"code":"validation_error"}`); err != nil {
			t.Errorf("writing the stub response: %v", err)
		}
	})

	// The 422 the retry sees is the handler's own, never a stored copy.
	first := keyedTemplateCall(ending, r, "failure-retry")
	retry := keyedTemplateCall(e.Admin(), r, "failure-retry")
	if calls != 2 {
		t.Fatalf("handler ran %d times, want 2 — a failed attempt must release the claim for the retry", calls)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"first": first, "retry": retry} {
		if rec.Code != http.StatusUnprocessableEntity || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s = %d %q, want 422 application/problem+json", name, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

// A refusal after a committed write keeps its key: the retry must not save twice.
func TestARefusalAfterACommittedWriteIsNotRunAgain(t *testing.T) {
	e := integration.Setup(t)
	ending, end := context.WithCancel(e.Admin())
	calls := 0
	r := keyedTemplateRouter(e, func(w http.ResponseWriter, req *http.Request) {
		calls++
		if err := database.WithWorkspaceTx(req.Context(), e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(req.Context(), `SELECT txid_current()`)
			return err
		}); err != nil {
			t.Errorf("committing the write: %v", err)
		}
		end()
		httperr.Write(w, req, req.Context().Err())
	})
	keyedTemplateCall(ending, r, "committed-then-failed")
	retry := keyedTemplateCall(e.Admin(), r, "committed-then-failed")
	if calls != 1 || !strings.Contains(retry.Body.String(), "idempotency_key_failed") {
		t.Fatalf("retry = %d %q after %d runs, want idempotency_key_failed and one run", retry.Code, retry.Body.String(), calls)
	}
}

// keyedContactRouter wires a stub on the contact-update route replayScope probes.
func keyedContactRouter(e *integration.Env, handler http.HandlerFunc) chi.Router {
	r := chi.NewRouter()
	r.With(idempotency(e.Pool, nil, nil)).Patch("/v1/contacts/{id}", handler)
	return r
}

func keyedContactCall(ctx context.Context, r chi.Router, contact ids.UUID, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/v1/contacts/"+contact.String(), strings.NewReader(`{"full_name":"Renamed"}`)).WithContext(ctx)
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// API-CC-8: the recorded body is a retransmission, not a receipt that outlives
// the authority it was produced under. A caller who can no longer see the
// record gets the read path's 404, not the bytes.
func TestReplayRefusesOnceTheCallerHasLostSightOfTheRecord(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	contact := e.SeedContact(t, "Visible then not", &e.Rep1)

	calls := 0
	r := keyedContactRouter(e, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, `{"id":"`+contact.String()+`","full_name":"Renamed"}`); err != nil {
			t.Errorf("writing the stub response: %v", err)
		}
	})

	const key = "replay-after-revocation"
	if first := keyedContactCall(rep1, r, contact, key); first.Code != http.StatusOK {
		t.Fatalf("first call = %d, want 200", first.Code)
	}

	// While the record is visible the key replays and does not re-execute;
	// otherwise a gate that broke replay outright would pass the 404 below.
	replay := keyedContactCall(rep1, r, contact, key)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay while visible = %d, want the recorded 200", replay.Code)
	}
	if !strings.Contains(replay.Body.String(), "Renamed") {
		t.Fatalf("replay while visible returned %q, want the recorded body", replay.Body.String())
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1 — the replay re-executed instead of replaying", calls)
	}

	// Capture-private to another rep takes the contact out of this caller's scope.
	e.MakeCapturePrivate(t, "contact", contact, e.Rep3)

	after := keyedContactCall(rep1, r, contact, key)
	if after.Code != http.StatusNotFound {
		t.Fatalf("replay after losing sight = %d, want 404 — a stored response must not outlive the authority it was produced under (API-CC-8)", after.Code)
	}
	if strings.Contains(after.Body.String(), "Renamed") {
		t.Fatalf("replay after losing sight leaked the recorded body: %q", after.Body.String())
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1 — the refused replay must not re-execute the mutation either", calls)
	}
}
