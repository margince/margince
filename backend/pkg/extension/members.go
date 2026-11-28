// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "context"

// MaxRosterMembers is the most members one List returns.
const MaxRosterMembers = 200

// MemberQuery narrows a roster list.
type MemberQuery struct {
	// Role is a role's key, such as `admin`. Empty lists every active member.
	Role string
}

// Member is one active member of the installation, as much as a unit needs to
// hand them work: no email, no roles.
type Member struct {
	ID          string
	DisplayName string
	// IsAgent marks an agent seat, which cannot take a task.
	IsAgent bool
}

// MemberRepo reads who holds a role.
type MemberRepo interface {
	// List answers the active members, ordered by display name, and at most
	// MaxRosterMembers. It answers ErrNotDeclared unless the unit lists
	// CapabilityMembers in Extension.Needs, and ErrForbidden to a job tick.
	List(ctx context.Context, q MemberQuery) ([]Member, error)
}
