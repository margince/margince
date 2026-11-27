// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package safedownload

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
)

func TestPolicyAllowsOnlyHTTPSOnListedHosts(t *testing.T) {
	p := Policy{Hosts: []string{"files.example.com", "*.cdn.example.com"}}
	for raw, want := range map[string]bool{
		"https://files.example.com/a":        true,
		"https://FILES.example.com/a":        true,
		"https://eu.cdn.example.com/a":       true,
		"https://cdn.example.com/a":          false,
		"https://evilcdn.example.com.evil/a": false,
		"http://files.example.com/a":         false,
		"https://other.example.com/a":        false,
		"ftp://files.example.com/a":          false,
		"://bad":                             false,
	} {
		if got := p.Allows(raw); got != want {
			t.Errorf("Allows(%q) = %v, want %v", raw, got, want)
		}
	}
}

func serve(t *testing.T, h http.HandlerFunc) (*httptest.Server, Policy) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv, Policy{Hosts: []string{"127.0.0.1"}, Control: NoDialGuardForTest}
}

func TestGetBoundsTheBodyAndReturnsOtherStatusesToTheCaller(t *testing.T) {
	srv, p := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if _, err := w.Write([]byte("12345")); err != nil {
				t.Error(err)
			}
		case "/missing":
			http.Error(w, "gone", http.StatusNotFound)
		}
	})
	hc := p.Client(srv.Client())
	if got, err := p.Get(context.Background(), hc, srv.URL+"/ok", nil, 5); err != nil || string(got.Body) != "12345" {
		t.Errorf("a body on the limit = %q, %v, want it whole", got.Body, err)
	}
	if _, err := p.Get(context.Background(), hc, srv.URL+"/ok", nil, 4); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a body over the limit: %v, want ErrTooLarge", err)
	}
	if got, err := p.Get(context.Background(), hc, srv.URL+"/missing", nil, 1); err != nil || got.Status != http.StatusNotFound {
		t.Errorf("a 404 = %d, %v, want the status and no error, even over the limit", got.Status, err)
	}
}

func TestGetRefusesAURLTheFirstRequestMayNotReach(t *testing.T) {
	p := Policy{Hosts: []string{"files.example.com"}}
	for _, target := range []string{"https://other.example.com/a", "http://files.example.com/a"} {
		if _, err := p.Get(context.Background(), p.Client(nil), target, nil, 10); !errors.Is(err, ErrRefused) {
			t.Errorf("Get(%q) = %v, want ErrRefused", target, err)
		}
	}
}

func TestARedirectOutsideThePolicyIsRefusedAndEachHopIsCounted(t *testing.T) {
	srv, p := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/out":
			http.Redirect(w, r, "https://elsewhere.example.test/x", http.StatusFound)
		case "/plain":
			http.Redirect(w, r, "http://127.0.0.1/x", http.StatusFound)
		default:
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	})
	hc := p.Client(srv.Client())
	for _, path := range []string{"/out", "/plain", "/loop"} {
		if _, err := p.Get(context.Background(), hc, srv.URL+path, nil, 10); !errors.Is(err, ErrRefused) {
			t.Errorf("GET %s: %v, want ErrRefused", path, err)
		}
	}
}

func TestATransportFailureNamesNoURL(t *testing.T) {
	srv, p := serve(t, func(http.ResponseWriter, *http.Request) {})
	target := srv.URL + "/secret-signature"
	srv.Close()
	_, err := p.Get(context.Background(), p.Client(srv.Client()), target, nil, 10)
	if !errors.Is(err, ErrTransport) || strings.Contains(err.Error(), "secret-signature") {
		t.Errorf("err = %v, want ErrTransport without the URL", err)
	}
}

