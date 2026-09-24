// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How Gemini says no, read for what the caller can do about it. Split from
// gemini.go because the refusal rules are their own subject: which detail
// separates an exhausted account from a per-minute limit is a question about
// Google's error contract, not about how a completion is requested.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// geminiError surfaces the API's error status and message only, so a logged
// failure can never echo the request (or the key).
func geminiError(ctx context.Context, resp *http.Response) error {
	var apiErr struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			// google.rpc.RetryInfo, present on a limit the caller may come
			// back from. Gemini spends RESOURCE_EXHAUSTED and the word
			// "quota" on BOTH an exhausted spending cap and an ordinary
			// per-minute limit, so the words cannot separate them — this
			// detail can, because only the retryable one carries it.
			// google.rpc.BadRequest names the fields a malformed body got
			// wrong, and is the one detail that says the request is malformed.
			Details []struct {
				Type            string                 `json:"@type"`
				RetryDelay      string                 `json:"retryDelay"`      //nolint:tagliatelle // Google's wire format (camelCase)
				FieldViolations []geminiFieldViolation `json:"fieldViolations"` //nolint:tagliatelle // Google's wire format (camelCase)
			} `json:"details"`
		} `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil || json.Unmarshal(raw, &apiErr) != nil || apiErr.Error.Status == "" {
		return providerRefusal(resp, "", fmt.Errorf("ai: gemini: http %d", resp.StatusCode))
	}
	limit, malformed := "", false
	for _, detail := range apiErr.Error.Details {
		if strings.Contains(detail.Type, "RetryInfo") && detail.RetryDelay != "" {
			limit = geminiRetryableLimit
		}
		if strings.HasSuffix(detail.Type, "google.rpc.BadRequest") {
			malformed = malformed || bodyFieldViolated(detail.FieldViolations)
		}
	}
	err := providerRefusal(resp, limit, fmt.Errorf("ai: gemini: %s: %s (http %d)",
		safeProviderText(ctx, apiErr.Error.Status), safeProviderText(ctx, apiErr.Error.Message), resp.StatusCode))
	// INVALID_ARGUMENT alone also carries a prompt over the model's token
	// limit and an invalid key; only a named field violation is the body's.
	if apiErr.Error.Status == "INVALID_ARGUMENT" && malformed {
		return rejectedRequest(err)
	}
	return err
}

// geminiFieldViolation is one google.rpc.BadRequest entry; field is the path
// into the request body, absent when Google names only a description.
type geminiFieldViolation struct {
	Field string `json:"field"`
}

// bodyFieldViolated reports whether a violation names the request's own shape.
// One under generation_config does not: those settings — a thinking budget, a
// mime type — are what one model accepts and another refuses, which is a
// reason to walk to the next rung rather than to stop.
func bodyFieldViolated(violations []geminiFieldViolation) bool {
	for _, v := range violations {
		path := strings.ReplaceAll(strings.ToLower(v.Field), "_", "")
		if !strings.HasPrefix(path, "generationconfig") {
			return true
		}
	}
	return false
}

// geminiRetryableLimit is the limit-source name a RetryInfo detail stands for.
// Spelled as a rate limit because that is what refusalKind reads it as, and
// what a delay the vendor is willing to name always means.
const geminiRetryableLimit = "vendor_rate_limit"
