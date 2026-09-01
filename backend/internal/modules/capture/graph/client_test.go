// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// msStub routes the handful of Microsoft identity/Graph endpoints the client
// calls onto canned responses, so the REST/parse logic is proven with no
// network. Delta continuation links are rewritten onto the stub's own origin
// at request time (the client refuses off-origin links by design).
func msStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		// Microsoft's token endpoint answers non-2xx on bad input; the client
		// maps any 4xx to ErrAuthRejected regardless of body, so a bare status
		// is all the stub needs (WriteHeader, not httperr — this fakes Microsoft).
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			writeJSON(w, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3599})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"access_token": "access-2", "refresh_token": "refresh-2", "expires_in": 3599})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	mux.HandleFunc("/me", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"mail": "rep@myco.com", "userPrincipalName": "rep@myco.onmicrosoft.com"})
	})

	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("$deltatoken") {
		case "gone":
			// Graph answers an expired delta state with 410 Gone.
			w.WriteHeader(http.StatusGone)
		case "d1":
			writeJSON(w, map[string]any{
				"value":            []map[string]any{{"id": "m3"}, {"id": "tombstone", "@removed": map[string]string{"reason": "deleted"}}},
				"@odata.deltaLink": srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=d2",
			})
		default:
			// The opening page of a fresh delta round: one page, then the next.
			if r.URL.Query().Get("$skiptoken") == "p2" {
				writeJSON(w, map[string]any{
					"value":            []map[string]any{{"id": "m2", "receivedDateTime": "2026-07-12T00:00:00Z"}},
					"@odata.deltaLink": srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=d1",
				})
				return
			}
			writeJSON(w, map[string]any{
				"value":           []map[string]any{{"id": "m1", "receivedDateTime": "2026-07-12T00:00:00Z"}},
				"@odata.nextLink": srv.URL + "/me/mailFolders/inbox/messages/delta?%24skiptoken=p2",
			})
		}
	})

	mux.HandleFunc("/me/mailFolders/{folder}/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ConsistencyLevel") != "eventual" {
			// $count without the header fails on real Graph; the stub makes
			// that contract visible as a client-side failure.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// The two the delta round hands back, so a walk of the whole folder
		// measures up and the anchor stands.
		writeJSON(w, map[string]any{"@odata.count": 2, "value": []map[string]any{{"id": "m1"}}})
	})

	mux.HandleFunc("/me/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("$count") == "true" {
			if r.Header.Get("ConsistencyLevel") != "eventual" {
				// $count without the header fails on real Graph; the stub makes
				// that contract visible as a client-side failure.
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"@odata.count": 4200, "value": []map[string]any{{"id": "m1"}}})
			return
		}
		if r.URL.Query().Get("$skiptoken") == "p2" {
			writeJSON(w, map[string]any{"value": []map[string]any{{"id": "m2", "parentFolderId": "sent-folder"}}})
			return
		}
		writeJSON(w, map[string]any{
			"value":           []map[string]any{{"id": "m1", "parentFolderId": "inbox-folder"}},
			"@odata.nextLink": srv.URL + "/me/messages?%24skiptoken=p2",
		})
	})

	mux.HandleFunc("/me/mailFolders/sentitems", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": "sent-folder"})
	})

	mux.HandleFunc("/me/messages/m1/$value", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "message/rfc822")
		//craft:ignore swallowed-errors test stub write; a short write surfaces as the client-side assertion failure
		_, _ = w.Write([]byte("Subject: hi\r\n\r\nbody"))
	})

	mux.HandleFunc("/me/messages/throttled/$value", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	mux.HandleFunc("/me/messages/gone/$value", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // deleted between enumeration and fetch
	})

	mux.HandleFunc("/me/messages/huge/$value", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "message/rfc822")
		// One byte past the 8 MiB cap — an oversized message.
		//craft:ignore swallowed-errors test stub write; a short write surfaces as the client-side assertion
		_, _ = w.Write(make([]byte, (8<<20)+1))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

