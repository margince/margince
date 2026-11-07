// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, count int) *httptest.Server {
	t.Helper()
	s := &server{byOwner: map[string]int{}, connected: map[string]bool{}, now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	for i, owner := range []string{"a@demo.test", "b@demo.test"} {
		s.boxes = append(s.boxes, mailbox{owner: owner, count: count, seed: uint64(i + 7)})
		s.byOwner[owner] = i
	}
	srv := httptest.NewServer(s.routes())
	t.Cleanup(srv.Close)
	return srv
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Error(err)
	}
}

//craft:ignore naked-any each call decodes a different Google wire shape
func getJSON(t *testing.T, srv *httptest.Server, path, token string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func consentCode(t *testing.T, srv *httptest.Server, hint string) string {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	q := url.Values{"redirect_uri": {"http://localhost:5173/cb"}, "state": {"s1"}}
	if hint != "" {
		q.Set("login_hint", hint)
	}
	resp, err := client.Get(srv.URL + "/o/oauth2/v2/auth?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, resp)
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return back.Query().Get("code")
}

func exchange(t *testing.T, srv *httptest.Server, code string) {
	t.Helper()
	resp, err := http.PostForm(srv.URL+"/token", url.Values{"code": {code}})
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, resp)
}

// A consent page opened twice before its code is exchanged offers the same
// mailbox, so a reload never shifts the seats onto each other's mail.
func TestAReloadedConsentPageOffersTheSameMailbox(t *testing.T) {
	srv := newTestServer(t, 5)
	first, again := consentCode(t, srv, ""), consentCode(t, srv, "")
	if first != "code-a@demo.test" || again != first {
		t.Fatalf("consent offered %q then %q, want a@demo.test twice", first, again)
	}
	exchange(t, srv, first)
	if next := consentCode(t, srv, ""); next != "code-b@demo.test" {
		t.Fatalf("after a@demo.test connected, consent offered %q, want b@demo.test", next)
	}
}

func TestAMessageIsServedOnlyToItsOwnMailbox(t *testing.T) {
	srv := newTestServer(t, 5)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+apiPrefix+"/messages/"+(&server{}).messageID(1, 0)+"?format=RAW", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer at-a@demo.test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("mailbox a read a message of mailbox b: status %d, want 404", resp.StatusCode)
	}
}

func TestTheConsentFlowConnectsTheHintedMailbox(t *testing.T) {
	srv := newTestServer(t, 5)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	consent := srv.URL + "/o/oauth2/v2/auth?" + url.Values{"redirect_uri": {"http://localhost:5173/cb"}, "state": {"s1"}, "login_hint": {"b@demo.test"}}.Encode()
	resp, err := client.Get(consent)
	if err != nil {
		t.Fatal(err)
	}
	closeBody(t, resp)
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || back.Query().Get("state") != "s1" {
		t.Fatalf("redirect %q does not return the state", resp.Header.Get("Location"))
	}
	resp, err = http.PostForm(srv.URL+"/token", url.Values{"code": {back.Query().Get("code")}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp)
	var grant struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&grant); err != nil {
		t.Fatal(err)
	}
	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}
	getJSON(t, srv, apiPrefix+"/profile", grant.AccessToken, &profile)
	if profile.EmailAddress != "b@demo.test" {
		t.Fatalf("connected %q, want the hinted mailbox", profile.EmailAddress)
	}
}

func TestEveryListedMessageIsServedAndRepliesNameAMessageOfTheMailbox(t *testing.T) {
	const count = 40
	srv := newTestServer(t, count)
	var ids []string
	for token := ""; ; {
		var page struct {
			Messages      []struct{ ID string }
			NextPageToken string
		}
		getJSON(t, srv, apiPrefix+"/messages?maxResults=15&pageToken="+token, "at-a@demo.test", &page)
		for _, m := range page.Messages {
			ids = append(ids, m.ID)
		}
		if token = page.NextPageToken; token == "" {
			break
		}
	}
	if len(ids) != count {
		t.Fatalf("listed %d messages, want %d", len(ids), count)
	}
	known := map[string]string{}
	replies := map[string]string{}
	peers := map[string]string{}
	for _, id := range ids {
		var msg struct{ Raw string }
		getJSON(t, srv, apiPrefix+"/messages/"+id+"?format=RAW", "at-a@demo.test", &msg)
		raw, err := base64.URLEncoding.DecodeString(msg.Raw)
		if err != nil {
			t.Fatal(err)
		}
		head, _, _ := strings.Cut(string(raw), "\r\n\r\n")
		var self string
		for line := range strings.SplitSeq(head, "\r\n") {
			name, value, _ := strings.Cut(line, ": ")
			switch name {
			case "Message-ID":
				self = value
			case "In-Reply-To":
				replies[id] = value
			case "From", "To":
				if !strings.Contains(value, "a@demo.test") {
					peers[id] = value
				}
			}
		}
		known[self] = id
	}
	if len(replies) == 0 {
		t.Fatal("the mailbox has no replies, so no import would build a thread")
	}
	// A reply near the end of the mailbox may answer a message older than the
	// listing reaches, as a real mailbox's oldest replies do.
	for id, parent := range replies {
		var box, n int
		if _, err := fmt.Sscanf(parent, "<m%d.%d@mx.demo>", &box, &n); err != nil {
			t.Fatalf("message %s names an unreadable parent %q", id, parent)
		}
		if n >= count {
			continue
		}
		parentID, served := known[parent]
		if !served {
			t.Errorf("message %s replies to %s, which the mailbox does not serve", id, parent)
			continue
		}
		if peers[id] != peers[parentID] {
			t.Errorf("message %s with %q replies to %s with %q: one thread, two correspondents", id, peers[id], parentID, peers[parentID])
		}
	}
}

// A later generation is new mail to an importer: other ids and other
// Message-IDs, and the first generation's ids are no longer served.
func TestALaterGenerationServesNewMail(t *testing.T) {
	first, later := &server{}, &server{generation: 2}
	for _, s := range []*server{first, later} {
		s.boxes = []mailbox{{owner: "a@demo.test", count: 5, seed: 7}}
	}
	if first.messageID(0, 3) == later.messageID(0, 3) || first.mailID(0, 3) == later.mailID(0, 3) {
		t.Fatal("a later generation repeats the first generation's identities")
	}
	if _, _, ok := later.parseID(later.messageID(0, 3)); !ok {
		t.Fatal("a later generation does not serve its own ids")
	}
	if _, _, ok := later.parseID(first.messageID(0, 3)); ok {
		t.Fatal("a later generation still serves the first generation's ids")
	}
}

// The consent page sends a browser back only to a stack on this machine.
func TestTheConsentPageRefusesARedirectOffThisMachine(t *testing.T) {
	srv := newTestServer(t, 1)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, target := range []string{"https://evil.example/cb", "http://localhost.evil.example/cb", "javascript:alert(1)", "//evil.example/cb"} {
		resp, err := client.Get(srv.URL + "/o/oauth2/v2/auth?" + url.Values{"redirect_uri": {target}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		closeBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("redirect_uri %q: status %d, want 400", target, resp.StatusCode)
		}
	}
	for _, target := range []string{"http://127.0.0.1:8080/cb", "http://[::1]:8080/cb", "http://app.localhost:5173/cb"} {
		resp, err := client.Get(srv.URL + "/o/oauth2/v2/auth?" + url.Values{"redirect_uri": {target}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		closeBody(t, resp)
		if resp.StatusCode != http.StatusFound {
			t.Errorf("redirect_uri %q: status %d, want 302", target, resp.StatusCode)
		}
	}
}