func TestChecksumsAcceptTheFormsProvidersState(t *testing.T) {
	data := []byte("abc")
	for _, tc := range []struct {
		name string
		got  bool
	}{
		{"sha256 hex", SHA256Matches(data, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")},
		{"sha256 base64", SHA256Matches(data, "ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0=")},
		{"md5 hex", MD5Matches(data, "900150983cd24fb0d6963f7d28e17f72")},
	} {
		if !tc.got {
			t.Errorf("%s did not match", tc.name)
		}
	}
	if SHA256Matches(data, "nope") || MD5Matches(data, "nope") {
		t.Error("a wrong digest matched")
	}
}

func TestPolicyHoldsTheEdgeCasesOfAHostPattern(t *testing.T) {
	p := Policy{Hosts: []string{"*.zdn.vn", "files.example.com"}}
	for raw, want := range map[string]bool{
		"https://f21.zdn.vn/a":              true,
		"https://F21.ZDN.VN/a":              true,
		"https://f21.zdn.vn:443/a":          true,
		"https://f21.zdn.vn:8443/a":         true,
		"https://evilzdn.vn/a":              false,
		"https://zdn.vn/a":                  false,
		"https://zdn.vn.evil.com/a":         false,
		"https://f21.zdn.vn@evil.com/a":     false,
		"https://evil.com@f21.zdn.vn/a":     false,
		"https://user:pw@files.example.com": false,
		"https://f21.zdn.vn./a":             false,
		"https://xn--zdn-9ua.vn/a":          false,
		"https://f21.zdn.vn.xn--p1ai/a":     false,
		"https://127.0.0.1/a":               false,
		"https://[::1]/a":                   false,
		"https://169.254.169.254/a":         false,
		"https://2130706433/a":              false,
	} {
		if got := p.Allows(raw); got != want {
			t.Errorf("Allows(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestTheDefaultGuardRefusesToDialAnInternalAddressAtConnectTime(t *testing.T) {
	var reached bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer srv.Close()
	for _, host := range []string{"127.0.0.1", "localhost"} {
		p := Policy{Hosts: []string{host}}
		target := strings.Replace(srv.URL, "127.0.0.1", host, 1)
		if _, err := p.Get(context.Background(), p.Client(srv.Client()), target, nil, 10); !errors.Is(err, ErrTransport) {
			t.Errorf("a host %q that reaches the loopback: %v, want the dial refused", host, err)
		}
	}
	if reached {
		t.Error("the server behind an internal address was reached")
	}
}

func TestARedirectHopIsDialledThroughTheSameGuard(t *testing.T) {
	var dialled []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/next", http.StatusFound)
		}
	}))
	defer srv.Close()
	p := Policy{Hosts: []string{"127.0.0.1"}, Control: func(_, address string, _ syscall.RawConn) error {
		dialled = append(dialled, address)
		return errors.New("refused")
	}}
	if _, err := p.Get(context.Background(), p.Client(srv.Client()), srv.URL+"/start", nil, 10); !errors.Is(err, ErrTransport) {
		t.Errorf("err = %v, want the guarded dial to fail", err)
	}
	if len(dialled) == 0 {
		t.Error("the connection did not pass the control")
	}
}

func TestACredentialIsDroppedOnARedirect(t *testing.T) {
	var seen string
	srv, p := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/next", http.StatusFound)
			return
		}
		seen = r.Header.Get("Authorization")
	})
	if _, err := p.Get(context.Background(), p.Client(srv.Client()), srv.URL+"/start", http.Header{"Authorization": {"Bearer secret"}}, 10); err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Errorf("the redirected request carried Authorization %q", seen)
	}
}

func TestABodyIsStoredAsSentAndNeverDecompressed(t *testing.T) {
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	if _, err := zw.Write(bytes.Repeat([]byte("A"), 1000)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	srv, p := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
		}
		if _, err := w.Write(packed.Bytes()); err != nil {
			t.Error(err)
		}
	})
	got, err := p.Get(context.Background(), p.Client(srv.Client()), srv.URL, nil, int64(packed.Len()))
	if err != nil || !bytes.Equal(got.Body, packed.Bytes()) {
		t.Errorf("got %d bytes and %v, want the %d gzip bytes as sent", len(got.Body), err, packed.Len())
	}
}
