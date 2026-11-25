// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package healthrank orders System health's conditions and findings.
package healthrank

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// ConditionPriority ranks a condition, the most urgent first.
func ConditionPriority(condition crmcontracts.HealthSourceCondition) int {
	switch condition {
	case crmcontracts.HealthSourceConditionTechnical:
		return 0
	case crmcontracts.HealthSourceConditionUnverified:
		return 1
	case crmcontracts.HealthSourceConditionWaitingHuman:
		return 2
	case crmcontracts.HealthSourceConditionRetrying:
		return 3
	case crmcontracts.HealthSourceConditionUnknown:
		return 4
	case crmcontracts.HealthSourceConditionProcessing:
		return 5
	default:
		return 6
	}
}

// FindingLess orders findings by urgency, then by what they are about, so one
// report always lists them alike.
func FindingLess(a, b crmcontracts.HealthFinding) bool {
	if a.Urgency != b.Urgency {
		return a.Urgency < b.Urgency
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	unit := func(f crmcontracts.HealthFinding) string {
		if f.Unit == nil {
			return ""
		}
		return string(*f.Unit)
	}
	if unit(a) != unit(b) {
		return unit(a) < unit(b)
	}
	if (a.Count == nil) != (b.Count == nil) {
		return a.Count != nil
	}
	if a.Count != nil && b.Count != nil && *a.Count != *b.Count {
		return *a.Count > *b.Count
	}
	if a.FirstObservedAt != nil && b.FirstObservedAt != nil && !a.FirstObservedAt.Equal(*b.FirstObservedAt) {
		return a.FirstObservedAt.Before(*b.FirstObservedAt)
	}
	if (a.FirstObservedAt == nil) != (b.FirstObservedAt == nil) {
		return a.FirstObservedAt != nil
	}
	return a.Id < b.Id
}
