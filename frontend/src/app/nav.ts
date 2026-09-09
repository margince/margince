import {
  Briefcase,
  Building2,
  ChartNoAxesCombined,
  Handshake,
  Home,
  Layers2,
  ListTodo,
  type LucideIcon,
  Sparkles,
  UserPlus,
  Users,
} from "lucide-react";
import type { MessageKey } from "../i18n/en";
import { CUSTOM_SCREEN, customNavItems } from "./custom";
import { SCREEN_ENTITY } from "./entity";
import { EXTENSION_SCREEN } from "./extensions";
import type { Route, Screen } from "./router";
import {
  type NavLevelEntry,
  type NavSection,
  type NavTrailLevel,
  navTrail,
} from "./subnav";

// The level registry lives beside this list and is reached through it: a caller
// asking where the sidebar can go has one module to import.
export type {
  NavCounts,
  NavLevelEntry,
  NavLevelGroup,
  NavSection,
  NavTrailLevel,
} from "./subnav";
export {
  entryLabel,
  navEntryHref,
  navEntryRoute,
  navLevelHref,
  navLevelRoute,
} from "./subnav";

// The primary nav. Order is normative and rail.test.tsx pins it. Brief stands
// alone above three labeled groups; the groups are the expanded sidebar's own
// structure and collapse to hairline rules at 56px, so the collapsed rail is the
// flat list WDS-NAV-1 describes.
//
// It carries ten rows. Filters & views and Projects are destinations here and
// Automations is not: Automations is set-and-forget configuration and lives
// inside Settings → AI, where the product already offered a second door to it,
// while the filter builder is a full authoring surface, and a screen this list
// does not name is a screen only a typed URL reaches.
//
// Today is the single door to the work that waits on a person — the approval
// queue, the task queue and the duplicate queue are lanes inside it rather than
// rows of their own, because three sidebar entries for one question ("what
// needs me?") read as three separate piles.
//
// `screen` is the route id and never changes with a label. The id is the
// address a reader bookmarked and the word every test and every `#/` link
// spells, so a row is free to be renamed without moving anything.
export type NavItem = {
  screen: Screen;
  labelKey: MessageKey;
  icon: LucideIcon;
  // Words this destination is also known by, for the palette: a reader who
  // learned one of them must still find the row by typing it.
  aliases?: readonly string[];
};

export type NavGroup = {
  headingKey?: MessageKey;
  items: readonly NavItem[];
};

export const NAV_GROUPS: readonly NavGroup[] = [
  { items: [{ screen: "brief", labelKey: "nav.brief", icon: Home }] },
  {
    headingKey: "nav.group.records",
    items: [
      {
        screen: "contacts",
        labelKey: "nav.contacts",
        icon: Users,
        aliases: ["people"],
      },
      { screen: "companies", labelKey: "nav.companies", icon: Building2 },
      { screen: "leads", labelKey: "nav.leads", icon: UserPlus },
      // A deal is one of the records this group names: it is created, owned,
      // listed, opened and reported on exactly as the three rows above it are,
      // and the board this route draws is a VIEW of that record type rather
      // than a fourth kind of thing. So it closes the group it belongs to.
      //
      // The agreement, not the furniture it is filed in: a deal is a bargain
      // two parties strike, and a board is one way this screen happens to draw
      // them. `Handshake` is already the deal glyph where a search result names
      // one (screens/tagresult.tsx), so the product draws this noun once. A
      // reader scanning five glyphs on a phone bar with no labels under them
      // has only the shape to go on.
      {
        screen: "deals",
        labelKey: "nav.deals",
        icon: Handshake,
        aliases: ["pipeline"],
      },
    ],
  },
  {
    headingKey: "nav.group.work",
    items: [
      // The day's own surface, and the only door to the work that waits on a
      // person: decisions to answer, tasks to finish and duplicates to merge are
      // lanes inside it. It leads the group because it is what a reader opens
      // when the question is "what needs me?".
      // A checklist rather than a sunrise: what this surface holds is the work
      // that waits on a person, and `Sun` says "morning" — which is the Brief's
      // claim, not this one.
      { screen: "worklist", labelKey: "nav.today", icon: ListTodo },
      // The body of work a deal is about. It starts during the deal and
      // outlives close-won, so what it belongs to is the work rather than the
      // record of the sale: a project in delivery has no deal column to stand
      // in.
      { screen: "projects", labelKey: "nav.projects", icon: Briefcase },
      // Closes the group as the AUTHORING surface over lists: a filter written
      // here becomes a dynamic list, a saved view or an export, and writing one
      // is work somebody does rather than a record they keep. Nothing on this
      // screen aggregates, so it is not Intelligence either. It reuses the
      // screen's own title rather than a `nav.*` label, because a surface named
      // twice gets renamed once.
      //
      // A funnel is the glyph every CRM draws a sales pipeline with, and the
      // deals board is a row on this list; stacked layers say what this surface
      // produces — a view laid over a list — and cannot be read as a second
      // door to that board.
      { screen: "filters", labelKey: "filters.title", icon: Layers2 },
    ],
  },
  {
    headingKey: "nav.group.intelligence",
    items: [
      {
        screen: "analytics",
        labelKey: "nav.analytics",
        icon: ChartNoAxesCombined,
      },
      { screen: "ai", labelKey: "nav.ai", icon: Sparkles },
    ],
  },
];

