// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The Google Workspace Directory lookup behind the group→role map. Google's ID
// tokens carry no `groups` claim, so a member's groups are read from the Admin
// SDK Directory API. A service account does that, acting for one admin under
// domain-wide delegation. Only the read-only group scope is requested.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/platform/netguard"
)

const (
	directoryGroupScope = "https://www.googleapis.com/auth/admin.directory.group.readonly"
	directoryAPIBase    = "https://admin.googleapis.com/admin/directory/v1"
	// directoryTimeout bounds one whole lookup, so a slow directory cannot hold
	// a sign-in for longer than a few seconds.
	directoryTimeout = 5 * time.Second
	// directoryAssertionTTL is one assertion's life. The token is used once and
	// never cached, so this is only the margin Google's clock check needs.
	directoryAssertionTTL = 5 * time.Minute
	directoryPageSize     = 200
	// directoryMaxPages bounds a member's groups at 2000. Past that the answer
	// is refused, because a cut-short list would grant some roles and skip the rest.
	directoryMaxPages    = 10
	maxDirectoryResponse = 1 << 20
)

// DirectoryFailure is the closed set of reasons a lookup does not answer. It is
// what an operator reads, so none of it is Google's own words, which can carry
// the key's address or a proxy's HTML.
type DirectoryFailure string

// The reasons a directory lookup fails.
const (
	// DirectoryKeyInvalid is a stored key file that cannot sign.
	DirectoryKeyInvalid DirectoryFailure = "key_invalid"
	// DirectoryDelegationRefused is Google refusing to exchange the assertion: the
	// delegation was never granted for this scope, or for this admin.
	DirectoryDelegationRefused DirectoryFailure = "delegation_refused"
	// DirectoryPermissionDenied is the Directory API refusing the read.
	DirectoryPermissionDenied DirectoryFailure = "permission_denied"
	// DirectoryNotFound is Google not knowing the member's address.
	DirectoryNotFound DirectoryFailure = "member_not_found"
	// DirectoryTimeout is a lookup that did not answer in time.
	DirectoryTimeout DirectoryFailure = "timeout"
	// DirectoryUnavailable is Google being unreachable, throttling or failing.
	DirectoryUnavailable DirectoryFailure = "unavailable"
	// DirectoryBadAnswer is an answer this reader cannot use.
	DirectoryBadAnswer DirectoryFailure = "bad_answer"
)

// DirectoryError is a failed lookup, carrying only its reason class.
type DirectoryError struct{ Reason DirectoryFailure }

func (e DirectoryError) Error() string {
	return "identity: google directory lookup failed: " + string(e.Reason)
}

// directoryCredential is the sealed bundle: the service account's key file and
// the admin the service account acts for. Directory reads under delegation need
// an admin subject, which the key file itself does not name.
type directoryCredential struct {
	ServiceAccountKey string `json:"service_account_key"`
	AdminSubject      string `json:"admin_subject"`
}

// parseDirectoryCredential reads the sealed bundle. It names the field at fault
// and never repeats a value.
func parseDirectoryCredential(sealed string) (directoryCredential, error) {
	var cred directoryCredential
	if err := json.Unmarshal([]byte(sealed), &cred); err != nil {
		return directoryCredential{}, errors.New("the stored Google directory credential is not readable")
	}
	return cred, cred.validate()
}

func (c directoryCredential) validate() error {
	if _, err := googlejwt.ParseServiceAccount(c.ServiceAccountKey); err != nil {
		return err
	}
	addr, err := mail.ParseAddress(c.AdminSubject)
	if err != nil || addr.Address != c.AdminSubject {
		return errors.New("admin_subject must be the admin's email address")
	}
	return nil
}

// covers reports whether a token's hosted domain is the admin's own domain. A
// token with no hosted domain is a personal account, which no Workspace
// directory vouches for.
func (c directoryCredential) covers(hostedDomain string) bool {
	_, domain, ok := strings.Cut(c.AdminSubject, "@")
	return ok && hostedDomain != "" && strings.EqualFold(domain, hostedDomain)
}

// GoogleGroupsReader lists the groups one address belongs to.
type GoogleGroupsReader struct {
	http     *http.Client
	now      func() time.Time
	timeout  time.Duration
	tokenURL string
	apiBase  string
}