//craft:ignore naked-any v is an arbitrary canned JSON response body for the stub
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	//craft:ignore swallowed-errors test stub write; an encode failure surfaces as the client-side decode error the assertion checks
	_ = json.NewEncoder(w).Encode(v)
}

func newTestClients(t *testing.T) (OAuth, API) {
	srv := msStub(t)
	oauth := NewOAuth(OAuthConfig{
		ClientID:     "cid",
		ClientSecret: "secret",
		Scopes:       []string{"offline_access", "User.Read", "Mail.Read"},
		AuthURL:      "https://login.example/auth",
		TokenURL:     srv.URL + "/token",
	})
	api := NewAPI(srv.Client(), srv.URL)
	return oauth, api
}

func TestAuthCodeURLCarriesStateAndScopes(t *testing.T) {
	oauth, _ := newTestClients(t)
	got := oauth.AuthCodeURL("state-xyz", "https://app/callback")
	for _, want := range []string{"state=state-xyz", "client_id=cid", "offline_access", "Mail.Read", "response_type=code"} {
		if !strings.Contains(got, want) {
			t.Errorf("authorize_url missing %q: %s", want, got)
		}
	}
}

func TestDefaultEndpointsUseTheConfiguredTenant(t *testing.T) {
	got := NewOAuth(OAuthConfig{ClientID: "cid", Tenant: "contoso.example"}).AuthCodeURL("s", "https://app/cb")
	if !strings.HasPrefix(got, "https://login.microsoftonline.com/contoso.example/oauth2/v2.0/authorize?") {
		t.Errorf("tenant endpoint = %q, want the contoso.example authorize URL", got)
	}
	common := NewOAuth(OAuthConfig{ClientID: "cid"}).AuthCodeURL("s", "https://app/cb")
	if !strings.HasPrefix(common, "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?") {
		t.Errorf("default endpoint = %q, want the common authorize URL", common)
	}
}

func TestExchangeReturnsRefreshToken(t *testing.T) {
	oauth, _ := newTestClients(t)
	grant, err := oauth.Exchange(context.Background(), "the-code", "https://app/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if grant.RefreshToken != "refresh-1" {
		t.Errorf("refresh token = %q, want refresh-1", grant.RefreshToken)
	}
}

func TestAccessTokenRefreshes(t *testing.T) {
	oauth, _ := newTestClients(t)
	at, err := oauth.AccessToken(context.Background(), "refresh-1")
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if at != "access-2" {
		t.Errorf("access token = %q, want access-2", at)
	}
}

func TestAccessTokenRejectedRefreshMapsSentinel(t *testing.T) {
	oauth, _ := newTestClients(t)
	if _, err := oauth.AccessToken(context.Background(), "revoked"); !errors.Is(err, ErrAuthRejected) {
		t.Fatalf("want ErrAuthRejected for a revoked refresh, got %v", err)
	}
}

func TestProfilePrefersMailOverUPN(t *testing.T) {
	_, api := newTestClients(t)
	email, err := api.Profile(context.Background(), "access-2")
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if email != "rep@myco.com" {
		t.Errorf("Profile = %q, want the mail attribute over userPrincipalName", email)
	}
}

func TestProfileFallsBackToUPN(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"userPrincipalName": "rep@myco.onmicrosoft.com"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	email, err := NewAPI(srv.Client(), srv.URL).Profile(context.Background(), "at")
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if email != "rep@myco.onmicrosoft.com" {
		t.Errorf("Profile = %q, want the userPrincipalName fallback", email)
	}
}

func TestDeltaInitWalksPagesFiltersTombstonesAndReturnsDeltaLink(t *testing.T) {
	_, api := newTestClients(t)
	ids, delta, err := api.DeltaInit(context.Background(), "access-2", folderInbox, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeltaInit: %v", err)
	}
	if strings.Join(ids, ",") != "m1,m2" {
		t.Errorf("ids = %v, want [m1 m2] across both pages", ids)
	}
	if !strings.Contains(delta, "%24deltatoken=d1") && !strings.Contains(delta, "$deltatoken=d1") {
		t.Errorf("deltaLink = %q, want the d1 link that closes the round", delta)
	}
}