export const NAV: readonly NavItem[] = NAV_GROUPS.flatMap(
  (group) => group.items,
);

// A badge counts only what wants a human's attention, and no row on the primary
// nav does today: the queues that had counts (approvals, tasks) are lanes inside
// Today, which reports its own numbers on the page rather than on its row. The
// set stays because `badgeIds` is how a level declares badgeable rows and the
// deeper levels use the same mechanism — the empty set says "this level badges
// nothing", which is the honest answer, where deleting it would say the rail
// cannot badge at all. Ambient totals are deliberately absent whatever the
// level: the list endpoints are keyset-paginated and are not known to return
// one, and a decorative count contradicts the badge rule.
export const BADGE_SCREENS: ReadonlySet<Screen> = new Set();

// At phone width the sidebar becomes a bottom bar, which fits five thumb-sized
// cells — ten destinations would need horizontal scrolling, and a nav you have
// to scroll is a nav you cannot see. The CENTRE cell is not a destination: it is
// the agent, which is app-level chrome and reports rather than navigates, so
// three destinations ride the bar and More carries the rest.
//
// Today is one of the three the bar gives up, and it is the one worth saying out
// loud: the 390px approval path runs on it. It is a tap away in the sheet, which
// is the same distance every destination this list omits already is, and what
// the centre cell buys instead is the agent reachable without opening anything.
export const MOBILE_PRIMARY: ReadonlySet<Screen> = new Set([
  "brief",
  "contacts",
  "deals",
]);

// Every RECORD page keeps ONE reading column, so a reader who walks from a
// company to a deal to the lead behind it meets the same measure on each. That
// is the invariant this set states: a screen whose record page exists belongs
// here, and a record page missing from it is the one page in the walk that
// jumps. Settings is always capped and is not listed here — it is a whole
// section, not a record.
//
// A record reads DOWN rather than across: a rail of facts beside prose, where a
// measured line length is the point and a fact a monitor away from its label is
// worse, not wider. A list, a board or a report is scanned ACROSS, and the cap
// only ever pushed columns off the right edge of a wide display — which is why
// the key is the id and not the screen alone.
//
// Keyed on the screen, applied only when the route carries a RECORD id:
// `#/companies` is the list and belongs to the other family, `#/companies/<id>`
// is the record.
export const GRIDDED_RECORD_SCREENS: ReadonlySet<Screen> = new Set([
  "companies",
  "contacts",
  "leads",
  "deals",
  "projects",
]);

// The one id segment that is not a record id: `#/deals/new` is the deals LIST
// with its create form open (App.tsx, DealsRoute), so it is scanned across like
// every other list and must not take the record column.
export const CREATE_ID = "new";

// Screens that keep the same reading column with NO id, because they are not
// records and never carry one. Every one of them reads DOWN: Brief is a
// briefing in sentences beside a rail of context, with decision cards carrying
// drafted prose somebody has to read before they can decide; Today is a queue
// worked top to bottom; Filters is a list of saved views; Analytics is a column
// of report sections; AI is a settings-like page of stacked choices. Uncapped,
// each of them ran a line of text the full width of a wide display with the
// words hugging the left edge, which is the failure the cap exists to prevent.
//
// The rest of the app is scanned ACROSS — a list, a board, a table — and the
// cap only ever pushed columns off the right edge there.
export const GRIDDED_SCREENS: ReadonlySet<Screen> = new Set([
  "brief",
  "worklist",
  "filters",
  "analytics",
  "ai",
]);

// Documented rail-less exceptions (AC-shell layout exception): onboarding,
// the public booking page, the extension client surfaces, and the OAuth
// consent screen — a human lending an agent their authority reads it apart
// from the rest of the app, not framed inside it.
export const RAIL_LESS_SCREENS: ReadonlySet<Screen> = new Set([
  "onboarding",
  "book",
  "client",
  "preferences",
  "unsubscribe",
  "confirm",
  "room",
  "oauth-consent",
]);