// NewGoogleGroupsReader builds the reader against Google's own endpoints. The
// client refuses redirects: the bearer token must not follow one anywhere else.
func NewGoogleGroupsReader() *GoogleGroupsReader {
	return &GoogleGroupsReader{
		http:     netguard.NoRedirect(&http.Client{Timeout: directoryTimeout}),
		now:      time.Now,
		timeout:  directoryTimeout,
		tokenURL: googlejwt.TokenURI,
		apiBase:  directoryAPIBase,
	}
}

// Groups returns the addresses of the groups userKey is a direct member of,
// as Google lists them. Nested groups are not resolved, because the Directory
// API's userKey query answers direct memberships only.
func (r *GoogleGroupsReader) Groups(ctx context.Context, cred directoryCredential, userKey string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	token, err := r.accessToken(ctx, cred)
	if err != nil {
		return nil, err
	}
	var groups []string
	pageToken := ""
	for range directoryMaxPages {
		found, next, err := r.groupsPage(ctx, token, userKey, pageToken)
		if err != nil {
			return nil, err
		}
		groups = append(groups, found...)
		if next == "" {
			return groups, nil
		}
		pageToken = next
	}
	return nil, DirectoryError{DirectoryBadAnswer}
}

func (r *GoogleGroupsReader) accessToken(ctx context.Context, cred directoryCredential) (string, error) {
	account, err := googlejwt.ParseServiceAccount(cred.ServiceAccountKey)
	if err != nil {
		return "", DirectoryError{DirectoryKeyInvalid}
	}
	assertion, err := account.Assertion(googlejwt.Claims{
		Scope: directoryGroupScope, Subject: cred.AdminSubject, IssuedAt: r.now(), TTL: directoryAssertionTTL,
	})
	if err != nil {
		return "", DirectoryError{DirectoryKeyInvalid}
	}
	token, err := googlejwt.Exchange(ctx, r.http, r.tokenURL, assertion)
	var refused googlejwt.ErrExchangeRefused
	switch {
	case err == nil:
		return token.AccessToken, nil
	case errors.As(err, &refused) && refused.Permanent():
		return "", DirectoryError{DirectoryDelegationRefused}
	default:
		return "", transportFailure(ctx, err)
	}
}

// transportFailure classes an error with no Google answer behind it.
func transportFailure(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return DirectoryError{DirectoryTimeout}
	}
	return DirectoryError{DirectoryUnavailable}
}

func (r *GoogleGroupsReader) groupsPage(ctx context.Context, token, userKey, pageToken string) (groups []string, next string, err error) {
	q := url.Values{"userKey": {userKey}, "maxResults": {strconv.Itoa(directoryPageSize)}}
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.apiBase+"/groups?"+q.Encode(), nil)
	if err != nil {
		return nil, "", DirectoryError{DirectoryBadAnswer}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, "", transportFailure(ctx, err)
	}
	//craft:ignore swallowed-errors best-effort close of a bounded body already read; the status and decode decide the outcome
	defer func() { _ = resp.Body.Close() }()
	if reason := directoryStatusFailure(resp.StatusCode); reason != "" {
		return nil, "", DirectoryError{reason}
	}
	var body struct {
		Groups []struct {
			Email string `json:"email"`
		} `json:"groups"`
		NextPageToken string `json:"nextPageToken"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxDirectoryResponse))
	if readErr != nil {
		return nil, "", transportFailure(ctx, readErr)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, "", DirectoryError{DirectoryBadAnswer}
	}
	for _, g := range body.Groups {
		groups = append(groups, g.Email)
	}
	return groups, body.NextPageToken, nil
}

// directoryStatusFailure maps the Directory API's status onto a reason, empty
// for a 200.
func directoryStatusFailure(status int) DirectoryFailure {
	switch {
	case status == http.StatusOK:
		return ""
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return DirectoryPermissionDenied
	case status == http.StatusNotFound:
		return DirectoryNotFound
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return DirectoryUnavailable
	default:
		return DirectoryBadAnswer
	}
}

// directoryReason is the reason class of a lookup error, "unavailable" for one
// that is not a DirectoryError.
func directoryReason(err error) DirectoryFailure {
	if failed, ok := errors.AsType[DirectoryError](err); ok {
		return failed.Reason
	}
	return DirectoryUnavailable
}
