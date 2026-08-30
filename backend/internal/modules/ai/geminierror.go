// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How Gemini says no, read for what the caller can do about it. Split from
// gemini.go because the refusal rules are their own subject: which detail
// separates an exhausted account from a per-minute limit is a question about
// Google's error contract, not about how a completion is requested.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// geminiError surfaces the API's error status and message only, so a logged
// failure can never echo the request (or the key).
func geminiError(resp *http.Response) error {
	var apiErr struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			// google.rpc.RetryInfo, present on a limit the caller may come
			// back from. Gemini spends RESOURCE_EXHAUSTED and the word
			// "quota" on BOTH an exhausted spending cap and an ordinary
			// per-minute limit, so the words cannot separate them — this
			// detail can, because only the retryable one carries it.
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"` //nolint:tagliatelle // Google's wire format (camelCase)
			} `json:"details"`
		} `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr == nil && json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Status != "" {
		limit := ""
		for _, detail := range apiErr.Error.Details {
			if strings.Contains(detail.Type, "RetryInfo") && detail.RetryDelay != "" {
				limit = geminiRetryableLimit
				break
			}
		}
		return providerRefusal(resp, limit, fmt.Errorf("ai: gemini: %s: %s (http %d)", apiErr.Error.Status, apiErr.Error.Message, resp.StatusCode))
	}
	return providerRefusal(resp, "", fmt.Errorf("ai: gemini: http %d", resp.StatusCode))
}

// geminiRetryableLimit is the limit-source name a RetryInfo detail stands for.
// Spelled as a rate limit because that is what refusalKind reads it as, and
// what a delay the vendor is willing to name always means.
const geminiRetryableLimit = "vendor_rate_limit"
