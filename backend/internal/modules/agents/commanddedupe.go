// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The two review-queue commands, whichever door asked: dismissing a pair and
// re-opening one. Both are auto_execute and stage nothing on today's tiers; they
// are registered because a tier floor tightening either would otherwise leave the
// REST door with no subject to name an approval after.
//
// The target is the pair's own id, filed under the contact type the policy table
// declares for the verb: the contract admits record types by RBAC object, and a
// queue pair has none of its own. The store probes write authority over both
// records itself.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// DedupeCommand is one queue verb on one pair.
type DedupeCommand struct {
	ID ids.UUID
}

// NewDismissDuplicateCall binds a dismissal to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDismissDuplicateCall(language baselanguage.Resolver, cmd DedupeCommand) GovernedCall {
	return bind[DedupeCommand](dedupeResolver{language: language, reopen: false}, cmd)
}

// NewReopenDuplicateCall binds a re-open to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewReopenDuplicateCall(language baselanguage.Resolver, cmd DedupeCommand) GovernedCall {
	return bind[DedupeCommand](dedupeResolver{language: language, reopen: true}, cmd)
}

type dedupeResolver struct {
	language baselanguage.Resolver
	reopen   bool
}

// Subject names the PAIR the approval binds to.
func (r dedupeResolver) Subject(ctx context.Context, cmd DedupeCommand) (StageInfo, error) {
	words := summaryIn(ctx, r.language)
	sentence := words.dismissDuplicate
	if r.reopen {
		sentence = words.reopenDuplicate
	}
	return StageInfo{
		TargetType: importObjectContact,
		TargetID:   cmd.ID,
		Summary:    fmt.Sprintf(sentence, cmd.ID),
	}, nil
}

// Guards stands down: the store refuses a pair the caller cannot write, with the
// probe the web screen takes.
func (r dedupeResolver) Guards(context.Context, DedupeCommand) error { return nil }
