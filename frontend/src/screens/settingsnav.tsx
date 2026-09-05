// The settings catalog, its addresses, and who may open each entry.
//
// Split from the screen so `src/app/**` can ask those three questions without
// importing the cards. The shell, the topbar, the palette, the account menu and
// the search kinds each need an address or a visibility answer; none of them
// renders a settings page, and every one was pulling in a hundred and fifty
// imports — every card, every mutation, the whole lucide set — to get one.
//
// Nothing here renders and nothing here imports a card. That is what keeps the
// cost of asking "where does this go" separate from the cost of drawing it, and
// it is the property settings-imports.test.ts holds.

import {
  Activity,
  BadgeCheck,
  Blocks,
  BookOpen,
  Building2,
  Database,
  KeyRound,
  type LucideIcon,
  Mail,
  Mic,
  Plug,
  ShieldCheck,
  Sparkles,
  UserRound,
  UsersRound,
  Webhook,
  Wrench,
} from "lucide-react";
import { useCan, useHoldsAdminRole } from "../app/capability";
import { unitsForSecretScope } from "../app/extensions";
import type { NavLevelEntry, NavLevelGroup, NavSection } from "../app/nav";
import type { Route } from "../app/router";
import { useMe } from "./common";

// The entry register: one section nav entry per settings SUBJECT. Only surfaces
// this app actually renders get one — the mockup's Booking / Flow /
// Connected-surfaces tabs have no live seam here, so they are omitted rather
// than stubbed (STATE-5). The entry is selected by the route id
// (#/settings/<id>), so it is linkable and the palette can deep-link one.
//
// It used to be fifteen tabs plus nine routes outside them. What collapsed and
// why: two surfaces both called "Capture" became one; the
// installation and the company profile were always the same organization;
// currency rates joined the base currency they convert to while model prices
// joined the AI runtime they price; user administration and extension
// permissions are one question about authority; the field editor, pipeline
// designer, product list and offer templates all define the shape a record
// takes; and the operational verbs that were hiding beside the field editor — a
// reindex, job health, the danger zone — became a place of their own.
//
// One of those merges was later UNDONE: connectors and the overlay both answer
// "what are we connected to" and were merged on that reading, but the question
// has two different owners — see the split below. Capture activity is newer and
// additive: it answers what those connections DID, which no existing entry
// could say.
//
// No sentence here counts the entries. Three of them used to, and by the time
// anyone looked they said eleven, twelve and thirteen for a register holding
// fourteen — a number in prose beside a list is a second source of truth that
// nothing updates and no test can check. The list below is the count.
//
// Two groups: "you" (per-user, every member) and "admin" (installation
// posture). The group NAMES the subject, not an audience — every entry in it
// carries its own predicate, which is the grant the cards on it actually ask
// for, and the heading renders when at least one member survives.
//
// There is no second gate above those predicates. One used to sit here — a
// seat check for admin-or-ops — and it answered false for the whole group
// whatever the entry underneath had decided. That is a guess about a
// heterogeneous set: it spans surfaces with clean object grants (data model,
// organization, knowledge) and surfaces the server gates on the role itself
// (users, extensions). Every seeded role holds `pipeline`, `custom_field`,
// `knowledge_corpus`, `automation`, `product`, `offer_template` and `tag`
// reads, so the server answered those seats 200 while the product showed them
// nothing — and showed it by ABSENCE, so nobody could see the disagreement.
// A role edited to drop `license:read` loses that row and keeps the rest,
// which is the same rule applied to everyone rather than to two role names.
// The server stays the RBAC authority on every card within.
//
// The personal group is where a credential or a connection the PERSON holds
// lives: `agents` carries the caller's own passports, so gating it would regress
// passport minting for every seat that is not an admin, and `connections` carries
// their own mailbox and their own LinkedIn network.
//
// `connections` and `integrations` were ONE row, and that row was the reason the
// list had an entry with no predicate at all: it held a rep's own mailbox and the
// installation's webhooks together, so any honest gate on it took a personal task
// away from whoever it hid it from. The seam was never a missing group — it was
// one entry belonging to both. Split by WHOSE thing each surface is, they both get
// an honest predicate, and the ungated special case is gone rather than moved.
//
// Width is NOT an entry's business. Every page takes the whole column, so they
// line up with each other and with the rest of the app: a reader
// moving between two settings pages sees the content start and end where the
// last one did. A per-page measure buys a nicer form column at the cost of the
// page appearing to change size as you navigate, and of a knob each new entry
// has to answer for. Where a single control would otherwise stretch to the full
// column, the control constrains itself (`.settingrow-measure`, and each
// surface's own field widths) — that is a property of the control, which knows
// how wide it wants to be, not of the page, which does not.
// Exported for the nav suite, which derives its expected label list from THIS
// register rather than restating it. A restated list is a second source of truth
// that nothing updates: the copy in the test omitted `license` for as long as
// that entry existed, so a fully wired fourteenth tab — register, predicate,
// content, sidebar deep link, two locales — was invisible to every assertion in
// the file, including the two that claim to check the whole level.
// The two audience groups the rail renders, in order. Beside the register they
// group, so a group added to one is visible from the other.
const SETTINGS_GROUPS = ["you", "admin"] as const;

