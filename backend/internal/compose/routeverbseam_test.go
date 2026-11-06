// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

var shareVerb = agents.RouteVerb{
	Verb: "share_record", Method: http.MethodPost, Pattern: "/v1/records/{record_type}/{id}/grants",
	PathParams: []string{"record_type", "id"}, Body: true, IfMatch: true,
}

// The route runs as the call names it: method, path, query, body, version, and
// the marker naming that one route.
func TestARouteCallReachesItsRouteAsTheCallNamesIt(t *testing.T) {
	var got *http.Request
	var body string
	door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"g1"}`))
	})}
	out, err := routeThrough(door)(context.Background(), agents.RouteCall{
		Verb: shareVerb, Path: map[string]string{"record_type": "deal", "id": "d 1"},
		Query: url.Values{"notify": {"true"}}, Version: "7", Body: json.RawMessage(`{"team_id":"t1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"id":"g1"}` {
		t.Errorf("answer = %s, want the route's body", out)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/v1/records/deal/d 1/grants" || got.URL.RawQuery != "notify=true" {
		t.Errorf("request = %s %s?%s", got.Method, got.URL.Path, got.URL.RawQuery)
	}
	if got.Header.Get("If-Match") != "7" || got.Header.Get("Content-Type") != "application/json" || body != `{"team_id":"t1"}` {
		t.Errorf("headers %v body %q", got.Header, body)
	}
	if meant, _ := got.Context().Value(admittedByTheToolKey{}).(string); meant != "POST /v1/records/{record_type}/{id}/grants" {
		t.Errorf("marker = %q, want the one route the verb meant", meant)
	}
}

func TestARouteAnswerSaysWhatTheWriteDid(t *testing.T) {
	answer := func(status int, body string) (json.RawMessage, error) {
		door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})}
		return routeThrough(door)(context.Background(), agents.RouteCall{Verb: shareVerb, Path: map[string]string{"record_type": "deal", "id": "d1"}})
	}
	if out, err := answer(http.StatusNoContent, ""); err != nil || string(out) != `{"status":204}` {
		t.Errorf("204 answered %s, %v; want the status, not an empty text body", out, err)
	}
	if _, err := answer(http.StatusConflict, `{"code":"version_skew","detail":"the deal changed"}`); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("version_skew answered %v, want ErrVersionSkew", err)
	}
	if _, err := answer(http.StatusConflict, `{"code":"taken","detail":"already shared"}`); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("another conflict answered %v, want ErrConflict", err)
	}
}

// The effect has committed before the body overflows, so the overflow is a
// success that says it was cut. An error would read as "retry".
func TestAnOverflowingWriteAnswerIsStillASuccess(t *testing.T) {
	door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`"` + strings.Repeat("x", maxViewAnswerBytes) + `"`))
	})}
	out, err := routeThrough(door)(context.Background(), agents.RouteCall{Verb: shareVerb, Path: map[string]string{"record_type": "deal", "id": "d1"}})
	if err != nil || string(out) != `{"status":201,"truncated":true}` {
		t.Errorf("overflow answered %s, %v", out, err)
	}
}

// Nothing has happened when a read overflows, so it is refused rather than
// answered in a shape its schema does not declare.
func TestAnOverflowingReadAnswerIsRefused(t *testing.T) {
	door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"` + strings.Repeat("x", maxViewAnswerBytes) + `"`))
	})}
	verb := agents.RouteVerb{Verb: "read_big", Method: http.MethodGet, Pattern: "/v1/big"}
	out, err := routeThrough(door)(context.Background(), agents.RouteCall{Verb: verb})
	if err == nil || !strings.Contains(err.Error(), "narrow it") {
		t.Errorf("an overflowing read answered %s, %v, want it refused", out, err)
	}
}

func TestARouteVerbNeedsTheAPIProcess(t *testing.T) {
	_, err := routeThrough(nil)(context.Background(), agents.RouteCall{Verb: shareVerb})
	if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Errorf("no door answered %v, want ErrUnsupportedBySoR", err)
	}
}
