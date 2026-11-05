// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type scopedRiskRead struct {
	owners *storekit.OwnerSet
	calls  int
}

func (r *scopedRiskRead) Quiet(_ context.Context, owners *storekit.OwnerSet) ([]RiskyDeal, bool, error) {
	r.owners = owners
	r.calls++
	return nil, false, nil
}

func TestRiskCandidatesReceiveOwnershipBeforeTheirRead(t *testing.T) {
	ctx := pageReader()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing fixture reader")
	}
	other, former := ids.NewV7(), ids.NewV7()
	checks := []struct {
		name    string
		service Service
		want    *storekit.OwnerSet
	}{
		{"mine", Service{taskScope: TasksMine}, &storekit.OwnerSet{IDs: []ids.UUID{actor.UserID}}},
		{"named member", Service{taskScope: TasksOwnedBy, taskOwner: other}, &storekit.OwnerSet{IDs: []ids.UUID{other}}},
		{"unassigned", Service{taskScope: TasksUnassigned}, &storekit.OwnerSet{Unassigned: true}},
		{"all visible", Service{}, nil},
		{"empty team", Service{noticeOwners: []ids.UUID{}}, &storekit.OwnerSet{IDs: []ids.UUID{}, Unassigned: true}},
		{"team and former members", Service{noticeOwners: []ids.UUID{other}, teamRoster: &rosterRead{former: []ids.UUID{former}}}, &storekit.OwnerSet{IDs: []ids.UUID{other, former}, Unassigned: true}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			boundary := &scopedRiskRead{}
			check.service.atRisk = boundary
			if _, err := check.service.readRisk(ctx, &crmcontracts.Attention{}); err != nil {
				t.Fatal(err)
			}
			if boundary.calls != 1 || !reflect.DeepEqual(boundary.owners, check.want) {
				t.Fatalf("risk query received %+v, want %+v", boundary.owners, check.want)
			}
		})
	}
}

func TestAnUnresolvedOwnerNeverRunsAnUnrestrictedRiskRead(t *testing.T) {
	for _, check := range []struct {
		name    string
		ctx     context.Context
		service Service
		want    error
	}{
		{"missing human", context.Background(), Service{taskScope: TasksMine}, apperrors.ErrPermissionDenied},
	} {
		t.Run(check.name, func(t *testing.T) {
			boundary := &scopedRiskRead{}
			check.service.atRisk = boundary
			_, err := check.service.readRisk(check.ctx, &crmcontracts.Attention{})
			if !errors.Is(err, check.want) || boundary.calls != 0 {
				t.Fatalf("unresolved selection reached risk reader: calls=%d, err=%v", boundary.calls, err)
			}
		})
	}
}

func TestTheTeamBoardScopesRiskCandidatesToItsMeasuredRoster(t *testing.T) {
	member := ids.NewV7()
	boundary := &scopedRiskRead{}
	service := Service{atRisk: boundary}
	load := teamCounts{counts: map[ids.UUID]crmcontracts.TeamBoardCounts{}}
	if err := service.addAtRisk(pageReader(), &load, []TeamMember{{UserID: member}}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	want := &storekit.OwnerSet{IDs: []ids.UUID{member}, Unassigned: true}
	if !reflect.DeepEqual(boundary.owners, want) {
		t.Fatalf("board risk query received %+v, want %+v", boundary.owners, want)
	}
}

func TestTheRosterIsReadOnceWithSeparateRecoveryAndPersonalAudiences(t *testing.T) {
	live, former := ids.NewV7(), ids.NewV7()
	past := &countedFormerRead{ids: []ids.UUID{former}}
	service := Service{teammates: roster{{UserID: live}}, former: past}
	narrowed, err := service.forNoticeTeam(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	want := []ids.UUID{live, former}
	if !reflect.DeepEqual(narrowed.noticeOwners, []ids.UUID{live}) {
		t.Fatalf("disclosure audience = %v, want %v", narrowed.noticeOwners, want)
	}
	owners, err := narrowed.recordOwners(pageReader())
	if err != nil || !reflect.DeepEqual(owners.IDs, want) {
		t.Fatalf("risk audience = %+v, %v", owners, err)
	}
	if got := narrowed.approvalQuery(10).RequestedForOwners; !reflect.DeepEqual(got, []ids.UUID{ids.Nil, live}) {
		t.Fatalf("decision audience = %v", got)
	}
	if _, err := narrowed.formerTeammates(pageReader()); err != nil {
		t.Fatal(err)
	}
	if past.calls != 1 {
		t.Fatalf("former roster read %d times", past.calls)
	}
	if service.noticeOwners != nil || service.teamRoster != nil {
		t.Fatal("request audience escaped onto the shared service")
	}
	past.err = apperrors.ErrPermissionDenied
	if _, err := service.forNoticeTeam(pageReader()); !errors.Is(err, past.err) {
		t.Fatalf("roster failure was treated as an empty team: %v", err)
	}
}

type countedFormerRead struct {
	ids   []ids.UUID
	calls int
	err   error
}

func (r *countedFormerRead) FormerTeammatesOfCaller(context.Context) ([]ids.UUID, error) {
	r.calls++
	return r.ids, r.err
}
