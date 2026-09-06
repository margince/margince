// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

/**
 * Reading a grant out of the access snapshot, with no React in the way.
 *
 * Split from `capability.ts` because that module's other exports are HOOKS: it
 * imports `useMe`, which reaches `screens/common`, which reaches the design
 * system and its stylesheets. Anything importing it therefore pulls the
 * component graph — fine inside the app, and fatal for a consumer that only
 * wants to read the vocabulary.
 *
 * `e2e/ac.spec.ts` is that consumer. It derives its settings sweep from
 * `SETTINGS_PAGES`, and the catalog reached this predicate through
 * `capability.ts` — so Playwright's transform followed the chain into a
 * stylesheet and the whole suite failed to boot before a single test ran.
 *
 * `capability.ts` re-exports what is here, so every call site inside the app is
 * unchanged and there is still ONE answer to "does this snapshot grant this".
 */

type CoreRbacObject = components["schemas"]["RbacObject"];

/**
 * An extension unit's own object, which the contract cannot enumerate: which
 * units an installation composed is not known until one is chosen. Only the
 * CORE half of the union below catches a typo at compile time — a deliberate
 * trade-off, and the reason `capability.ts` says so at length above its own
 * re-export of this type.
 */
export type ExtensionRbacObject = `ext_${string}`;

export type RbacObject = CoreRbacObject | ExtensionRbacObject;
export type RbacAction = components["schemas"]["RbacAction"];

/**
 * The access snapshot every predicate reads: `/me`, or nothing yet.
 *
 * Optional all the way down because each level is genuinely absent in a state
 * the app reaches — the query before it resolves, a server too old to send the
 * field, an object this principal holds no grant on.
 */
export type AccessSnapshot = components["schemas"]["MeResponse"] | undefined;

/**
 * Whether `snapshot` grants `action` on `object` — the same question `useCan`
 * answers, without the hook.
 *
 * The settings catalog evaluates a page's requirement outside a component:
 * navigation, the palette, search and the page itself must agree about which
 * settings exist, and the only way they cannot disagree is that one function
 * answers all four. A second copy of this chain is how the rail and the palette
 * came to disagree about the company page.
 *
 * Fails closed on every uncertainty. An unknown object resolves to the zero
 * grant on the server too, so a missing key is a denial rather than a gap to
 * fill in optimistically.
 */
export function grants(
  snapshot: AccessSnapshot,
  object: RbacObject,
  action: RbacAction,
): boolean {
  // Two optional steps, both load-bearing. `objects` is an index signature, so
  // TypeScript types a miss as PRESENT — only the chain makes an absent grant
  // deny at runtime; and the chain must cover `objects` itself, or a snapshot
  // that arrived without it would throw in render rather than deny.
  const grant = snapshot?.authorization?.objects?.[object];
  return grant?.[action] ?? false;
}
