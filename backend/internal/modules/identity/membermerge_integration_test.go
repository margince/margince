// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A used account merges only after the holder of each side that can still sign
// in has confirmed through a mailed link. These tests drive the preview,
// request, confirmation and merge the endpoints call. The work that moves with
// a merge belongs to other modules and is proven where they are; here it is an
// empty answer, so each test is about who must confirm.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noMemberWork is retirement's collaborator with no work to move.
type noMemberWork struct{}

func (noMemberWork) Inspect(context.Context, pgx.Tx, ids.UUID, *ids.UUID, bool) (MemberWorkInspection, error) {
	return MemberWorkInspection{}, nil
}

func (noMemberWork) Retire(context.Context, pgx.Tx, ids.UUID, *ids.UUID, bool) error { return nil }

type mergeEnv struct {
	*revocationEnv
	t   *testing.T
	ctx context.Context
}

func setupMergeEnv(t *testing.T, slug string) *mergeEnv {
	t.Helper()
	e := setupRevocationEnv(t, slug)
	e.svc.WithMemberWork(noMemberWork{})
	return &mergeEnv{revocationEnv: e, t: t, ctx: e.wsCtx(e.admin)}
}

// account is an invited member, which nobody has used.
func (m *mergeEnv) account(handle string) ids.UserID {
	m.t.Helper()
	return m.seat(m.t, handle, "rep").UserID
}

// used gives the account a password, which is what makes it used.
func (m *mergeEnv) used(handle string) ids.UserID {
	m.t.Helper()
	id := m.account(handle)
	if _, err := m.owner.Exec(context.Background(),
		`UPDATE app_user SET password_hash = 'not-a-real-hash', status = 'active' WHERE id = $1`, id); err != nil {
		m.t.Fatal(err)
	}
	return id
}

func (m *mergeEnv) preview(source, survivor ids.UserID) crmcontracts.MemberRetirementPreview {
	m.t.Helper()
	target := survivor.UUID
	preview, err := m.svc.PreviewMemberRetirement(m.ctx, m.admin, source, &target, true)
	if err != nil {
		m.t.Fatal(err)
	}
	return preview
}

// request asks for the confirmations the current preview lists and answers the
// mailed tokens by the account they are for.
func (m *mergeEnv) request(source, survivor ids.UserID) map[ids.UUID]string {
	m.t.Helper()
	links, err := m.svc.RequestMemberMerge(m.ctx, m.admin, source, survivor.UUID, m.preview(source, survivor).Token)
	if err != nil {
		m.t.Fatal(err)
	}
	out := map[ids.UUID]string{}
	for _, link := range links {
		out[link.UserID] = link.RawToken
	}
	return out
}

func (m *mergeEnv) confirm(raw string) {
	m.t.Helper()
	if err := m.svc.ConfirmMemberMerge(m.ctx, raw); err != nil {
		m.t.Fatalf("confirming: %v", err)
	}
}

func (m *mergeEnv) merge(source, survivor ids.UserID, token string) error {
	target := survivor.UUID
	return m.svc.RetireMember(m.ctx, m.admin, source, &target, token, true)
}

func (m *mergeEnv) deactivate(user ids.UserID) {
	m.t.Helper()
	if err := m.svc.DeactivateUser(m.ctx, m.admin, DeactivateUserInput{UserID: user}); err != nil {
		m.t.Fatal(err)
	}
}

func (m *mergeEnv) count(sql string, args ...any) int {
	m.t.Helper()
	var n int
	if err := m.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		m.t.Fatal(err)
	}
	return n
}

// states names each side's state in the preview, source first.
func states(preview crmcontracts.MemberRetirementPreview) []string {
	var out []string
	if preview.Confirmations != nil {
		for _, side := range *preview.Confirmations {
			out = append(out, string(side.Side)+":"+string(side.State))
		}
	}
	return out
}

func wantStates(t *testing.T, preview crmcontracts.MemberRetirementPreview, want ...string) {
	t.Helper()
	if got := states(preview); !slices.Equal(got, want) {
		t.Fatalf("confirmation states = %v, want %v", got, want)
	}
}

func holds(preview crmcontracts.MemberRetirementPreview, blocker string) bool {
	return slices.Contains(preview.Blockers, crmcontracts.MemberRetirementBlocker(blocker))
}