export const SETTINGS_TABS = [
  { id: "account", icon: UserRound, group: "you" },
  { id: "voice", icon: Mic, group: "you" },
  { id: "agents", icon: KeyRound, group: "you" },
  { id: "connections", icon: Plug, group: "you" },
  { id: "capture-activity", icon: Activity, group: "you" },
  { id: "general", icon: Building2, group: "admin" },
  { id: "users", icon: UsersRound, group: "admin" },
  { id: "integrations", icon: Webhook, group: "admin" },
  { id: "extensions", icon: Blocks, group: "admin" },
  { id: "capture", icon: Mail, group: "admin" },
  { id: "data-model", icon: Database, group: "admin" },
  { id: "ai", icon: Sparkles, group: "admin" },
  { id: "knowledge", icon: BookOpen, group: "admin" },
  { id: "privacy", icon: ShieldCheck, group: "admin" },
  { id: "license", icon: BadgeCheck, group: "admin" },
  { id: "maintenance", icon: Wrench, group: "admin" },
] as const satisfies readonly {
  id: string;
  icon: LucideIcon;
  group: "you" | "admin";
}[];

// Exported alongside the register: a caller that needs the label for an entry
// builds the key from this, and `settings.tab.${SettingsTabId}` is then a
// literal union TypeScript can check against MessageKey — no assertion, so a
// typo is a compile error rather than a lookup that silently falls back to the
// raw key and lets a test validate a label that does not exist.
export type SettingsTabId = (typeof SETTINGS_TABS)[number]["id"];

// Exported for the placement gate below the register: a settings card that
// configures the INSTALLATION has to sit on an admin entry, and the only way to
// check that without rendering every tab is to walk what each one returns.

export const ADMIN_SEGMENT = "admin";

// The route this screen answers, named once: the shell mounts the settings level
// by matching it, and the section published below declares it.
export const SETTINGS_SCREEN = "settings";

type AdminTabId = Extract<
  (typeof SETTINGS_TABS)[number],
  { group: "admin" }
>["id"];

