// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package calendarwire carries bounded calendar requests without exposing provider bodies.
package calendarwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Request accepts only adapter-built URLs; redirects cannot carry credentials
// to a provider-supplied destination.
//
//craft:ignore naked-any JSON serialization accepts adapter-defined wire structs.
func Request(ctx context.Context, client *http.Client, token, method, address string, body, out any) (int, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("calendar: encode request: %w", err)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", outbound.CalendarHeader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", `outlook.timezone="UTC", IdType="ImmutableId"`)
	bounded := *client
	bounded.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := bounded.Do(req)
	if err != nil {
		return 0, fmt.Errorf("calendar: request failed: %w", connector.ErrUnreachable)
	}
	content, readErr := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	closeErr := resp.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return resp.StatusCode, err
	}
	if len(content) > 4<<20 {
		return resp.StatusCode, fmt.Errorf("calendar: response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class := connector.ErrUnreachable
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			class = connector.ErrAuthRejected
		}
		return resp.StatusCode, fmt.Errorf("calendar: provider status %d: %w", resp.StatusCode, class)
	}
	if out != nil && len(content) > 0 {
		if err := json.Unmarshal(content, out); err != nil {
			return resp.StatusCode, fmt.Errorf("calendar: invalid response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