func TestTwoUsedAccountsMergeOnlyAfterBothHoldersConfirm(t *testing.T) {
	m := setupMergeEnv(t, "merge-both")
	source, survivor := m.used("both-source"), m.used("both-survivor")

	preview := m.preview(source, survivor)
	wantStates(t, preview, "source:not_requested", "survivor:not_requested")
	if !holds(preview, "confirmation_missing") {
		t.Fatalf("blockers = %v, want confirmation_missing", preview.Blockers)
	}
	if err := m.merge(source, survivor, preview.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merge with no confirmation = %v, want a conflict", err)
	}

	links := m.request(source, survivor)
	if len(links) != 2 {
		t.Fatalf("links minted = %d, want one for each side", len(links))
	}
	wantStates(t, m.preview(source, survivor), "source:requested", "survivor:requested")

	m.confirm(links[source.UUID])
	half := m.preview(source, survivor)
	wantStates(t, half, "source:confirmed", "survivor:requested")
	if !holds(half, "confirmation_missing") {
		t.Fatal("one confirmation of two lifted the blocker")
	}
	if err := m.merge(source, survivor, half.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merge with one confirmation = %v, want a conflict", err)
	}

	m.confirm(links[survivor.UUID])
	ready := m.preview(source, survivor)
	wantStates(t, ready, "source:confirmed", "survivor:confirmed")
	if len(ready.Blockers) != 0 {
		t.Fatalf("blockers after both confirmed = %v", ready.Blockers)
	}
	if err := m.merge(source, survivor, ready.Token); err != nil {
		t.Fatal(err)
	}
	if n := m.count(`SELECT count(*) FROM app_user WHERE id = $1 AND removed_at IS NOT NULL AND merged_into_user_id = $2`, source, survivor); n != 1 {
		t.Fatal("the source is not recorded as merged into the survivor")
	}
	if n := m.count(`SELECT count(*) FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1 AND r.key = 'rep'`, survivor); n != 1 {
		t.Fatal("the survivor's roles changed")
	}
	if n := m.count(`SELECT coalesce(max(jsonb_array_length(after -> 'merge_confirmations')), 0) FROM audit_log
 WHERE entity_type = 'user' AND entity_id = $1 AND action = 'archive'`, source); n != 2 {
		t.Fatalf("the merge audit names %d confirmations, want 2", n)
	}
	if n := m.count(`SELECT count(*) FROM auth_token WHERE purpose = 'member_merge_confirm' AND expires_at > now()`); n != 0 {
		t.Fatalf("%d confirmations still live after the merge", n)
	}
}

func TestADeactivatedSideNeedsNoConfirmation(t *testing.T) {
	m := setupMergeEnv(t, "merge-deactivated-source")
	source, survivor := m.used("deact-source"), m.used("deact-survivor")
	m.deactivate(source)

	wantStates(t, m.preview(source, survivor), "source:not_needed", "survivor:not_requested")
	links := m.request(source, survivor)
	if len(links) != 1 || links[survivor.UUID] == "" {
		t.Fatalf("links = %v, want one for the active survivor alone", links)
	}
	m.confirm(links[survivor.UUID])
	ready := m.preview(source, survivor)
	if len(ready.Blockers) != 0 {
		t.Fatalf("blockers = %v", ready.Blockers)
	}
	if err := m.merge(source, survivor, ready.Token); err != nil {
		t.Fatalf("merging a deactivated source with its survivor's confirmation: %v", err)
	}
}

func TestTwoDeactivatedAccountsMergeOnAnAdministratorsSayAlone(t *testing.T) {
	m := setupMergeEnv(t, "merge-both-deactivated")
	source, survivor := m.used("gone-source"), m.used("gone-survivor")
	m.deactivate(source)
	m.deactivate(survivor)

	ready := m.preview(source, survivor)
	wantStates(t, ready, "source:not_needed", "survivor:not_needed")
	if len(ready.Blockers) != 0 {
		t.Fatalf("blockers = %v", ready.Blockers)
	}
	if err := m.merge(source, survivor, ready.Token); err != nil {
		t.Fatal(err)
	}
}

func TestTheAdministratorCannotBeEitherSideOfARequestedMerge(t *testing.T) {
	m := setupMergeEnv(t, "merge-admin-side")
	used := m.used("admin-side-used")
	other := m.used("admin-side-other")

	intoAdmin := m.preview(used, m.admin.UserID)
	if _, err := m.svc.RequestMemberMerge(m.ctx, m.admin, used, m.admin.UserID.UUID, intoAdmin.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("requesting a merge into the administrator = %v, want a conflict", err)
	}
	if _, err := m.svc.RequestMemberMerge(m.ctx, m.admin, m.admin.UserID, other.UUID, "any"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("requesting a merge of the administrator = %v, want a conflict", err)
	}
	if n := m.count(`SELECT count(*) FROM auth_token WHERE purpose = 'member_merge_confirm'`); n != 0 {
		t.Fatalf("%d confirmations minted for a refused request", n)
	}
}