// Which Organization entries this principal can use, one answer per entry, each
// asking for the grant the cards on it ask for. The nav then describes the seat
// instead of the role name it was assigned: a principal granted product writes by
// an edited role reaches the data model, and nobody is offered a page whose every
// card would refuse them.
//
// TWO GATES, and they answer different questions.
//
// The SEAT decides whether this half of settings exists for the reader at all:
// installation posture is an operator's work, so `admin` and `ops` reach it and
// nobody else does. That is a product decision about who configures an
// installation, not a claim about what the server would answer — and it is worth
// being precise about the difference, because the server has NOT changed: `GET
// /users`, the automations read and the consent registry still answer 200 to
// every authenticated seat. A REST or MCP caller holding a rep's passport reads
// them exactly as before. What this gate scopes is the product's own navigation.
//
// The GRANT then decides whether a page an operator may open has anything in it,
// and OPENING AN ENTRY IS A READ — so every predicate below asks for a READ
// grant. They asked for write grants once, because each was written to answer
// "can you USE this", and the cost was measured against the live API: a
// read-only seat was hidden from eight of eleven entries the server answers 200
// on, including three surfaces (products, offer templates, custom fields) that
// were ungated routes of their own before the merge. Within the section that
// lesson still holds in full: the entry opens if the principal may READ any part
// of it, and the write affordances inside say for themselves who may use them.
//
// A read grant is not a formality even where every seeded role holds it. The
// predicate asks the live grant, so a role edited to drop `custom_field:read`
// loses the Data model row — which `true` could never express, and which is the
// difference between a predicate that happens to be satisfied and no predicate.
//
// A merged entry takes the UNION of what its parts asked for, never the
// intersection: an entry that opened before this change must still open, or a
// restructure quietly becomes a permission change. Where a part is narrower than
// its page, that part gates itself inside — and a part withheld by a PERMISSION
// says so rather than vanishing (design-system/README.md).
//
// The licensing seat is deliberately not folded in (see capability.ts): a read
// seat still reads the pages behind these entries.
//
// EVERY predicate is evaluated here, unconditionally, before anything composes
// them. The number of hooks a render runs must not depend on which grants came
// back — so the `||` sits on the results, never around the calls, and no hook
// may move into the filter over the tab list.
/**
 * Which Organization entries this principal may open.
 *
 * Exported because the command palette must answer the SAME question: it offers a
 * shortcut to two of these entries, and a shortcut that lands on the Account
 * fallback is a command that lied. One predicate map, two readers.
 *
 * Both readers now resolve one cached snapshot, so they cannot disagree about
 * which pages exist. They could before: the rail probed the company-context
 * rollout over the network and the palette deliberately did not, which made
 * General reachable from one and absent from the other for a caller whose only
 * readable part of that page was the company profile.
 */
export function useSettingsEntryVisibility(): Readonly<
  Record<AdminTabId, boolean>
