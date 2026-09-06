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
// `accessgrants`, not `capability`: the latter's other exports are hooks that
// reach `useMe` and through it the design system's stylesheets, so importing it
// pulls the component graph into anything that only wants to read this table.
// `e2e/ac.spec.ts` derives its sweep from SETTINGS_PAGES, and that chain took
// Playwright's transform into a CSS file — the whole suite failed to boot
// before a single test ran.
import {
  type AccessSnapshot,
  grants,
  type RbacObject,
} from "../app/accessgrants";

type RbacAction = components["schemas"]["RbacAction"];

/**
 * Which installation-level facts a page can depend on, beyond permissions.
 *
 * A surface can be absent for two unrelated reasons — this installation never
 * enabled it, or this reader holds no grant on it — and settings navigation has
 * to tell them apart: the first is not a destination at all, the second is a
 * destination that explains itself.
 */
export type SettingsAvailabilityKey = "company_context";

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
  | { kind: "all"; of: readonly CapabilityExpression[] };

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
export const anyOf = (
  ...of: readonly CapabilityExpression[]
): CapabilityExpression => ({ kind: "any", of });
export const allOf = (
  ...of: readonly CapabilityExpression[]
): CapabilityExpression => ({ kind: "all", of });
export const always: CapabilityExpression = { kind: "always" };

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

/**
 * Whose state a setting changes, in the reader's words.
 *
 * `workspace` is the internal enum's name for it and is deliberately not shown:
 * a person reading a settings page knows "Company", not the tenancy model.
 */
export type SettingsScope = "self" | "team" | "workspace" | "installation";

/**
 * Every settings page, its group, its scope and what it takes to open it.
 *
 * `requires` says who may SEE the page. It is not automatically the read grant:
 * a page whose subject is the installation's own configuration asks for the
 * verb that changes it, because every seeded role reads far more than it may
 * change and the read arm handed a rep six destinations she could only look at.
 * A page she genuinely consults — the sales vocabulary, her own settings —
 * still opens on the read.
 *
 * It never decides what a card may DO. Each card asks its own write question,
 * because a page is routinely readable and only partly writable, and a
 * page-level write flag would either hide a page somebody may read or promise
 * controls they cannot use. A card narrower than its page is the safe
 * direction: the card withholds itself.
 */
