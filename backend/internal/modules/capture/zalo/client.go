// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/safedownload"
)

const (
	defaultOAuthBase   = "https://oauth.zaloapp.com"
	defaultOpenAPIBase = "https://openapi.zalo.me"
	httpTimeout        = 30 * time.Second
	maxResponseBytes   = 1 << 20
)

// Client calls Zalo's OAuth and OA Open API hosts. Every call takes its
// credentials as arguments, so one client serves every connected OA.
type Client struct {
	http    *http.Client
	oauth   string
	openapi string
	// media fetches attachment files. It is a copy of the HTTP client with a
	// redirect policy, so the API calls above keep the caller's own.
	media *http.Client
	cdn   safedownload.Policy
	hosts []string
}

// Option adjusts a Client.
type Option func(*Client)

// WithMediaHosts replaces the hosts an attachment may be downloaded from; tests
// use it to admit a local server.
func WithMediaHosts(patterns ...string) Option {
	return func(c *Client) { c.hosts = patterns }
}

// WithoutDialGuardForTest lets the client dial a server on the loopback.
func WithoutDialGuardForTest() Option {
	return func(c *Client) { c.cdn.Control = safedownload.NoDialGuardForTest }
}

// NewClient builds a Client. A nil httpClient gets one with a bounded timeout;
// an empty base takes Zalo's own host.
func NewClient(httpClient *http.Client, oauthBase, openAPIBase string, opts ...Option) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}
	if oauthBase == "" {
		oauthBase = defaultOAuthBase
	}
	if openAPIBase == "" {
		openAPIBase = defaultOpenAPIBase
	}
	c := &Client{
		http: httpClient, oauth: strings.TrimRight(oauthBase, "/"), openapi: strings.TrimRight(openAPIBase, "/"),
		hosts: DefaultMediaHosts,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.cdn.Hosts = c.hosts
	c.media = c.cdn.Client(httpClient)
	return c
}

// App is the Zalo application's identity, shared by every OA that authorizes it.
type App struct {
	ID     string
	Secret string
}

// TokenPair is what a refresh returns. A refresh token is single use.
// Zalo invalidates the one presented and issues RefreshToken in its place.
// The caller must store both fields together and discard the old token.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn is the access token's lifetime from the moment of the answer.
	ExpiresIn time.Duration
}