> {
  // Read off /me rather than probed. The company-context capabilities endpoint
  // is a screen's own hook, and importing it here dragged twenty-two imports
  // into a module whose whole reason for existing is to stay light. The same
  // fact rides /me as settings_availability.company_context, computed from the
  // rollout the endpoints themselves gate on.
  //
  // It also removes the reason probeCompanyFlag existed. The palette passed
  // false to avoid spending a request per session on a question it never asked;
  // there is no request now, so both callers read one cached snapshot and can no
  // longer disagree about which pages exist.
  const availability = useMe().data?.settings_availability;
  const companyContext = availability?.company_context ?? false;
  const pipeline = useCan("pipeline", "read");
  const product = useCan("product", "read");
  const offerTemplate = useCan("offer_template", "read");
  const knowledgeCorpus = useCan("knowledge_corpus", "read");
  const customField = useCan("custom_field", "read");
  const tag = useCan("tag", "read");
  const fxRate = useCan("fx_rate", "read");
  const aiModelRate = useCan("ai_model_rate", "read");
  const embeddingReindex = useCan("embedding_reindex", "read");
  const organization = useCan("organization", "read");
  const installation = useCan("installation_settings", "read");
  const captureSettings = useCan("capture_settings", "read");
  const licenseRead = useCan("license", "read");
  const automation = useCan("automation", "read");
  const webhook = useCan("webhook_subscription", "read");
  // The consent registry's server gate, which is not a role and not "any member":
  // consent/store.go's ListPurposes calls auth.Require(ctx, "person", read).
  const person = useCan("person", "read");
  const overlay = useCan("overlay_connection", "read");
  // The one predicate below that is a ROLE rather than a grant. `GET /admin/reset-data`
  // and the job-health read are gated on the literal admin role server-side and no
  // RBAC object describes them — a `role` object would encode a constant, and an
  // admin who revoked their own grant on it could never restore it (capability.ts).
  // Everything else above is a `read`, because opening a page is reading it.
  const isAdmin = useHoldsAdminRole();
  // Each entry's own read predicate, and the whole answer. There is no second
  // gate above these: a reader reaches an entry when they hold what it asks
  // for, which is the same question the SERVER answers on every route behind
  // it. The seat check that used to sit here returned false for every one of
  // these entries unless the reader held `admin` or `ops`, so a seat holding
  // `pipeline:read` — which every seeded role holds — was shown nothing while
  // the API answered it 200. That is the client disagreeing with the authority,
  // and the disagreement was invisible: the page was absent rather than
  // refused. What an entry offers once opened is still each card's own
  // question, and the cards ask their own writes.
  const granted = {
    // The organization, its profile and its currency table are one entry now, so
    // the predicate is the union of what they each asked for. Each is gated on
    // the SAME live grant the card inside asks for rather than on a role name:
    // deriving it from admin/ops would disagree with the cards in both
    // directions — an admin whose installation_settings grant was removed would
    // get a page of disabled fields, and a principal holding the grant under an
    // edited role could not reach the surface they may use.
    //
    // The company profile carries a second condition that is a rollout FLAG
    // rather than a permission, so its grant ANDs with it: PUT /company is gated
    // on organization writes, and the flag says whether the surface exists on
    // this installation at all.
    general: installation || (organization && companyContext) || fxRate,
    // The member roster, the roles on it, and what a role may reach. No RBAC
    // object describes identity administration and none can — a `role` object
    // would encode a constant, and an admin who revoked their own grant on it
    // could never restore it (capability.ts) — so the server gates the VERBS on
    // the role directly and serves the roster itself to anyone signed in.
    //
    // `true` matches that: `GET /users` answers 200 to any authenticated
    // principal, and "who is on my team" is not an admin's private question.
    // The same handler decides what the answer CONTAINS — role keys and the
    // inactive view are an admin's, everyone else gets the active roster the
    // share and assignee pickers already show them (handlers_roster.go).
    //
    // The invite form and every role control below withhold themselves on their
    // own authority, so a reader without them sees the roster and none of the
    // controls.
    users: true,
    // `GET /extensions` is admin-only server-side, so the entry follows the
    // role rather than a grant: any reader who is not an admin would open a
    // page whose only read answers 403.
    extensions: isAdmin,
    capture: captureSettings,
    // The installation's own outside wiring — the shared provider credential, the
    // outbound subscriptions, the incumbent mirror. Either read opens it, and the
    // provider card carries no grant of its own because the server answers for it.
    //
    // The system-of-record chip in the topbar is shown to EVERY seat and points
    // here, so an entry this narrow would strand whoever follows it on the Account
    // fallback — the overlay read every seeded role holds is what keeps that link
    // honest, and it is a live grant rather than an exemption.
    //
    // The composed units are the third card, and they open the entry on their
    // own PRESENCE rather than on a grant: this page is the only place a
    // workspace-scoped unit is offered at all — it has no rail row and the
    // palette never carried one — so a role holding neither read would lose the
    // unit itself, not merely the two cards above it. Presence is the honest
    // predicate because the card asks for no grant; the unit's own screen is
    // what refuses, on the object it declares.
    integrations:
      webhook || overlay || unitsForSecretScope("workspace").length > 0,
    // Everything that defines the shape a record takes: the field editor, the
    // pipeline designer, the product list, the offer templates. Any one of their
    // reads opens the page; the authoring controls inside each ask for their own
    // write.
    // Tags joined this entry, so the union widens: a seat holding `tag:read`
    // and none of the other three must still find the vocabulary. A merged
    // entry takes the union of what its parts ask for, never the intersection.
    "data-model": customField || pipeline || product || offerTemplate || tag,
    // The automations the installation runs, what it spent, and what it charges
    // per model. The automations read is the one every seeded role holds, and it
    // is what keeps this page reachable for manager, rep and read_only — the
    // server answers their automations read 200, and the editor was a route of
    // its own before this page absorbed it.
    ai: automation || aiModelRate,
    // The consent purpose registry, the retention ladder, the subject-request
    // queue and the audit trail. `consent_config` is a governed object upstream and
    // absent from the shipped RBAC vocabulary, so there is no grant NAMED for the
    // registry — but the server does not gate it on a role either: ListPurposes
    // demands `person:read`, so that is the grant to ask for, and asking it is what
    // keeps this from being `true` standing in for a permission. Every seeded role
    // holds it, and a role edited to drop it would otherwise reach a page of four
    // refusals. The three surfaces below the registry are narrower and each says so.
    privacy: person,
    // The document sets a person can ask questions of, and the files in them.
    // `knowledge_corpus:read` is the ASK, and the RBAC migration grants it to
    // every seeded role — so this entry opens for a manager or a rep, and the
    // card inside shows them the sets with no verbs. That is deliberate and is
    // the same shape as the automations read on `ai` above: a page that lists
    // what a reader may ask is not an administrator's page merely because
    // creating one is.
    knowledge: knowledgeCorpus,
    // The operational verbs, and the one entry that genuinely narrows. The reindex
    // read is admin/ops; job health and the danger zone are admin-ONLY (the server
    // spells both with RequireAdmin), so an ops seat reaches this page for the
    // reindex and finds the other two withheld. Nobody below ops has anything to
    // read here at all. The reindex is an ordinary grant an edited role can hold, so
    // the entry opens on either and the cards inside decide.
    // What the license grants and how much of it is used. Admin/ops-only, read
    // included — the narrowest predicate on the rail beside Maintenance's,
    // because a seat meter is the installation's commercial standing and a rep
    // reads their own seat elsewhere (UC-ADMIN-03 F1). A live grant rather than a
    // role name, like every other entry here: an ops principal whose license read
    // was removed by an edited role loses the row, which is the difference
    // between asking the grant and asking who somebody is.
    license: licenseRead,
    maintenance: isAdmin || embeddingReindex,
  } satisfies Readonly<Record<AdminTabId, boolean>>;
  return granted;
}

