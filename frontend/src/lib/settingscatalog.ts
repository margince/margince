// The settings catalog: which pages exist, where each sits, and what a reader
// must hold to open it.
//
// PURE DATA AND PURE FUNCTIONS. No React, no lucide, no imports that reach a
// card. That is what lets the four surfaces which must agree — the rail, the
// command palette, search, and the page itself — resolve from one table instead
// of four opinions. They have disagreed before: the rail and the palette each
// decided whether the company page existed, and answered differently, because
// the check was spelled twice.
//
// The requirement is an EXPRESSION rather than a predicate so it can be
// evaluated against a snapshot outside React, and so a reader can see what a
// page asks for without running it.

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
// `accessgrants`, not `capability`: the latter's hooks pull the component graph
// and its CSS into `e2e/ac.spec.ts`, whose Playwright transform cannot load it.
import { type AccessSnapshot, grants, type RbacObject } from "./accessgrants";
import { BUSINESS_PAGES } from "./settingscatalog.business";
import { PLATFORM_PAGES } from "./settingscatalog.platform";

type RbacAction = components["schemas"]["RbacAction"];

/**
 * Which installation-level facts a page can depend on, beyond permissions.
 *
 * A surface can be absent for two unrelated reasons — this installation never
 * enabled it, or this reader holds no grant on it — and settings navigation has
 * to tell them apart: the first is not a destination at all, the second is a
 * destination that explains itself.
 */
export type SettingsAvailabilityKey = "embedding_reindex";

/**
 * What a page requires, as a value.
 *
 * `role` is deliberately absent. Every arm here is a grant, a deployment fact,
 * or a combination — a role arm would encode the seeded matrix into the client,
 * which is exactly the inference that breaks on any workspace whose stored
 * grants have drifted.
 */
export type CapabilityExpression =
  | { kind: "always" }
  | { kind: "grant"; object: RbacObject; action: RbacAction }
  | { kind: "availability"; key: SettingsAvailabilityKey }
  // A top-level /me flag that is not a permission and not a settings-availability
  // key. `data_reset_available` is the deployment's own consent to a wipe, and
  // it rides /me at the root because it predates that object.
  | { kind: "flag"; flag: "data_reset_available" }
  // Whether this BUILD composed any extension unit keeping secrets at `scope`.
  // A composition fact, not a permission and not a deployment flag: the unit's
  // settings live on the page below and there is no other route to them, so a
  // page that ignored this would strand an installation's only way in.
  | { kind: "units"; scope: UnitSecretScope }
  | { kind: "any"; of: readonly CapabilityExpression[] }
  | { kind: "all"; of: readonly CapabilityExpression[] }
  // Only ever a page's `changes`, and only `settingsReach` resolves it — it
  // means "the same as this page's `requires`", which no expression can say
  // about itself.
  | { kind: "reading-is-the-act" }
  // The full-seat ceiling, which sits ABOVE the object grants: a read seat keeps
  // every grant it holds and may still not issue a mutating request. Its own arm
  // rather than folded into `writes`, because some GETs are gated on a write
  // verb and a read seat may genuinely see those.
  | { kind: "seat" };

/**
 * Resolve one requirement against an access snapshot.
 *
 * Fails closed throughout: an unresolved `/me` denies every grant arm, and an
 * absent availability field reads as "this installation does not have that
 * surface" rather than as permission.
 *
 * `any` over an empty list is false and `all` over an empty list is true, which
 * are the identities that make the two compose — but no entry in the table
 * relies on either, because an empty requirement is almost always a mistake
 * rather than a statement.
 */
export function holds(
  expression: CapabilityExpression,
  snapshot: AccessSnapshot,
  context: CatalogContext = {},
): boolean {
  switch (expression.kind) {
    case "always":
      return true;
    case "grant":
      return grants(snapshot, expression.object, expression.action);
    case "availability":
      return snapshot?.settings_availability?.[expression.key] ?? false;
    case "flag":
      return snapshot?.[expression.flag] ?? false;
    case "units":
      // Not from the snapshot: which units this binary composed is fixed at
      // build time and identical for every reader. Absent context means none,
      // which fails closed the same way every other arm does.
      return (context.composedUnitScopes ?? []).includes(expression.scope);
    case "any":
      return expression.of.some((each) => holds(each, snapshot, context));
    case "all":
      return expression.of.every((each) => holds(each, snapshot, context));
    case "seat":
      // Fails closed on an unresolved snapshot like every other arm: absent
      // authorization is not a full seat.
      return snapshot?.authorization?.seat_type === "full";
    case "reading-is-the-act":
      // Unresolvable here by construction: it names the page's own `requires`,
      // and `holds` is handed an expression with no page attached. Reaching
      // this arm means a caller evaluated a `changes` field directly instead of
      // going through `settingsReach`, so it fails closed rather than guessing.
      return false;
  }
}

/**
 * The seven groups, in the order they are shown.
 *
 * They replace the two-audience split ("you" and "admin"), which encoded who a
 * page was FOR rather than what it was ABOUT — and so hid every installation
 * page behind an operator-seat check, whatever grants the reader actually held.
 */
export const SETTINGS_GROUPS = [
  "me",
  "company",
  "people",
  "sales",
  "data",
  "ai",
  "governance",
] as const;
export type SettingsGroupId = (typeof SETTINGS_GROUPS)[number];

