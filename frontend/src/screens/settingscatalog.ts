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

/**
 * Whose state a setting changes, in the reader's words.
 *
 * `workspace` is the internal enum's name for it and is deliberately not shown:
 * a person reading a settings page knows "Company", not the tenancy model.
 */
export type SettingsScope =
  | "self"
  | "team"
  | "workspace"
  | "installation"
  // A page whose surfaces do not agree. My connections is the case: most of its
  // cards are the reader's own mailboxes and network, and one is not —
  // ConnectorsCard's second panel is the workspace's Telegram bot, connected
  // once for everybody, sitting beside the mailboxes each reader connects for
  // themselves.
  //
  // Its own value rather than picking the wider of the two, because "Company"
  // over a page that is mostly personal is as wrong as "Only you" over a
  // page carrying a company switch. The head says the page is mixed and stops
  // claiming to answer for every card on it.
  | "mixed";

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
 *
 * `changes` is the second question, and it is about PROMINENCE rather than
 * permission: is this page one the reader can act on, or one they can only
 * consult? The rail lists what they can act on; everything else they may open
 * stays reachable from the settings home and from search. Both halves matter —
 * a reader who cannot change the pipeline vocabulary should not have it in the
 * furniture they navigate every day, and must still be able to look it up.
 *
 * Its value is read off the write verbs the page's own cards ask, never off the
 * heading the page sits under. On a page whose entire purpose IS a read — the
 * spend, the model calls, the audit trail, the seat count — reading is the
 * action, so `changes` is the same expression as `requires` and says so.
 */
