// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Doer keeps provider HTTP at an injectable network boundary.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// HTTP applies provider authentication and response checks at one transport boundary.
type HTTP struct {
	Client Doer
	Base   string
	Token  string
}

// ErrNotFound marks an absent provider resource without asserting source deletion.
var ErrNotFound = fmt.Errorf("provider resource is missing: %w", ErrUnavailable)

const responseLimit = 4 << 20

// Call translates provider responses into the Messenger error contract.
func (h HTTP) Call(ctx context.Context, method, path string, body json.RawMessage) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.Base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalid
	}
	request.Header.Set("Authorization", "Bearer "+h.Token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", outbound.ChatHeader)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := h.Client.Do(request)
	if err != nil {
		return nil, requestFailure(method)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(raw) > responseLimit {
		return nil, requestFailure(method)
	}
	return providerResponse(method, response, raw)
}

func providerResponse(method string, response *http.Response, raw json.RawMessage) (json.RawMessage, error) {
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case response.StatusCode == http.StatusTooManyRequests:
		wait := retryafter.Of(response)
		if wait <= 0 || wait > 24*time.Hour {
			wait = time.Minute
		}
		return nil, &connector.RateLimitedError{RetryAfter: wait}
	case response.StatusCode >= 500:
		return nil, requestFailure(method)
	case response.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return nil, fmt.Errorf("provider refused the request: %w", ErrInvalid)
	}
	return raw, nil
}

func requestFailure(method string) error {
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
		return ErrDeliveryUnknown
	}
	return ErrUnavailable
}
