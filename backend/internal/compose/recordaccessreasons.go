// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Why a member can open or change a record, and the page the panel reads.
//
// The reasons EXPLAIN a verdict; they never decide one. Whether a member is
// listed and whether they can change the record come from the admissions in
// recordaccess.go. A member the admission lets in for a reason this file does
// not name is still listed, with the reasons it could find.

import (
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// accessFacts is what the reasons are read from.
type accessFacts struct {
	subject    accessSubject
	shares     []identity.AccessShare
	ownerTeams []ids.UUID
	// detail is the management view: role keys and team names ride the rows.
	detail    bool
	teamNames map[ids.UUID]string
}

// readReasons says why the record opens for p.
func readReasons(f accessFacts, p principal.Principal) []crmcontracts.RecordAccessReason {
	out := []crmcontracts.RecordAccessReason{}
	if f.subject.visibility == crmcontracts.RecordAccessVisibilityWorkspace {
		out = append(out, crmcontracts.RecordAccessReason{Code: crmcontracts.RecordAccessReasonCodeWorkspaceVisible})
	}
	if owns(f, p) {
		out = append(out, crmcontracts.RecordAccessReason{Code: crmcontracts.RecordAccessReasonCodeOwner})
	}
	for _, s := range f.shares {
		if reason, ok := shareReason(f, p, s, false); ok {
			out = append(out, reason)
		}
	}
	return out
}

// changeReasons says why p can change the record. The caller asks only for a
// member the edit admission let in.
func changeReasons(f accessFacts, p principal.Principal) []crmcontracts.RecordAccessReason {
	out := []crmcontracts.RecordAccessReason{}
	if owns(f, p) {
		out = append(out, crmcontracts.RecordAccessReason{Code: crmcontracts.RecordAccessReasonCodeOwner})
	}
	switch p.Permissions.RowScope {
	case principal.RowScopeAll:
		out = append(out, crmcontracts.RecordAccessReason{Code: crmcontracts.RecordAccessReasonCodeAllRecords})
	case principal.RowScopeTeam:
		if f.subject.ownerID != nil && !owns(f, p) && sharesATeam(p.TeamIDs, f.ownerTeams) {
			out = append(out, crmcontracts.RecordAccessReason{Code: crmcontracts.RecordAccessReasonCodeSameTeamAsOwner})
		}
	case principal.RowScopeOwn:
	}
	for _, s := range f.shares {
		if reason, ok := shareReason(f, p, s, true); ok {
			out = append(out, reason)
		}
	}
	return out
}

func owns(f accessFacts, p principal.Principal) bool {
	return f.subject.ownerID != nil && *f.subject.ownerID == p.UserID
}

func sharesATeam(mine, theirs []ids.UUID) bool {
	for _, team := range mine {
		if slices.Contains(theirs, team) {
			return true
		}
	}
	return false
}

// shareReason names one share that reaches p. For a change reason only a write
// share counts. The team behind a team share is named only in the management
// view, because it says which team the member belongs to.
func shareReason(f accessFacts, p principal.Principal, s identity.AccessShare, forChange bool) (crmcontracts.RecordAccessReason, bool) {
	write := s.Access == string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite)
	if forChange && !write {
		return crmcontracts.RecordAccessReason{}, false
	}
	access := crmcontracts.RecordAccessReasonAccessRead
	if write {
		access = crmcontracts.RecordAccessReasonAccessWrite
	}
	reason := crmcontracts.RecordAccessReason{Access: &access, ExpiresAt: s.ExpiresAt}
	switch {
	case s.SubjectType == string(crmcontracts.RecordGrantSubjectTypeUser) && s.SubjectID == p.UserID:
		reason.Code = crmcontracts.RecordAccessReasonCodeUserShare
	case s.SubjectType == string(crmcontracts.RecordGrantSubjectTypeTeam) && slices.Contains(p.TeamIDs, s.SubjectID):
		reason.Code = crmcontracts.RecordAccessReasonCodeTeamShare
		if f.detail {
			team := crmcontracts.Id(s.SubjectID)
			name := f.teamNames[s.SubjectID]
			reason.TeamId, reason.TeamName = &team, &name
		}
	default:
		return crmcontracts.RecordAccessReason{}, false
	}
	if forChange {
		reason.Code = crmcontracts.RecordAccessReasonCodeWriteShare
	}
	return reason, true
}