export const SETTINGS_PAGES = [
  {
    id: "account",
    group: "me",
    scope: "self",
    requires: always,
    // Yours to change, all of it: identity, password, signature, language.
    changes: always,
  },
  {
    id: "voice",
    group: "me",
    scope: "self",
    requires: always,
    // The profile is the reader's own, but writing one is still a grant:
    // voice-dna.tsx asks `voice_profile` create and update, and a read-only seat
    // holds the read alone. `always` here would have put Voice in that reader's
    // rail with nothing on it they could touch.
    changes: acts(writes("voice_profile")),
  },
  {
    id: "agents",
    group: "me",
    scope: "self",
    requires: always,
    // Passports, connected agents and the autonomy choice are all this reader's.
    changes: always,
  },
  // MIXED, not self: most of its cards are the reader's own, and ConnectorsCard
  // carries the workspace's Telegram bot beside them. A page-level "Only you"
  // over that panel would tell a reader a shared connection is private to them.
  //
  // The mail-sharing SWITCH left this page for Capture rules, where the rest of
  // the installation's capture posture lives. What stays is a value-only row
  // saying what the rule currently is, which claims nothing about who may
  // change it.
  {
    id: "connections",
    group: "me",
    scope: "mixed",
    requires: always,
    // Acting, not consulting: the mailbox, sender and LinkedIn controls are the
    // reader's own and need no grant, and the mail-sharing row states a value
    // and offers no control at all.
    //
    // The Telegram panel is the exception and it is NOT gated — connectors.tsx
    // asks no capability question, so its Connect, Edit and Disconnect are
    // offered to every reader and refused by the server (`channel_connection`
    // is admin/ops to mutate). That is a pre-existing gap in that card rather
    // than something this field can fix: `changes` decides which pages reach
    // the rail, and it cannot withhold one control on a page whose other nine
    // surfaces are genuinely the reader's.
    changes: always,
  },
  // MIXED for the same reason as `connections`, one page along:
  // CaptureExclusionsCard carries BOTH scopes by design — its own comment says
  // so — and its workspace rules keep a correspondent out of the CRM for
  // everybody. The workspace activity view behind `capture_trace:read` is the
  // installation's too.
  {
    id: "capture-activity",
    group: "me",
    scope: "mixed",
    requires: always,
    // The reader's own capture trace and their own exclusions. The workspace
    // half of CaptureExclusionsCard asks `capture_settings:update` itself.
    changes: always,
  },

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
    // The three cards, by the verb each performs. InstallationSettingsCard and
    // FxRatesCard both write; CompanyContextCard asks `useCanUpsert("organization")`,
    // which is create-or-update plus the seat — spelled here as `writes`.
    changes: acts(
      writes("installation_settings", ["update"]),
      allOf(writes("organization"), available("company_context")),
      writes("fx_rate"),
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
    // SignInMethodsCard writes `installation_settings:update`. The OAuth cards
    // ask `capture_settings:update`, which is a wider audience than this page's
    // own read, so it is not an arm: a reader who can only reach the OAuth half
    // still consults the page rather than owning it.
    // SignInMethodsCard writes `installation_settings:update`; the two OAuth
    // application cards beside it save and remove through
    // `capture_settings:update`, which is a different grant and a wider
    // audience. Both are on this page, so either makes it the reader's to work
    // in — a custom role holding only the OAuth half has working controls.
    changes: acts(
      writes("installation_settings", ["update"]),
      writes("capture_settings", ["update"]),
    ),
  },

  // `GET /users` answers 200 to any authenticated principal, and that is right:
  // the share and assignee pickers every seat uses read this roster. But a
  // DIRECTORY is not an administration page. A reader who may not invite, change
  // a role or switch a seat off has nothing to do here — they look a colleague
  // up in the app, where the answer is already beside the record.
  //
  // So the settings ENTRY follows the verbs, and the safe roster stays open
  // underneath it: `user_admin` gates Members, `team_admin` gates Teams, and
  // both pages' cards still withhold their own controls verb by verb.
  {
    id: "members",
    group: "people",
    scope: "workspace",
    // The READ, and only the read. Every write verb needs what it carries, so a
    // holder without it has no usable page:
    //
    //   - `include_inactive` is honoured only for a caller who passes the read
    //     (handlers_roster.go), so a `delete` holder without it never sees a
    //     deactivated member to reactivate.
    //   - the roster omits `roles` without it, and `ChangeUserRole` REPLACES
    //     the whole set — so an `update` holder would pick a role against a
    //     list they cannot see.
    //
    // Opening the page on a write alone would offer exactly those two broken
    // affordances. The card ANDs the same way, so the page and its controls
    // agree.
    requires: reads("user_admin"),
    // Invite, role change and deactivate — the three verbs UsersAdminCard offers,
    // and `delete` is deactivate rather than a row removal.
    changes: acts(writes("user_admin"), destroys("user_admin")),
  },
  {
    id: "teams",
    group: "people",
    scope: "workspace",
    // The team verbs, or the roster read that carries `team_ids` — a holder of
    // `user_admin:read` alone sees who is in which team, which is the page's
    // whole content even when they may change none of it.
    requires: anyOf(writes("team_admin"), reads("user_admin")),
    // TeamsCard creates and renames teams. Membership rides `user_admin:read`,
    // which is a read, so it does not make this page the reader's to change.
    changes: acts(writes("team_admin")),
  },
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
    // LicenseCard has no write of any kind. Reading the seat count IS the action
    // here, so the two questions have one answer.
    changes: readingIsTheAct,
  },

  {
    id: "pipelines",
    group: "sales",
    scope: "workspace",
    requires: reads("pipeline"),
    // PipelinesCard creates, renames and removes stages.
    changes: acts(writes("pipeline"), destroys("pipeline")),
  },
  {
    id: "stageautomation",
    group: "sales",
    scope: "workspace",
    // The report reads stage_progression_outcome through a pipeline-gated
    // endpoint, so `pipeline` read is exactly what opens it — the same grant
    // that shows the stages the transitions are between.
    requires: reads("pipeline"),
    // The page now carries the per-transition switches beneath its report, so
    // it WRITES. The table above them stays read-only, which is a layout
    // decision rather than a permission one.
    //
    // UPDATE only. A rule is created by the same PUT that edits one, and this
    // page never creates a pipeline — so a custom role holding pipeline:create
    // without update would open a page whose every switch is refused.
    changes: acts(writes("pipeline", ["update"])),
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
    // The three lead-vocabulary cards all write `custom_field`, delete included.
    changes: acts(writes("custom_field"), destroys("custom_field")),
  },
  {
    id: "fields",
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    // customfields.tsx creates and edits; it offers no delete.
    changes: acts(writes("custom_field")),
  },
  {
    id: "tags",
    group: "sales",
    scope: "workspace",
    requires: reads("tag"),
    // tagadmin.tsx separates create, rename/merge and archive. Applying a tag TO
    // a record is a different question and lives on the record, not here.
    changes: acts(writes("tag"), destroys("tag")),
  },
  {
    id: "products",
    group: "sales",
    scope: "workspace",
    requires: anyOf(reads("product"), reads("offer_template")),
    // Two cards, two objects: products.tsx and offertemplates.tsx, each with its
    // own create/update/archive trio. Either one makes the page actionable.
    changes: acts(
      writes("product"),
      destroys("product"),
      writes("offer_template"),
      destroys("offer_template"),
    ),
  },

  {
    id: "capture",
    group: "data",
    scope: "workspace",
    requires: reads("capture_settings"),
    // Five cards. Four write `capture_settings:update` — the sharing rule, the
    // posture, the own-domain list and the consumer-mailbox list; the fifth,
    // BlockedDomainsCard, writes `organization:update`, which every seeded sales
    // role holds — so a rep keeps this page in the rail.
    changes: acts(
      writes("capture_settings", ["update"]),
      writes("organization", ["update"]),
    ),
  },
  {
    id: "integrations",
    group: "data",
    // Mixed, and the provider card is why: `PATCH /integrations/settings` is
    // "the installation's provider-lookup posture" in its own contract summary,
    // while webhooks, the overlay mapping and the workspace extension units
    // stay inside one workspace. One badge cannot name both.
    scope: "mixed",
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
    // Provider, webhooks and the overlay pair, each with its own object.
    // Every verb the four cards offer, delete included: WebhooksCard archives a
    // subscription and OverlayCard disconnects a mirror, and both are acting on
    // the page as much as creating one is.
    //
    // The composed-unit arm is not a grant and carries no seat: ExtensionUnitsCard
    // renders an Open link for every composed workspace unit unconditionally, so
    // on a build that composed one this page has a working control for anybody
    // who can see it. That arm is what makes the page visible in the first place.
    changes: anyOf(
      acts(
        writes("integrations"),
        destroys("integrations"),
        writes("webhook_subscription"),
        destroys("webhook_subscription"),
        writes("overlay_connection"),
        destroys("overlay_connection"),
      ),
      composedUnits("workspace"),
    ),
  },
  {
    id: "knowledge",
    group: "data",
    scope: "workspace",
    requires: reads("knowledge_corpus"),
    // KnowledgeCard adds a corpus; it asks the create alone.
    changes: acts(writes("knowledge_corpus", ["create"])),
  },
  {
    id: "import",
    group: "data",
    scope: "workspace",
    requires: reads("import_run"),
    // ImportCard starts a run and advances it — create and update, two verbs the
    // card asks separately.
    // BOTH verbs, because ImportCard asks for both: `mayImport` is
    // `mayCreate && mayAdvance` — the dry run parks the run and the approval
    // moves it, so create alone reaches the card and is refused at the first
    // button.
    changes: acts(
      allOf(writes("import_run", ["create"]), writes("import_run", ["update"])),
    ),
  },

  {
    id: "models",
    group: "ai",
    // Installation, not workspace: `PUT /ai/routing` re-points which vendor
    // processes the installation's text, and `/ai/provider-keys` writes the
    // installation key vault. Both contract summaries say "installation".
    scope: "installation",
    // Two cards, two grants. The routing and provider-key cards read on
    // `ai_routing`; `AiHealthCard` reads on `ai_diagnostics` (ai/health.go),
    // and Models is the ONLY page that renders it. Management is seeded
    // diagnostics WITHOUT routing, so on the routing grant alone this page was
    // shut to the one role the health card was widened for.
    requires: anyOf(reads("ai_routing"), reads("ai_diagnostics")),
    // The routing binding and the provider keys, both on `ai_routing:update`.
    // AiHealthCard is a read and does not widen this.
    changes: acts(writes("ai_routing", ["update"])),
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
    // Not the sentinel: this page's `requires` is already a write, so resolving
    // `changes` to it would inherit a requirement that deliberately carries no
    // seat ceiling — a read seat holding the automation grants would be told the
    // page is theirs to work in. The card offers delete as well.
    changes: acts(writes("automation"), destroys("automation")),
  },
  {
    id: "usage",
    group: "ai",
    scope: "workspace",
    // The diagnostics read, or the price grant that authors the table beside it.
    // Both cards on this page ask `ai_diagnostics:read` now; `ai_model_rate`
    // stays in the union because its holder authors the rate sheet here.
    requires: anyOf(reads("ai_diagnostics"), reads("ai_model_rate")),
    // ModelCostsCard writes `ai_model_rate` through `useCanUpsert`. The spend and
    // usage cards beside it are reads, so a reader without that grant consults
    // this page rather than owning it.
    changes: acts(writes("ai_model_rate")),
  },
  {
    id: "model-calls",
    group: "ai",
    scope: "workspace",
    requires: reads("ai_diagnostics"),
    // AiCallsCard is a read of what the models were asked. Reading it is the act.
    changes: readingIsTheAct,
  },

  {
    id: "privacy",
    group: "governance",
    // Installation: retention is installation-wide on both endpoints the card
    // writes — `/retention/settings` is "the installation's retention posture"
    // and `/retention-policies` lists "the installation's retention policies".
    // What this page destroys, it destroys everywhere.
    scope: "installation",
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
    // Four cards, four objects. `person:update` is what PrivacyInboxCard asks to
    // open a subject request — the request is about a person's record, so the
    // grant is the person's, not the queue's.
    changes: acts(
      writes("consent_config", ["create"]),
      writes("retention_policy"),
      destroys("retention_policy"),
      writes("privacy_request", ["update"]),
      writes("person", ["update"]),
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
    // The trail is a read by construction — nothing writes it from here, and
    // reading it is precisely the operator's act.
    changes: readingIsTheAct,
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
    // EmbedReindexCard spends tokens to rebuild the embed store. Watching the
    // queue beside it is a read, and watching a stalled queue is an operator
    // acting, so the job-health read is the second arm.
    // The reindex is a mutation and takes the seat; watching the queue beside it
    // is a read, and watching a stalled queue is an operator acting — so that arm
    // stands outside the ceiling.
    changes: anyOf(
      acts(writes("embedding_reindex", ["update"])),
      reads("job_health"),
    ),
  },
  {
    id: "extensions",
    group: "governance",
    // Workspace, though the page READS the installation's unit inventory. Scope
    // names whose state a page CHANGES, and the only write here is
    // `PATCH /roles/{key}/objects/{object}` — one workspace's role grants.
    scope: "workspace",
    // BOTH reads the card makes, because it makes them behind ONE flag: the
    // unit inventory from `GET /v1/extensions` and every role's grant on every
    // object from `GET /v1/roles`. On the inventory grant alone the page opens
    // and the card 403s on its second query, which is an unreadable page rather
    // than a narrower one.
    requires: allOf(reads("extension_access"), reads("role_admin")),
    // ExtensionAccessCard's only write is the role grant it PATCHes; the unit
    // inventory above it is a read.
    changes: acts(writes("role_admin", ["update"])),
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
    // Not the sentinel, for the same reason as Automations: emptying an
    // installation is a mutating request, and a read seat holding
    // `system_reset:delete` is refused it. The armed flag rides along, so the
    // page cannot be acted on where the deployment has not armed it.
    changes: allOf(
      fullSeat,
      { kind: "grant", object: "system_reset", action: "delete" },
      flagged("data_reset_available"),
    ),
  },
] as const satisfies readonly {
  id: string;
  group: SettingsGroupId;
  scope: SettingsScope;
  requires: CapabilityExpression;
  changes: CapabilityExpression;
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

/**
 * What this reader can act on, and what they can only consult.
 *
 * The two halves partition `visibleSettingsPages` — every page a reader may
 * open is in exactly one of them, and neither is a permission: a page in
 * `looksUp` opens normally, answers its address and appears in search. The
 * split decides PROMINENCE. The rail carries `acts`, so the furniture somebody
 * navigates every day is the work they can do; the settings home carries both,
 * under headings that say which is which.
 *
 * Splitting here rather than at each caller is what keeps the rail, the home
 * and the read-only banner agreeing. Three readers deriving "can this person
 * act?" separately is three chances to disagree in front of one user.
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
