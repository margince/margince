// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"unicode/utf8"
)

// Raw mail includes base64 overhead, so its response cap exceeds metadata's.
// It leaves room for large messages the provider accepts.
const (
	maxJSONResponseBytes = 8 << 20  // 8 MiB for metadata.
	maxRawMessageBytes   = 96 << 20 // 96 MiB for raw mail.
)

// getOnce reads an authorized response within the endpoint's byte limit.
// Only the HTTP status and classified failure escape the provider boundary.
//
//craft:ignore naked-any out is an endpoint's JSON decode target; its type varies.
func (a *httpAPI) getOnce(ctx context.Context, accessToken, path string, q url.Values, out any, maxBytes int64) (int, error) {
	u := a.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("gmail: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("gmail: %s: %w", path, ErrUnreachable)
	}
	//craft:ignore swallowed-errors best-effort close of the response body — the decoded result/status is what matters
	defer func() { _ = resp.Body.Close() }()
	limit := maxBytes
	knownLength := resp.StatusCode == http.StatusOK && resp.ContentLength >= 0
	if knownLength {
		if resp.ContentLength > maxBytes {
			return resp.StatusCode, fmt.Errorf("gmail: response too large: %w", ErrUnreachable)
		}
		limit = resp.ContentLength
		reserveResponse(ctx, limit)
	}
	body, err := readResponseBody(resp.Body, limit, knownLength)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("gmail: reading %s: %w", path, ErrUnreachable)
	}
	if err := classifyStatus(resp, path, body); err != nil {
		return resp.StatusCode, err
	}
	if int64(len(body)) > limit {
		return resp.StatusCode, fmt.Errorf("gmail: response too large: %w", ErrUnreachable)
	}
	if knownLength && int64(len(body)) != limit {
		return resp.StatusCode, fmt.Errorf("gmail: incomplete response: %w", ErrUnreachable)
	}
	if !utf8.Valid(body) {
		return resp.StatusCode, fmt.Errorf("gmail: invalid response encoding: %w", ErrUnreachable)
	}
	if err := json.Unmarshal(body, out); err != nil {
		if errors.Is(err, errResponseArrayLimit) {
			return resp.StatusCode, errors.Join(ErrUnreachable, errResponseArrayLimit)
		}
		return resp.StatusCode, fmt.Errorf("gmail: decoding %s: %w", path, ErrUnreachable)
	}
	return resp.StatusCode, nil
}
