// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type responseCodecServer struct {
	crmcontracts.Unimplemented
	serve http.HandlerFunc
}

func (s responseCodecServer) GetCurrentPrincipal(w http.ResponseWriter, r *http.Request, _ crmcontracts.GetCurrentPrincipalParams) {
	s.serve(w, r)
}

func TestContractRouterNegotiatesJSONCompressionAndLeavesStreamsFlushable(t *testing.T) {
	payload := []byte(`{"reading":"` + strings.Repeat("opening reading ", 512) + `"}`)
	for _, tt := range []struct {
		name, mime, accept string
		compressed, flush  bool
		// sameForEveryClient: the client offered gzip and the type is never
		// compressed, so the answer does not vary on Accept-Encoding.
		sameForEveryClient bool
	}{
		{"JSON with gzip", "application/json; charset=utf-8", "gzip", true, false, false},
		{"problem JSON with gzip", "application/problem+json", "gzip", true, false, false},
		{"JSON without negotiation", "application/json", "", false, false, false},
		{"refused gzip", "application/json", "gzip;q=0", false, false, false},
		{"weighted gzip", "application/json", "gzip;q=0.5", true, false, false},
		{"explicit refusal overrides wildcard", "application/json", "*;q=1, gzip;q=0", false, false, false},
		{"wildcard accepts gzip", "application/json", "*;q=0.8", true, false, false},
		{"identity preferred", "application/json", "gzip;q=0.5, identity;q=1", false, false, false},
		{"uppercase coding", "application/json", "GZIP", true, false, false},
		{"different coding token", "application/json", "notgzip", false, false, false},
		{"malformed weight", "application/json", "gzip;q=invalid", false, false, false},
		{"nonfinite weight", "application/json", "gzip;q=NaN", false, false, false},

		{"event stream", "text/event-stream", "gzip", false, true, true},
		{"attachment", "application/octet-stream", "gzip", false, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := responseCodecServer{serve: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.mime)
				w.Header().Add("Vary", "Accept-Language")
				if _, err := w.Write(payload); err != nil {
					t.Fatal(err)
				}
				if tt.flush {
					flusher, ok := w.(http.Flusher)
					if !ok {
						t.Fatal("stream lost its flushing support")
					}
					flusher.Flush()
				}
			}}
			req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
			req.Header.Set("Accept-Encoding", tt.accept)
			rec := httptest.NewRecorder()
			contractRouter(server).ServeHTTP(rec, req)
			got := rec.Body.Bytes()
			if tt.compressed {
				if rec.Header().Get("Content-Encoding") != "gzip" {
					t.Fatal("JSON was not compressed")
				}
				got = gunzipResponse(t, got)
			} else if rec.Header().Get("Content-Encoding") != "" {
				t.Fatal("unnegotiated or non-JSON response was compressed")
			}
			vary := strings.Join(rec.Header().Values("Vary"), ",")
			want := 1
			if tt.sameForEveryClient {
				want = 0
			}
			if strings.Count(vary, "Accept-Encoding") != want || !strings.Contains(vary, "Accept-Language") {
				t.Fatalf("Vary = %q, want Accept-Language and Accept-Encoding %d time(s)", vary, want)
			}
			if !bytes.Equal(got, payload) {
				t.Fatal("response body changed")
			}
			if tt.flush && !rec.Flushed {
				t.Fatal("event stream was buffered")
			}
		})
	}
}

func gunzipResponse(t *testing.T, encoded []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestTheDefaultCacheControlYieldsToAStatedOne(t *testing.T) {
	for _, tc := range []struct{ name, outer, handler, want string }{
		{"nothing stated", "", "", apiDefaultCache},
		{"the handler states its own", "", "private, no-cache", "private, no-cache"},
		{"an outer layer stated one", "no-store", "", "no-store"},
	} {
		rec := httptest.NewRecorder()
		if tc.outer != "" {
			rec.Header().Set("Cache-Control", tc.outer)
		}
		privateUnlessStated(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if tc.handler != "" {
				w.Header().Set("Cache-Control", tc.handler)
			}
		})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.name, got, tc.want)
		}
	}
}