// groupOf is the group a member is listed under: their first reason in the
// order owner, direct share, team share, everyone.
func groupOf(reasons []crmcontracts.RecordAccessReason) crmcontracts.RecordAccessMemberGroup {
	has := func(code crmcontracts.RecordAccessReasonCode) bool {
		return slices.ContainsFunc(reasons, func(r crmcontracts.RecordAccessReason) bool { return r.Code == code })
	}
	switch {
	case has(crmcontracts.RecordAccessReasonCodeOwner):
		return crmcontracts.RecordAccessMemberGroupOwner
	case has(crmcontracts.RecordAccessReasonCodeUserShare):
		return crmcontracts.RecordAccessMemberGroupShared
	case has(crmcontracts.RecordAccessReasonCodeTeamShare):
		return crmcontracts.RecordAccessMemberGroupTeamShared
	default:
		return crmcontracts.RecordAccessMemberGroupEveryone
	}
}

func verdictWire(f accessFacts, p principal.Principal, v accessVerdict) crmcontracts.RecordAccessVerdict {
	out := crmcontracts.RecordAccessVerdict{
		CanChange:     v.canChange,
		ReadReasons:   readReasons(f, p),
		ChangeReasons: []crmcontracts.RecordAccessReason{},
	}
	if v.canChange {
		out.ChangeReasons = changeReasons(f, p)
	}
	return out
}

func memberWire(f accessFacts, j judgedMember) crmcontracts.RecordAccessMember {
	v := verdictWire(f, j.principal, j.verdict)
	out := crmcontracts.RecordAccessMember{
		UserId:        crmcontracts.Id(j.member.ID),
		DisplayName:   j.member.DisplayName,
		Group:         groupOf(v.ReadReasons),
		CanChange:     v.CanChange,
		ReadReasons:   v.ReadReasons,
		ChangeReasons: v.ChangeReasons,
	}
	// The roster fills roles only in its management view, so a caller outside
	// it gets none here without a second copy of that decision.
	if j.member.Roles != nil {
		roles := slices.Clone(j.member.Roles)
		out.Roles = &roles
	}
	return out
}

// assembleRecordAccess counts every judged member and serves one page of them
// in roster order, keyed by (created_at, id) the way GET /users pages.
func assembleRecordAccess(
	f accessFacts, judged []judgedMember, you crmcontracts.RecordAccessVerdict, cursor *string, limit *int,
) (crmcontracts.RecordAccess, error) {
	out := crmcontracts.RecordAccess{
		Visibility: f.subject.visibility,
		OwnerId:    ownerWire(f.subject.ownerID),
		Archived:   f.subject.archived,
		Detail:     f.detail,
		You:        you,
		Data:       []crmcontracts.RecordAccessMember{},
		RefreshAt:  earliestExpiry(f.shares),
	}
	members := make([]crmcontracts.RecordAccessMember, len(judged))
	for i, j := range judged {
		members[i] = memberWire(f, j)
		countGroup(&out.GroupCounts, members[i].Group)
		if members[i].CanChange {
			out.CanChangeCount++
		}
	}
	start, err := pageStart(judged, cursor)
	if err != nil {
		return crmcontracts.RecordAccess{}, err
	}
	end := min(start+storekit.ClampLimit(limit), len(judged))
	out.Data = append(out.Data, members[start:end]...)
	total := len(judged)
	out.Page = crmcontracts.PageInfo{HasMore: end < len(judged), Total: &total}
	if out.Page.HasMore {
		last := judged[end-1].member
		next, err := storekit.EncodeCursor(last.CreatedAt, last.ID)
		if err != nil {
			return crmcontracts.RecordAccess{}, err
		}
		out.Page.NextCursor = &next
	}
	return out, nil
}

// pageStart is the index of the first member after the cursor's position.
func pageStart(judged []judgedMember, cursor *string) (int, error) {
	if cursor == nil || *cursor == "" {
		return 0, nil
	}
	c, err := storekit.DecodeCursor(*cursor)
	if err != nil {
		return 0, err
	}
	for i, j := range judged {
		at := j.member.CreatedAt
		if at.After(c.CreatedAt) || (at.Equal(c.CreatedAt) && j.member.ID.String() > c.ID.String()) {
			return i, nil
		}
	}
	return len(judged), nil
}

func countGroup(counts *crmcontracts.RecordAccessGroupCounts, group crmcontracts.RecordAccessMemberGroup) {
	switch group {
	case crmcontracts.RecordAccessMemberGroupOwner:
		counts.Owner++
	case crmcontracts.RecordAccessMemberGroupShared:
		counts.Shared++
	case crmcontracts.RecordAccessMemberGroupTeamShared:
		counts.TeamShared++
	case crmcontracts.RecordAccessMemberGroupEveryone:
		counts.Everyone++
	}
}

func ownerWire(owner *ids.UUID) *crmcontracts.Id {
	if owner == nil {
		return nil
	}
	id := crmcontracts.Id(*owner)
	return &id
}

func earliestExpiry(shares []identity.AccessShare) *time.Time {
	var out *time.Time
	for _, s := range shares {
		if s.ExpiresAt != nil && (out == nil || s.ExpiresAt.Before(*out)) {
			out = s.ExpiresAt
		}
	}
	return out
}
