import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Undoability = components["schemas"]["Undoability"];
export type UndoRefusal = NonNullable<Undoability["reason"]>;

// Why a change cannot be put back, in words.
//
// A greyed button that says nothing is the shape this feature exists to avoid,
// so every refusal the contract can name has a sentence — and it is the SAME
// sentence in both places a reader meets it: under a button that is refused up
// front, and after a press the server refused with the same code. A refusal
// discovered at press time and one shown up front must not read as two
// different products, which is why there is one map rather than one per
// surface.
//
// Keyed by the contract's own enum, and `historyundo.test.ts` holds the two
// sets equal in both directions: a reason added upstream with no sentence
// fails there rather than reaching a reader as an empty button.
const UNDO_REFUSALS: Readonly<Record<UndoRefusal, MessageKey>> = {
  no_before_image: "history.undo.noBeforeImage",
  not_a_replayable_verb: "history.undo.notReplayable",
  unsupported_record_type: "history.undo.unsupportedRecordType",
  superseded: "history.undo.superseded",
  behind_erasure_boundary: "history.undo.behindErasureBoundary",
  already_undone: "history.undo.alreadyUndone",
  not_restorable_by_this_path: "history.undo.notRestorableByThisPath",
  record_archived: "history.undo.recordArchived",
  null_unwritable_by_module: "history.undo.nullUnwritable",
  not_writable_by_caller: "history.undo.notWritableByCaller",
  edge_relink_unsupported: "history.undo.edgeRelinkUnsupported",
};

// The record moved under the caller. Not a refusal ABOUT the change — the same
// press may well succeed against what the record says now — so it is not in the
// map above and the copy tells the reader the history has been re-read.
export const VERSION_SKEW_CODE = "version_skew";

// The same map seen as a plain lookup, for the codes that reach this build as
// strings — a `409`'s `code`, which the contract types as free text. Widening
// an assignment costs nothing and keeps the map above exhaustive over the
// enum, where a cast at the call site would give up both.
const REFUSALS_BY_NAME: Readonly<Record<string, MessageKey | undefined>> =
  UNDO_REFUSALS;

// The sentence for one refusal, or undefined for a code this build has no word
// for. Undefined rather than a placeholder: a server naming a reason this
// frontend predates must fall back to the server's own detail, not to a
// sentence invented here about a case nobody has described.
export function undoRefusalKey(
  reason: string | null | undefined,
): MessageKey | undefined {
  return reason ? REFUSALS_BY_NAME[reason] : undefined;
}

// Every refusal this build has words for — read by the census that holds the
// set against the contract.
export function undoRefusalsNamed(): string[] {
  return Object.keys(UNDO_REFUSALS);
}
