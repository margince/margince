// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const testToken = "EAAtesttoken"

var payload = []byte("PDF-BYTES")

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func b64Sum(b []byte) string {
	s := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(s[:])
}

// mediaServer serves Graph metadata at /v23.0/MEDIA and the bytes at /bin.
func mediaServer(t *testing.T, size int64, sum string, binStatus int, bin []byte) *Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v23.0/MEDIA":
			if _, err := fmt.Fprintf(w, `{"url":%q,"mime_type":"application/pdf","sha256":%q,"file_size":%d}`, srv.URL+"/bin", sum, size); err != nil {
				t.Errorf("writing the answer: %v", err)
			}
		case "/bin":
			w.WriteHeader(binStatus)
			if _, err := w.Write(bin); err != nil {
				t.Errorf("writing the answer: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.Client(), srv.URL, "v23.0", WithMediaHosts("127.0.0.1"))
}

func TestDownloadMedia(t *testing.T) {
	cases := []struct {
		name      string
		size      int64
		metaSum   string
		webhook   string
		binStatus int
		max       int64
		wantErr   error
	}{
		{"hex checksum", int64(len(payload)), hexSum(payload), "", 200, 100, nil},
		{"webhook base64 checksum", int64(len(payload)), "", b64Sum(payload), 200, 100, nil},
		{"no checksum given", int64(len(payload)), "", "", 200, 100, nil},
		{"metadata says too large", 500, hexSum(payload), "", 200, 100, ErrMediaTooLarge},
		{"body larger than metadata said", 1, "", "", 200, 4, ErrMediaTooLarge},
		{"metadata checksum mismatch", int64(len(payload)), hexSum([]byte("other")), "", 200, 100, ErrMediaChecksum},
		{"webhook checksum mismatch", int64(len(payload)), "", b64Sum([]byte("other")), 200, 100, ErrMediaChecksum},
		{"bytes url 401", int64(len(payload)), "", "", 401, 100, ErrAuth},
		{"bytes url 500", int64(len(payload)), "", "", 500, 100, ErrUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mediaServer(t, tc.size, tc.metaSum, tc.binStatus, payload)
			got, err := c.DownloadMedia(context.Background(), testToken, "MEDIA", tc.max, tc.webhook)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (string(got.Data) != string(payload) || got.MimeType != "application/pdf") {
				t.Fatalf("download = %q %q", got.Data, got.MimeType)
			}
		})
	}
}

func TestDownloadMediaMetadataAuthFailure(t *testing.T) {
	c := mediaServer(t, 1, "", 200, payload)
	if _, err := c.DownloadMedia(context.Background(), "wrong", "MEDIA", 100, ""); !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
}

func TestDownloadMediaRefusesANonHTTPURL(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "http://lookaside.fbsbx.com/x", "https://evil.example.com/x", "https://fbsbx.com.evil.example/x", "https://fbsbx.com/x"} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := fmt.Fprintf(w, `{"url":%q,"file_size":1}`, bad); err != nil {
				t.Errorf("writing the answer: %v", err)
			}
		}))
		c := NewClient(srv.Client(), srv.URL, "v23.0")
		if _, err := c.DownloadMedia(context.Background(), testToken, "M", 100, ""); !errors.Is(err, ErrRejected) {
			t.Errorf("%s: err = %v, want ErrRejected", bad, err)
		}
		srv.Close()
	}
}

func TestDefaultMediaHostsAndVersion(t *testing.T) {
	c := NewClient(nil, "", "")
	for url, want := range map[string]bool{
		"https://lookaside.fbsbx.com/a": true, "https://scontent.xx.fbsbx.com/a": true,
		"https://mmg.whatsapp.net/a": true, "https://whatsapp.net/a": false, "http://mmg.whatsapp.net/a": false,
	} {
		if got := c.allowedMediaURL(url); got != want {
			t.Errorf("allowedMediaURL(%s) = %v, want %v", url, got, want)
		}
	}
	if c.version != "v27.0" {
		t.Errorf("default version = %s", c.version)
	}
}

