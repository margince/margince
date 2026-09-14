// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The capability-expression builders the settings catalog's table is written
// in. Their own file so the table beside them stays readable as a table: the
// two grew past one screen together, and the shorthand is the half that never
// changes when a page is added.
//
// React-free by construction, like the catalog itself — a node test and a
// plain script both import this.

import type { RbacObject } from "../app/capability";
import type {
  CapabilityExpression,
  SettingsAvailabilityKey,
  UnitSecretScope,
} from "./settingscatalog";

/** Shorthand builders, so the table below reads as requirements rather than syntax. */
export const reads = (object: RbacObject): CapabilityExpression => ({
  kind: "grant",
  object,
  action: "read",
});
/**
 * A page whose subject is the installation's own configuration asks for the
 * verb that changes it, not the one that displays it.
 *
 * The seeded roles read far more than they may change — every seat reads
 * `installation_settings` for the base currency, `automation` to see what ran,
 * and the integration objects to see whether capture is working. Gating those
 * pages on the read put six administration destinations in a rep's navigation
 * that she could only look at. `writes` is how a page says its subject is
 * somebody's job rather than everybody's reference.
 *
 * Either write verb counts. A custom role holding `create` without `update` may
 * still add a webhook or an automation, and adding one is the whole reason to
 * open the page; the seeded roles hold both together, so a stricter spelling
 * would only ever strand a hand-built role — quietly, which is the bad way.
 *
 * Not a replacement for `reads`: a page a reader genuinely consults, like the
 * sales vocabulary she works in every day, still opens on the read.
 */
export const writes = (
  object: RbacObject,
  // Which write verbs the object's own endpoints actually offer. Defaults to
  // both, which is the common case; pass `["update"]` for an object that has no
  // create operation, so a custom role granted a verb the API does not expose
  // cannot open a page on it. `installation_settings` is the worked example:
  // `/installation/settings` is GET and PATCH, and nothing else.
  actions: readonly ("create" | "update")[] = ["update", "create"],
): CapabilityExpression => ({
  kind: "any",
  of: actions.map((action) => ({ kind: "grant", object, action })),
});
/**
 * The delete verb alone.
 *
 * Separate from `writes` on purpose: `writes` answers "may this reader author
 * something here", which is the question a page's `requires` asks, and delete is
 * not part of it — an archive verb on an object whose create the reader lacks
 * should not open a page for them. In a `changes` expression it belongs beside
 * `writes`, because archiving a tag is acting on the page as much as renaming
 * one is.
 */
/**
 * The full seat, as an expression.
 *
 * Only ever an arm of a page's `changes`. A `requires` must NOT fold it: a read
 * seat may open every page its grants open, and hiding one would be a
 * permission change rather than a statement about prominence.
 */
export const fullSeat: CapabilityExpression = { kind: "seat" };

export const destroys = (object: RbacObject): CapabilityExpression => ({
  kind: "grant",
  object,
  action: "delete",
});
export const available = (
  key: SettingsAvailabilityKey,
): CapabilityExpression => ({ kind: "availability", key });
export const flagged = (
  flag: "data_reset_available",
): CapabilityExpression => ({ kind: "flag", flag });
export const composedUnits = (
  scope: UnitSecretScope,
): CapabilityExpression => ({
  kind: "units",
  scope,
});

export const anyOf = (
  ...of: readonly CapabilityExpression[]
): CapabilityExpression => ({ kind: "any", of });
export const allOf = (
  ...of: readonly CapabilityExpression[]
): CapabilityExpression => ({ kind: "all", of });
/**
 * What a page's `changes` says when acting on it means issuing a mutating
 * request: the grants, AND the seat ceiling above them.
 *
 * This mirrors `useCanWrite`, which every mutating control in the tree already
 * uses — grant plus `useCanMutate`. Without the ceiling, a read-seat operator
 * keeping their create and update grants would be told a page is theirs to work
 * in, walk into it, and find every control closed. The backend refuses the same
 * request independently at `identity/admission.go`, so the rail would be
 * promising something two layers below it already deny.
 */
export const acts = (
  ...of: readonly CapabilityExpression[]
): CapabilityExpression => allOf(fullSeat, anyOf(...of));

export const always: CapabilityExpression = { kind: "always" };

/**
 * A page whose whole purpose is a read: the seat count, the AI usage figures,
 * the model calls, the audit trail. Consulting it IS the act, so `changes` is
 * whatever `requires` is.
 *
 * Only legitimate where `requires` is itself a read. A page whose requirement is
 * a mutation must NOT use this — `requires` deliberately carries no seat ceiling,
 * so the sentinel would tell a read seat that a page it cannot write is theirs to
 * work in. Automations and Reset were both written this way and both were wrong;
 * a test in settingscatalog.test.ts now fails that combination.
 *
 * A sentinel rather than the expression written twice. The two would drift —
 * somebody narrows the requirement, misses the copy, and the page silently
 * leaves the rail for a reader who may still open it. `settingsReach` resolves
 * it against the page's own `requires`, so there is one spelling per page.
 */
export const readingIsTheAct: CapabilityExpression = {
  kind: "reading-is-the-act",
};