// Refresh exchanges a refresh token for a new pair. It never returns a pair
// without a new refresh token, so a caller cannot keep the old one by accident.
func (c *Client) Refresh(ctx context.Context, app App, refreshToken string) (TokenPair, error) {
	form := url.Values{"refresh_token": {refreshToken}, "app_id": {app.ID}, "grant_type": {"refresh_token"}}
	req, err := c.request(ctx, http.MethodPost, c.oauth+"/v4/oa/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenPair{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("secret_key", app.Secret)
	var out struct {
		AccessToken  string   `json:"access_token"`
		RefreshToken string   `json:"refresh_token"`
		ExpiresIn    idString `json:"expires_in"`
		Error        idString `json:"error"`
		ErrorName    string   `json:"error_name"`
		ErrorDesc    string   `json:"error_description"`
	}
	status, err := c.do(req, &out)
	if err != nil {
		return TokenPair{}, err
	}
	if out.AccessToken == "" && (out.Error != "" || out.ErrorName != "") {
		return TokenPair{}, refreshFailure(status, string(out.Error), firstNonEmpty(out.ErrorName, out.ErrorDesc))
	}
	seconds, convErr := strconv.ParseFloat(string(out.ExpiresIn), 64)
	if status != http.StatusOK || out.AccessToken == "" || out.RefreshToken == "" || convErr != nil || seconds <= 0 {
		return TokenPair{}, fmt.Errorf("zalo: refresh answered without a complete token pair: %w", ErrUnreachable)
	}
	return TokenPair{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		ExpiresIn:    time.Duration(seconds * float64(time.Second)),
	}, nil
}

// refreshFailure classifies an OAuth error answer. Every refusal of the grant
// needs the OA admin to authorize again: an invalid or expired refresh token,
// or a wrong app secret. Only a throttle is retried.
func refreshFailure(status int, code, name string) error {
	kind := ErrReauthRequired
	if status == http.StatusTooManyRequests {
		kind = ErrRateLimited
	}
	n, _ := strconv.Atoi(code)
	return &APIError{Code: n, Message: name, kind: kind}
}

// SendText sends a customer-service text message and returns Zalo's message id.
func (c *Client) SendText(ctx context.Context, accessToken, userID, text string) (string, error) {
	body := map[string]map[string]string{
		"recipient": {"user_id": userID},
		"message":   {"text": text},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("zalo: encoding the send request: %w", ErrRejected)
	}
	req, err := c.request(ctx, http.MethodPost, c.openapi+"/v3.0/oa/message/cs", strings.NewReader(string(encoded)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", accessToken)
	var out struct {
		Data struct {
			MessageID idString `json:"message_id"`
		} `json:"data"`
	}
	if err := c.openAPI(req, &out); err != nil {
		return "", err
	}
	if out.Data.MessageID == "" {
		return "", fmt.Errorf("zalo: send answered without a message id: %w", ErrUnreachable)
	}
	return string(out.Data.MessageID), nil
}

// User is the part of Zalo's user detail this package reads.
type User struct {
	ID          string
	UserIDByApp string
	DisplayName string
	AvatarURL   string
}

// UserDetail reads a customer's profile.
func (c *Client) UserDetail(ctx context.Context, accessToken, userID string) (User, error) {
	query, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return User{}, fmt.Errorf("zalo: encoding the user request: %w", ErrRejected)
	}
	req, err := c.request(ctx, http.MethodGet, c.openapi+"/v3.0/oa/user/detail?data="+url.QueryEscape(string(query)), nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("access_token", accessToken)
	var out struct {
		Data struct {
			UserID      idString `json:"user_id"`
			UserIDByApp idString `json:"user_id_by_app"`
			DisplayName string   `json:"display_name"`
			Avatar      string   `json:"avatar"`
		} `json:"data"`
	}
	if err := c.openAPI(req, &out); err != nil {
		return User{}, err
	}
	d := out.Data
	return User{ID: string(d.UserID), UserIDByApp: string(d.UserIDByApp), DisplayName: d.DisplayName, AvatarURL: d.Avatar}, nil
}

// openAPI performs an OA Open API call and maps Zalo's {error, message}
// envelope. Zalo answers refusals with HTTP 200 and a non-zero error code.
//
//craft:ignore naked-any out is the caller's per-endpoint JSON decode target
func (c *Client) openAPI(req *http.Request, out any) error {
	var env struct {
		Error   idString `json:"error"`
		Message string   `json:"message"`
	}
	raw, status, err := c.roundTrip(req)
	if err != nil {
		return err
	}
	if status == http.StatusTooManyRequests {
		return &APIError{Code: 429, Message: "http 429", kind: ErrRateLimited}
	}
	if status >= http.StatusInternalServerError {
		return fmt.Errorf("zalo: %s answered %d: %w", req.URL.Path, status, ErrUnreachable)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("zalo: decoding the %s answer: %w", req.URL.Path, ErrUnreachable)
	}
	code, convErr := strconv.Atoi(string(env.Error))
	if convErr != nil {
		return fmt.Errorf("zalo: the %s answer carries no error code: %w", req.URL.Path, ErrUnreachable)
	}
	if code != 0 {
		return &APIError{Code: code, Message: env.Message, kind: kindOf(code)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("zalo: decoding the %s result: %w", req.URL.Path, ErrUnreachable)
	}
	return nil
}

// do performs a request and decodes a JSON body into out regardless of status.
//
//craft:ignore naked-any out is the caller's per-endpoint JSON decode target
func (c *Client) do(req *http.Request, out any) (int, error) {
	raw, status, err := c.roundTrip(req)
	if err != nil {
		return 0, err
	}
	if status >= http.StatusInternalServerError {
		return status, fmt.Errorf("zalo: %s answered %d: %w", req.URL.Path, status, ErrUnreachable)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return status, fmt.Errorf("zalo: decoding the %s answer: %w", req.URL.Path, ErrUnreachable)
	}
	return status, nil
}

// roundTrip returns the bounded body and status. A transport error is reported
// without its cause, whose text carries the request URL.
func (c *Client) roundTrip(req *http.Request) ([]byte, int, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("zalo: %s: %w", req.URL.Path, ErrUnreachable)
	}
	//craft:ignore swallowed-errors best-effort close of the response body — the decoded answer is what matters
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("zalo: reading the %s answer: %w", req.URL.Path, ErrUnreachable)
	}
	return raw, resp.StatusCode, nil
}

// request builds one Zalo request. A failed build does not wrap its cause: a
// *url.Error message carries the whole URL.
func (c *Client) request(ctx context.Context, method, target string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("zalo: the request could not be built: %w", ErrRejected)
	}
	return req, nil
}