func TestDeltaResumeCollectsAddedAndSkipsRemoved(t *testing.T) {
	srv := msStub(t)
	api := NewAPI(srv.Client(), srv.URL)
	ids, delta, err := api.Delta(context.Background(), "access-2", srv.URL+"/me/mailFolders/inbox/messages/delta?%24deltatoken=d1")
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	if strings.Join(ids, ",") != "m3" {
		t.Errorf("ids = %v, want [m3] (the tombstoned entry is not fetched)", ids)
	}
	if !strings.Contains(delta, "d2") {
		t.Errorf("advanced deltaLink = %q, want the d2 link", delta)
	}
}

func TestDeltaGoneMapsCursorSentinel(t *testing.T) {
	srv := msStub(t)
	api := NewAPI(srv.Client(), srv.URL)
	if _, _, err := api.Delta(context.Background(), "access-2", srv.URL+"/me/mailFolders/inbox/messages/delta?%24deltatoken=gone"); !errors.Is(err, ErrDeltaGone) {
		t.Fatalf("want ErrDeltaGone for a 410 delta, got %v", err)
	}
}

func TestDeltaRefusesOffOriginLink(t *testing.T) {
	srv := msStub(t)
	api := NewAPI(srv.Client(), srv.URL)
	if _, _, err := api.Delta(context.Background(), "access-2", "https://attacker.example/steal-token"); err == nil {
		t.Fatal("Delta must refuse a deltaLink that points off the Graph API")
	}
}

func TestGetMIMEReturnsRawBytes(t *testing.T) {
	_, api := newTestClients(t)
	raw, err := api.GetMIME(context.Background(), "access-2", "m1")
	if err != nil {
		t.Fatalf("GetMIME: %v", err)
	}
	if !strings.Contains(string(raw), "Subject: hi") {
		t.Errorf("MIME = %q, want it to contain the header", raw)
	}
}

func TestGetMIMESkipsAVanishedMessage(t *testing.T) {
	_, api := newTestClients(t)
	_, err := api.GetMIME(context.Background(), "access-2", "gone")
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("a 404 (deleted after enumeration) must skip, not wedge the sync, got %v", err)
	}
}

func TestGetMIMERefusesOversizedMessage(t *testing.T) {
	_, api := newTestClients(t)
	_, err := api.GetMIME(context.Background(), "access-2", "huge")
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("an oversized message must be a skip, not truncated capture, got %v", err)
	}
}

func TestThrottledCallMapsRateLimitWithRetryAfter(t *testing.T) {
	_, api := newTestClients(t)
	_, err := api.GetMIME(context.Background(), "access-2", "throttled")
	if !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("want ErrRateLimited for a 429, got %v", err)
	}
	var rl *connector.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, want 17s from the provider header", rl)
	}
}

func TestEstimateAfterReadsODataCount(t *testing.T) {
	_, api := newTestClients(t)
	n, err := api.EstimateAfter(context.Background(), "access-2", time.Date(2026, 1, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EstimateAfter: %v", err)
	}
	if n != 4200 {
		t.Errorf("estimate = %d, want 4200 (@odata.count)", n)
	}
}

// messageIDs projects a listed page down to its ids so a page assertion reads
// as the id sequence it is about.
func messageIDs(msgs []MessageRef) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}

