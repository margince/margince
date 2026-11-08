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
	}{
		{"JSON with gzip", "application/json; charset=utf-8", "gzip", true, false},
		{"problem JSON with gzip", "application/problem+json", "gzip", true, false},
		{"JSON without negotiation", "application/json", "", false, false},
		{"refused gzip", "application/json", "gzip;q=0", false, false},
		{"weighted gzip", "application/json", "gzip;q=0.5", true, false},
		{"explicit refusal overrides wildcard", "application/json", "*;q=1, gzip;q=0", false, false},
		{"wildcard accepts gzip", "application/json", "*;q=0.8", true, false},
		{"identity preferred", "application/json", "gzip;q=0.5, identity;q=1", false, false},
		{"uppercase coding", "application/json", "GZIP", true, false},
		{"different coding token", "application/json", "notgzip", false, false},
		{"malformed weight", "application/json", "gzip;q=invalid", false, false},
		{"nonfinite weight", "application/json", "gzip;q=NaN", false, false},

		{"event stream", "text/event-stream", "gzip", false, true},
		{"attachment", "application/octet-stream", "gzip", false, false},
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
			if !strings.Contains(vary, "Accept-Encoding") || !strings.Contains(vary, "Accept-Language") {
				t.Fatalf("Vary lost a negotiated header: %q", vary)
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
