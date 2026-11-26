// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package safedownload fetches a file from a URL that a message payload named.
// A payload is a stranger's input, so the fetch is held to three limits. The
// URL is https on an allowed host, every redirect hop is checked the same way,
// and the body has a size bound. Every chat network's media download goes
// through here, so no network can forget one of the three.
package safedownload

import (
	"context"
	"crypto/md5" //nolint:gosec // G501: Zalo states its file checksum as MD5; this is an integrity check on a download, not a security boundary
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	// ErrRefused marks a URL or redirect outside the policy. Retrying never helps.
	ErrRefused = errors.New("safedownload: the url is not allowed")
	// ErrTooLarge marks a body over the caller's bound.
	ErrTooLarge = errors.New("safedownload: the body exceeds the size limit")
	// ErrTransport marks a failure with no answer (connection, timeout, a read
	// that broke). It carries no cause, because a *url.Error message holds the
	// request URL, which may carry a signature.
	ErrTransport = errors.New("safedownload: no usable answer")
)

// maxRedirects bounds the hops of one fetch.
const maxRedirects = 3

// Policy says which hosts a fetch may reach.
type Policy struct {
	// Hosts may be the target and any redirect hop. "*.example.com" matches
	// any subdomain of example.com, and any other entry matches one host.
	Hosts []string
	// RedirectHosts are further exact hosts a redirect may reach, such as the
	// API host the first request went to.
	RedirectHosts []string
}

// Allows reports whether raw is an https URL on one of the policy's hosts.
func (p Policy) Allows(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, pattern := range p.Hosts {
		if suffix, wild := strings.CutPrefix(pattern, "*"); wild {
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		} else if host == pattern {
			return true
		}
	}
	return false
}

func (p Policy) allowsRedirect(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	for _, h := range p.RedirectHosts {
		if strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return p.Allows(u.String())
}

// errRedirectRefused is what the redirect policy stops a request with.
var errRedirectRefused = errors.New("safedownload: redirect refused")

// Client copies hc with a redirect policy that checks every hop like the first
// request. A redirect from an allowed host then cannot carry a credential to
// any other. The caller's own client keeps its policy. A nil hc gets a bare client.
func (p Policy) Client(hc *http.Client) *http.Client {
	own := http.Client{}
	if hc != nil {
		own = *hc
	}
	own.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects || !p.allowsRedirect(req.URL) {
			return errRedirectRefused
		}
		return nil
	}
	return &own
}

// Response is one answer, whatever its status.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do sends req and reads at most limit bytes of the answer. A status outside
// 2xx is returned with its body cut to limit+1 bytes, for the caller to
// classify. Only a 2xx body over limit fails with ErrTooLarge. hc must come
// from Policy.Client for a request that may be redirected.
func Do(hc *http.Client, req *http.Request, limit int64) (Response, error) {
	resp, err := hc.Do(req) //nolint:gosec // G704: the target is a Graph API URL built here, or a payload URL Policy.Get has checked
	if errors.Is(err, errRedirectRefused) {
		return Response{}, ErrRefused
	}
	if err != nil {
		return Response{}, ErrTransport
	}
	//craft:ignore swallowed-errors best-effort close of the response body — the status and bytes already read are what matter
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Response{}, ErrTransport
	}
	out := Response{Status: resp.StatusCode, Header: resp.Header, Body: data}
	if out.Status >= 200 && out.Status < 300 && int64(len(data)) > limit {
		return out, ErrTooLarge
	}
	return out, nil
}

// Get fetches target with GET when the policy allows it. A status outside 2xx
// is returned, not an error.
func (p Policy) Get(ctx context.Context, hc *http.Client, target string, header http.Header, limit int64) (Response, error) {
	if !p.Allows(target) {
		return Response{}, ErrRefused
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Response{}, ErrRefused
	}
	for k, v := range header {
		req.Header[k] = v
	}
	return Do(hc, req, limit)
}

// SHA256Matches reports whether data hashes to want, given as hex or base64.
func SHA256Matches(data []byte, want string) bool {
	sum := sha256.Sum256(data)
	return strings.EqualFold(want, hex.EncodeToString(sum[:])) ||
		want == base64.StdEncoding.EncodeToString(sum[:])
}

// MD5Matches reports whether data hashes to want, given as hex.
func MD5Matches(data []byte, want string) bool {
	sum := md5.Sum(data) //nolint:gosec // G401: see the import
	return strings.EqualFold(want, hex.EncodeToString(sum[:]))
}