// The backfill's T1 evidence: Graph names the folder it filed each message in,
// and SentFolderID resolves the well-known Sent Items id the comparison needs.
// Without the pairing the backfill cannot tell the owner's own mail from an
// inbound message whose From header merely claims to be theirs.
func TestListAfterCarriesTheParentFolderAgainstSentItems(t *testing.T) {
	_, api := newTestClients(t)
	sent, err := api.SentFolderID(context.Background(), "access-2")
	if err != nil {
		t.Fatalf("SentFolderID: %v", err)
	}
	inbox, next, err := api.ListAfter(context.Background(), "access-2", time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListAfter: %v", err)
	}
	if len(inbox) != 1 || inbox[0].ParentFolderID == sent {
		t.Fatalf("first page = %v, want one message filed outside %q", inbox, sent)
	}
	outbox, _, err := api.ListAfter(context.Background(), "access-2", time.Time{}, next, 100)
	if err != nil {
		t.Fatalf("ListAfter page 2: %v", err)
	}
	if len(outbox) != 1 || outbox[0].ParentFolderID != sent {
		t.Fatalf("second page = %v, want one message filed in %q", outbox, sent)
	}
}

func TestListAfterPagesViaNextLink(t *testing.T) {
	_, api := newTestClients(t)
	msgs, next, err := api.ListAfter(context.Background(), "access-2", time.Date(2026, 1, 18, 0, 0, 0, 0, time.UTC), "", 100)
	if err != nil {
		t.Fatalf("ListAfter: %v", err)
	}
	if strings.Join(messageIDs(msgs), ",") != "m1" || next == "" {
		t.Fatalf("first page = %v next=%q, want [m1] and a nextLink", msgs, next)
	}
	msgs2, next2, err := api.ListAfter(context.Background(), "access-2", time.Time{}, next, 100)
	if err != nil {
		t.Fatalf("ListAfter page 2: %v", err)
	}
	if strings.Join(messageIDs(msgs2), ",") != "m2" || next2 != "" {
		t.Errorf("second page = %v next=%q, want [m2] and the end of the walk", msgs2, next2)
	}
}

func TestListAfterRefusesOffOriginToken(t *testing.T) {
	_, api := newTestClients(t)
	if _, _, err := api.ListAfter(context.Background(), "access-2", time.Time{}, "https://attacker.example/page", 100); err == nil {
		t.Fatal("ListAfter must refuse a page token that points off the Graph API")
	}
}

func TestClientUnreachableWhenServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NewServeMux())
	srv.Close() // nothing will answer
	api := NewAPI(srv.Client(), srv.URL)
	if _, err := api.Profile(context.Background(), "at"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("Profile against a closed server = %v, want ErrUnreachable", err)
	}
}

