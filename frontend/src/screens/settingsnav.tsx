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
  House,
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
import { unitsForSecretScope } from "../app/extensions";
import type { NavLevelEntry, NavLevelGroup, NavSection } from "../app/nav";
import type { Route } from "../app/router";
import { useMe } from "./common";
import {
  SETTINGS_GROUPS as CATALOG_GROUPS,
  type SettingsPage,
  type SettingsReach,
  settingsReach,
  visibleSettingsPages,
} from "./settingscatalog";
import { PAGE_ICONS } from "./settingsnavicons";
import { settingsRouteTarget } from "./settingsrouting";
import { SettingsSearchBox } from "./settingssearchbox";

// The entry register: one section nav entry per settings SUBJECT. Only surfaces
// this app actually renders get one — the mockup's Booking / Flow /
// Connected-surfaces tabs have no live seam here, so they are omitted rather
// than stubbed (STATE-5). The entry is selected by the route id
// (#/settings/<id>), so it is linkable and the palette can deep-link one.
//
// It used to be fifteen tabs plus nine routes outside them. What collapsed and
// why: two surfaces both called "Capture" became one; the
// installation and the company profile were always the same company;
// currency rates joined the base currency they convert to while model prices
// joined the AI runtime they price; user administration and extension
// permissions are one question about authority; the field editor, pipeline
// designer, product list and offer templates all define the shape a record
// takes; and the operational verbs that were hiding beside the field editor — a
// reindex, job health, the danger zone — became a place of their own.
//
// Capture activity is newer and additive: it answers what those connections
// DID, which no existing entry could say.
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
// company, knowledge) and surfaces the server gates on the role itself
// (users, extensions). Every seeded role holds `pipeline`, `custom_field`,
// `knowledge_corpus`, `automation`, `product`, `offer_template` and `tag`
// reads, so the server answered those seats 200 while the product showed them
// nothing — and showed it by ABSENCE, so nobody could see the disagreement.
// A role edited to drop `license:read` loses that row and keeps the rest,
// which is the same rule applied to everyone rather than to two role names.
// The server stays the RBAC authority on every card within.
//
// The personal group is where a credential or a connection the COLLEAGUE holds
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
  contacts: "users",
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
    // that nothing mints and nothing rewrites. Unresolved, it meets the boundary
    // like any other address the register does not answer.
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
 * typed, and `SettingsScreen` answers it with the boundary rather than a page.
 */
export function settingsAddress(tab?: string): Route {
  const entry = SETTINGS_TABS.find((candidate) => candidate.id === tab);
  return entry?.group === "admin"
    ? { screen: SETTINGS_SCREEN, id: ADMIN_SEGMENT, id2: entry.id }
    : { screen: SETTINGS_SCREEN, id: tab };
}

/**
 * The row id of Settings home.
 *
 * Deliberately not a `SettingsPageId`: home is the settings address with NO
 * page segment, so no page can be current there and no page id may name it.
 * Exported because the screen and the sidebar have to agree which row that is.
 */
export const SETTINGS_HOME_ID = "home";

/**
 * The settings level, as data the sidebar can render.
 *
 * The shell asks for this and renders it as the second navigation level; it
 * never learns what a grant is. The two groups are the ones this screen has
 * always had — "You" is per-user work, "Company" is posture an admin
 * curates — and a group with no visible member is dropped rather than printed
 * empty. They are named for the SUBJECT rather than repeating the word the level
 * above them already carries: "Settings / Your settings / …" said it twice in a
 * 200px column.
 */
/** One rail row for a page, with everything the chrome reads off it. */
function navEntry(page: SettingsPage): NavLevelEntry {
  return {
    id: page.id,
    labelKey: `settings.tab.${page.id}`,
    // One line under the page's heading, saying what the label cannot: which
    // state it changes and whose. Composed from the id like the label above it,
    // and NOT cast — the field's own MessageKey type is what narrows the
    // template literal, so a page whose `.sub` key is missing from the catalogs
    // is a compile error rather than a subtitle that silently translates to
    // nothing.
    subKey: `settings.page.${page.id}.sub`,
    // Whose state the page changes, from the catalog's own `scope` rather than a
    // second table: the catalog already declares it for every page, and it had
    // no reader until now. Same MessageKey narrowing as the subtitle above — a
    // missing scope label is a compile error, not a silent blank.
    scopeKey: `settings.scope.${page.scope}`,
    icon: PAGE_ICONS[page.id],
  };
}

