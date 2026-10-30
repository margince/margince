// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "slices"

// LifecycleLive is the company lifecycle only a signed-in user may set: it
// states that the agreed first scope is in use, which nothing in the product
// can observe.
const LifecycleLive = "live"

// companyLifecycles is every company lifecycle in wire order, each marked
// engaged when the account is one we sell to or deliver for now. Engaged
// accounts get mail reminders and ghosted-thread nudges, and a mail saying the
// contract ended offers the move to former customer.
// Held by: TestEveryCompanyLifecycleIsJudgedEngagedOrNot (backend/gates/companylifecycleengaged_test.go)
var companyLifecycles = []struct {
	name    string
	engaged bool
}{
	{"unknown", false},
	{"target", false},
	{"prospect", true},
	{"opportunity", true},
	{"onboarding", true},
	{LifecycleLive, true},
	{"customer", true},
	{"former_customer", false},
	{"disqualified", false},
}

// ClassifiedLifecycles is every company lifecycle this package has judged,
// in wire order.
// Held by: TestEveryCompanyLifecycleIsJudgedEngagedOrNot (backend/gates/companylifecycleengaged_test.go)
func ClassifiedLifecycles() []string {
	out := make([]string, 0, len(companyLifecycles))
	for _, stage := range companyLifecycles {
		out = append(out, stage.name)
	}
	return out
}

// EngagedLifecycles is the lifecycles an engaged account may hold, in wire
// order, for an SQL `= ANY($n)` argument.
func EngagedLifecycles() []string {
	var out []string
	for _, stage := range companyLifecycles {
		if stage.engaged {
			out = append(out, stage.name)
		}
	}
	return out
}

// EngagedLifecycle reports whether an account at stage is engaged.
func EngagedLifecycle(stage string) bool {
	return slices.Contains(EngagedLifecycles(), stage)
}