export const SETTINGS_PAGES = [
  { id: "account", group: "me", scope: "self", requires: always },
  { id: "voice", group: "me", scope: "self", requires: always },
  { id: "agents", group: "me", scope: "self", requires: always },
  { id: "connections", group: "me", scope: "self", requires: always },
  { id: "capture-activity", group: "me", scope: "self", requires: always },

  {
    id: "company",
    group: "company",
    scope: "installation",
    // The union of what the three cards on it ask for, and each arm is the verb
    // that card's controls perform.
    //
    // `installation_settings` is asked as the UPDATE, not the read. Every
    // seeded role reads it — a rep needs the base currency to render a deal —
    // so the read arm opened the installation's own facts to the whole
    // workspace. Only admin and ops may change them.
    //
    // The company profile is different and stays a write a rep really holds:
    // `organization:update` is hers, and the profile the AI reads is a thing
    // she legitimately edits. Its second condition is a deployment FLAG rather
    // than a permission, so the grant ANDs with it — the surface may simply not
    // exist on this installation.
    requires: anyOf(
      writes("installation_settings", ["update"]),
      allOf(writes("organization"), available("company_context")),
      reads("fx_rate"),
    ),
  },
  {
    id: "authentication",
    group: "company",
    scope: "installation",
    // `authentication_policy`, which SignInMethodsCard now reads through
    // `GET /installation/authentication-policy`.
    //
    // Not `installation_settings`: that grant is held by every seeded role — a
    // rep reads it for the base currency — so a page opening on it would put
    // the installation's sign-in policy in front of the whole workspace, which
    // is the disclosure the backend split existed to close.
    //
    // The two OAuth cards on this page still ride `capture_settings`, so they
    // are readable by a rep who cannot reach the page at all. That is the safe
    // direction — a card narrower than its page withholds itself — and it
    // closes when they move to `oauth_application`.
    requires: reads("authentication_policy"),
  },

  // `GET /users` answers 200 to any authenticated principal and the roster is
  // not an admin's private question — the handler decides what the answer
  // CONTAINS, and role keys are the privileged part. So the page opens for
  // everyone and its controls withhold themselves.
  { id: "members", group: "people", scope: "workspace", requires: always },
  { id: "teams", group: "people", scope: "workspace", requires: always },
  // `roles` is NOT here, and its absence is the point.
  //
  // The plan gives it a full page — role definitions, row scope, field masks,
  // extension grants, preview-as-role — and none of that is built. A catalog
  // entry would put a "Roles & permissions" row in front of every admin and ops
  // seat, because `role_admin:read` OPENS the page rather than closing it, and
  // the row would lead to a blank column.
  //
  // A destination becomes reachable when it is complete, not when its id is
  // decided. It joins the table in the change that builds it.
  {
    id: "seats",
    group: "people",
    scope: "installation",
    // Either grant, and they show different things: `LicenseCard` reads the
    // entitlement for a `license` holder and falls back to the capacity
    // endpoint for a `seat_usage` one — management sees how full the
    // installation is without seeing what it pays.
    requires: anyOf(reads("seat_usage"), reads("license")),
  },

  {
    id: "pipelines",
    group: "sales",
    scope: "workspace",
    requires: reads("pipeline"),
  },
  {
    id: "leads",
    group: "sales",
    scope: "workspace",
    // `custom_field`, not `pipeline`. The three cards here — lead sources,
    // disqualify reasons, handling — are stored as custom-field vocabulary and
    // the server gates their reads on that object. Asking for `pipeline` would
    // hide the page from a holder who may read it, and open it for one whose
    // reads then 403.
    requires: reads("custom_field"),
  },
  {
    id: "fields",
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
  },
  { id: "tags", group: "sales", scope: "workspace", requires: reads("tag") },
  {
    id: "products",
    group: "sales",
    scope: "workspace",
    requires: anyOf(reads("product"), reads("offer_template")),
  },

  {
    id: "capture",
    group: "data",
    scope: "workspace",
    requires: reads("capture_settings"),
  },
  {
    id: "integrations",
    group: "data",
    scope: "workspace",
    // The writes, not the reads: every seeded role reads both objects, because
    // "is capture working?" is everyone's question and the answer shows up on
    // the records they already open. Connecting an overlay or pointing a
    // webhook somewhere is admin and ops work.
    requires: anyOf(
      writes("overlay_connection"),
      writes("webhook_subscription"),
      // A composed workspace-scoped unit puts its settings on this page and
      // nowhere else, so the page has to open for it even when the reader holds
      // none of the grants above.
      composedUnits("workspace"),
    ),
  },
  {
    id: "knowledge",
    group: "data",
    scope: "workspace",
    requires: reads("knowledge_corpus"),
  },
  {
    id: "import",
    group: "data",
    scope: "workspace",
    requires: reads("import_run"),
  },

  {
    id: "models",
    group: "ai",
    scope: "workspace",
    // Two cards, two grants. The routing and provider-key cards read on
    // `ai_routing`; `AiHealthCard` reads on `ai_diagnostics` (ai/health.go),
    // and Models is the ONLY page that renders it. Management is seeded
    // diagnostics WITHOUT routing, so on the routing grant alone this page was
    // shut to the one role the health card was widened for.
    requires: anyOf(reads("ai_routing"), reads("ai_diagnostics")),
  },
  {
    id: "automations",
    group: "ai",
    scope: "workspace",
    // The write, which admin and ops alone hold. Management and manager read
    // `automation` — they see what ran, on the records it touched — but a role
    // that cannot change an automation has nothing to do on the page that
    // defines them.
    requires: writes("automation"),
  },
  {
    id: "usage",
    group: "ai",
    scope: "workspace",
    // The diagnostics read, or the price grant that authors the table beside it.
    // Both cards on this page ask `ai_diagnostics:read` now; `ai_model_rate`
    // stays in the union because its holder authors the rate sheet here.
    requires: anyOf(reads("ai_diagnostics"), reads("ai_model_rate")),
  },
  {
    id: "model-calls",
    group: "ai",
    scope: "workspace",
    requires: reads("ai_diagnostics"),
  },

  {
    id: "privacy",
    group: "governance",
    scope: "workspace",
    // `person` is deliberately NOT an arm, though the purposes card reads
    // through it. `person:read` is held by every seeded role, so that arm put
    // the governance page in front of the whole workspace — the retention
    // ladder, the subject-request queue and the restricted-record list, none of
    // which a rep can act on.
    //
    // The purposes LIST stays gated on `person` server-side and must not move:
    // that endpoint feeds the Person 360, and narrowing it would 403 every rep
    // on a screen they use all day. A card narrower than its page is the safe
    // direction — the card withholds itself.
    //
    // `consent_config` buys no read either (consent/store.go ListPurposes is on
    // `person`), so it is not an arm; its holders all hold retention or the
    // request queue anyway.
    requires: anyOf(
      reads("retention_policy"),
      reads("privacy_request"),
      // The consent vocabulary, as a PAIR. `person:read` is what the purposes
      // endpoint asks (consent/store.go ListPurposes) and every seeded role
      // holds it, so it cannot open the page alone. `consent_config` is who the
      // vocabulary belongs to — management is seeded its read and nothing else
      // on this page, so without this arm the one role deliberately granted the
      // vocabulary could not reach the only page that renders it.
      allOf(reads("person"), reads("consent_config")),
    ),
  },
  {
    id: "audit",
    group: "governance",
    scope: "workspace",
    // The trail's own object, which the card now asks for too. A role edited to
    // carry `audit_log:read` reaches it and one that lost it does not — which
    // the admin role name could not say either way.
    requires: reads("audit_log"),
  },
  {
    id: "system-health",
    group: "governance",
    scope: "installation",
    // Either card's grant. `JobHealthCard` asks `job_health:read` and the
    // reindex card asks its own, so a reader holding one finds that card and
    // the other withheld — which is the union being a union rather than one
    // object with a decorative term.
    requires: anyOf(reads("job_health"), reads("embedding_reindex")),
  },
  {
    id: "extensions",
    group: "governance",
    scope: "installation",
    // BOTH reads the card makes, because it makes them behind ONE flag: the
    // unit inventory from `GET /v1/extensions` and every role's grant on every
    // object from `GET /v1/roles`. On the inventory grant alone the page opens
    // and the card 403s on its second query, which is an unreadable page rather
    // than a narrower one.
    requires: allOf(reads("extension_access"), reads("role_admin")),
  },
  {
    id: "reset",
    group: "governance",
    scope: "installation",
    // The grant AND the deployment's consent. `system_reset:delete` says this
    // reader may wipe an installation that permits wiping; `data_reset_available`
    // says whether this one does — the compiled default is false everywhere, and
    // a deployment that never opted in has no such destination at all.
    //
    // Both, not either: a page offering a reset the server would refuse is worse
    // here than anywhere else in settings.
    requires: allOf(
      { kind: "grant", object: "system_reset", action: "delete" },
      flagged("data_reset_available"),
    ),
  },
] as const satisfies readonly {
  id: string;
  group: SettingsGroupId;
  scope: SettingsScope;
  requires: CapabilityExpression;
}[];

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