// Microsoft's error.code is what tells an operator WHICH refusal this was —
// a revoked token reads differently from an app that was never granted the
// permission — so it has to survive the transport while the class stays put.
func TestGraphRefusalCarriesMicrosoftsErrorCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/messages", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		//craft:ignore swallowed-errors test stub write
		_, _ = w.Write([]byte(`{"error":{"code":"Authorization_RequestDenied","message":"Insufficient privileges."}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	api := NewAPI(srv.Client(), srv.URL)

	_, err := api.EstimateAfter(context.Background(), "access", time.Time{})

	if !errors.Is(err, ErrAuthRejected) {
		t.Fatalf("err = %v, want ErrAuthRejected", err)
	}
	if got := connector.ProviderReason(err); got != "Authorization_RequestDenied" {
		t.Errorf("ProviderReason = %q, want Authorization_RequestDenied", got)
	}
	pe, ok := errors.AsType[*connector.ProviderError](err)
	if !ok {
		t.Fatalf("err = %v, want a *connector.ProviderError", err)
	}
	if pe.Status != http.StatusForbidden {
		t.Errorf("Status = %d, want 403", pe.Status)
	}
}

// requestOp names the endpoint as a PATH: the query carries cursors and filters
// that have no place in an error string, and the scheme+host are fixed for the
// deployment — so an op reads like the other connectors' rather than embedding a
// host (an ephemeral port, under test).
func TestRequestOpReducesAURLToItsPath(t *testing.T) {
	a := &httpAPI{base: "https://graph.microsoft.com/v1.0"}
	for name, tc := range map[string]struct{ in, want string }{
		"filter and count": {
			"https://graph.microsoft.com/v1.0/me/messages?$filter=x&$count=true",
			"/me/messages",
		},
		"delta token": {
			"https://graph.microsoft.com/v1.0/me/messages/delta?$deltatoken=abc123",
			"/me/messages/delta",
		},
		"no query": {"https://graph.microsoft.com/v1.0/me", "/me"},
		// A URL that is not under the base keeps its full form rather than being
		// silently mangled — sameAPIOrigin refuses those before they are fetched,
		// so one appearing here is worth seeing whole.
		"off base": {"https://elsewhere.example/x", "https://elsewhere.example/x"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := a.requestOp(tc.in); got != tc.want {
				t.Errorf("requestOp(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A 2xx that names no folder is not an answer. Left as "", it would compare
// equal to the empty parentFolderId of any message the same degraded response
// returned, so the ABSENCE of the evidence would read as the evidence and
// attest a whole page — and the activity natural key would make it permanent.
func TestSentFolderIDRefusesA2xxThatNamesNoFolder(t *testing.T) {
	srv := jsonStub(t, map[string]any{})
	_, err := NewAPI(srv.Client(), srv.URL).SentFolderID(context.Background(), "access-2")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("SentFolderID on an id-less 2xx = %v, want ErrUnreachable — an empty id must never reach the comparison", err)
	}
}

// The mirror case, and it must fail the same way. A listed message naming no
// parent folder cannot be captured on a guess in either direction: attested, an
// empty id would match an unresolved folder; un-attested, the activity natural
// key would permanently discard evidence the owner really did produce.
func TestListAfterRefusesAMessageWithNoParentFolder(t *testing.T) {
	srv := jsonStub(t, map[string]any{"value": []map[string]any{{"id": "m-truncated"}}})
	_, _, err := NewAPI(srv.Client(), srv.URL).ListAfter(context.Background(), "access-2", time.Time{}, "", 100)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("ListAfter on a folder-less message = %v, want ErrUnreachable", err)
	}
}

// A BODY PAST THE BUDGET IS REFUSED, not decoded from its prefix.
//
// io.ReadAll over a LimitReader answers exactly the cap with no error, so a
// longer body arrived as a prefix — and a prefix that happens to parse is the
// dangerous shape: a subscription listing missing its tail reads as a mailbox
// with no subscription, and the renewal then creates a second one delivering
// the same notification.
//
// The fixture is a valid document whose first maxResponseBytes bytes are ALSO a
// valid document, which is what makes the truncation invisible without this.
func TestAResponsePastTheBudgetIsRefusedRatherThanDecodedFromItsPrefix(t *testing.T) {
	// A COMPLETE OBJECT FOLLOWED BY WHITESPACE, which is the fixture that can
	// tell the two readings apart. Padding inside a string value would leave the
	// prefix ending mid-string, and THAT fails to decode either way — the case
	// would pass against the very reading it is meant to catch. Trailing
	// whitespace is legal JSON, so both the whole body and every prefix past the
	// closing brace decode cleanly, and only refusing the oversize body reports
	// anything at all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1"}` + strings.Repeat(" ", maxResponseBytes)))
	}))
	t.Cleanup(srv.Close)

	_, err := NewAPI(srv.Client(), srv.URL).SentFolderID(context.Background(), "access-2")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a body past the budget = %v, want ErrUnreachable — a prefix that parses is not the answer", err)
	}
}

// jsonStub serves one canned JSON body on every path — enough for the
// degraded-response cases, which are about what the client REFUSES to decode.
func jsonStub(t *testing.T, body map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// anchorStub serves one folder: a single-page delta round holding the given
// ids, and a $count answering held. It records the filter the count carried.
func anchorStub(t *testing.T, ids []string, held int) (*httptest.Server, *string) {
	t.Helper()
	return datedAnchorStub(t, entriesFor(ids), &held)
}

// anchorAfter is the lower bound every anchor test asks for, so the entries a
// stub hands back and the window they are reconciled against agree.
var anchorAfter = time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

// entriesFor renders delta entries received inside that window, which is what a
// round over a folder of recent mail looks like. Dated, because the round asks
// Graph for the field and an entry without it does not count.
func entriesFor(ids []string) []map[string]any {
	value := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		value = append(value, map[string]any{
			"id":               id,
			"receivedDateTime": anchorAfter.Add(time.Hour).Format(time.RFC3339),
		})
	}
	return value
}

