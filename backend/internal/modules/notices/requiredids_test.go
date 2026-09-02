// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// The coaching mapping's own obligation: refuse an id the caller did not send,
// rather than letting the zero UUID reach the membership question and come back
// as "not your teammate" — a refusal about a person the caller never named, and
// one that reads exactly like the real permission failure beside it.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// teammatesAlways answers yes to every membership question, so a refusal in
// these tests is the id guard and cannot be the membership check.
type teammatesAlways struct{}

func (teammatesAlways) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return true, nil
}

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()

	// A coach who may coach, so nothing else in the chain can be the refusal.
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-000000000001"),
		Permissions: principal.Permissions{RoleKeys: []string{"manager"}},
	})

	var store *Store
	_, err := store.RaiseCoachNotice(ctx, teammatesAlways{}, ids.UserID{}, crmcontracts.CoachGeneral, "a word")

	var parse *values.ParseError
	if !errors.As(err, &parse) {
		t.Fatalf("an omitted recipient_user_id answered %v, wanted a validation error naming the field", err)
	}
	if parse.Field != "recipient_user_id" {
		t.Fatalf("the refusal named %q, wanted recipient_user_id", parse.Field)
	}
	// A permission sentinel here would be the wrong answer even though it also
	// refuses: it says "you may not coach that person" about nobody.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatal("an omitted recipient was refused as a permission failure, which describes a person the caller never named")
	}
}
