// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

// Policy is the allowances an installation set and the window they are counted over.
type Policy struct {
	Limits Limits
	Window WindowPlan
}