func TestDownloadURLDirectFromEcho(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if _, err := w.Write(payload); err != nil {
			t.Errorf("writing the answer: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client(), srv.URL, "", WithMediaHosts("127.0.0.1"))
	got, err := c.DownloadURL(context.Background(), testToken, srv.URL+"/m", 100, b64Sum(payload))
	if err != nil || string(got.Data) != string(payload) {
		t.Fatalf("DownloadURL = %q, %v", got.Data, err)
	}
	if _, err := c.DownloadURL(context.Background(), testToken, srv.URL+"/m", 100, b64Sum([]byte("x"))); !errors.Is(err, ErrMediaChecksum) {
		t.Fatalf("err = %v, want ErrMediaChecksum", err)
	}
}

type sentRequest struct {
	path, auth string
	body       map[string]any
}

//craft:ignore naked-any test decodes an arbitrary request body to assert on it
func sendServer(t *testing.T, status int, answer string, header http.Header) (*Client, *sentRequest) {
	t.Helper()
	got := &sentRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.path, got.auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Errorf("request body: %v", err)
		}
		maps.Copy(w.Header(), header)
		w.WriteHeader(status)
		if _, err := fmt.Fprint(w, answer); err != nil {
			t.Errorf("writing the answer: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.Client(), srv.URL, "v23.0"), got
}

func TestSendText(t *testing.T) {
	c, got := sendServer(t, 200, `{"messaging_product":"whatsapp","contacts":[{"input":"491701234567","wa_id":"491701234567"}],"messages":[{"id":"wamid.SENT1"}]}`, nil)
	id, err := c.SendText(context.Background(), testToken, "PNID1", "491701234567", "Hi there")
	if err != nil || id != "wamid.SENT1" {
		t.Fatalf("SendText = %q, %v", id, err)
	}
	text, ok := got.body["text"].(map[string]any)
	if got.path != "/v23.0/PNID1/messages" || got.auth != "Bearer "+testToken || !ok || text["body"] != "Hi there" ||
		got.body["messaging_product"] != "whatsapp" || got.body["recipient_type"] != "individual" ||
		got.body["to"] != "491701234567" || got.body["type"] != "text" {
		t.Fatalf("request = %+v", got)
	}
}

func TestSendTextMapsGraphErrors(t *testing.T) {
	envelope := func(code int) string {
		return fmt.Sprintf(`{"error":{"message":"m","type":"OAuthException","code":%d,"error_subcode":7,"fbtrace_id":"x"}}`, code)
	}
	cases := []struct {
		name      string
		status    int
		answer    string
		wantErr   error
		retryable bool
	}{
		{"token expired", 401, envelope(190), ErrAuth, false},
		{"auth code on 400", 400, envelope(190), ErrAuth, false},
		{"outside window", 400, envelope(131047), ErrOutsideWindow, false},
		{"recipient not on whatsapp", 400, envelope(131026), ErrRecipientInvalid, false},
		{"recipient not allowed", 400, envelope(131030), ErrRecipientInvalid, false},
		{"other rejection", 400, envelope(131008), ErrRejected, false},
		{"unparseable 400", 400, `oops`, ErrRejected, false},
		{"server fault", 503, `oops`, ErrUnreachable, true},
		{"empty success", 200, `{"messages":[]}`, ErrUnreachable, true},
		{"unreadable success", 200, `oops`, ErrUnreachable, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := sendServer(t, tc.status, tc.answer, nil)
			_, err := c.SendText(context.Background(), testToken, "P", "1", "x")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if IsRetryable(err) != tc.retryable {
				t.Errorf("IsRetryable = %v, want %v", IsRetryable(err), tc.retryable)
			}
		})
	}
}

func TestSendTextRateLimitCarriesRetryAfter(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		answer string
		header http.Header
		want   time.Duration
	}{
		"429 with header":    {429, `{}`, http.Header{"Retry-After": {"12"}}, 12 * time.Second},
		"graph code 130429":  {400, `{"error":{"code":130429}}`, nil, 0},
		"huge header capped": {429, `{}`, http.Header{"Retry-After": {"99999"}}, maxRetryAfter},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := sendServer(t, tc.status, tc.answer, tc.header)
			_, err := c.SendText(context.Background(), testToken, "P", "1", "x")
			var rl *connector.RateLimitedError
			if !errors.Is(err, connector.ErrRateLimited) || !errors.As(err, &rl) || rl.RetryAfter != tc.want {
				t.Fatalf("err = %v (retry after %v), want a rate limit waiting %v", err, rl, tc.want)
			}
			if !IsRetryable(err) {
				t.Error("a throttle must be retryable")
			}
		})
	}
}

func TestErrorsNeverCarryTokenOrText(t *testing.T) {
	c, _ := sendServer(t, 400, `{"error":{"code":131047,"message":"secret customer text"}}`, nil)
	_, err := c.SendText(context.Background(), testToken, "P", "1", "private body")
	for _, leak := range []string{testToken, "private body", "secret customer text"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("error %q leaks %q", err, leak)
		}
	}
}

func TestTransportFailureIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c := NewClient(srv.Client(), srv.URL, "v23.0")
	srv.Close()
	_, err := c.SendText(context.Background(), testToken, "P", "1", "x")
	if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadURLRefusesARedirectToAHostOutsideTheAllowlist(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/steal", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client(), srv.URL, "", WithMediaHosts("127.0.0.1"))
	if _, err := c.DownloadURL(context.Background(), testToken, srv.URL+"/m", 100); !errors.Is(err, ErrRejected) {
		t.Fatalf("a redirect off the allowlist gave %v, want ErrRejected", err)
	}
}

func TestDownloadURLStopsAfterTooManyRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client(), srv.URL, "", WithMediaHosts("127.0.0.1"))
	if _, err := c.DownloadURL(context.Background(), testToken, srv.URL+"/m", 100); !errors.Is(err, ErrRejected) {
		t.Fatalf("an endless redirect gave %v, want ErrRejected", err)
	}
}
