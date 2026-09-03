// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The vocabulary command: folding one tag into another.
//
// It sits apart from the record-lifecycle commands because its subject is a
// WORD rather than a record — no owner, no row scope, and an existence probe
// where those have a scope clause (approvals/targetvisibility.go classifies
// `tag` that way already, which is what archive_record's tag arm rests on).
//
// Merging is the one tag verb that confirms. Coining and editing a word are
// auto-execute: both are visible in the vocabulary screen and both are
// reversible by the same seat that made them. A merge is neither — it rewrites
// every record carrying the source and releases the source's name, and nothing
// records where those taggings went.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeTagsCommand is one vocabulary fold, whichever door asked for it: the
// source is named by the route on REST and by an argument on the tool door,
// and the target travels in both.
type MergeTagsCommand struct {
	SourceID ids.UUID
	TargetID ids.UUID
}

// NewMergeTagsCall binds one fold to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewMergeTagsCall(tags Tags, cmd MergeTagsCommand) GovernedCall {
	return bind[MergeTagsCommand](&mergeTagsResolver{tags: tags}, cmd)
}

type mergeTagsResolver struct{ tags Tags }

// Subject names the SOURCE tag, which is the row the merge destroys: the
// target survives the act unchanged in every column, so an approval bound to
// it would pin a version nothing is about to move.
//
// The summary spells BOTH words, because the two tags are the whole of the
// decision and the inbox shows nothing else about them. A human releasing
// "Fold 'automation' into 'workflow'" is answering the question they were
// asked; one releasing "Merge tag <uuid>" is not.
//
// It states NO COUNT, deliberately. The counts GetTag answers are filtered to
// what the READER may see (collections/tagvocab.go countVisibleTagged gates
// each type on its own read grant), so a number rendered here would describe
// the stager's reach rather than the merge's, and the merge moves every
// carrying record whatever any one seat can list. A summary promising "12
// taggings" for an act that moves ninety is worse than one promising none.
func (r *mergeTagsResolver) Subject(ctx context.Context, cmd MergeTagsCommand) (StageInfo, error) {
	source, err := r.tags.GetTag(ctx, cmd.SourceID)
	if err != nil {
		return StageInfo{}, err
	}
	target, err := r.tags.GetTag(ctx, cmd.TargetID)
	if err != nil {
		return StageInfo{}, err
	}
	return StageInfo{
		TargetType: tagRecordType,
		TargetID:   cmd.SourceID,
		Summary: fmt.Sprintf("Fold tag %q into %q, releasing the name %q",
			source.Name, target.Name, source.Name),
	}, nil
}

// Guards refuses, before anything is staged, every merge that was never going
// to run: one whose tags the caller cannot see — GetTag answers a row-scope
// miss as not-found, the existence-hiding answer the merge itself would give —
// one folding a tag into itself, and one naming a RETIRED word on either side.
//
// The retired case is the one that costs most to miss, and it is not merely a
// refusal arriving late. A staging whose target row is archived is refused by
// approvals' own decidability probe (`targetVisible` requires
// `archived_at IS NULL`), so the approval is written, appears in NOBODY's
// inbox, and cannot be released by anyone — the zombie authority object this
// whole confirm-first path exists to avoid. It has to be refused HERE, because
// once the row is written there is no reader left to complain to.
//
// The source's own archived state is checked for the reason the self-merge is:
// the store refuses it at the end of its transaction, so a human's one-shot
// approval would be spent reaching that refusal with the card gone.
func (r *mergeTagsResolver) Guards(ctx context.Context, cmd MergeTagsCommand) error {
	if cmd.SourceID == cmd.TargetID {
		return &BadArgsError{Cause: fmt.Errorf("a tag cannot be folded into itself")}
	}
	source, err := r.tags.GetTag(ctx, cmd.SourceID)
	if err != nil {
		return err
	}
	if source.Archived {
		return &BadArgsError{Cause: fmt.Errorf("the word being folded is already retired")}
	}
	target, err := r.tags.GetTag(ctx, cmd.TargetID)
	if err != nil {
		return err
	}
	if target.Archived {
		return &BadArgsError{Cause: fmt.Errorf("the surviving word is retired, so it cannot take the records")}
	}
	return nil
}
