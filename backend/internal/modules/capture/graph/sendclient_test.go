// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sendStub records the one request the send path makes, so a test can assert on
// what Microsoft would actually receive.
type sendStub struct {
	srv    *httptest.Server
	method string
	path   string
	query  string
	auth   string
	ctype  string
	body   string
}

func newSendStub(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *sendStub {
	t.Helper()
	s := &sendStub{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.method, s.path, s.query = r.Method, r.URL.Path, r.URL.RawQuery
		s.auth, s.ctype, s.body = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(body)
		handle(w, r)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func TestSendMIMESubmitsTheMessageAsMicrosoftExpectsIt(t *testing.T) {
	stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted) // Microsoft's own answer: 202, no body
	})
	raw := []byte("From: rep@myco.com\r\nSubject: hi\r\n\r\nbody")

	if err := NewAPI(stub.srv.Client(), stub.srv.URL).SendMIME(context.Background(), "tok", raw); err != nil {
		t.Fatalf("SendMIME: %v", err)
	}
	if stub.method != http.MethodPost || stub.path != sendMIMEPath {
		t.Errorf("request = %s %s, want POST %s", stub.method, stub.path, sendMIMEPath)
	}
	if stub.auth != "Bearer tok" {
		t.Errorf("Authorization = %q", stub.auth)
	}
	// text/plain is Microsoft's convention for "the body IS the MIME";
	// application/json would make it read the base64 as a message resource and
	// refuse the whole send.
	if stub.ctype != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain — the MIME submit path", stub.ctype)
	}
	decoded, err := base64.StdEncoding.DecodeString(stub.body)
	if err != nil {
		t.Fatalf("the body is not base64: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Errorf("the transmitted message decoded to %q, want the bytes handed in", decoded)
	}
}

func TestSendMIMEMapsARefusalOntoTheSharedVocabulary(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnauthorized:        connector.ErrAuthRejected,
		http.StatusForbidden:           connector.ErrAuthRejected,
		http.StatusTooManyRequests:     connector.ErrRateLimited,
		http.StatusInternalServerError: connector.ErrUnreachable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})
			err := NewAPI(stub.srv.Client(), stub.srv.URL).SendMIME(context.Background(), "tok", []byte("x"))
			if !errors.Is(err, want) {
				t.Fatalf("status %d = %v, want %v", status, err, want)
			}
		})
	}
}

func TestFindSentByMessageIDFiltersSentItemsOnTheBracketedIdentity(t *testing.T) {
	stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"AAMk-1"}]}`))
	})

	id, found, err := NewAPI(stub.srv.Client(), stub.srv.URL).
		FindSentByMessageID(context.Background(), "tok", "outbound-1@margince.test")
	if err != nil || !found || id != "AAMk-1" {
		t.Fatalf("FindSentByMessageID = (%q, %v, %v)", id, found, err)
	}
	if !strings.Contains(stub.path, "/me/mailFolders/sentitems/messages") {
		t.Errorf("path = %q, want the Sent Items collection", stub.path)
	}
	// Filtered on the message's OWN property, and bracketed because that is the
	// form the property holds — the unbracketed form this system stores would
	// match nothing.
	for _, want := range []string{"internetMessageId+eq", "%3Coutbound-1%40margince.test%3E"} {
		if !strings.Contains(stub.query, want) {
			t.Errorf("query = %q, missing %q", stub.query, want)
		}
	}
}

func TestFindSentByMessageIDReportsAMissAsAMiss(t *testing.T) {
	stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[]}`))
	})
	api := NewAPI(stub.srv.Client(), stub.srv.URL)

	// An identity Exchange rewrote on submission matches nothing, and that is a
	// miss rather than a fault — the caller sends, it does not fail.
	if _, found, err := api.FindSentByMessageID(context.Background(), "tok", "gone@margince.test"); found || err != nil {
		t.Fatalf("a non-match = (found %v, err %v), want a clean miss", found, err)
	}
	// An empty identity is asked about at all only by a caller with nothing to
	// look up; it must cost no round trip.
	stub.path = ""
	if _, found, err := api.FindSentByMessageID(context.Background(), "tok", ""); found || err != nil {
		t.Fatalf("an empty identity = (found %v, err %v), want a clean miss", found, err)
	}
	if stub.path != "" {
		t.Error("an empty identity still went to Microsoft")
	}
}

// The identity reaches an OData filter as a caller-controlled string. A literal
// quote inside it would end the string literal and leave the rest to be read as
// filter syntax, so it is doubled — OData's own escape — before url.Values
// escapes it for the query.
func TestAnIdentityCarryingAQuoteCannotEndTheODataLiteral(t *testing.T) {
	if got := odataQuote(`a'b`); got != `a''b` {
		t.Errorf("odataQuote(%q) = %q, want the quote doubled", `a'b`, got)
	}
	if got := odataQuote(`plain`); got != `plain` {
		t.Errorf("odataQuote left an unquoted value alone as %q", got)
	}

	stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[]}`))
	})
	if _, _, err := NewAPI(stub.srv.Client(), stub.srv.URL).
		FindSentByMessageID(context.Background(), "tok", `evil'+or+true@x.test`); err != nil {
		t.Fatalf("FindSentByMessageID: %v", err)
	}
	// Doubled on the wire: a single quote surviving unescaped is the injection.
	if !strings.Contains(stub.query, "%27%27") {
		t.Errorf("query = %q, want the embedded quote doubled before escaping", stub.query)
	}
}

func TestFindSentByMessageIDMapsARefusalOntoTheSharedVocabulary(t *testing.T) {
	stub := newSendStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, _, err := NewAPI(stub.srv.Client(), stub.srv.URL).
		FindSentByMessageID(context.Background(), "tok", "x@y.test")
	if !errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("a 401 = %v, want the auth rejection", err)
	}
}