// datedAnchorStub serves one folder: a single-page delta round holding the given
// raw entries, and a $count answering held — omitting @odata.count entirely when
// held is nil. It records the filter the count carried.
func datedAnchorStub(t *testing.T, value []map[string]any, held *int) (*httptest.Server, *string) {
	t.Helper()
	var countFilter string
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value":            value,
			"@odata.deltaLink": srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=d1",
		})
	})
	mux.HandleFunc("/me/mailFolders/inbox/messages", func(w http.ResponseWriter, r *http.Request) {
		countFilter = r.URL.Query().Get("$filter")
		body := map[string]any{"value": []map[string]any{}}
		if held != nil {
			body["@odata.count"] = *held
		}
		writeJSON(w, body)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &countFilter
}

func TestAnAnchorHoldingFewerThanTheFolderReportsIsRefused(t *testing.T) {
	srv, _ := anchorStub(t, []string{"m1"}, 5)
	ids, delta, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, anchorAfter)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable — a round that closed short of the folder's own tally", err)
	}
	if ids != nil || delta != "" {
		t.Errorf("ids = %v, deltaLink = %q, want neither: a watermark stored here would drop the 4 the walk never saw", ids, delta)
	}
	if !strings.Contains(err.Error(), "holds 5") {
		t.Errorf("err = %q, want the tally the walk was measured against", err)
	}
}

func TestAnAnchorHoldingMoreThanTheFolderReportsStands(t *testing.T) {
	// A message arriving mid-walk is inside the walk and outside the closed
	// window the tally covers. That gap must not read as a short round.
	srv, _ := anchorStub(t, []string{"m1", "m2", "m3"}, 2)
	ids, delta, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, anchorAfter)
	if err != nil {
		t.Fatalf("DeltaInit: %v", err)
	}
	if len(ids) != 3 || delta == "" {
		t.Errorf("ids = %v, deltaLink = %q, want all three and the link that closed the round", ids, delta)
	}
}

func TestTheAnchorsTallyCoversTheWindowTheWalkOpenedIn(t *testing.T) {
	after := anchorAfter
	opened := time.Now().UTC()
	srv, filter := anchorStub(t, []string{"m1"}, 1)
	if _, _, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, after); err != nil {
		t.Fatalf("DeltaInit: %v", err)
	}
	closed := time.Now().UTC()
	if !strings.HasPrefix(*filter, "receivedDateTime ge "+after.Format(time.RFC3339)+" and ") {
		t.Fatalf("count filter = %q, want the anchor's own lower bound", *filter)
	}
	_, upper, found := strings.Cut(*filter, " and receivedDateTime lt ")
	if !found {
		t.Fatalf("count filter = %q, want a closed upper bound: an open tally would count arrivals the walk could not have seen", *filter)
	}
	at, err := time.Parse(time.RFC3339, upper)
	if err != nil {
		t.Fatalf("upper bound %q: %v", upper, err)
	}
	// Second-grain rendering floors the bound, so allow the second it truncates.
	if at.Before(opened.Truncate(time.Second)) || at.After(closed) {
		t.Errorf("upper bound = %s, want the instant the walk opened (between %s and %s)", at, opened, closed)
	}
}

