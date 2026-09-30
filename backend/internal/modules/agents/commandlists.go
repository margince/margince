// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// listRecordType names ListChangeCommand's fixed target, a plain string for
// the reason tagRecordType is one: the record seam has never served a list.
const listRecordType = "list"

// ListChangeCommand is one change to a list or its Shortlist members,
// whichever door asked for it: the routed list id, zero for a new list. What
// changes is the list's own module's to judge; the approval binds the list.
type ListChangeCommand struct {
	ID ids.UUID
}

// NewListChangeCall binds one list change to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewListChangeCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd ListChangeCommand) GovernedCall {
	return bind[ListChangeCommand](listChangeResolver{
		language: language,
		target:   routedRecordTarget{records: records, recordType: listRecordType},
	}, cmd)
}

type listChangeResolver struct {
	target   routedRecordTarget
	language baselanguage.Resolver
}

// Subject names the LIST the approval binds to.
func (r listChangeResolver) Subject(ctx context.Context, cmd ListChangeCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: listRecordType,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(summaryIn(ctx, r.language).changeList, cmd.ID),
	}, nil
}

// Guards stands down: the seam has never served `list`.
func (r listChangeResolver) Guards(ctx context.Context, cmd ListChangeCommand) error {
	return r.target.refuse(ctx, cmd.ID)
}
