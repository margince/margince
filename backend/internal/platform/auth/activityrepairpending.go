// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
)

// ActivityRepairPendingClause detects incomplete evidence within the caller's discovery reach.
func ActivityRepairPendingClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	pending := ActivityRepairPendingSQL(alias)
	if UnboundedFor(p, linkTargetTables...) {
		return pending, nil
	}
	return fmt.Sprintf(`(%s) AND (%s OR %s)`, pending,
		activityReachArms(p, alias, arg), assignedCommitmentArm(p, alias, arg, activityUnrestrictedClause("assigned_source"))), nil
}

// ActivityRepairPendingSQL preserves an existing metadata scope without returning message content.
func ActivityRepairPendingSQL(alias string) string {
	return activityUnrestrictedClause(alias) + " AND NOT activity_thread_available(" + alias + ".thread_key)"
}
