// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil is success", nil, ""},
		{"budget deferred", fmt.Errorf("wrap: %w", ErrBudgetDeferred), "budget_deferred"},
		{"served but metering failed", fmt.Errorf("wrap: %w", errMeteringFailed), "metering_failed"},
		{"other is provider_error", errors.New("connection reset"), "provider_error"},
		// THE THREE A 429 PRODUCES, each with a different remedy. Read as one
		// bucket they send an operator to a console where nothing is wrong:
		// an exhausted account is topped up, a burst limit clears by itself,
		// and a refusal with no stated cause is not a claim about either.
		//
		// Built the way providerRefusal builds them — a quota and a throttle
		// both WRAP a refusal — so these also pin the order the arms are in.
		{
			"an exhausted account", fmt.Errorf("%w: %w", ErrProviderQuota,
				fmt.Errorf("%w: %w", errProviderRefused, errors.New("http 429"))),
			"provider_quota",
		},
		{
			"an ordinary burst limit", fmt.Errorf("%w: %w", ErrProviderThrottled,
				fmt.Errorf("%w: %w", errProviderRefused, errors.New("http 429"))),
			"provider_throttled",
		},
		{
			"a 429 the provider did not explain", fmt.Errorf("%w: %w", errProviderRefused,
				errors.New("http 429")),
			"provider_refused",
		},
		// Outcomes, not provider failures: a model was reached and decided.
		{"an answer the model withheld", withheldError{wire: providerAnthropic, reason: finishRefusal}, "output_withheld"},
		{"a request the provider rejected", fmt.Errorf("%w: http 400", model.ErrRequestRejected), "request_rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyError(tc.err); got != tc.want {
				t.Fatalf("classifyError = %q; want %q", got, tc.want)
			}
		})
	}
}

// The health read excludes answeredSentinels by spelling, so the sentinels the
// answered errors classify to and that list must be one set: a sentinel
// renamed on one side alone would count every withheld answer as an outage,
// or hide a real failure from the health read.
func TestTheAnsweredErrorsClassifyToExactlyTheAnsweredSentinels(t *testing.T) {
	answered := []error{model.ErrOutputWithheld, model.ErrRequestRejected, errMeteringFailed}
	classified := make([]string, 0, len(answered))
	for _, err := range answered {
		classified = append(classified, classifyError(fmt.Errorf("wrap: %w", err)))
	}
	assertSetEqual(t, "answered sentinels", classified, answeredSentinels)
}