/**
 * Which entry an address names, and whether that address is the current one.
 *
 * Two shapes reach here. `#/settings/admin/privacy` is what the product mints
 * today; `#/settings/privacy` is what every link written before the admin half
 * had a segment of its own says, and it still resolves — a bookmark, a pasted
 * link and a docs page must not land nowhere because the IA grew a level of
 * naming. `legacy` is what tells `SettingsScreen` to rewrite the address it was
 * given, so the two spellings do not both stay in circulation.
 *
 * A personal entry addressed THROUGH the admin segment (`#/settings/admin/voice`)
 * is not resolved: it is not an address the product ever minted, and answering
 * it would make two live addresses for one page.
 */
/**
 * Entry ids this product used to mint, and the entry each one is now.
 *
 * A renamed tab keeps answering under its old id so a bookmark, a pasted link
 * and the two handbook copies do not land nowhere — and it resolves through
 * `legacy`, so the address bar is rewritten to the current spelling rather than
 * leaving both in circulation. The map is the only place the old id survives;
 * `SETTINGS_TABS` carries the current one alone.
 */
const RENAMED_TABS: Readonly<Record<string, SettingsTabId>> = {
  people: "users",
};

export function settingsRouteTab(route: Route): {
  readonly tab: string | undefined;
  readonly legacy: boolean;
} {
  if (route.id === ADMIN_SEGMENT) {
    const renamed =
      route.id2 === undefined ? undefined : RENAMED_TABS[route.id2];
    if (renamed !== undefined) {
      return { tab: renamed, legacy: true };
    }
    // Only an ADMIN entry answers under the admin segment. A personal id here
    // resolves to nothing rather than to its page: the page already has an
    // address, and serving it under a second one puts a spelling in circulation
    // that nothing mints and nothing rewrites. Unresolved, it falls back like any
    // other address the register does not answer.
    const deep = SETTINGS_TABS.find((candidate) => candidate.id === route.id2);
    return {
      tab: deep?.group === "admin" ? deep.id : undefined,
      legacy: false,
    };
  }
  const renamed = route.id === undefined ? undefined : RENAMED_TABS[route.id];
  if (renamed !== undefined) {
    return { tab: renamed, legacy: true };
  }
  const entry = SETTINGS_TABS.find((candidate) => candidate.id === route.id);
  return { tab: route.id, legacy: entry?.group === "admin" };
}