// A TALLY THAT DID NOT ANSWER is not a tally of zero.
//
// A 2xx without @odata.count — the shape a request Graph decides not to count
// can take — would read as "the folder holds none", and no walk can fall short
// of none. The reconciliation would pass every round while measuring nothing.
func TestATallyWithNoCountIsRefusedRatherThanReadAsZero(t *testing.T) {
	srv, _ := datedAnchorStub(t, entriesFor([]string{"m1"}), nil)
	_, _, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, anchorAfter)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable — a tally read as zero passes every round while "+
			"measuring nothing, which is this guard wearing the costume of a green check", err)
	}
}

// AN ENTRY FROM OUTSIDE THE WINDOW DOES NOT COUNT TOWARD IT.
//
// A delta tracks a FOLDER, not a query: Microsoft may name a message that falls
// outside the filter because something about it changed — a read/unread flip on
// an old one. Counted, it stands in for an in-window message the round never
// reached, and the short round passes.
func TestAnEntryOlderThanTheWindowDoesNotStandInForOneInsideIt(t *testing.T) {
	after := anchorAfter
	held := 2
	srv, _ := datedAnchorStub(t, []map[string]any{
		{"id": "in-window", "receivedDateTime": after.Add(time.Hour).Format(time.RFC3339)},
		{"id": "changed-old-message", "receivedDateTime": after.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	}, &held)

	ids, _, err := NewAPI(srv.Client(), srv.URL).DeltaInit(context.Background(), "at", folderInbox, after)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable: the round holds ONE message from the window and the "+
			"folder reports two, and an entry from a month earlier is not the missing one", err)
	}
	if ids != nil {
		t.Errorf("ids = %v, want none alongside the refusal", ids)
	}
}

// AND AN ENTRY THE ROUND NAMED IS STILL FETCHED, whatever window it belongs to:
// the reconciliation narrows what COUNTS, never what is captured.
func TestAnEntryOutsideTheWindowIsStillReturnedToBeFetched(t *testing.T) {
	after := anchorAfter
	held := 1
	srv, _ := datedAnchorStub(t, []map[string]any{
		{"id": "in-window", "receivedDateTime": after.Add(time.Hour).Format(time.RFC3339)},
		{"id": "changed-old-message", "receivedDateTime": after.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	}, &held)

	ids, _, err := NewAPI(srv.Client(), srv.URL).DeltaInit(context.Background(), "at", folderInbox, after)
	if err != nil {
		t.Fatalf("DeltaInit: %v", err)
	}
	if strings.Join(ids, ",") != "in-window,changed-old-message" {
		t.Errorf("ids = %v, want both — a message the round named is a message to fetch", ids)
	}
}

// A DELTALINK IS CHECKED FOR ORIGIN LIKE A NEXTLINK, and needs it more: this one
// is STORED. An off-origin deltaLink becomes the cursor, and every later cycle
// then refuses it before fetching anything — a refusal that is not the 410 a
// re-anchor answers, so the mailbox has no way back.
func TestAnOffOriginDeltaLinkIsRefusedRatherThanStored(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"value":            []map[string]any{{"id": "m1"}},
			"@odata.deltaLink": "https://attacker.example/me/mailFolders/inbox/messages/delta?%24deltatoken=d1",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, delta, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, anchorAfter)
	if err == nil {
		t.Fatalf("DeltaInit accepted an off-origin deltaLink and would have stored %q as the cursor", delta)
	}
	if !strings.Contains(err.Error(), "does not point at the graph api") {
		t.Errorf("err = %v, want the origin refusal", err)
	}
}

// THE UPPER BOUND KEEPS ITS SUBSECOND DIGITS. Format truncates, so a
// second-grain bound moves the window back by up to a second and drops whatever
// arrived in it out of the tally — quietly, which is the failure this window is
// for.
func TestTheTallysUpperBoundKeepsSubsecondPrecision(t *testing.T) {
	after := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 8, 1, 9, 30, 15, 123456789, time.UTC)
	got := receivedBetweenFilter(after, before)
	if !strings.HasSuffix(got, "receivedDateTime lt 2026-08-01T09:30:15.123456789Z") {
		t.Errorf("filter = %q, want the bound at the instant it was taken, subseconds and all", got)
	}
}