export function useSettingsSection(route: Route): NavSection {
  // The rail carries what this reader can ACT on. A page they may open and
  // cannot change is still theirs to reach — it answers its address, appears in
  // search and is listed on the settings home under a heading that says so —
  // but it is not furniture they navigate past every day. `reach.acts` and
  // `reach.looksUp` partition exactly the pages `useVisibleSettingsPages`
  // returns, so nothing leaves the product by being left out here.
  const reach = useSettingsReach();
  const pages = reach.acts;
  // Everything this reader may open, rail or not. The two below need it: a page
  // reached from the settings home or from a search hit is a real destination
  // and has to be recognised as the current one, and the search box must offer
  // every page the reader can open rather than only the ones they can change.
  const openable = [...reach.acts, ...reach.looksUp];
  const target = settingsRouteTarget(route);
  const named =
    target.kind === "page"
      ? openable.find((page) => page.id === target.page)
      : undefined;
  // Both message keys are composed from the ids, and both annotations are what
  // make them KEYS: a template literal narrows to the catalog's union only where
  // something expects one, and unannotated it would compile as any old string —
  // an unknown key has to stay a compile error.
  // Seven headings from the catalog, in its order. No `prefix`: every page
  // addresses flat now, so a row's depth is no longer a property of the group
  // it happens to sit under.
  //
  // An empty group is dropped rather than rendered as a heading with nothing
  // beneath it — which is what a reader holding one grant in a group of five
  // would otherwise see.
  const groups = CATALOG_GROUPS.map(
    (group): NavLevelGroup => ({
      headingKey: `settings.group.${group}`,
      items: pages.filter((page) => page.group === group).map(navEntry),
    }),
  ).filter((group) => group.items.length > 0);
  // The page the reader is ON, when it is one the rail does not carry.
  //
  // The chrome resolves the current page by searching the rendered rows —
  // `sectionHead` in app/pagemeta.ts does — so a page that is only on the
  // settings home would publish an `activeId` no row matches, and the heading,
  // the breadcrumb, the subtitle, the scope badge and the phone switcher would
  // all fall back to the section's own name. Reaching a page from search or
  // from the home would land the reader somewhere that says "Settings".
  //
  // So it joins the rail for exactly as long as it is open, in its own group.
  // Its own rather than its catalog group's: the group heading would then
  // appear for one page the reader cannot act on, which is the clutter the
  // partition exists to remove.
  const openedOffRail =
    named !== undefined && !pages.some((page) => page.id === named.id)
      ? [{ items: [navEntry(named)] } satisfies NavLevelGroup]
      : [];
  // Settings home, above the seven groups and in a group of its own. Not a
  // member of one: it belongs to no topic, and a group that owned it would take
  // it away on the day that group had no other visible page.
  //
  // Its heading is the SECTION's name, and it is the only place the sidebar says
  // "Settings": the level names itself through the heading over its first group
  // rather than through a title of its own, so this group carries the name for
  // the whole level.
  //
  // `SETTINGS_HOME_ID` rather than a page id, because home is not a page — it
  // is the address with no page segment, and `settingsHref()` with no argument
  // is exactly that. A row keyed on a page id would go current on that page.
  const home: NavLevelGroup = {
    headingKey: "nav.settings",
    items: [
      {
        id: SETTINGS_HOME_ID,
        labelKey: "settings.home",
        icon: House,
        // Addresses the LEVEL — `#/settings`, not `#/settings/home`.
        level: true,
      },
    ],
  };
  return {
    screen: SETTINGS_SCREEN,
    titleKey: "nav.settings",
    // The search stands above the rows and reaches FURTHER than they do: every
    // page this reader may open, including the ones the rail leaves out because
    // they cannot change them. Narrowing it to the rail's own list would make a
    // readable page unfindable by the one affordance built to find it.
    lead: <SettingsSearchBox pages={openable} />,
    // The same box for the phone drawer, which has to dismiss itself once the
    // box has moved the reader — see NavSection.leadFor.
    leadFor: (onPick) => <SettingsSearchBox pages={openable} onPick={onPick} />,
    // No row is current on a route that is not one of the tabs. An extension
    // unit's page keeps this level in the sidebar — it is reached from here and
    // its trail says so — but it is not a settings tab, and `settingsRouteTab`
    // answers with the default one for any address it cannot read. Marking
    // Account current there would point at a page the reader is not on.
    // Exactly three answers, and the third is the one that was missing: home on
    // the home address, the page on a page this reader resolved, and NOTHING on
    // a boundary.
    //
    // A boundary used to publish `pages[0]` as current, so while the content
    // said "not yours" the sidebar, the breadcrumb, the mobile switcher and the
    // page heading all said Account — which is half of the false fallback this
    // change exists to remove, kept alive in the chrome. An id no row carries
    // marks nothing, which is the honest answer: the reader is on no page.
    activeId:
      route.screen !== SETTINGS_SCREEN
        ? ""
        : target.kind === "home"
          ? SETTINGS_HOME_ID
          : (named?.id ?? ""),
    groups: [home, ...groups, ...openedOffRail],
  };
}

/**
 * The catalog pages this reader may open, in declaration order.
 *
 * The hook half of `visibleSettingsPages`: it supplies the two facts the pure
 * function takes — the access snapshot off /me, and which extension units this
 * build composed — so navigation, the palette, search and the screen all resolve
 * one table through one evaluator and cannot disagree about which pages exist.
 */
export function useVisibleSettingsPages(): readonly SettingsPage[] {
  const snapshot = useMe().data;
  return visibleSettingsPages(snapshot, {
    // Read HERE rather than inside the catalog: the extensions registry reaches
    // `@composition/screens`, and importing it there would drag React and a
    // build alias into a module whose whole purpose is being importable from
    // anywhere. The catalog takes the answer instead of fetching it.
    composedUnitScopes:
      unitsForSecretScope("workspace").length > 0 ? ["workspace"] : [],
  });
}

/**
 * The same pages, split into the ones this reader can act on and the ones they
 * can only consult.
 *
 * The hook half of `settingsReach`, taking the same two facts as the visibility
 * hook above so the rail, the settings home and the read-only banner resolve one
 * partition rather than each deciding "can this contact act?" for itself.
 */
export function useSettingsReach(): SettingsReach {
  const snapshot = useMe().data;
  return settingsReach(snapshot, {
    composedUnitScopes:
      unitsForSecretScope("workspace").length > 0 ? ["workspace"] : [],
  });
}
