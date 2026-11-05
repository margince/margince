// The settings pages of the me, company, people and sales groups, in strip
// order. SETTINGS_PAGES in settingscatalog.ts joins them with the platform's.

import {
  acts,
  always,
  anyOf,
  destroys,
  readingIsTheAct,
  reads,
  writes,
} from "./settingscapability";
import type { SettingsPageRow } from "./settingscatalog";

export const BUSINESS_PAGES = [
  {
    id: "account",
    group: "me",
    scope: "self",
    requires: always,
    // Yours to change, all of it: identity, password, signature, language.
    changes: always,
  },
  {
    id: "meetings",
    group: "me",
    scope: "self",
    requires: always,
    changes: always,
  },
  {
    id: "voice",
    group: "me",
    scope: "self",
    requires: always,
    // The profile is the reader's own, but writing one is still a grant.
    // voice-dna.tsx asks `voice_profile` create and update, and a read-only seat
    // holds the read alone. `always` here would put Voice in that reader's rail
    // with nothing on it they could touch.
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
  {
    id: "notifications",
    group: "me",
    scope: "self",
    requires: always,
    // Both endpoints read and write the calling seat's rows, so there is
    // nothing here an admin sets for somebody else and no grant to withhold.
    changes: always,
  },
  // Self: every card is the reader's own. The mail-sharing plate states a
  // company rule and carries its own "Whole company" badge, changed on Capture
  // rules; the Telegram bot lives on Integrations.
  {
    id: "connections",
    group: "me",
    scope: "self",
    requires: always,
    // The mailbox and LinkedIn controls are the reader's own and need no
    // grant; the mail-sharing plate states a value and offers no control.
    changes: always,
  },
  // Mixed: CaptureExclusionsCard carries both scopes by design, and the
  // workspace activity view behind `capture_trace:read` is the installation's.
  // The held threads and senders cards are the reader's own.
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
    // What the installation and currency cards ask for. `installation_settings`
    // is asked as the update: every seeded role reads it for the base currency,
    // and only admin and ops may change it.
    //
    // The company profile adds no arm. Its read and write both ask the admin
    // role, which no grant here can spell. So its card asks the role itself and
    // draws nothing for anyone else on this page.
    requires: anyOf(
      writes("installation_settings", ["update"]),
      reads("fx_rate"),
    ),
    changes: acts(
      writes("installation_settings", ["update"]),
      writes("fx_rate"),
    ),
  },
  {
    id: "authentication",
    group: "company",
    scope: "installation",
    // Not `installation_settings`, which every role holds. The OAuth cards ride
    // `capture_settings`: a card narrower than its page withholds itself.
    requires: reads("authentication_policy"),
    // Each card saves on its own grant, and any one makes the page the reader's.
    changes: acts(
      writes("installation_settings", ["update"]),
      writes("authentication_policy", ["update"]),
      writes("capture_settings", ["update"]),
    ),
  },

  // `GET /users` answers 200 to any authenticated principal, because the share
  // and assignee pickers every seat uses read this roster. But a directory is
  // not an administration page. A reader who may not invite, change a role or
  // switch a seat off has nothing to do here. They look a colleague up beside
  // the record.
  //
  // So the settings entry follows the verbs, and the roster stays open
  // underneath it. `user_admin` gates Members and `team_admin` gates Teams, and
  // both pages' cards withhold their own controls verb by verb.
  {
    id: "members",
    group: "people",
    scope: "workspace",
    // The read, and only the read. Every write verb needs what it carries, so a
    // holder without it has no usable page:
    //
    //   - `include_inactive` is honoured only for a caller who passes the read
    //     (handlers_roster.go), so a `delete` holder without it never sees a
    //     deactivated member to reactivate.
    //   - the roster omits `roles` without it, and `ChangeUserRole` replaces
    //     the whole set. An `update` holder would pick a role from a list they
    //     cannot see.
    //
    // Opening the page on a write alone would offer those two broken
    // affordances. The card requires both the same way, so the page and its
    // controls agree.
    requires: reads("user_admin"),
    // Invite, role change and deactivate: the three verbs UsersAdminCard offers.
    // `delete` is deactivate rather than a row removal.
    changes: acts(writes("user_admin"), destroys("user_admin")),
  },
  {
    id: "teams",
    group: "people",
    scope: "workspace",
    // The team verbs, or the roster read that carries `team_ids`. A holder of
    // `user_admin:read` alone sees who is in which team. That is the page's
    // content even when they may change none of it.
    requires: anyOf(writes("team_admin"), reads("user_admin")),
    // TeamsCard creates and renames teams. Membership rides `user_admin:read`,
    // which is a read, so it does not make this page the reader's to change.
    changes: acts(writes("team_admin")),
  },
  {
    id: "roles",
    group: "people",
    scope: "workspace",
    // `GET /roles` asks `role_admin:read`, and the role list is the page, so
    // the read opens it even for a reader who may change nothing.
    requires: reads("role_admin"),
    // Create is POST /roles, a rename, row scope or grant edit is update, and
    // archive and restore are delete (identity/rolelifecycle.go).
    changes: acts(writes("role_admin"), destroys("role_admin")),
  },
  {
    id: "seats",
    group: "people",
    scope: "installation",
    // Either grant, and they show different things. `LicenseCard` reads the
    // entitlement for a `license` holder and the capacity endpoint for a
    // `seat_usage` one. Management sees how full the installation is without
    // seeing what it pays.
    requires: anyOf(reads("seat_usage"), reads("license")),
    // LicenseCard has no write of any kind. Reading the seat count is the action
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
    // endpoint, so `pipeline` read opens it. That grant also shows the stages
    // the transitions are between.
    requires: reads("pipeline"),
    // The page carries the per-transition switches beneath its report, so it
    // writes. The table above them is read-only as a layout choice, not a
    // permission one.
    //
    // Update only. A rule is created by the same PUT that edits one, and this
    // page never creates a pipeline. A custom role with pipeline:create but no
    // update would see every switch refused.
    changes: acts(writes("pipeline", ["update"])),
  },
  {
    id: "leads",
    group: "sales",
    scope: "workspace",
    // `custom_field`, not `pipeline`. The three cards here (lead sources,
    // disqualify reasons, handling) are stored as custom-field vocabulary and
    // the server gates their reads on that object. Asking for `pipeline` would
    // hide the page from a holder who may read it, and open it for one whose
    // reads then 403.
    requires: reads("custom_field"),
    // The three lead-vocabulary cards all write `custom_field`, delete included.
    changes: acts(writes("custom_field"), destroys("custom_field")),
  },
  {
    id: "acquisition",
    // `custom_field` for the lead page's reason above; no destroys because a
    // source is retired through its switch rather than deleted.
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    changes: acts(writes("custom_field")),
  },
  {
    id: "lossreasons",
    // `custom_field`, as the vocabularies above: the reasons a deal is lost
    // for are read by everyone and changed by admin/ops. No destroys: a reason
    // is retired through its switch, because a lost deal keeps its code.
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    changes: acts(writes("custom_field")),
  },
  {
    id: "partners",
    // `custom_field`, as the vocabularies above: everyone reads the partner
    // ladder, admin/ops change it. A custom stage nothing uses may be removed.
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    changes: acts(writes("custom_field"), destroys("custom_field")),
  },
  {
    id: "reviewtemplates",
    // `custom_field` read, which is what the templates endpoint requires.
    // Editing a template's questions is `custom_field` update on the server.
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    changes: acts(writes("custom_field", ["update"])),
  },
  {
    id: "recordroles",
    // Same `custom_field` authority as the vocabularies above: everyone reads,
    // admin/ops write. No destroys: a role is retired through its switch,
    // because an assignment that carried it must stay resolvable.
    group: "sales",
    scope: "workspace",
    requires: reads("custom_field"),
    changes: acts(writes("custom_field")),
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
    // tagadmin.tsx separates create, rename/merge and archive. Applying a tag to
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
] as const satisfies readonly SettingsPageRow[];
