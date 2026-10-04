// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The two batch relinks, as commands both doors resolve. They are the single
// relink at scale: the same destination question decides the tier
// (destinationTieredCall), the same vocabulary guards the destination
// (requireLinkTarget), and the owning store performs the same guarded write
// per row.
//
// What a staged card BINDS TO differs from the single form, and on purpose.
// The single relink binds to the activity it moves. A batch names many rows,
// and no one of them is the decision: the decision is the DESTINATION — every
// card this family stages is "file these under THAT project", so the approval
// targets the project, and deciding it takes the project's read floor and row
// scope exactly as advance_project_phase's card does. A card targeting nothing
// would be decidable by anyone holding activity.update over rows they cannot
// write, which is the authority the per-row write check exists to withhold.
//
// And what a card COVERS must be exact. An approval is redeemed by retrying the
// identical call, so the rows a human released are the rows that call names.
// A named set names them; a thread key does not — it is re-read at redemption,
// and a message joining the conversation in between would be filed and stamped
// under an approval that never described it. So the thread form NEVER stages:
// a destination that needs a human is refused with the instruction to name the
// set through relink_activities, whose diff hash binds the ids verbatim. A
// thread cannot pin a version, so EVERY destination needs that confirmation
// and the form refuses them all.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// RelinkThreadCommand is one conversation re-association, whichever door asked
// for it: the thread key and the record every writable member is filed under.
type RelinkThreadCommand struct {
	ThreadKey  string
	EntityType string
	EntityID   ids.UUID
}

// NewRelinkThreadCall binds one thread move to the resolver that answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRelinkThreadCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd RelinkThreadCommand) GovernedCall {
	return destinationTieredCall{
		GovernedCall: bind[RelinkThreadCommand](&relinkThreadResolver{
			language:    language,
			destination: destinationRecord(records, cmd.EntityType),
		}, cmd),
		entityType: cmd.EntityType,
	}
}

type relinkThreadResolver struct {
	destination anchoredRecord
	language    baselanguage.Resolver
}

// Subject names the destination. It is reached only after Guards, which
// refuses every thread an agent could stage, so no card is minted for one; the
// answer keeps the resolver whole rather than a gap a later change falls
// through.
func (r *relinkThreadResolver) Subject(ctx context.Context, cmd RelinkThreadCommand) (StageInfo, error) {
	said := summaryIn(ctx, r.language)
	return destinationSubject(ctx, &r.destination, cmd.EntityType, cmd.EntityID,
		fmt.Sprintf(said.relinkThread, cmd.ThreadKey, said.noun(cmd.EntityType), cmd.EntityID))
}

// Guards refuses a blank key and a destination outside the vocabulary, and
// then refuses EVERY thread move it is asked to stage.
//
// A thread cannot pin a version, so the gate raises each one to approval
// whatever its destination — and a key is no description of what a human
// approves, because the conversation may grow before the retry. The move that
// can be approved names its rows, so the caller is sent to relink_activities.
func (r *relinkThreadResolver) Guards(_ context.Context, cmd RelinkThreadCommand) error {
	if cmd.ThreadKey == "" {
		return &BadArgsError{Cause: fmt.Errorf("thread_key names the conversation to move; it cannot be blank")}
	}
	if err := requireLinkTarget(cmd.EntityType); err != nil {
		return err
	}
	return &BadArgsError{
		Cause: fmt.Errorf("moving a conversation under a %s needs a confirmation, and a thread key cannot be "+
			"confirmed: the conversation may grow between the confirmation and the retry", cmd.EntityType),
		Guidance: "List the thread's activities (list_records with thread_key) and call relink_activities " +
			"with exactly those ids; that call stages for confirmation and moves precisely the rows confirmed.",
	}
}

// RelinkActivitiesCommand is one named-set re-association, whichever door
// asked for it.
type RelinkActivitiesCommand struct {
	ActivityIDs []ids.UUID
	EntityType  string
	EntityID    ids.UUID
}

// NewRelinkActivitiesCall binds one named-set move to the resolver that
// answers for it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRelinkActivitiesCall(records datasource.SystemOfRecordProvider, language baselanguage.Resolver, cmd RelinkActivitiesCommand) GovernedCall {
	return destinationTieredCall{
		GovernedCall: bind[RelinkActivitiesCommand](&relinkActivitiesResolver{
			language:    language,
			destination: destinationRecord(records, cmd.EntityType),
		}, cmd),
		entityType: cmd.EntityType,
	}
}

type relinkActivitiesResolver struct {
	destination anchoredRecord
	language    baselanguage.Resolver
}

// Subject names the destination record and pins its version; the count is in
// the sentence because that is the scale a human is asked to release. The ids
// themselves are in the call the diff hash binds, not in the sentence: an inbox
// line is read by whoever may decide the destination, and the rows are not
// theirs to be shown by name.
func (r *relinkActivitiesResolver) Subject(ctx context.Context, cmd RelinkActivitiesCommand) (StageInfo, error) {
	said := summaryIn(ctx, r.language)
	return destinationSubject(ctx, &r.destination, cmd.EntityType, cmd.EntityID,
		fmt.Sprintf(said.relinkActivities, len(cmd.ActivityIDs), said.noun(cmd.EntityType), cmd.EntityID))
}

// Guards refuses the destination vocabulary, an empty or oversized set — the
// store's own bound, asked before a human spends an approval on a call the
// store would refuse — and then the destination itself, the same two ways
// patchResolver.Guards refuses its own target.
func (r *relinkActivitiesResolver) Guards(ctx context.Context, cmd RelinkActivitiesCommand) error {
	if len(cmd.ActivityIDs) == 0 || len(cmd.ActivityIDs) > maxRelinkActivities {
		return &BadArgsError{Cause: fmt.Errorf("activity_ids names between 1 and %d activities; this call names %d",
			maxRelinkActivities, len(cmd.ActivityIDs))}
	}
	if err := requireLinkTarget(cmd.EntityType); err != nil {
		return err
	}
	return r.destination.refuse(ctx, cmd.EntityID)
}

// maxRelinkActivities mirrors the contract's bound on relink-bulk, so a set
// the store would refuse is refused before it reaches the store.
const maxRelinkActivities = 500

// destinationRecord anchors a resolver on the record a relink files under.
// The type is the caller's, already admitted by requireLinkTarget before any
// read happens.
func destinationRecord(records datasource.SystemOfRecordProvider, entityType string) anchoredRecord {
	return anchoredRecord{records: records, entityType: datasource.EntityType(entityType)}
}

// destinationSubject is the one spelling of a batch card's subject: the
// destination's type and id, its version as read, and the sentence.
func destinationSubject(ctx context.Context, destination *anchoredRecord, entityType string, entityID ids.UUID, summary string) (StageInfo, error) {
	rec, err := destination.row(ctx, entityID)
	if err != nil {
		return StageInfo{}, err
	}
	return StageInfo{
		TargetType:    entityType,
		TargetID:      entityID,
		TargetVersion: &rec.Version,
		Summary:       summary,
	}, nil
}
