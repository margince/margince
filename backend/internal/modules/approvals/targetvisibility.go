// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The target half of the decision-authority predicate: whether the record a
// staged row points at is one the asking human may see. It composes what the
// owning store's read path composes — the object-level READ grant on the
// target's type, and then that store's own row rule — because a probe wider than
// that store discloses a record through the inbox, and a probe narrower than it
// strands the staged row where nobody can release or reject it.
//
// The object half is applied ONCE, above every arm; each arm below is the row
// half alone.

package approvals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// targetProbe names HOW the ROW half of a target type's visibility is decided —
// the object-read grant on the type is composed above every one of them, in
// targetVisible. It exists so the answer "is a staged row against this target
// decidable at all" has ONE source: targetVisible dispatches on it,
// TargetShapeDecidable reports on it, and ClassifiedTargetTypes enumerates what
// carries one.
//
// That mattered the moment the tool surface started minting staged rows for types
// nobody had checked. A type with no rule is not decidable, which means its
// staged row is invisible in the inbox AND undecidable at the decision — an
// authority object a human can neither release nor reject, and the fan-out that
// would have told them about it is dropped for the same reason. The composition
// layer derives the obligation over the generated policy table
// (TestEveryConfirmFirstTargetTypeIsDecidable), so a confirm-first verb whose
// staged shape has no rule here fails a gate instead of shipping a zombie.
type targetProbe int

const (
	// probeNoRule is the zero value on purpose: an unrecognized type falls here
	// and fails closed.
	probeNoRule targetProbe = iota
	// probeOwnScope — the row carries owner_id, so its own row scope answers.
	probeOwnScope
	// probeInheritedScope — the row owns no owner_id and inherits from what it
	// points at: an offer from its deal, a signal from its subject, an activity
	// from any linked record, an edge from ALL of its endpoints.
	probeInheritedScope
	// probeExistence — workspace-shared config with no row scope of its own, so
	// the row half is existence alone: a staging against a row that is gone is
	// not decidable. The object-read floor and the decision grants are the
	// authority.
	probeExistence
	// probeOwnerOnly — the row is PERSONAL state, which its owning store serves
	// to one human and nobody else, so the probe is that same ownership. It is
	// distinct from probeOwnScope because own/team/all is WIDER than the API on
	// such a table: it would show a manager or an admin a colleague's private
	// row, and the change staged against it, in an inbox the read path refuses
	// them the row itself in.
	probeOwnerOnly
	// probeActingWorkspace — the target IS a workspace (an effective-dated price
	// sheet with no row of its own yet), so the floor is that it is THIS one.
	probeActingWorkspace
)

// targetProbes is the classification itself: which probe answers for each staged
// target type. It is a TABLE rather than a switch because two readers need it —
// targetVisible dispatches on it, and ClassifiedTargetTypes hands the composition
// layer's parity gate the set it has to cover, which a switch cannot enumerate.
// A gate that cannot enumerate its subject set silently shrinks to whatever
// source it can enumerate, which is quieter than being wrong.
//
// The existence-probed types are FOLDED IN from existenceProbes rather than
// listed again: that map already names them together with the query each needs,
// and a second spelling is how one of them ends up classified with no query to
// answer it. The fold-in wins on a key it shares with the literal above, so the
// two must stay disjoint — TestEveryTargetProbeMirrorsItsOwningStoresReadRule
// pins each type to its own probe, which is what a collision breaks.
var targetProbes = func() map[string]targetProbe {
	probes := map[string]targetProbe{
		tableContact:       probeOwnScope,
		tableCompany:       probeOwnScope,
		tableDeal:          probeOwnScope,
		tableLead:          probeOwnScope,
		tableProject:       probeOwnScope,
		tableList:          probeOwnScope,
		targetOffer:        probeInheritedScope,
		targetSignal:       probeInheritedScope,
		objectActivity:     probeInheritedScope,
		targetRelationship: probeInheritedScope,
		targetSavedView:    probeOwnerOnly,
		targetFxRate:       probeActingWorkspace,
		targetAIModelRate:  probeActingWorkspace,
	}
	for targetType := range existenceProbes {
		probes[targetType] = probeExistence
	}
	return probes
}()

// targetShape is a staged target reduced to which halves it carries, which is
// all the shape rule below needs — and naming the two at every call site is what
// keeps a caller from transposing them.
type targetShape struct{ hasType, hasID bool }