// AN ENTRY WITH NO INSTANT DOES NOT COUNT TOWARD THE WINDOW.
//
// The opening request asks Graph for receivedDateTime, so an entry that carries
// none is not the ordinary silence of a field nobody asked for: it is an entry
// that cannot be shown to belong to the window being reconciled, and one that
// cannot be shown to belong must not stand in for a message the round never
// reached. The cost is a refusal the next cycle retries; the alternative is a
// watermark stored over a gap, and the watermark only moves forward.
func TestAnUndatedEntryDoesNotStandInForOneInsideTheWindow(t *testing.T) {
	held := 2
	srv, _ := datedAnchorStub(t, []map[string]any{
		{"id": "in-window", "receivedDateTime": anchorAfter.Add(time.Hour).Format(time.RFC3339)},
		{"id": "no-instant"},
	}, &held)

	_, _, err := NewAPI(srv.Client(), srv.URL).DeltaInit(context.Background(), "at", folderInbox, anchorAfter)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable: one entry belongs to the window, one says nothing, "+
			"and the folder reports two", err)
	}
}

// AND THE ROUND ASKS FOR THE FIELD IT RECONCILES ON. Graph does not promise
// receivedDateTime on every entry of a delta round; $select is the request that
// asks, and it rides the deltaLink into every later round of the same stream.
func TestTheAnchorAsksForTheFieldItReconcilesOn(t *testing.T) {
	var selected string
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/me/mailFolders/inbox/messages/delta", func(w http.ResponseWriter, r *http.Request) {
		selected = r.URL.Query().Get("$select")
		writeJSON(w, map[string]any{
			"value":            entriesFor([]string{"m1"}),
			"@odata.deltaLink": srv.URL + "/me/mailFolders/inbox/messages/delta?%24deltatoken=d1",
		})
	})
	mux.HandleFunc("/me/mailFolders/inbox/messages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"@odata.count": 1, "value": []map[string]any{}})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, _, err := NewAPI(srv.Client(), srv.URL).
		DeltaInit(context.Background(), "at", folderInbox, anchorAfter); err != nil {
		t.Fatalf("DeltaInit: %v", err)
	}
	if !strings.Contains(selected, "receivedDateTime") {
		t.Errorf("$select = %q, want receivedDateTime — without asking, the field the "+
			"reconciliation reads is one Graph does not promise", selected)
	}
	if !strings.Contains(selected, "id") {
		t.Errorf("$select = %q, want the id too — there is nothing to fetch without it", selected)
	}
}

// THE TWO SIDES READ THE LOWER BOUND THE SAME WAY.
//
// The filter renders `after` at second grain, so Microsoft counts everything
// from the top of that second. Comparing against the exact instant instead would
// drop a message arriving in the fractional remainder from the walk's side while
// the tally kept it — a complete anchor refused as short, on nothing but a
// rounding difference between the two questions.
func TestASubsecondLowerBoundDoesNotRefuseACompleteAnchor(t *testing.T) {
	// The anchor window is now-minus-a-span, so `after` carries nanoseconds in
	// every real call; this is the ordinary case, not a contrived one.
	after := time.Date(2026, 7, 11, 0, 0, 0, 500_000_000, time.UTC)
	held := 1
	srv, _ := datedAnchorStub(t, []map[string]any{
		// Inside the second the filter opens on, before the instant itself.
		{"id": "arrived-in-the-truncated-remainder", "receivedDateTime": "2026-07-11T00:00:00Z"},
	}, &held)

	ids, _, err := NewAPI(srv.Client(), srv.URL).DeltaInit(context.Background(), "at", folderInbox, after)
	if err != nil {
		t.Fatalf("DeltaInit: %v — Microsoft counted this message and the reconciliation did not, "+
			"which is a rounding difference reported as a short round", err)
	}
	if len(ids) != 1 {
		t.Errorf("ids = %v, want the one message the round named", ids)
	}
}
