// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The Graph change-notification webhook end to end over the real mux: a
// notification for a connected mailbox zeroes its pacing clock and enqueues its
// sync, a wrong token is refused before any work, and Microsoft's
// endpoint-ownership handshake is answered — but only for a caller that already
// cleared the token, or the endpoint would reflect attacker-chosen bytes under
// our own origin.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// graphNotification is the batch envelope Microsoft posts. clientState is the
// mailbox owner's address, which is what the subscription put there.
func graphNotification(email string) []byte {
	return []byte(`{"value":[{"subscriptionId":"sub-1","changeType":"created",` +
		`"resource":"Users/00000000-0000-0000-0000-000000000000/Messages/AAA",` +
		`"clientState":"` + email + `"}]}`)
}

func TestGraphNotificationRoutesToTheConnection(t *testing.T) {
	e := integration.SetupSearch(t)
	const mailbox = "outlook-owner@ws.example"

	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(&moodyConnector{name: "graph"})
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "graph", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var seededNext time.Time
	// The connector's cursor carries the provider-owned mailbox identity —
	// exactly what a real graph sync writes — and the pacing clock sits in the
	// future so only the notification can make the connection due.
	err = database.WithWorkspaceTx(grantCtx, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			UPDATE capture_connection SET sync_cursor = $2 WHERE id = $1`,
			connID, `{"delta_link":"https://graph/delta?token=d1","email":"`+mailbox+`"}`); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `
			INSERT INTO capture_sync_state (connection_id, next_sync_at)
			SELECT id, now() + interval '1 hour' FROM capture_connection WHERE id = $1
			RETURNING next_sync_at`,
			connID).Scan(&seededNext)
	})
	if err != nil {
		t.Fatal(err)
	}

	integration.ApplyRiverSchema(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	inserter, err := jobs.NewInserter(e.Pool, quiet)
	if err != nil {
		t.Fatal(err)
	}
	const token = "graph-secret"
	handler := compose.New(e.Pool, quiet, compose.WithGraphPush(inserter, compose.GraphPushConfig{Token: token}))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	postFull := func(path string, body []byte) (int, string, http.Header) {
		t.Helper()
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		read, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %v", err)
		}
		return resp.StatusCode, string(read), resp.Header
	}
	post := func(path string, body []byte) (int, string) {
		t.Helper()
		code, read, _ := postFull(path, body)
		return code, read
	}

	t.Run("wrong token is refused before any work", func(t *testing.T) {
		code, _ := post("/webhooks/graph?token=wrong", graphNotification(mailbox))
		if code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
		// BEFORE any work, not merely refused after it: a handler that bumped
		// the clock and enqueued the sync and then answered 401 would pass a
		// status-only check, and the later valid delivery — unique while
		// incomplete — would find the job already there and look correct.
		next, jobRows := graphSyncState(grantCtx, t, e, connID)
		if !next.Equal(seededNext) {
			t.Errorf("next_sync_at moved to %s on a refused delivery", next)
		}
		if jobRows != 0 {
			t.Errorf("%d sync job(s) enqueued by a refused delivery", jobRows)
		}
	})

	// Microsoft will not create a subscription until the URL echoes this back,
	// so a deployment whose handshake fails has push nowhere at all.
	t.Run("the handshake is answered for an authenticated caller", func(t *testing.T) {
		const validation = "Validation: Testing client application reachability"
		q := url.Values{"token": {token}, "validationToken": {validation}}
		code, body, hdr := postFull("/webhooks/graph?"+q.Encode(), nil)
		if code != http.StatusOK || body != validation {
			t.Fatalf("handshake = (%d, %q), want (200, the token verbatim)", code, body)
		}
		// The headers are the point, not decoration: these are the provider's
		// bytes reflected under this origin, and text/plain + nosniff are what
		// stop a browser re-reading them as markup.
		if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("Content-Type = %q, want text/plain", ct)
		}
		if hdr.Get("X-Content-Type-Options") != "nosniff" {
			t.Error("the echo is served without nosniff; reflected provider bytes could be sniffed as markup")
		}
	})

	t.Run("and refused, reflecting nothing, for anybody else", func(t *testing.T) {
		q := url.Values{"token": {"wrong"}, "validationToken": {"reflect-me"}}
		code, body := post("/webhooks/graph?"+q.Encode(), nil)
		if code != http.StatusUnauthorized || body != "" {
			t.Fatalf("unauthenticated handshake = (%d, %q), want (401, nothing)", code, body)
		}
	})

	t.Run("a notification zeroes the pacing clock and enqueues the sync", func(t *testing.T) {
		code, _ := post("/webhooks/graph?token="+token, graphNotification(mailbox))
		// 202, which is what Microsoft treats as delivered.
		if code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", code)
		}
		next, jobRows := graphSyncState(grantCtx, t, e, connID)
		if !next.Before(seededNext) {
			t.Fatalf("next_sync_at = %s, want moved earlier than the seeded %s", next, seededNext)
		}
		if jobRows != 1 {
			t.Fatalf("capture_sync jobs for the connection = %d, want exactly 1", jobRows)
		}
	})

	t.Run("a redelivery cannot double-enqueue", func(t *testing.T) {
		if code, _ := post("/webhooks/graph?token="+token, graphNotification(mailbox)); code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", code)
		}
		if _, jobRows := graphSyncState(grantCtx, t, e, connID); jobRows != 1 {
			t.Fatalf("capture_sync jobs after redelivery = %d, want still 1 (unique while incomplete)", jobRows)
		}
	})

	t.Run("a mailbox nobody connected is accepted, not retried", func(t *testing.T) {
		// Nothing here a redelivery would fix, and Microsoft counts a failure
		// against the endpoint's health — enough of those drop the subscription.
		if code, _ := post("/webhooks/graph?token="+token, graphNotification("stranger@elsewhere.example")); code < 200 || code >= 300 {
			t.Fatalf("status = %d, want 2xx", code)
		}
	})

	t.Run("malformed input is poison, not retried", func(t *testing.T) {
		if code, _ := post("/webhooks/graph?token="+token, []byte("not json")); code < 200 || code >= 300 {
			t.Fatalf("garbage body status = %d, want 2xx", code)
		}
		// A notification with no clientState belongs to a subscription somebody
		// else made against this URL: unroutable, and not ours to retry.
		if code, _ := post("/webhooks/graph?token="+token, []byte(`{"value":[{"subscriptionId":"x"}]}`)); code < 200 || code >= 300 {
			t.Fatalf("missing clientState status = %d, want 2xx", code)
		}
	})
}

// graphSyncState reads the pacing clock and the enqueued sync count for one
// connection.
func graphSyncState(ctx context.Context, t *testing.T, e *integration.SearchEnv, connID interface{ String() string }) (time.Time, int) {
	t.Helper()
	var next time.Time
	var jobRows int
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			SELECT next_sync_at FROM capture_sync_state WHERE connection_id = $1`, connID).Scan(&next); err != nil {
			return err
		}
		// river_job is not tenant-scoped; count the enqueued sync for this
		// connection.
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM river_job
			WHERE kind = 'capture_sync' AND args->>'connection_id' = $1`, connID.String()).Scan(&jobRows)
	}); err != nil {
		t.Fatal(err)
	}
	return next, jobRows
}
