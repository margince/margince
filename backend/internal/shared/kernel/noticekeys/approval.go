// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package noticekeys shares durable notice identities with their erasure paths.
package noticekeys

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const approvalPrefix = "approval_pending:"

// Approval names the announcement of one approval across recipients.
func Approval(id ids.UUID) string { return approvalPrefix + id.String() }

// ParseApproval returns the approval named by an announcement key.
func ParseApproval(key string) (ids.UUID, bool) {
	rest, ok := strings.CutPrefix(key, approvalPrefix)
	if !ok {
		return ids.Nil, false
	}
	id, err := ids.Parse(rest)
	return id, err == nil
}
