// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadFetchesAnAttachmentAndChecksItsDigest(t *testing.T) {
	payload := []byte("%PDF- the offer")
	sum := md5.Sum(payload)
	good := hex.EncodeToString(sum[:])
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("access_token") != "" || r.Header.Get("Authorization") != "" {
				t.Error("a credential reached the file host")
			}
			if _, err := w.Write(payload); err != nil {
				t.Error(err)
			}
		case "/busy":
			http.Error(w, "later", http.StatusServiceUnavailable)
		case "/throttled":
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case "/out":
			http.Redirect(w, r, "https://elsewhere.example.test/x", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.Client(), "", "", WithMediaHosts("127.0.0.1"))

	for _, tc := range []struct {
		name string
		att  Attachment
		max  int64
		want error
	}{
		{"kept with its md5", Attachment{Type: "file", URL: srv.URL + "/ok", Checksum: good}, 100, nil},
		{"kept with no checksum", Attachment{Type: "file", URL: srv.URL + "/ok"}, 100, nil},
		{"kept with a checksum in a form it cannot test", Attachment{Type: "file", URL: srv.URL + "/ok", Checksum: "not-a-digest"}, 100, nil},
		{"digest differs", Attachment{Type: "file", URL: srv.URL + "/ok", Checksum: hex.EncodeToString(make([]byte, 16))}, 100, ErrMediaChecksum},
		{"stated size over the limit", Attachment{Type: "file", URL: srv.URL + "/ok", Size: 101}, 100, ErrMediaTooLarge},
		{"body over the limit", Attachment{Type: "file", URL: srv.URL + "/ok"}, 3, ErrMediaTooLarge},
		{"missing", Attachment{Type: "file", URL: srv.URL + "/none"}, 100, ErrRejected},
		{"server error", Attachment{Type: "file", URL: srv.URL + "/busy"}, 100, ErrUnreachable},
		{"throttled", Attachment{Type: "file", URL: srv.URL + "/throttled"}, 100, ErrRateLimited},
		{"redirect off the allowed hosts", Attachment{Type: "file", URL: srv.URL + "/out"}, 100, ErrRejected},
		{"host not allowed", Attachment{Type: "file", URL: "https://elsewhere.example.test/ok"}, 100, ErrRejected},
		{"not https", Attachment{Type: "file", URL: "http://127.0.0.1/ok"}, 100, ErrRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.Download(context.Background(), tc.att, tc.max)
			if !errors.Is(err, tc.want) || (tc.want == nil && string(got) != string(payload)) {
				t.Errorf("Download = %q, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestOnlyFileAttachmentsAreDownloadable(t *testing.T) {
	for typ, want := range map[string]bool{"image": true, "file": true, "gif": true, "audio": true, "video": true, "link": false, "location": false, "sticker": false} {
		if got := (Attachment{Type: typ, URL: "https://x.zdn.vn/a"}).Downloadable(); got != want {
			t.Errorf("%s downloadable = %v, want %v", typ, got, want)
		}
	}
	if (Attachment{Type: "image"}).Downloadable() {
		t.Error("an attachment with no URL is downloadable")
	}
}

func TestTheDefaultHostsAreZalosOwn(t *testing.T) {
	c := NewClient(nil, "", "")
	for raw, want := range map[string]bool{
		"https://f21-zpc.zdn.vn/a":      true,
		"https://photo-zpl.zadn.vn/a":   true,
		"https://example.test/a":        false,
		"https://zdn.vn.example.test/a": false,
		"http://f21-zpc.zdn.vn/a":       false,
	} {
		if got := c.cdn.Allows(raw); got != want {
			t.Errorf("Allows(%q) = %v, want %v", raw, got, want)
		}
	}
}