func TestAConfirmationCountsOnlyForItsOwnPair(t *testing.T) {
	m := setupMergeEnv(t, "merge-own-pair")
	source, survivor, third := m.used("pair-source"), m.used("pair-survivor"), m.account("pair-third")
	links := m.request(source, survivor)
	m.confirm(links[source.UUID])
	m.confirm(links[survivor.UUID])

	wantStates(t, m.preview(source, third), "source:not_requested", "survivor:not_requested")
	if err := m.svc.ConfirmMemberMerge(m.ctx, links[source.UUID]); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a token used twice = %v, want not found", err)
	}
	if err := m.svc.ConfirmMemberMerge(m.ctx, strings.Repeat("0", 64)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unknown token = %v, want not found", err)
	}
}

func TestAnExpiredConfirmationNoLongerCounts(t *testing.T) {
	m := setupMergeEnv(t, "merge-expired")
	source, survivor := m.used("expired-source"), m.used("expired-survivor")
	links := m.request(source, survivor)
	m.confirm(links[source.UUID])
	if _, err := m.owner.Exec(context.Background(),
		`UPDATE auth_token SET expires_at = created_at + interval '1 microsecond' WHERE purpose = 'member_merge_confirm'`); err != nil {
		t.Fatal(err)
	}

	wantStates(t, m.preview(source, survivor), "source:not_requested", "survivor:not_requested")
	if err := m.svc.ConfirmMemberMerge(m.ctx, links[survivor.UUID]); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an expired link = %v, want not found", err)
	}
}

func TestAConfirmationGivenAfterThePreviewChangesTheReviewedToken(t *testing.T) {
	m := setupMergeEnv(t, "merge-stale-preview")
	source, survivor := m.used("stale-source"), m.used("stale-survivor")
	links := m.request(source, survivor)
	m.confirm(links[source.UUID])
	stale := m.preview(source, survivor)
	m.confirm(links[survivor.UUID])

	if err := m.merge(source, survivor, stale.Token); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("merging from the preview read before the last confirmation = %v, want version skew", err)
	}
	if n := m.count(`SELECT count(*) FROM app_user WHERE id = $1 AND removed_at IS NULL`, source); n != 1 {
		t.Fatal("a stale merge removed the source")
	}
}

func TestReactivatingADeactivatedSourceAsksItsHolderAgain(t *testing.T) {
	m := setupMergeEnv(t, "merge-reactivated")
	source, survivor := m.used("react-source"), m.used("react-survivor")
	m.deactivate(source)
	links := m.request(source, survivor)
	m.confirm(links[survivor.UUID])

	if err := m.svc.ReactivateUser(m.ctx, m.admin, source); err != nil {
		t.Fatal(err)
	}
	back := m.preview(source, survivor)
	wantStates(t, back, "source:not_requested", "survivor:confirmed")
	if err := m.merge(source, survivor, back.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("merging a reactivated source without its confirmation = %v, want a conflict", err)
	}
}

func TestDeactivatingASideWithdrawsItsConfirmation(t *testing.T) {
	m := setupMergeEnv(t, "merge-withdrawn")
	source, survivor := m.used("withdraw-source"), m.used("withdraw-survivor")
	links := m.request(source, survivor)
	m.confirm(links[source.UUID])
	m.confirm(links[survivor.UUID])

	m.deactivate(survivor)
	if err := m.svc.ReactivateUser(m.ctx, m.admin, survivor); err != nil {
		t.Fatal(err)
	}
	wantStates(t, m.preview(source, survivor), "source:not_requested", "survivor:not_requested")
}

func TestAnAddressChangeWithdrawsTheConfirmationsOfItsPair(t *testing.T) {
	m := setupMergeEnv(t, "merge-address")
	source, survivor := m.used("addr-source"), m.account("addr-survivor")
	links := m.request(source, survivor)
	m.confirm(links[source.UUID])
	m.confirm(links[survivor.UUID])

	row, err := m.svc.GetUser(m.ctx, m.admin, survivor)
	if err != nil {
		t.Fatal(err)
	}
	edit := MemberEdit{Name: row.DisplayName, Email: "moved@" + m.slug + ".test", ExpectedUpdatedAt: row.UpdatedAt}
	if err := m.svc.UpdateMember(m.ctx, m.admin, survivor, edit); err != nil {
		t.Fatal(err)
	}
	wantStates(t, m.preview(source, survivor), "source:not_requested", "survivor:not_requested")
}

func TestAnUnusedSourceStillMergesWithoutAnyConfirmation(t *testing.T) {
	m := setupMergeEnv(t, "merge-unused")
	source, survivor := m.account("unused-source"), m.used("unused-survivor")
	ready := m.preview(source, survivor)
	if ready.Confirmations != nil || len(ready.Blockers) != 0 {
		t.Fatalf("an unused source asks for %+v, blockers %v", ready.Confirmations, ready.Blockers)
	}
	if err := m.merge(source, survivor, ready.Token); err != nil {
		t.Fatal(err)
	}
}