// settledByShape answers the staged shapes whose decidability the target PAIR
// settles on its own, before any row is probed:
//
//   - NO target id — whether the row names a target type (a staged CREATE,
//     whose record does not exist yet) or nothing at all (a cold-start
//     proposal, which is about no record yet) — carries no ROW half at all.
//     There is nothing whose scope could bound it, so what remains is the
//     object-read floor on the type it names (targetVisible applies it) and the
//     grants requireDecisionGrants demands of the caller. One id-less shape
//     needs more than that floor and gets it from the staged row rather than
//     from the pair: stagedForStagerOnly below.
//   - An id with NO type is not decidable. It names a concrete record the
//     probe cannot resolve, and treating it as unbounded would put that
//     record's summary and proposed change in the inbox of everyone holding
//     the object grant, and let any of them decide a write against a row their
//     own scope hides.
//
// A pair carrying BOTH halves is not settled here: it goes to the target type's
// own probe. ONE spelling of the rule, because targetVisible runs it for the
// inbox and the decision while TargetShapeDecidable reports it to the
// composition layer's gate — a second copy would let the gate read green over
// the predicate a human's inbox actually runs.
func settledByShape(shape targetShape) (settled, visible bool) {
	if !shape.hasID {
		return true, true
	}
	if !shape.hasType {
		return true, false
	}
	return false, false
}

// stagedForStagerOnly reports the staged shape whose only honest bound is the
// IDENTITY of the member it was staged for: a target type whose owning store
// serves its rows to one human each, staged with no target id.
//
// Read on the type is the floor for every other id-less create, and for an
// owner-only type it is the wrong floor — read on a personal table is held by
// every seat allowed rows of its own there, so it would put one human's private
// row in front of all of them, since the summary and the proposed change ARE
// that row's content. Neither is existence a floor: the record does not exist
// yet. What remains is what the staging itself recorded — the member it was
// staged for — which is the same predicate selfOnlyKinds applies for a kind
// whose subject is one member's own business.
//
// Derived from the probe classification rather than named type by type, so a
// personal table enrolled in probeOwnerOnly later inherits the rule instead of
// falling into the read-on-the-type default.
func stagedForStagerOnly(targetType *string, hasTargetID bool) bool {
	return targetType != nil && !hasTargetID && probeFor(*targetType) == probeOwnerOnly
}

// TargetShapeDecidable reports whether a staged row carrying this target SHAPE
// — the target type, plus whether the staging names a concrete target id — can
// be seen and decided at all. Exported for the composition layer's gate: a
// confirm-first verb whose staged shape answers false mints authority objects
// no human can ever release or reject.
//
// The type alone is not the question, and asking it that way reads green over
// half the class: a staging with no target id is decidable whatever its type
// is, and a type with a probe below is still undecidable when the id that probe
// needs is absent.
//
// It answers the shape and the row half's classification, which is all a SHAPE
// can answer. The object-read floor targetVisible composes above every arm is a
// question about the asking human, so the gate proves that half its own way:
// read on the staged target's own type has to be a grant some role document may
// name, or no principal that can exist passes the floor.
func TargetShapeDecidable(targetType string, hasTargetID bool) bool {
	if settled, visible := settledByShape(targetShape{hasType: true, hasID: hasTargetID}); settled {
		return visible
	}
	return probeFor(targetType) != probeNoRule
}

// targetVisible applies the target record's OWN read rule to the approval —
// BOTH halves of it, in the order the owning store applies them. The object-READ
// grant on the target's type comes first: holding tag.delete does not entitle a
// human to the tags they may not read, nor to the change staged against one. The
// arm then applies that store's row rule: holding deal.update does not entitle a
// rep to see — or decide — a staged change against another team's deal, and
// holding saved_view.delete does not entitle anyone to a colleague's private
// view. With the decision grants requireDecisionGrants demands (authority.go),
// those are the whole predicate — read the target's type, see its row, hold what
// the effect needs — so the inbox can never disclose more than the record would.
//
// A pair missing either half is answered by settledByShape, which states that
// rule — still under the floor whenever it names a type at all; a pair carrying
// both goes to the type's probe below, and a target the probe errors on stays
// invisible.
//
// It takes the pair rather than a row because a target-FILTERED read asks the
// same question about a target the client named, before any row is in hand.
// That read is entered only with BOTH halves in hand (ListInput.targeted), so
// the id-less shape can never turn a type filter into a page of rows whose own
// targets the caller cannot see. An unrecognized type must fail closed there
// too: auth.VisibleTo errors on a table it does not row-scope, so the switch
// below — not the caller — is what keeps a made-up target_entity_type from
// reaching it.
// targetAuthority selects which half of the target's own rule the probe applies:
// what the inbox may SHOW, or what a decider may CHANGE.
//
// They are not the same question, and until #1373 they were the same code. A
// staged change is a mutation of the record it names — the effect writes it —
// so a colleague handed a `read` share of that record could see the proposal
// (right: the record is theirs to open) and also approve it, which committed a
// write their own PATCH would have been refused.
type targetAuthority bool

