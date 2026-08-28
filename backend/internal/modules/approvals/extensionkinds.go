// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The one place this module learns about a kind it did not write.
//
// Every other stageable kind is a constant here, and deliberately: a kind is
// three separate statements — the grants deciding it needs, the rule that says
// whether the staged target is one the asking human may see, and the query that
// proves the target still exists — and a kind carrying fewer than three is
// half-governed with nothing saying so. A staged row missing the visibility
// rule is invisible in the inbox AND undecidable at the decision: an authority
// object nobody can release or reject.
//
// An extension's verbs cannot be constants. A unit names its own tool, its own
// table and its own RBAC object, and this module may not import the layer that
// composes units. So the composition root REGISTERS them at boot, and the
// registration carries all three statements at once — there is no shape here
// that lets a caller supply two of them.
//
// WHAT A UNIT'S KIND IS ALLOWED TO BE, and why each bound is here rather than
// trusted to the caller:
//
//   - it may not take a core kind's name, or a unit would silently re-govern a
//     verb this module decides;
//   - it may not take a core target type, or its existence probe would answer
//     against another store's table;
//   - its table must be a namespaced identifier, because that name is
//     interpolated into the probe. pkg/extension checks this at declaration,
//     and a check that lived only there would be one this module never
//     re-applied to a value it is about to put in a statement.
//
// The registration happens once, before anything is served, and the mutex
// guards the write against the reads rather than against a second write.

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExtensionKind is one confirm-first extension verb, with everything deciding
// it requires. Every field is required; a zero one is refused rather than
// defaulted, because each default would be a governance decision made by
// omission.
type ExtensionKind struct {
	// Verb is the tool name, which IS the staged kind — the same identity core
	// verbs are keyed by, and what a redemption matches against.
	Verb string
	// TargetTable is the unit table the staged row lives in. It is the staged
	// target's TYPE as well as the table the existence probe reads: one name,
	// so a staged row can never be classified against a table it is not in.
	TargetTable string
	// RbacObject and RbacAction are the grant the operation itself gates on,
	// and therefore the grant deciding it requires. Deciding takes the grant
	// PERFORMING it takes — anything less puts the confirm-first control point
	// with someone who could not do the thing they are releasing.
	RbacObject string
	RbacAction principal.Action
}

