// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// MemberWorkCount includes every transferable kind in a handover snapshot.
func MemberWorkCount(c crmcontracts.HandoverCounts) int {
	return c.Contact + c.Company + c.Deal + c.Lead + c.Project + c.Task + c.List + c.Webhook + c.SdrHandoff + c.RecordAssignment
}