/** The words naming a settings group, wherever a group is shown. */
export const groupHeadingKey = (group: SettingsGroupId): MessageKey =>
  `settings.group.${group}`;

/**
 * Whose state a setting changes, in the reader's words.
 *
 * `workspace` is the internal enum's name for it and is deliberately not shown:
 * a reader reading a settings page knows "Company", not the tenancy model.
 */
export type SettingsScope =
  | "self"
  | "team"
  | "workspace"
  | "installation"
  // A page whose surfaces do not agree. Capture activity is the case: the
  // reader's own trace sits beside exclusion rules that keep a correspondent
  // out of the CRM for everybody.
  //
  // Its own value rather than picking the wider of the two, because "Company"
  // over a page that is mostly personal is as wrong as "Only you" over a
  // page carrying a company switch. The head says the page is mixed and stops
  // claiming to answer for every card on it.
  | "mixed";

/**
 * Where a composed unit keeps its secrets. Mirrored rather than imported: the
 * extensions registry reaches `@composition/screens` and so pulls React and a
 * build alias into whatever imports it, which is the one thing this module may
 * not do — being importable from anywhere is its whole purpose.
 *
 * `settingscatalog.units.test.ts` holds the two spellings equal.
 */
export type UnitSecretScope = "workspace" | "user";
/**
 * How the caller answers the composition question.
 *
 * Injected rather than read, for the import reason above. `holds` takes it as
 * an option so the pure table stays evaluable in a test, a story or a script
 * with no registry at all — and the one caller that has a registry passes it.
 */
export type CatalogContext = {
  composedUnitScopes?: readonly UnitSecretScope[];
};

// The shorthand the page tables are written in. Re-exported because a dozen
// callers and two tests import these from the catalog.
export {
  acts,
  allOf,
  always,
  anyOf,
  available,
  composedUnits,
  destroys,
  flagged,
  fullSeat,
  readingIsTheAct,
  reads,
  writes,
} from "./settingscapability";

/**
 * Every settings page, its group, its scope and what it takes to open it.
 *
 * `requires` says who may see the page. It is not always the read grant. A
 * page whose subject is the installation's own configuration asks for the verb
 * that changes it. Every seeded role reads far more than it may change, and
 * the read arm would hand a rep destinations they could only look at. A page
 * the rep consults, such as the sales vocabulary or their own settings, still
 * opens on the read.
 *
 * It never decides what a card may do. Each card asks its own write question,
 * because a page is often readable and only partly writable. A page-level
 * write flag would either hide a page somebody may read or promise controls
 * they cannot use. A card narrower than its page is the safe direction: the
 * card withholds itself.
 *
 * `changes` is about prominence, not permission: can the reader act on this
 * page, or only consult it? The rail lists what they can act on. Everything
 * else they may open stays reachable from the settings home and from search.
 *
 * Its value is read off the write verbs the page's own cards ask, never off
 * the heading the page sits under. On a page whose purpose is a read (the
 * spend, the model calls, the audit trail, the seat count), reading is the
 * action. So `changes` is the same expression as `requires`.
 */
export type SettingsPageRow = {
  id: string;
  group: SettingsGroupId;
  scope: SettingsScope;
  requires: CapabilityExpression;
  changes: CapabilityExpression;
};

export const SETTINGS_PAGES = [
  ...BUSINESS_PAGES,
  ...PLATFORM_PAGES,
] as const satisfies readonly SettingsPageRow[];

export type SettingsPageId = (typeof SETTINGS_PAGES)[number]["id"];

/** One row of the table, for callers that carry a page around rather than an id. */
export type SettingsPage = (typeof SETTINGS_PAGES)[number];

/** The pages this snapshot may open, in declaration order. */
export function visibleSettingsPages(
  snapshot: AccessSnapshot,
  context: CatalogContext = {},
): readonly SettingsPage[] {
  return SETTINGS_PAGES.filter((page) =>
    holds(page.requires, snapshot, context),
  );
}

/**
 * What this reader can act on, and what they can only consult.
 *
 * The two halves partition `visibleSettingsPages` — every page a reader may
 * open is in exactly one of them, and neither is a permission: a page in
 * `looksUp` opens normally, answers its address and appears in search. The
 * split decides prominence: the rail carries `acts`, and the settings home
 * lists both, with a lock on each page in `looksUp`.
 *
 * Splitting here keeps the rail, the home, the read-only banner and the cards
 * that defer to it agreeing on whether this contact can act.
 */
export type SettingsReach = {
  /** Pages with at least one control this reader may use. */
  readonly acts: readonly SettingsPage[];
  /** Pages they may read and cannot change. */
  readonly looksUp: readonly SettingsPage[];
};

export function settingsReach(
  snapshot: AccessSnapshot,
  context: CatalogContext = {},
): SettingsReach {
  const acts: SettingsPage[] = [];
  const looksUp: SettingsPage[] = [];
  for (const page of visibleSettingsPages(snapshot, context)) {
    // `reading-is-the-act` names the page's own requirement, which is the one
    // thing the expression cannot carry: resolve it against `requires` here,
    // where the page is in hand. Every other kind goes to `holds` unchanged.
    const expression =
      page.changes.kind === "reading-is-the-act" ? page.requires : page.changes;
    (holds(expression, snapshot, context) ? acts : looksUp).push(page);
  }
  return { acts, looksUp };
}

export const readsOnly = (reach: SettingsReach, page: SettingsPageId) =>
  reach.looksUp.some((each) => each.id === page);
