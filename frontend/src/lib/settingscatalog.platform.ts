// The settings pages of the data, ai and governance groups, in strip order.
// SETTINGS_PAGES in settingscatalog.ts joins them after the business pages.

import {
  acts,
  allOf,
  anyOf,
  composedUnits,
  destroys,
  flagged,
  fullSeat,
  readingIsTheAct,
  reads,
  writes,
} from "./settingscapability";
import type { SettingsPageRow } from "./settingscatalog";

export const PLATFORM_PAGES = [
  {
    id: "capture",
    group: "data",
    scope: "workspace",
    requires: reads("capture_settings"),
    // Five cards. Four write `capture_settings:update`: the sharing rule, the
    // posture, the own-domain list and the consumer-mailbox list. The fifth,
    // BlockedDomainsCard, writes `company:update`. Every seeded sales role holds
    // that, so a rep keeps this page in the rail.
    changes: acts(
      writes("capture_settings", ["update"]),
      writes("company", ["update"]),
    ),
  },
  {
    id: "integrations",
    group: "data",
    // Mixed, and the provider card is why. Its contract summary calls
    // `PATCH /integrations/settings` "the installation's provider-lookup
    // posture". Webhooks and the workspace extension units stay inside one
    // workspace, and one badge cannot name both.
    scope: "mixed",
    // The writes, not the reads. Every seeded role reads the object, because
    // "is capture working?" is everyone's question, answered on the records
    // they open. Pointing a webhook somewhere is admin and ops work.
    requires: anyOf(
      writes("webhook_subscription"),
      writes("channel_connection"),
      // A composed workspace-scoped unit puts its settings on this page and
      // nowhere else. So the page opens for it even when the reader holds none
      // of the grants above.
      composedUnits("workspace"),
    ),
    // Provider and webhooks, each with its own object. Every verb the cards
    // offer, delete included: WebhooksCard archives a subscription, which is
    // acting on the page as much as creating one is.
    //
    // The composed-unit arm is not a grant and carries no seat.
    // ExtensionUnitsCard renders an Open link for every composed workspace unit
    // unconditionally. So on a build that composed one, anybody who can see this
    // page has a working control on it.
    changes: anyOf(
      acts(
        writes("integrations"),
        destroys("integrations"),
        writes("webhook_subscription"),
        destroys("webhook_subscription"),
        writes("channel_connection"),
        destroys("channel_connection"),
      ),
      composedUnits("workspace"),
    ),
  },
  {
    id: "web-forms",
    group: "data",
    scope: "workspace",
    // The write, as on Integrations: every seeded role reads the object, and
    // putting a form on a website is admin and ops work.
    requires: writes("integrations"),
    // WebFormsCard creates, edits and archives forms on the one object.
    changes: acts(writes("integrations"), destroys("integrations")),
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
    // Both verbs, because ImportCard asks for both: `mayImport` is
    // `mayCreate && mayAdvance`. The dry run parks the run and the approval
    // moves it, so create alone is refused at the first button.
    changes: acts(
      allOf(writes("import_run", ["create"]), writes("import_run", ["update"])),
    ),
  },

  {
    id: "usage",
    group: "ai",
    scope: "workspace",
    // The spend card reads on `ai_diagnostics`, the allowance on `ai_budget`.
    requires: anyOf(reads("ai_diagnostics"), reads("ai_budget")),
    // The allowance is the one write here; the spend is a read.
    changes: acts(writes("ai_budget", ["update"])),
  },
  {
    id: "models",
    group: "ai",
    // Installation, not workspace: `PUT /ai/routing` re-points which vendor
    // processes the installation's text, and `/ai/provider-keys` writes the
    // installation key vault. Both contract summaries say "installation".
    scope: "installation",
    // The routing and provider-key cards read on `ai_routing`. The tiers card's
    // health and the AI tasks card read on `ai_diagnostics` (ai/health.go).
    // On that grant alone the tiers card still answers (ai-routing.tsx
    // HealthOnly). Management is seeded diagnostics without routing.
    // The model prices panel reads on `ai_model_rate`.
    requires: anyOf(
      reads("ai_routing"),
      reads("ai_diagnostics"),
      reads("ai_model_rate"),
    ),
    // Bindings and keys write on `ai_routing:update`; model prices, here and in
    // a provider sheet, on the `ai_model_rate` upsert (removal is admitted on
    // update).
    changes: acts(writes("ai_routing", ["update"]), writes("ai_model_rate")),
  },
  {
    id: "mcp",
    group: "ai",
    scope: "installation",
    requires: reads("agent_client"),
    changes: acts(
      writes("agent_client"),
      destroys("agent_client"),
      writes("installation_settings", ["update"]),
    ),
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
    id: "automations",
    group: "ai",
    scope: "workspace",
    // The write, which admin and ops alone hold. Management and manager read
    // `automation` to see what ran, on the records it touched. A role that
    // cannot change an automation has nothing to do on the page that defines them.
    requires: writes("automation"),
    // Not the sentinel: this page's `requires` is a write that carries no seat
    // ceiling. Resolving `changes` to it would tell a read seat holding the
    // automation grants that the page is theirs to work in. The card offers
    // delete as well.
    changes: acts(writes("automation"), destroys("automation")),
  },

  {
    id: "privacy",
    group: "governance",
    // Installation: both retention endpoints the card writes are
    // installation-wide, so what this page destroys, it destroys everywhere.
    scope: "installation",
    // `contact` is not an arm, though the purposes card reads through it:
    // every seeded role holds `contact:read`, so it would open the page to the
    // whole workspace. The purposes list stays gated on `contact` server-side,
    // because the Contact 360 reads it. A card narrower than its page
    // withholds itself. `consent_config` alone buys no read
    // (consent/store.go ListPurposes is on `contact`).
    requires: anyOf(
      reads("retention_policy"),
      reads("privacy_request"),
      // The consent vocabulary, as a pair. `contact:read` is what the purposes
      // endpoint asks. `consent_config` is whose vocabulary it is, and the one
      // read management holds on this page.
      allOf(reads("contact"), reads("consent_config")),
    ),
    // Four cards, four objects. `contact:update` is what PrivacyInboxCard asks to
    // open a subject request. The request is about a contact's record, so the
    // grant is the contact's, not the queue's.
    changes: acts(
      writes("consent_config", ["create"]),
      writes("retention_policy"),
      destroys("retention_policy"),
      writes("privacy_request", ["update"]),
      writes("contact", ["update"]),
    ),
  },
  {
    id: "jurisdiction",
    group: "governance",
    scope: "installation",
    // The update, as on Company: every role holds `installation_settings:read`.
    requires: writes("installation_settings", ["update"]),
    // A weakening change also writes the carried floor, retention's object.
    changes: acts(
      writes("installation_settings", ["update"]),
      writes("retention_policy", ["update"]),
    ),
  },
  {
    id: "audit",
    group: "governance",
    scope: "workspace",
    // The trail's own object, which the card asks for too. A role edited to
    // carry `audit_log:read` reaches it and one without it does not. The admin
    // role name cannot say that either way.
    requires: reads("audit_log"),
    // The trail is a read by construction. Nothing writes it from here, and
    // reading it is the operator's act.
    changes: readingIsTheAct,
  },
  {
    id: "system-health",
    group: "governance",
    scope: "installation",
    // Any card's grant, so a reader holding one finds that card and the others
    // withheld. The schedules answer to the settings editor's grant, not the
    // read every role holds.
    requires: anyOf(
      reads("job_health"),
      reads("data_task"),
      reads("embedding_reindex"),
      writes("installation_settings", ["update"]),
    ),
    // The reindex and the schedules are writes and take the seat; watching a
    // stalled queue is an operator acting, so that arm stands outside it.
    changes: anyOf(
      acts(writes("embedding_reindex", ["update"])),
      acts(writes("data_task", ["update"])),
      acts(writes("installation_settings", ["update"])),
      reads("job_health"),
    ),
  },
  {
    id: "configuration",
    group: "governance",
    scope: "installation",
    // The update, as on Company: the endpoint answers only that grant.
    requires: writes("installation_settings", ["update"]),
    changes: acts(writes("installation_settings", ["update"])),
  },
  {
    id: "extensions",
    group: "governance",
    // Workspace, though the page reads the installation's unit inventory. Scope
    // names whose state a page changes, and the only write here is
    // `PATCH /roles/{key}/objects/{object}`: one workspace's role grants.
    scope: "workspace",
    // Both reads the card makes, because it makes them behind one flag: the
    // unit inventory from `GET /v1/extensions` and every role's grant on every
    // object from `GET /v1/roles`. On the inventory grant alone the card 403s on
    // its second query, an unreadable page rather than a narrower one.
    requires: allOf(reads("extension_access"), reads("role_admin")),
    // ExtensionAccessCard's only write is the role grant it patches; the unit
    // inventory above it is a read.
    changes: acts(writes("role_admin", ["update"])),
  },
  {
    id: "reset",
    group: "governance",
    scope: "installation",
    // The grant and the deployment's consent. `system_reset:delete` says this
    // reader may wipe an installation that permits wiping. `data_reset_available`
    // says whether this one does. Its compiled default is false, and a deployment
    // that never opted in has no such destination at all.
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
] as const satisfies readonly SettingsPageRow[];
