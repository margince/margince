// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package model

import "time"

// ProviderHealth is whether a provider is answering for everyone, as opposed
// to refusing one message. It is the provider's own account of itself: every
// layer above brakes on it instead of re-deriving it from error strings.
type ProviderHealth string

const (
	HealthOK ProviderHealth = "ok"
	// HealthDegraded: calls fail or time out intermittently, but some still pass.
	HealthDegraded ProviderHealth = "degraded"
	// HealthDown: nothing is getting through (unreachable, or a 5xx run).
	HealthDown ProviderHealth = "down"
	// HealthOutOfCredit: the account is empty; only an operator fixes it.
	HealthOutOfCredit ProviderHealth = "out_of_credit"
	// HealthUnauthorized: the key is revoked or invalid; only an operator fixes it.
	HealthUnauthorized ProviderHealth = "unauthorized"
)

// Blocking reports whether a call to a provider in this state is certain to
// fail until something changes. Degraded is not: some calls still pass.
func (h ProviderHealth) Blocking() bool {
	return h == HealthDown || h == HealthOutOfCredit || h == HealthUnauthorized
}

// ProviderHealthStatus is a provider's health with the moments that explain it.
type ProviderHealthStatus struct {
	Health ProviderHealth
	// Since is when the provider left HealthOK; zero while it is OK.
	Since time.Time
	// RetryAfter is when one probe call is next allowed; zero while it is OK.
	RetryAfter time.Time
}

// Blocked reports whether a call made at now should not be attempted.
func (s ProviderHealthStatus) Blocked(now time.Time) bool {
	return s.Health.Blocking() && now.Before(s.RetryAfter)
}

// NoHealth is embedded by a Client that does not track its own health: it
// reports OK, so nothing brakes on it.
type NoHealth struct{}

func (NoHealth) Health() ProviderHealthStatus { return ProviderHealthStatus{Health: HealthOK} }
