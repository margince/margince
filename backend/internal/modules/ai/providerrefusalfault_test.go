// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A provider's 429 produced no usable answer, and a job whose call met one must
// be classified by the layer above, which knows only the shared sentinels.
func TestAProviderRefusalIsAlsoAnUnusableProvider(t *testing.T) {
	throttle := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"30"}}}
	for name, cause := range map[string]error{
		"a throttle": providerRefusal(throttle, "", errors.New("rate limit exceeded, retry in 30s")),
		"a quota":    providerRefusal(&http.Response{StatusCode: http.StatusTooManyRequests}, "", errors.New("insufficient quota: add credit")),
	} {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(cause, apperrors.ErrProviderUnusable) {
				t.Errorf("%v does not reach the shared provider_unusable sentinel", cause)
			}
		})
	}
	if !errors.Is(providerRefusal(throttle, "", errors.New("retry in 30s")), ErrProviderThrottled) {
		t.Error("wrapping lost the throttle sentinel the AI layer's own callers read")
	}
}