// extensionTableGrammar is the identifier shape a registered table must have.
// Restated from pkg/extension rather than imported: this is the last check
// before the name reaches a statement, and a check performed only by the
// producer is one the consumer never made.
var extensionTableGrammar = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*$`)

var extensionKinds struct {
	mu    sync.RWMutex
	byRaw map[string]ExtensionKind
}

// RegisterExtensionKinds records the confirm-first verbs a composed extension
// set serves. Called once by the composition root at boot; a second call
// REPLACES the set rather than adding to it, so a process that rebuilds its
// composition cannot accumulate kinds no unit serves any more.
//
// It validates before it stores: a boot that would install a half-governed
// kind fails loudly here, which is the same fail-closed posture the rest of
// the extension boot takes.
func RegisterExtensionKinds(kinds []ExtensionKind) error {
	registered := make(map[string]ExtensionKind, len(kinds))
	tables := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		// The COLLISIONS first, then the shape. A unit that named `person` gets
		// told it collided with a core target rather than that its identifier
		// is not namespaced — the first sentence is the one that explains what
		// went wrong, and it is also the only order in which the collision
		// check is reachable, since the namespace grammar below would refuse
		// every core name before it was asked about.
		if _, core := decisionGrants[kind.Verb]; core {
			return fmt.Errorf("crmapprovals: extension verb %q is already a core staged kind — a unit may not "+
				"re-govern a verb this module decides", kind.Verb)
		}
		if _, taken := registered[kind.Verb]; taken {
			return fmt.Errorf("crmapprovals: extension verb %q registered twice", kind.Verb)
		}
		if _, core := targetProbes[kind.TargetTable]; core {
			return fmt.Errorf("crmapprovals: extension verb %q stages against %q, which is a core target type — "+
				"its existence probe would read another store's table", kind.Verb, kind.TargetTable)
		}
		if err := kind.validate(); err != nil {
			return err
		}
		if owner, shared := tables[kind.TargetTable]; shared && owner != kind.Verb {
			// Two verbs over one table is legitimate — a unit may confirm both
			// an edit and a removal of the same row — so this is only a guard
			// against a table registered with two different grants, which would
			// make "who may decide this" depend on which verb staged it.
			if registered[owner].RbacObject != kind.RbacObject || registered[owner].RbacAction != kind.RbacAction {
				return fmt.Errorf("crmapprovals: %q and %q both stage against %q but require different grants — "+
					"a staged row's decider must not depend on which verb parked it", owner, kind.Verb, kind.TargetTable)
			}
		}
		tables[kind.TargetTable] = kind.Verb
		registered[kind.Verb] = kind
	}
	extensionKinds.mu.Lock()
	defer extensionKinds.mu.Unlock()
	extensionKinds.byRaw = registered
	return nil
}

func (k ExtensionKind) validate() error {
	if k.Verb == "" {
		return fmt.Errorf("crmapprovals: an extension kind with no verb names nothing a staged row could be")
	}
	if !extensionTableGrammar.MatchString(k.TargetTable) {
		return fmt.Errorf("crmapprovals: extension verb %q stages against %q, which is not a namespaced table "+
			"identifier — this name is read into the probe that proves the staged row exists", k.Verb, k.TargetTable)
	}
	if k.RbacObject == "" || k.RbacAction == "" {
		return fmt.Errorf("crmapprovals: extension verb %q registers no RBAC object and action — deciding a "+
			"staged call requires the grant performing it requires, and there would be nothing to require",
			k.Verb)
	}
	return nil
}

// extensionKind answers the registered kind for one verb.
func extensionKind(verb string) (ExtensionKind, bool) {
	extensionKinds.mu.RLock()
	defer extensionKinds.mu.RUnlock()
	kind, ok := extensionKinds.byRaw[verb]
	return kind, ok
}

// extensionTarget answers the registered kind that stages against one target
// type, which is how the visibility half is reached: the probe is asked about
// a TYPE, and a type is a unit's table.
func extensionTarget(targetType string) (ExtensionKind, bool) {
	extensionKinds.mu.RLock()
	defer extensionKinds.mu.RUnlock()
	for _, kind := range extensionKinds.byRaw {
		if kind.TargetTable == targetType {
			return kind, true
		}
	}
	return ExtensionKind{}, false
}

// extensionTargetTypes are the registered tables, sorted — the extension half
// of ClassifiedTargetTypes, so the composition layer's parity gate sees the
// whole vocabulary rather than the static part of it.
func extensionTargetTypes() []string {
	extensionKinds.mu.RLock()
	defer extensionKinds.mu.RUnlock()
	seen := map[string]bool{}
	var types []string
	for _, kind := range extensionKinds.byRaw {
		if seen[kind.TargetTable] {
			continue
		}
		seen[kind.TargetTable] = true
		types = append(types, kind.TargetTable)
	}
	sort.Strings(types)
	return types
}

// extensionSchema is where every unit's tables live. The ext schema is shared
// by all of them and the unit namespace in the table NAME is what keeps two
// units from addressing each other's rows (backend/migrations/core/0213).
//
// Spelled here rather than assumed on a search_path, because this repository's
// statements resolve names explicitly — a probe that relied on the search path
// would read a core table of the same name the day one existed.
const extensionSchema = "ext"

// extensionExistenceQuery is the probe for a registered table: existence
// alone, which is the same floor every other workspace-shared target takes.
//
// No archived_at predicate, and that is a decision rather than an omission:
// this surface cannot know whether a unit's table has such a column, and
// demanding one would refuse a table with no archive concept at all. What the
// probe owes is that the staged row still EXISTS — a staging against a deleted
// row is not decidable — and the unit's own handler is what refuses a row it
// considers retired, exactly as it does outside an approval.
//
// The identifier is sanitized here as well as grammar-checked at registration:
// two independent reasons the name cannot carry a statement, because this is
// the line that writes one.
func extensionExistenceQuery(table string) string {
	return `SELECT EXISTS (SELECT 1 FROM ` + pgx.Identifier{extensionSchema, table}.Sanitize() + ` WHERE id = $1)`
}

// ExtensionTargetExists answers whether a registered unit row is still there.
//
// Exported for the composition layer, which asks the same question one frame
// earlier: it refuses to STAGE against a row that is not there, because an
// approval nobody can find is one nobody can release or reject. Both askers
// get the same query rather than each writing one — the second copy is how a
// staging check and the inbox that reads it come to disagree about what "still
// exists" means, and the second copy would also be a second place a table name
// reaches a statement.
func ExtensionTargetExists(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	if !extensionTableGrammar.MatchString(table) {
		return false, fmt.Errorf("crmapprovals: %q is not a namespaced extension table", table)
	}
	var exists bool
	if err := tx.QueryRow(ctx, extensionExistenceQuery(table), id).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