const (
	forReading  targetAuthority = false
	forDeciding targetAuthority = true
)

// targetVisible is the READ half — what the inbox may show and what a
// target-filtered list may page over.
func targetVisible(ctx context.Context, tx pgx.Tx, targetType *string, targetID *ids.UUID) (bool, error) {
	visible, err := targetsPermitted(ctx, tx, []targetRef{{targetType, targetID}}, forReading)
	if err != nil {
		return false, err
	}
	return visible[0], nil
}

// objectReadable reports whether the asking human holds the object-level READ
// grant on a staged target's type — the auth.Require half every owning store
// applies before it looks at a row at all.
//
// A denial is an ANSWER, not an error: the inbox hides an approval it may not
// disclose exactly as the record's own read hides the record, so a permission
// denial must reach the caller as not-visible and become a 404, never a 403 that
// would confirm the staged row exists. Any other resolution failure surfaces.
func objectReadable(ctx context.Context, object string) (bool, error) {
	switch err := auth.Require(ctx, object, principal.ActionRead); {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return false, nil
	default:
		return false, err
	}
}

// probeAnswer turns a row probe's error into the ANSWER the inbox needs: absent
// or out of scope is not-permitted — the two are indistinguishable by design,
// which is what lets the inbox hide a staged row the same way the record's own
// read hides the record — and anything else is a real failure the caller
// surfaces rather than reads as a refusal.
//
// ErrPermissionDenied joins ErrNotFound here because the DECIDE probe can raise
// it: a caller holding a `read` share of the target can see the record, so the
// write probe has nothing left to hide and answers 403 rather than 404. Reading
// that as a failure instead of a refusal would turn one undecidable row into a
// 500 for the whole INBOX — every other pending approval included — which is
// how this arrived: the first version of the decide split did exactly that, and
// the integration suite caught it on the list call rather than the decision.
func probeAnswer(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		return false, nil
	default:
		return false, err
	}
}

// existenceProbes are the target types whose owning store applies NO row scope
// of its own, workspace-shared config an object grant governs. Each maps to the
// existence query that is its whole visibility floor. Each query answers which
// ids in $1 still exist. The decision-grant check is the authority question; a
// staging against a record that does not exist is still not decidable.
//
// Each query mirrors what its own store's read path admits, and the archive
// predicate differs across them for ONE reason, stated here rather than repeated
// per query: where archive IS the delete, an archived row is not a live target
// and the effect staged against it could never land, so the probe requires a
// live row. custom_field is the single exception — retire is a status flip that
// keeps the row live, so a staged edit against a retired field stays decidable.
var existenceProbes = map[string]string{
	objectUserAdmin:           `SELECT id FROM app_user WHERE id = ANY($1) AND archived_at IS NULL AND removed_at IS NULL AND NOT is_agent`,
	targetProduct:             `SELECT id FROM product WHERE id = ANY($1) AND archived_at IS NULL`,
	targetTag:                 `SELECT id FROM tag WHERE id = ANY($1) AND archived_at IS NULL`,
	targetOfferTemplate:       `SELECT id FROM offer_template WHERE id = ANY($1) AND archived_at IS NULL`,
	targetWebhookSubscription: `SELECT id FROM webhook_subscription WHERE id = ANY($1) AND archived_at IS NULL`,
	"custom_field":            `SELECT id FROM custom_field WHERE id = ANY($1)`,
	// An import run is workspace-shared work over the estate, with no owner and
	// no row scope of its own — so the row half is existence alone, and the
	// authority is the object-read floor plus the decision grants. There is no
	// archived_at on the table: a run's history is its own status, and a
	// finished one is still the thing an approval named.
	targetImportRun: `SELECT id FROM import_run WHERE id = ANY($1)`,
}