// The destinations as a LEVEL, so the renderer that walks a trail treats level
// one exactly like any level below it — same rows, same tooltips, same active
// rule, one place where a nav row is spelled. The badge and phone-bar sets ride
// the level for the same reason: a row asks its level which ids badge rather
// than reaching for a module-scope set of its own.
//
// It prints no heading: the navigation landmark names it, and its GROUPS are the
// level-2 headings the sidebar promises.
//
// It carries the product's OWN destinations and nothing else. A composed unit
// had a group of its own here once; it does not any more. An installation
// enabling a unit is not the same as the product growing a twelfth
// destination, and the rail is the one surface where that distinction is
// visible to every person who uses the app.
//
// The unit's screen is still reachable at `#/ext/<unit>` — what changed is
// where it is OFFERED: Settings, on the page that already holds the credential
// the unit is configured with (see screens/extension-units.tsx). That is also
// what makes the offer honest about permission, because the two settings pages
// are already split by whose thing each surface is.
// Which of the three headings a fork screen names, keyed by the heading it
// declares. A fork says "records" and this is what that means to the rail — the
// mapping lives here rather than in app/custom.ts because the group KEYS are
// this list's, and a fork naming a fourth one fails to typecheck against
// CustomScreen["nav"]["group"] rather than rendering into nothing.
const CUSTOM_NAV_GROUPS: Partial<
  Record<MessageKey, "records" | "work" | "intelligence">
> = {
  "nav.group.records": "records",
  "nav.group.work": "work",
  "nav.group.intelligence": "intelligence",
};

// A fork's own destinations, in the group each declared (app/custom.ts).
//
// Appended to the group rather than given one of theirs, and after the
// product's rows rather than among them: a fork owns its build and may
// legitimately grow the rail — which a composed UNIT may not, and the comment
// above says why — but the product's own order is what every reader of this
// codebase and every upstream test knows, so an addition goes at the end where
// it reads as one.
//
// Addressed through `prefix`, which the level already has for a level that
// spans two depths: `["x"]` plus the screen's key is `#/x/<key>`, and the entry
// keeps its key as its `id` so `activeId` matching needs nothing new.
//
// Upstream's registry is empty, so in vanilla every one of these is a no-op and
// the rail is the same ten rows rail.test.tsx pins.
function forkItems(
  headingKey: MessageKey | undefined,
): readonly NavLevelEntry[] {
  const group = headingKey ? CUSTOM_NAV_GROUPS[headingKey] : undefined;
  if (!group) {
    return [];
  }
  return customNavItems(group).map((screen) => ({
    id: screen.key,
    prefix: [CUSTOM_SCREEN],
    label: screen.nav.label,
    icon: screen.nav.icon,
  }));
}

function primaryLevel(route: Route): NavTrailLevel {
  return {
    groups: NAV_GROUPS.map((group) => ({
      headingKey: group.headingKey,
      items: [
        ...group.items.map((item) => ({
          id: item.screen,
          labelKey: item.labelKey,
          icon: item.icon,
        })),
        ...forkItems(group.headingKey),
      ],
    })),
    ancestor: opensARecord(route),
    path: [],
    badgeIds: BADGE_SCREENS,
    barIds: MOBILE_PRIMARY,
  };
}

// Whether the active row is only the SECTION the page sits in rather than the
// page itself — which is true exactly when the route opens a RECORD, because a
// record is the one thing a segment under a screen reaches that is a page of its
// own. The test is the top bar's own: `SCREEN_ENTITY` is what decides whether
// the trail up there ends in a record and claims to be the page, so deriving the
// row's answer from the same map is what keeps exactly one element claiming
// `aria-current="page"`. Asking `route.id !== undefined` instead read every
// segment as a page: `#/filters/companies` picks the object tab OF the filters
// page, and the row that leads there was demoted to an ancestor of a page that
// does not exist.
function opensARecord(route: Route): boolean {
  return route.id !== undefined && SCREEN_ENTITY[route.screen] !== undefined;
}

// Which primary row a route makes current. It is the route's screen for every
// destination the product owns, and `settings` for a unit's — a unit screen
// routes as `{screen: "ext", id: "<unit>"}` and has no row of its own. Nothing
// in the sidebar renders that id today: Settings left the rail for the account
// menu, so a unit route marks no row and the top bar's trail is what says where
// the reader is (`Settings / <unit>`). The mapping stays because it is the
// honest answer to the question — a unit IS reached from Settings — and because
// the level machinery asks it for every route, unit routes included.
function activeRowFor(route: Route): string {
  if (route.screen === EXTENSION_SCREEN) {
    return "settings";
  }
  // A fork's rows are keyed by the screen's KEY, not by `x` — one segment
  // names every one of them, so a row identified by the segment could only ever
  // be "some fork screen". `#/x/warranty` marks the warranty row and nothing
  // else, which is what the rest of this list does for its own destinations.
  //
  // Falling back to the segment for `#/x` with no key: that address resolves to
  // no screen and marks no row, which is the honest answer for one.
  if (route.screen === CUSTOM_SCREEN) {
    return route.id ?? route.screen;
  }
  return route.screen;
}

// The levels the sidebar shows for a route: the destinations, then whatever the
// screen on that route published under them.
export function railTrail(
  route: Route,
  section?: NavSection,
): readonly NavTrailLevel[] {
  return navTrail(primaryLevel(route), route, activeRowFor(route), section);
}