/**
 * The address one settings entry lives at.
 *
 * Every caller that mints a settings link goes through this — the redirect
 * below, the command palette's two shortcuts, the test kit's rail. The admin
 * group's extra segment is a property of the ENTRY, so a caller that knew only
 * the tab id would have to look the group up to build the link, and one of them
 * would eventually not bother.
 *
 * An id no entry answers keeps the shallow shape: it is the address a reader
 * typed, and `useVisibleSettingsTabs` is what decides what it lands on.
 */
export function settingsAddress(tab?: string): Route {
  const entry = SETTINGS_TABS.find((candidate) => candidate.id === tab);
  return entry?.group === "admin"
    ? { screen: SETTINGS_SCREEN, id: ADMIN_SEGMENT, id2: entry.id }
    : { screen: SETTINGS_SCREEN, id: tab };
}

// Which tabs this principal may use, and which of them the route selects. The
// nav in the sidebar and the content on the page both read this, so the two
// cannot disagree about what is current — including on the fallback below.
// Exported for SettingsScreen, which lives beside the cards and still has to
// resolve which entry an address lands on.
export function useVisibleSettingsTabs(tab?: string) {
  const adminTabVisible = useSettingsEntryVisibility();
  const tabs = SETTINGS_TABS.filter(
    (entry) => entry.group !== "admin" || adminTabVisible[entry.id],
  );
  // Unknown / absent id (or one this principal cannot see) falls back to the
  // first visible tab — a stale deep-link lands on Account, never a blank
  // screen. A rep who follows somebody's link to an admin page lands there too,
  // silently: the section is absent for them, and a page announcing that it
  // exists but is not theirs would be the one thing an absent section is chosen
  // to avoid saying.
  return { tabs, active: tabs.find((entry) => entry.id === tab) ?? tabs[0] };
}

/**
 * The settings level, as data the sidebar can render.
 *
 * The shell asks for this and renders it as the second navigation level; it
 * never learns what a grant is. The two groups are the ones this screen has
 * always had — "You" is per-user work, "Organization" is posture an admin
 * curates — and a group with no visible member is dropped rather than printed
 * empty. They are named for the SUBJECT rather than repeating the word the level
 * above them already carries: "Settings / Your settings / …" said it twice in a
 * 200px column.
 */
export function useSettingsSection(route: Route): NavSection {
  const { tabs, active } = useVisibleSettingsTabs(settingsRouteTab(route).tab);
  // Both message keys are composed from the ids, and both annotations are what
  // make them KEYS: a template literal narrows to the catalog's union only where
  // something expects one, and unannotated it would compile as any old string —
  // an unknown key has to stay a compile error.
  const groups = SETTINGS_GROUPS.map(
    (group): NavLevelGroup => ({
      headingKey: `settings.group.${group}`,
      items: tabs
        .filter((entry) => entry.group === group)
        .map(
          (entry): NavLevelEntry => ({
            id: entry.id,
            // Where the row actually points. The admin group sits a segment
            // deeper than the personal one, and the panel shows both under one
            // pair of headings — so the depth is the ENTRY's, not the level's.
            prefix: entry.group === "admin" ? [ADMIN_SEGMENT] : undefined,
            labelKey: `settings.tab.${entry.id}`,
            icon: entry.icon,
          }),
        ),
    }),
  ).filter((group) => group.items.length > 0);
  return {
    screen: SETTINGS_SCREEN,
    titleKey: "nav.settings",
    // No row is current on a route that is not one of the tabs. An extension
    // unit's page keeps this level in the sidebar — it is reached from here and
    // its trail says so — but it is not a settings tab, and `settingsRouteTab`
    // answers with the default one for any address it cannot read. Marking
    // Account current there would point at a page the reader is not on.
    activeId: route.screen === SETTINGS_SCREEN ? active.id : "",
    groups,
  };
}
